package web

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/websocket"
)

type BuildResponse struct {
	UUID string `json:"uuid"`
}

// HandleQueueBuild creates a new build job and adds it to the queue
func HandleQueueBuild(c echo.Context) error {
	var req jobstorage.JobData
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate required fields
	if req.Version == "" || req.Image == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields (Version, Image)"})
	}

	id, err := uuid.NewV4()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate UUID"})
	}

	now := time.Now()
	job := jobstorage.BuildJob{
		JobData:   req,
		Status:    jobstorage.JobStatusQueued,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}

	// Create job directory
	jobPath, err := jobstorage.GetJobPath(id.String())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create job directory"})
	}
	if err := os.MkdirAll(jobPath, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create job directory"})
	}

	// Write job metadata
	if err := jobstorage.WriteJob(id.String(), job); err != nil {
		os.RemoveAll(jobPath) // Clean up on error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write job metadata"})
	}

	return c.JSON(http.StatusOK, BuildResponse{UUID: id.String()})
}

// HandleBindBuildJob allows a worker to claim a queued job
func HandleBindBuildJob(c echo.Context) error {
	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	jobID, job, err := jobstorage.BindNextAvailableJob(workerID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to bind job"})
	}

	if jobID == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "No queued jobs available"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"job":    job,
	})
}

// HandleUpdateJobStatus allows a worker to update the status of their assigned job
func HandleUpdateJobStatus(c echo.Context) error {
	jobID := c.Param("job_id")
	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	job, err := jobstorage.ReadJob(jobID)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	if job.WorkerID != workerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Job is assigned to a different worker"})
	}

	var statusUpdate struct {
		Status jobstorage.JobStatus `json:"status"`
	}
	if err := c.Bind(&statusUpdate); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid status update"})
	}

	// Validate status transition
	if !jobstorage.IsValidStatusTransition(job.Status, statusUpdate.Status) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid status transition"})
	}

	job.Status = statusUpdate.Status
	job.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := jobstorage.WriteJob(jobID, job); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update job status"})
	}

	return c.JSON(http.StatusOK, job)
}

// HandleGetBuild returns a job by ID
func HandleGetBuild(c echo.Context) error {
	jobID := c.Param("job_id")
	job, err := jobstorage.ReadJob(jobID)
	if err != nil {
		// Check if it's a not found error (either invalid job ID or job doesn't exist)
		if os.IsNotExist(err) || strings.Contains(err.Error(), "invalid job ID format") {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	return c.JSON(http.StatusOK, job)
}

// HandleGetBuildLogs returns the build logs for a job
func HandleGetBuildLogs(c echo.Context) error {
	jobID := c.Param("job_id")

	// Verify the job exists
	if _, err := jobstorage.ReadJob(jobID); err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	// Open the log file in read-only mode
	logFile, err := jobstorage.GetJobLogPath(jobID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	file, err := os.OpenFile(logFile, os.O_RDONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Log file not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to open log file: %v", err)})
	}
	defer file.Close()

	// Handle websocket upgrade
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		// Start from the beginning of the file
		if _, err := file.Seek(0, 0); err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Error seeking to start of log file: %v", err))
			return
		}

		// Read and send the logs
		buf := make([]byte, 1024)
		for {
			n, err := file.Read(buf)
			if err != nil {
				// If we've reached the end of the file, wait for more data
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				break
			}
			if n > 0 {
				if err := websocket.Message.Send(ws, string(buf[:n])); err != nil {
					break
				}
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

// HandleWriteBuildLogs handles streaming logs for a job via WebSocket
func HandleWriteBuildLogs(c echo.Context) error {
	jobID := c.Param("job_id")
	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	// Verify the job exists and is assigned to this worker
	job, err := jobstorage.ReadJob(jobID)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	if job.WorkerID != workerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Job is assigned to a different worker"})
	}

	// Handle websocket upgrade
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		logFile, err := jobstorage.GetJobLogPath(jobID)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Error getting log file path: %v\n", err))
			return
		}
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Error opening log file: %v\n", err))
			return
		}
		defer file.Close()

		// Continuously read messages from the websocket and write them to the log file
		for {
			var message string
			if err := websocket.Message.Receive(ws, &message); err != nil {
				if err != io.EOF {
					websocket.Message.Send(ws, fmt.Sprintf("Error receiving message: %v\n", err))
				}
				break
			}

			if message == "" {
				continue
			}

			// Ensure the message ends with a newline
			if !strings.HasSuffix(message, "\n") {
				message += "\n"
			}

			if _, err := file.WriteString(message); err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Error writing logs: %v\n", err))
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

// HandleUploadArtifact handles uploading build artifacts from workers
func HandleUploadArtifact(c echo.Context) error {
	jobID := c.Param("job_id")
	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	// Verify the job exists and is assigned to this worker
	job, err := jobstorage.ReadJob(jobID)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	if job.WorkerID != workerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Job is assigned to a different worker"})
	}

	// Get the filename from the URL
	filename := c.Param("filename")
	if filename == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename is required"})
	}

	// Create the job's artifacts directory if it doesn't exist
	jobPath, err := jobstorage.GetJobPath(jobID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	artifactsDir := filepath.Join(jobPath, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create artifacts directory"})
	}

	// Create the destination file
	dstPath := filepath.Join(artifactsDir, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create destination file"})
	}
	defer dst.Close()

	// Copy the uploaded file to the destination
	if _, err := io.Copy(dst, c.Request().Body); err != nil {
		os.Remove(dstPath) // Clean up on error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save uploaded file"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Artifact uploaded successfully"})
}

// HandleGetArtifacts returns the list of artifacts for a job
func HandleGetArtifacts(c echo.Context) error {
	jobID := c.Param("job_id")

	// Verify the job exists
	if _, err := jobstorage.ReadJob(jobID); err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	// Get the artifacts directory
	jobPath, err := jobstorage.GetJobPath(jobID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	artifactsDir := filepath.Join(jobPath, "artifacts")

	// List all files in the artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, []interface{}{}) // Return empty list if no artifacts
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read artifacts directory"})
	}

	// Build the response with artifact information
	artifacts := make([]map[string]string, 0)
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Map file extensions to friendly names
		name := file.Name()
		friendlyName := name
		switch filepath.Ext(name) {
		case ".tar":
			friendlyName = "Container image"
		case ".iso":
			friendlyName = "ISO image"
		case ".raw":
			friendlyName = "Raw disk image"
		}

		artifacts = append(artifacts, map[string]string{
			"name": friendlyName,
			"url":  name,
		})
	}

	return c.JSON(http.StatusOK, artifacts)
}
