package worker_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kairos-io/AuroraBoot/internal/web"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/kairos-io/AuroraBoot/internal/worker"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Worker", func() {
	var (
		w *worker.Worker
	)

	BeforeEach(func() {
		w = worker.NewWorker(serverURL, "test-worker")
	})

	It("should process jobs successfully", func() {
		// Create a test job
		jobData := jobstorage.JobData{
			Variant:     "core",
			Model:       "test-model",
			Image:       "test-image",
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

		// Wait for the job to be processed
		Eventually(func() jobstorage.JobStatus {
			// Get the job status
			resp, err := http.Get(serverURL + "/api/v1/builds/" + response.UUID)
			if err != nil {
				return ""
			}
			defer resp.Body.Close()

			var job jobstorage.BuildJob
			err = json.NewDecoder(resp.Body).Decode(&job)
			if err != nil {
				return ""
			}

			return job.Status
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(jobstorage.JobStatusComplete))
	})
})
