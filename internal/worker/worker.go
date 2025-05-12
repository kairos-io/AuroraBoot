package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kairos-io/AuroraBoot/internal/web"
	"golang.org/x/net/websocket"
)

const (
	retryInterval = 10 * time.Second
)

type Worker struct {
	endpoint string
	workerID string
	client   *http.Client
}

func NewWorker(endpoint, workerID string) *Worker {
	return &Worker{
		endpoint: endpoint,
		workerID: workerID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *Worker) Start() error {
	fmt.Printf("Worker %s starting. Will poll for jobs at %s every %v\n", w.workerID, w.endpoint, retryInterval)
	for {
		// Try to bind a job
		job, err := w.bindJob()
		if err != nil {
			time.Sleep(retryInterval)
			continue
		}

		// Update status to running
		if err := w.updateJobStatus(job.JobID, web.JobStatusRunning); err != nil {
			fmt.Printf("Failed to update job status to running: %v\n", err)
			continue
		}

		// Connect to websocket for logging
		// Convert http:// to ws:// for the websocket URL
		wsEndpoint := strings.Replace(w.endpoint, "http://", "ws://", 1)
		wsURL := fmt.Sprintf("%s/api/v1/builds/%s/logs/write?worker_id=%s", wsEndpoint, job.JobID, w.workerID)
		ws, err := websocket.Dial(wsURL, "", w.endpoint)
		if err != nil {
			fmt.Printf("Failed to connect to websocket: %v\n", err)
			continue
		}
		defer ws.Close()

		// Process the job
		logMessage := fmt.Sprintf("Processing job %s:\n", job.JobID)
		logMessage += fmt.Sprintf("  Variant: %s\n", job.Job.JobData.Variant)
		logMessage += fmt.Sprintf("  Model: %s\n", job.Job.JobData.Model)
		logMessage += fmt.Sprintf("  Image: %s\n", job.Job.JobData.Image)
		logMessage += fmt.Sprintf("  Version: %s\n", job.Job.JobData.Version)

		if err := websocket.Message.Send(ws, logMessage); err != nil {
			fmt.Printf("Failed to send log message: %v\n", err)
		}

		// Update status to complete
		if err := w.updateJobStatus(job.JobID, web.JobStatusComplete); err != nil {
			fmt.Printf("Failed to update job status to complete: %v\n", err)
			continue
		}
	}
}

type bindResponse struct {
	JobID string       `json:"job_id"`
	Job   web.BuildJob `json:"job"`
}

func (w *Worker) bindJob() (*bindResponse, error) {
	url := fmt.Sprintf("%s/api/v1/builds/bind?worker_id=%s", w.endpoint, w.workerID)
	resp, err := w.client.Post(url, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no jobs available")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result bindResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (w *Worker) updateJobStatus(jobID string, status web.JobStatus) error {
	url := fmt.Sprintf("%s/api/v1/builds/%s/status?worker_id=%s", w.endpoint, jobID, w.workerID)

	body := map[string]string{"status": string(status)}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
