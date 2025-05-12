package web

import (
	"net/http"
	"time"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

type BuildResponse struct {
	UUID string `json:"uuid"`
}

type JobStatus string

const (
	JobStatusQueued   JobStatus = "queued"
	JobStatusAssigned JobStatus = "assigned"
	JobStatusRunning  JobStatus = "running"
	JobStatusComplete JobStatus = "complete"
	JobStatusFailed   JobStatus = "failed"
)

type BuildJob struct {
	JobData
	Status    JobStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WorkerID  string    `json:"worker_id,omitempty"`
}

// HandleQueueBuild creates a new build job and adds it to the queue
func HandleQueueBuild(c echo.Context) error {
	mu.Lock()
	defer mu.Unlock()

	var req JobData
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate required fields
	if req.Variant == "" || req.Model == "" || req.Image == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
	}

	id, err := uuid.NewV4()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate UUID"})
	}

	now := time.Now()
	jobsData[id.String()] = BuildJob{
		JobData:   req,
		Status:    JobStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return c.JSON(http.StatusOK, BuildResponse{UUID: id.String()})
}

// HandleBindBuildJob allows a worker to claim a queued job
func HandleBindBuildJob(c echo.Context) error {
	mu.Lock()
	defer mu.Unlock()

	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	// Find a queued job
	var jobID string
	var job BuildJob
	for id, j := range jobsData {
		if j.Status == JobStatusQueued {
			jobID = id
			job = j
			break
		}
	}

	if jobID == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "No queued jobs available"})
	}

	// Update job status
	job.Status = JobStatusAssigned
	job.WorkerID = workerID
	job.UpdatedAt = time.Now()
	jobsData[jobID] = job

	return c.JSON(http.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"job":    job,
	})
}

// HandleUpdateJobStatus allows a worker to update the status of their assigned job
func HandleUpdateJobStatus(c echo.Context) error {
	mu.Lock()
	defer mu.Unlock()

	jobID := c.Param("job_id")
	workerID := c.QueryParam("worker_id")
	if workerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worker_id is required"})
	}

	job, exists := jobsData[jobID]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
	}

	if job.WorkerID != workerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Job is assigned to a different worker"})
	}

	var statusUpdate struct {
		Status JobStatus `json:"status"`
	}
	if err := c.Bind(&statusUpdate); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid status update"})
	}

	// Validate status transition
	if !isValidStatusTransition(job.Status, statusUpdate.Status) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid status transition"})
	}

	job.Status = statusUpdate.Status
	job.UpdatedAt = time.Now()
	jobsData[jobID] = job

	return c.JSON(http.StatusOK, job)
}

func isValidStatusTransition(current, next JobStatus) bool {
	switch current {
	case JobStatusQueued:
		return next == JobStatusAssigned
	case JobStatusAssigned:
		return next == JobStatusRunning
	case JobStatusRunning:
		return next == JobStatusComplete || next == JobStatusFailed
	default:
		return false
	}
}

// HandleGetBuild returns a job by ID
func HandleGetBuild(c echo.Context) error {
	mu.Lock()
	defer mu.Unlock()

	jobID := c.Param("job_id")
	job, exists := jobsData[jobID]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
	}

	return c.JSON(http.StatusOK, job)
}
