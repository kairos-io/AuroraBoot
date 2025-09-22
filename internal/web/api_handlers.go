package web

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/websocket"
)

// @title AuroraBoot API
// @version 1.0
// @description API for managing build jobs and artifacts in AuroraBoot
// @host localhost:8080
// @BasePath /api/v1

type BuildResponse struct {
	UUID string `json:"uuid"`
}

// BuildListResponse represents the response for listing builds
type BuildListResponse struct {
	Builds []BuildWithUUID `json:"builds"`
	Total  int             `json:"total"`
}

// BuildWithUUID represents a build job with its UUID
type BuildWithUUID struct {
	UUID string `json:"uuid"`
	jobstorage.BuildJob
}

// ConfigResponse represents the application configuration
type ConfigResponse struct {
	DefaultKairosInitVersion string `json:"default_kairos_init_version"`
}

// @Summary Get application configuration
// @Description Returns the application configuration including default values for the form
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} ConfigResponse
// @Router /config [get]
func HandleGetConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, ConfigResponse{
		DefaultKairosInitVersion: appConfig.DefaultKairosInitVersion,
	})
}

// @Summary List all build jobs
// @Description Returns a paginated list of all build jobs, optionally filtered by status
// @Tags builds
// @Accept json
// @Produce json
// @Param status query string false "Filter by job status (queued, assigned, running, complete, failed)"
// @Param limit query int false "Maximum number of builds to return (default: 50, max: 100)"
// @Param offset query int false "Number of builds to skip (default: 0)"
// @Success 200 {object} BuildListResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds [get]
func HandleListBuilds(c echo.Context) error {
	// Parse query parameters
	statusFilter := c.QueryParam("status")
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	// Set defaults and parse limits
	limit := 50
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Read all build directories
	entries, err := os.ReadDir(jobstorage.BuildsDir)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read builds directory"})
	}

	// Collect all builds with their UUIDs
	var allBuilds []BuildWithUUID
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		jobID := entry.Name()

		// Validate UUID format
		if _, err := uuid.FromString(jobID); err != nil {
			continue
		}

		job, err := jobstorage.ReadJob(jobID)
		if err != nil {
			continue // Skip builds that can't be read
		}

		// Apply status filter if specified
		if statusFilter != "" && string(job.Status) != statusFilter {
			continue
		}

		allBuilds = append(allBuilds, BuildWithUUID{
			UUID:     jobID,
			BuildJob: job,
		})
	}

	// Sort builds by creation time (newest first)
	sort.Slice(allBuilds, func(i, j int) bool {
		timeI, errI := time.Parse(time.RFC3339, allBuilds[i].CreatedAt)
		timeJ, errJ := time.Parse(time.RFC3339, allBuilds[j].CreatedAt)

		if errI != nil || errJ != nil {
			return false // Keep original order if we can't parse times
		}

		return timeI.After(timeJ)
	})

	// Apply pagination
	total := len(allBuilds)
	start := offset
	end := offset + limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedBuilds := allBuilds[start:end]

	response := BuildListResponse{
		Builds: paginatedBuilds,
		Total:  total,
	}

	return c.JSON(http.StatusOK, response)
}

// @Summary Queue a new build job
// @Description Creates a new build job and adds it to the queue
// @Tags builds
// @Accept json
// @Produce json
// @Param job body jobstorage.JobData true "Build job data"
// @Success 200 {object} BuildResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds [post]
func HandleQueueBuild(c echo.Context) error {
	var req jobstorage.JobData
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate required fields
	if req.Version == "" || req.Image == "" || req.Architecture == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields (Version, Image, Architecture)"})
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

// @Summary Bind a queued build job to a worker
// @Description Allows a worker to claim a queued job
// @Tags builds
// @Accept json
// @Produce json
// @Param worker_id query string true "Worker ID"
// @Success 200 {object} object{job_id=string,job=jobstorage.BuildJob} "Job ID and job details"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/bind [get]
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

// StatusUpdateRequest represents a request to update the status of a job
// @Description Request body for updating job status
// @Example {"status": "running"}
type StatusUpdateRequest struct {
	Status jobstorage.JobStatus `json:"status" example:"running"`
}

// @Summary Update build job status
// @Description Allows a worker to update the status of their assigned job
// @Tags builds
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Param worker_id query string true "Worker ID"
// @Param status body web.StatusUpdateRequest true "Status update"
// @Success 200 {object} jobstorage.BuildJob
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id}/status [put]
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

	var statusUpdate StatusUpdateRequest
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

// @Summary Get build job details
// @Description Returns a job by ID
// @Tags builds
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Success 200 {object} jobstorage.BuildJob
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id} [get]
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

// @Summary Get build logs
// @Description Streams build logs via WebSocket for all jobs (running, completed, or failed)
// @Tags builds
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Success 101 {string} string "Switching to WebSocket protocol"
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id}/logs [get]
func HandleGetBuildLogs(c echo.Context) error {
	jobID := c.Param("job_id")

	// Verify the job exists
	_, err := jobstorage.ReadJob(jobID)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read job metadata"})
	}

	// Get the log file path
	logFile, err := jobstorage.GetJobLogPath(jobID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Always handle as WebSocket streaming
	return handleWebSocketLogs(c, jobID, logFile)
}

// handleWebSocketLogs handles WebSocket connections for live log streaming
func handleWebSocketLogs(c echo.Context, jobID, logFile string) error {
	// Handle websocket upgrade
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		// Check if log file exists, wait if it doesn't (for queued/assigned jobs)
		var file *os.File
		var err error

		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			websocket.Message.Send(ws, "Waiting for worker to pick up the job.\nIf you're running locally, make sure to pass the --create-worker to get a worker running.")

			// Wait for the file to appear
			for {
				time.Sleep(1 * time.Second)
				if _, err := os.Stat(logFile); err == nil {
					break
				}

				// Check if we should still wait
				job, err := jobstorage.ReadJob(jobID)
				if err != nil {
					websocket.Message.Send(ws, fmt.Sprintf("Error reading job status: %v", err))
					return
				}

				// If job failed before log file was created, stop waiting
				if job.Status == jobstorage.JobStatusFailed {
					websocket.Message.Send(ws, "Job failed before logs were generated.")
					return
				}
			}
		}

		// Open the log file
		file, err = os.OpenFile(logFile, os.O_RDONLY, 0644)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Error opening log file: %v", err))
			return
		}
		defer file.Close()

		// Start from the beginning of the file
		if _, err := file.Seek(0, 0); err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Error seeking to start of log file: %v", err))
			return
		}

		// Read and send the logs
		buf := make([]byte, 1024)
		for {
			// Read all currently available data from the file
			for {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					websocket.Message.Send(ws, fmt.Sprintf("Error reading log file: %v", err))
					return
				}
				if n > 0 {
					if err := websocket.Message.Send(ws, string(buf[:n])); err != nil {
						return
					}
				}
				// If we got no data, break the inner loop to check job status
				if n == 0 {
					break
				}
			}

			// Check job status after reading all currently available data
			job, err := jobstorage.ReadJob(jobID)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Error reading job status: %v", err))
				return
			}

			// If job is complete or failed, send final message and close
			if job.Status == jobstorage.JobStatusComplete || job.Status == jobstorage.JobStatusFailed {
				websocket.Message.Send(ws, fmt.Sprintf("Job reached status: %s, closing connection.", job.Status))
				return // Connection will be closed by defer
			}

			time.Sleep(100 * time.Millisecond)
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

// @Summary Write build logs
// @Description Handles streaming logs for a job via WebSocket connection
// @Tags builds
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Param worker_id query string true "Worker ID"
// @Success 101 {string} string "Switching to WebSocket protocol"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id}/logs/write [get]
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

// @Summary Upload build artifact
// @Description Handles uploading build artifacts from workers
// @Tags builds
// @Accept multipart/form-data
// @Produce json
// @Param job_id path string true "Job ID"
// @Param worker_id query string true "Worker ID"
// @Param filename path string true "Artifact filename"
// @Param file formData file true "Artifact file"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id}/artifacts/{filename} [post]
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

// @Summary List build artifacts
// @Description Returns the list of artifacts for a job
// @Tags builds
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Success 200 {array} object{name=string,url=string} "List of artifacts with friendly names and URLs"
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /builds/{job_id}/artifacts [get]
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
		description := ""
		switch filepath.Ext(name) {
		case ".tar":
			friendlyName = "OCI image"
			description = "(.tar) For use with Docker or other OCI compatible container runtimes"
		case ".iso":
			friendlyName = "ISO image"
			description = "For generic installations (USB, VM, bare metal)"
		case ".raw":
			friendlyName = "RAW image"
			description = "For AWS, Raspberry Pi, and any platform that supports RAW images"
		case ".vhd":
			friendlyName = "VHD image"
			description = "For use with Microsoft Azure"
		case ".gz":
			if len(name) > 11 && name[len(name)-11:] == ".gce.tar.gz" {
				friendlyName = "GCE image"
				description = "(.tar.gz) For use with Google Compute Engine"
			}
		}

		artifacts = append(artifacts, map[string]string{
			"name":        friendlyName,
			"description": description,
			"url":         name,
		})
	}

	return c.JSON(http.StatusOK, artifacts)
}
