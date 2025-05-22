package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"golang.org/x/net/websocket"
)

var _ = Describe("API Handlers", func() {
	var (
		e         *echo.Echo
		rec       *httptest.ResponseRecorder
		req       *http.Request
		body      *bytes.Buffer
		testDir   string
		serverURL string
		port      int
	)

	BeforeEach(func() {
		e = echo.New()
		rec = httptest.NewRecorder()
		body = &bytes.Buffer{}

		// Create test directories with proper permissions
		tempDir := os.TempDir()
		fmt.Printf("System temp dir: %s\n", tempDir)
		testDir = filepath.Join(tempDir, fmt.Sprintf("auroraboot-test-%d", time.Now().UnixNano()))
		fmt.Printf("Creating test dir: %s\n", testDir)
		Expect(os.MkdirAll(testDir, 0755)).To(Succeed())
		jobstorage.BuildsDir = testDir

		// Get a free port for the test server
		var err error
		port, err = freeport.GetFreePort()
		Expect(err).NotTo(HaveOccurred())
		serverURL = fmt.Sprintf("http://localhost:%d", port)

		// Start the test server
		go func() {
			err := App(AppConfig{
				EnableLogger: false,
				ListenAddr:   fmt.Sprintf(":%d", port),
				OutDir:       filepath.Join(testDir, "artifacts"),
				BuildsDir:    testDir,
			})
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for server to be ready
		Eventually(func() error {
			resp, err := http.Get(serverURL + "/api/v1/builds")
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}, "5s", "100ms").Should(Succeed())
	})

	AfterEach(func() {
		// Clean up test directory
		if testDir != "" {
			fmt.Printf("Cleaning up test dir: %s\n", testDir)
			os.RemoveAll(testDir)
		}
	})

	// Helper function to check response status and print body if it fails
	checkResponse := func(resp *http.Response, expectedStatus int) {
		if resp.StatusCode != expectedStatus {
			body, _ := io.ReadAll(resp.Body)
			Fail(fmt.Sprintf("Expected status %d but got %d. Response body: %s", expectedStatus, resp.StatusCode, string(body)))
		}
	}

	Describe("QueueBuild", func() {
		Context("when valid build request is submitted", func() {
			BeforeEach(func() {
				buildReq := jobstorage.JobData{
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

				// Verify job was created
				job, err := jobstorage.ReadJob(response.UUID)
				Expect(err).To(BeNil())
				Expect(job.Variant).To(Equal("core"))
				Expect(job.Model).To(Equal("test-model"))
				Expect(job.Image).To(Equal("test-image"))
				Expect(job.Status).To(Equal(jobstorage.JobStatusQueued))
			})
		})

		Context("when invalid build request is submitted", func() {
			BeforeEach(func() {
				buildReq := jobstorage.JobData{
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
				buildReq := jobstorage.JobData{
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
				Expect(response["job"].(map[string]interface{})["status"]).To(Equal(string(jobstorage.JobStatusAssigned)), fmt.Sprintf("Response body: %s", rec.Body.String()))
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
				buildReq := jobstorage.JobData{
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
				statusUpdate := map[string]string{"status": string(jobstorage.JobStatusRunning)}
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

				var response jobstorage.BuildJob
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(response.Status).To(Equal(jobstorage.JobStatusRunning), fmt.Sprintf("Response body: %s", rec.Body.String()))
			})
		})

		Context("when invalid status transition is attempted", func() {
			var jobID string

			BeforeEach(func() {
				// Create and bind a job
				buildReq := jobstorage.JobData{
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
				statusUpdate := map[string]string{"status": string(jobstorage.JobStatusComplete)}
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
				buildReq := jobstorage.JobData{
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

				var job jobstorage.BuildJob
				err = json.Unmarshal(rec.Body.Bytes(), &job)
				Expect(err).To(BeNil(), fmt.Sprintf("Response body: %s", rec.Body.String()))
				Expect(job.Variant).To(Equal("core"))
				Expect(job.Model).To(Equal("test-model"))
				Expect(job.Image).To(Equal("test-image"))
				Expect(job.Version).To(Equal("1.0.0"))
				Expect(job.Status).To(Equal(jobstorage.JobStatusQueued))
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

	Describe("GetBuildLogs", func() {
		Context("when job exists", func() {
			var jobID string

			BeforeEach(func() {
				// Create a job first
				buildReq := jobstorage.JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				jsonData, err := json.Marshal(buildReq)
				Expect(err).NotTo(HaveOccurred())
				resp, err := http.Post(serverURL+"/api/v1/builds", "application/json", bytes.NewBuffer(jsonData))
				Expect(err).NotTo(HaveOccurred())
				checkResponse(resp, http.StatusOK)

				var response BuildResponse
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).NotTo(HaveOccurred())
				jobID = response.UUID

				// Bind the job
				resp, err = http.Post(serverURL+"/api/v1/builds/bind?worker_id=test-worker", "application/json", nil)
				Expect(err).NotTo(HaveOccurred())
				checkResponse(resp, http.StatusOK)

				// Write some logs via websocket
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs/write?worker_id=test-worker", port, jobID)
				ws, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).NotTo(HaveOccurred())
				defer ws.Close()

				logs := "test log line 1\ntest log line 2\n"
				err = websocket.Message.Send(ws, logs)
				Expect(err).NotTo(HaveOccurred())

				// Wait a bit for the logs to be written
				time.Sleep(100 * time.Millisecond)
			})

			It("should return the job logs", func() {
				// Connect to the websocket endpoint
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs", port, jobID)
				ws, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).NotTo(HaveOccurred())
				defer ws.Close()

				// Read the logs
				var logs string
				err = websocket.Message.Receive(ws, &logs)
				Expect(err).NotTo(HaveOccurred())
				Expect(logs).To(Equal("test log line 1\ntest log line 2\n"))
			})
		})

		Context("when job does not exist", func() {
			It("should return not found", func() {
				// Try to connect to a non-existent job's logs
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/non-existent/logs", port)
				_, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("WriteBuildLogs", func() {
		Context("when logs are written", func() {
			var jobID string

			BeforeEach(func() {
				// Create and bind a job
				buildReq := jobstorage.JobData{
					Variant:     "core",
					Model:       "test-model",
					Image:       "test-image",
					Version:     "1.0.0",
					TrustedBoot: false,
				}
				jsonData, err := json.Marshal(buildReq)
				Expect(err).NotTo(HaveOccurred())
				resp, err := http.Post(serverURL+"/api/v1/builds", "application/json", bytes.NewBuffer(jsonData))
				Expect(err).NotTo(HaveOccurred())
				checkResponse(resp, http.StatusOK)

				var response BuildResponse
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).NotTo(HaveOccurred())
				jobID = response.UUID

				// Bind the job
				resp, err = http.Post(serverURL+"/api/v1/builds/bind?worker_id=test-worker", "application/json", nil)
				Expect(err).NotTo(HaveOccurred())
				checkResponse(resp, http.StatusOK)
			})

			It("should write the logs via websocket", func() {
				// Connect to the websocket endpoint
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs/write?worker_id=test-worker", port, jobID)
				ws, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).NotTo(HaveOccurred())
				defer ws.Close()

				// Send logs through websocket
				logs := "test log line 1\ntest log line 2\n"
				err = websocket.Message.Send(ws, logs)
				Expect(err).NotTo(HaveOccurred())

				// Wait a bit for the logs to be written
				time.Sleep(100 * time.Millisecond)

				// Verify the log file contents
				logFile, err := jobstorage.GetJobLogPath(jobID)
				Expect(err).NotTo(HaveOccurred())
				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("test log line 1\ntest log line 2\n"))
			})

			It("should handle multiple log messages", func() {
				// Connect to the websocket endpoint
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs/write?worker_id=test-worker", port, jobID)
				ws, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).NotTo(HaveOccurred())
				defer ws.Close()

				// Send multiple log messages
				logs := []string{
					"test log line 1\n",
					"test log line 2\n",
					"test log line 3\n",
				}

				for _, log := range logs {
					err = websocket.Message.Send(ws, log)
					Expect(err).NotTo(HaveOccurred())
				}

				// Wait a bit for the logs to be written
				time.Sleep(100 * time.Millisecond)

				// Verify the log file contents
				logFile, err := jobstorage.GetJobLogPath(jobID)
				Expect(err).NotTo(HaveOccurred())
				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("test log line 1\ntest log line 2\ntest log line 3\n"))
			})

			It("should handle empty log messages", func() {
				// Connect to the websocket endpoint
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs/write?worker_id=test-worker", port, jobID)
				ws, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).NotTo(HaveOccurred())
				defer ws.Close()

				// Send empty log message
				err = websocket.Message.Send(ws, "")
				Expect(err).NotTo(HaveOccurred())

				// Wait a bit for the logs to be written
				time.Sleep(100 * time.Millisecond)

				// Verify the log file is empty
				logFile, err := jobstorage.GetJobLogPath(jobID)
				Expect(err).NotTo(HaveOccurred())
				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(BeEmpty())
			})

			It("should reject connection without worker_id", func() {
				// Try to connect without worker_id
				wsURL := fmt.Sprintf("ws://localhost:%d/api/v1/builds/%s/logs/write", port, jobID)
				_, err := websocket.Dial(wsURL, "", "http://localhost")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

// Mock websocket connection for testing
type mockWebsocketConn struct {
	messages []string
}

func (m *mockWebsocketConn) Send(v interface{}) error {
	if msg, ok := v.(string); ok {
		m.messages = append(m.messages, msg)
	}
	return nil
}

func (m *mockWebsocketConn) Close() error {
	return nil
}
