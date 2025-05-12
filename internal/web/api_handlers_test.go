package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("API Handlers", func() {
	var (
		e    *echo.Echo
		rec  *httptest.ResponseRecorder
		req  *http.Request
		body *bytes.Buffer
	)

	BeforeEach(func() {
		e = echo.New()
		rec = httptest.NewRecorder()
		body = &bytes.Buffer{}
		// Clear the jobs data before each test
		mu.Lock()
		jobsData = make(map[string]BuildJob)
		mu.Unlock()
	})

	Describe("QueueBuild", func() {
		Context("when valid build request is submitted", func() {
			BeforeEach(func() {
				buildReq := JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			})

			It("should return a job ID", func() {
				c := e.NewContext(req, rec)
				err := HandleQueueBuild(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusOK), fmt.Sprintf("Response body: %s", rec.Body.String()))

				var response BuildResponse
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response.UUID).NotTo(BeEmpty(), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})

		Context("when invalid build request is submitted", func() {
			BeforeEach(func() {
				buildReq := JobData{
					// Missing required fields
					Variant: "",
					Model:   "",
					Image:   "",
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			})

			It("should return bad request", func() {
				c := e.NewContext(req, rec)
				err := HandleQueueBuild(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusBadRequest), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})
	})

	Describe("BindBuildJob", func() {
		Context("when worker requests a job", func() {
			var jobID string

			BeforeEach(func() {
				// Create a job first
				buildReq := JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				c := e.NewContext(req, rec)
				HandleQueueBuild(c)

				var response BuildResponse
				json.Unmarshal(rec.Body.Bytes(), &response)
				jobID = response.UUID

				// Now try to bind the job
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds/bind?worker_id=test-worker", nil)
			})

			It("should assign the job to the worker", func() {
				c := e.NewContext(req, rec)
				err := HandleBindBuildJob(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusOK), fmt.Sprintf("Response body: %s", rec.Body.String()))

				var response map[string]interface{}
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response["job_id"]).NotTo(BeEmpty(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response["job_id"]).To(Equal(jobID), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response["job"].(map[string]interface{})["status"]).To(Equal(string(JobStatusAssigned)), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response["job"].(map[string]interface{})["worker_id"]).To(Equal("test-worker"), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})

		Context("when no jobs are available", func() {
			BeforeEach(func() {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds/bind?worker_id=test-worker", nil)
			})

			It("should return not found", func() {
				c := e.NewContext(req, rec)
				err := HandleBindBuildJob(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusNotFound), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})
	})

	Describe("UpdateJobStatus", func() {
		Context("when updating job status", func() {
			var jobID string

			BeforeEach(func() {
				// Create and bind a job
				buildReq := JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				c := e.NewContext(req, rec)
				HandleQueueBuild(c)

				var response BuildResponse
				json.Unmarshal(rec.Body.Bytes(), &response)
				jobID = response.UUID

				// Bind the job
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds/bind?worker_id=test-worker", nil)
				c = e.NewContext(req, rec)
				HandleBindBuildJob(c)

				// Prepare status update request
				rec = httptest.NewRecorder()
				statusUpdate := map[string]string{"status": string(JobStatusRunning)}
				json.NewEncoder(body).Encode(statusUpdate)
				req = httptest.NewRequest(http.MethodPut, "/api/v1/builds/"+jobID+"/status?worker_id=test-worker", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			})

			It("should update the status", func() {
				c := e.NewContext(req, rec)
				c.SetParamNames("job_id")
				c.SetParamValues(jobID)
				err := HandleUpdateJobStatus(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusOK), fmt.Sprintf("Response body: %s", rec.Body.String()))

				var response BuildJob
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response.Status).To(Equal(JobStatusRunning), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})

		Context("when invalid status transition is attempted", func() {
			var jobID string

			BeforeEach(func() {
				// Create and bind a job
				buildReq := JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				c := e.NewContext(req, rec)
				HandleQueueBuild(c)

				var response BuildResponse
				json.Unmarshal(rec.Body.Bytes(), &response)
				jobID = response.UUID

				// Bind the job
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds/bind?worker_id=test-worker", nil)
				c = e.NewContext(req, rec)
				HandleBindBuildJob(c)

				// Prepare invalid status update request
				rec = httptest.NewRecorder()
				statusUpdate := map[string]string{"status": string(JobStatusComplete)}
				json.NewEncoder(body).Encode(statusUpdate)
				req = httptest.NewRequest(http.MethodPut, "/api/v1/builds/"+jobID+"/status?worker_id=test-worker", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			})

			It("should return bad request", func() {
				c := e.NewContext(req, rec)
				c.SetParamNames("job_id")
				c.SetParamValues(jobID)
				err := HandleUpdateJobStatus(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusBadRequest), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})
	})

	Describe("GetBuild", func() {
		Context("when job exists", func() {
			var jobID string

			BeforeEach(func() {
				// Create a job first
				buildReq := JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				json.NewEncoder(body).Encode(buildReq)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/builds", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				c := e.NewContext(req, rec)
				HandleQueueBuild(c)

				var response BuildResponse
				json.Unmarshal(rec.Body.Bytes(), &response)
				jobID = response.UUID

				// Now try to get the job
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodGet, "/api/v1/builds/"+jobID, nil)
			})

			It("should return the job", func() {
				c := e.NewContext(req, rec)
				c.SetParamNames("job_id")
				c.SetParamValues(jobID)
				err := HandleGetBuild(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusOK), fmt.Sprintf("Response body: %s", rec.Body.String()))

				var job BuildJob
				err = json.Unmarshal(rec.Body.Bytes(), &job)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(job.JobData.Variant).To(Equal("core"))
				Expect(job.JobData.Model).To(Equal("test-model"))
				Expect(job.JobData.Image).To(Equal("test-image"))
				Expect(job.JobData.Version).To(Equal("1.0.0"))
				Expect(job.Status).To(Equal(JobStatusQueued))
			})
		})

		Context("when job does not exist", func() {
			BeforeEach(func() {
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodGet, "/api/v1/builds/non-existent", nil)
			})

			It("should return not found", func() {
				c := e.NewContext(req, rec)
				c.SetParamNames("job_id")
				c.SetParamValues("non-existent")
				err := HandleGetBuild(c)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(rec.Code).To(Equal(http.StatusNotFound), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})
	})
})
