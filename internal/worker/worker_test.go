package worker_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kairos-io/AuroraBoot/internal/web"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/kairos-io/AuroraBoot/internal/worker"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/websocket"
)

var _ = Describe("Worker", func() {
	var (
		w *worker.Worker
	)

	BeforeEach(func() {
		w = worker.NewWorker(serverURL, "test-worker")
	})

	// waitForTerminalStatus waits for a job to reach a terminal state and returns the final status
	waitForTerminalStatus := func(jobID string) jobstorage.JobStatus {
		var finalStatus jobstorage.JobStatus
		Eventually(func() jobstorage.JobStatus {
			// Get the job status
			resp, err := http.Get(serverURL + "/api/v1/builds/" + jobID)
			if err != nil {
				return ""
			}
			defer resp.Body.Close()

			var job jobstorage.BuildJob
			err = json.NewDecoder(resp.Body).Decode(&job)
			if err != nil {
				return ""
			}

			// If we're in a terminal state, store it and return
			if job.Status != jobstorage.JobStatusQueued &&
				job.Status != jobstorage.JobStatusAssigned &&
				job.Status != jobstorage.JobStatusRunning {
				finalStatus = job.Status
				return job.Status
			}

			return job.Status
		}, 10*time.Minute, 1*time.Second).Should(Or(
			Equal(jobstorage.JobStatusComplete),
			Equal(jobstorage.JobStatusFailed),
		))

		return finalStatus
	}

	// We can't get a proper build in tests without containers,
	// so we'll just test that the worker can handle a failed build.
	It("process jobs appropriately", func() {
		// Create a test job with an invalid image
		jobData := jobstorage.JobData{
			Variant:     "core",
			Model:       "test-model",
			Image:       "invalid-image-that-does-not-exist",
			Version:     "1.0.0",
			TrustedBoot: false,
		}

		// Submit the job
		jsonData, err := json.Marshal(jobData)
		Expect(err).NotTo(HaveOccurred())
		resp, err := http.Post(serverURL+"/api/v1/builds", "application/json", bytes.NewBuffer(jsonData))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Get the job ID from the response
		var response web.BuildResponse
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.UUID).NotTo(BeEmpty())

		// Start the worker in a goroutine
		go func() {
			err := w.Start()
			Expect(err).NotTo(HaveOccurred())
		}()

		By("waiting for the job to reach a terminal state")
		finalStatus := waitForTerminalStatus(response.UUID)
		Expect(finalStatus).To(Equal(jobstorage.JobStatusFailed))

		// Get the job logs
		wsURL := fmt.Sprintf("ws://%s/api/v1/builds/%s/logs", strings.TrimPrefix(serverURL, "http://"), response.UUID)
		ws, err := websocket.Dial(wsURL, "", serverURL)
		Expect(err).NotTo(HaveOccurred())
		defer ws.Close()

		// Read the logs
		var logs string
		err = websocket.Message.Receive(ws, &logs)
		Expect(err).NotTo(HaveOccurred())

		// Check that the logs contain the expected error
		Expect(logs).To(ContainSubstring("pull access denied"))
	})
})
