package jobstorage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/uuid"
)

// BuildsDir is the directory where all build jobs are stored
var BuildsDir string

// jobMutex protects concurrent access to job operations
var jobMutex sync.Mutex

// getJobPath returns the path to a job's directory
func GetJobPath(jobID string) (string, error) {
	// Validate that jobID is a valid UUID.
	// Also ensure that user provided input is not trying to traverse the file system.
	if _, err := uuid.FromString(jobID); err != nil {
		return "", fmt.Errorf("invalid job ID format: %v", err)
	}
	return filepath.Join(BuildsDir, jobID), nil
}

// getJobMetadataPath returns the path to a job's metadata file
func GetJobMetadataPath(jobID string) (string, error) {
	jobPath, err := GetJobPath(jobID)
	if err != nil {
		return "", err
	}
	return filepath.Join(jobPath, "job.json"), nil
}

// getJobLogPath returns the path to a job's log file
func GetJobLogPath(jobID string) (string, error) {
	jobPath, err := GetJobPath(jobID)
	if err != nil {
		return "", err
	}
	return filepath.Join(jobPath, "build.log"), nil
}

// BuildJob represents a build job in the system
type BuildJob struct {
	JobData
	Status    JobStatus `json:"status"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	WorkerID  string    `json:"worker_id,omitempty"`
}

// JobData contains the build configuration
// Artifacts specifies which artifacts to build
type Artifacts struct {
	RawImage      bool `json:"raw_image"`
	ISO           bool `json:"iso"`
	ContainerFile bool `json:"container_file"`
	AWS           bool `json:"aws"`
	GCP           bool `json:"gcp"`
	Azure         bool `json:"azure"`
}

type JobData struct {
	Variant                string    `json:"variant"`
	Model                  string    `json:"model"`
	Architecture           string    `json:"architecture"`
	TrustedBoot            bool      `json:"trusted_boot"`
	KubernetesDistribution string    `json:"kubernetes_distribution"`
	KubernetesVersion      string    `json:"kubernetes_version"`
	Image                  string    `json:"image"`
	Version                string    `json:"version"`
	Artifacts              Artifacts `json:"artifacts"`
	CloudConfig            string    `json:"cloud_config,omitempty"`
}

// JobStatus represents the current status of a build job
type JobStatus string

const (
	JobStatusQueued   JobStatus = "queued"
	JobStatusAssigned JobStatus = "assigned"
	JobStatusRunning  JobStatus = "running"
	JobStatusComplete JobStatus = "complete"
	JobStatusFailed   JobStatus = "failed"
)

// ReadJob reads a job's metadata from the filesystem
func ReadJob(jobID string) (BuildJob, error) {
	var job BuildJob
	metadataPath, err := GetJobMetadataPath(jobID)
	if err != nil {
		return job, err
	}
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return job, err
	}
	err = json.Unmarshal(data, &job)
	return job, err
}

// WriteJob writes a job's metadata to the filesystem
func WriteJob(jobID string, job BuildJob) error {
	metadataPath, err := GetJobMetadataPath(jobID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, data, 0644)
}

// IsValidStatusTransition checks if a status transition is valid
func IsValidStatusTransition(current, next JobStatus) bool {
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

// BindNextAvailableJob attempts to bind the next available queued job to a worker
// Returns the job ID and job data if successful, or empty values if no job is available
func BindNextAvailableJob(workerID string) (string, BuildJob, error) {
	jobMutex.Lock()
	defer jobMutex.Unlock()

	entries, err := os.ReadDir(BuildsDir)
	if err != nil {
		return "", BuildJob{}, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		jobID := entry.Name()
		job, err := ReadJob(jobID)
		if err != nil {
			continue
		}

		if job.Status == JobStatusQueued {
			// Update job status atomically
			job.Status = JobStatusAssigned
			job.WorkerID = workerID
			job.UpdatedAt = time.Now().Format(time.RFC3339)

			if err := WriteJob(jobID, job); err != nil {
				return "", BuildJob{}, err
			}

			return jobID, job, nil
		}
	}

	return "", BuildJob{}, nil
}
