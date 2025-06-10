package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/internal/config"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/spectrocloud-labs/herd"
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
		client:   &http.Client{Timeout: 0 * time.Second}, // No timeout
	}
}

// MultiWriterWithWebsocket wraps a multiwriter and provides websocket functionality
type MultiWriterWithWebsocket struct {
	writer io.Writer
	ws     *websocket.Conn
	prefix string
}

func NewMultiWriterWithWebsocket(writer io.Writer, prefix string) *MultiWriterWithWebsocket {
	return &MultiWriterWithWebsocket{
		writer: writer,
		prefix: prefix,
	}
}

func (m *MultiWriterWithWebsocket) SetWebsocket(ws *websocket.Conn) {
	m.ws = ws
}

// SetPrefix sets the prefix for the writer
func (m *MultiWriterWithWebsocket) SetPrefix(prefix string) {
	m.prefix = prefix
}

func (m *MultiWriterWithWebsocket) Write(p []byte) (n int, err error) {
	// Try to parse as JSON log message
	var logMsg struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(p, &logMsg); err == nil {
		// If it's a JSON log message, convert to plain text
		message := fmt.Sprintf("[%s] %s\n", strings.ToUpper(logMsg.Level), strings.TrimSpace(logMsg.Message))
		if m.prefix != "" {
			message = fmt.Sprintf("[%s] %s", m.prefix, message)
		}

		// Write to the regular writer
		n, err = m.writer.Write([]byte(message))
		if err != nil {
			return n, err
		}

		// Send to the websocket with retry if available
		if m.ws != nil {
			for i := 0; i < 3; i++ {
				if err := websocket.Message.Send(m.ws, message); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	} else {
		// If not a JSON log message, send as plain text
		message := string(p)
		if m.prefix != "" {
			message = fmt.Sprintf("[%s] %s", m.prefix, message)
		}

		// Write to the regular writer
		n, err = m.writer.Write([]byte(message))
		if err != nil {
			return n, fmt.Errorf("error while trying to write message %s: %v", message, err)
		}

		// Send to the websocket with retry if available
		if m.ws != nil {
			for i := 0; i < 3; i++ {
				if err := websocket.Message.Send(m.ws, message); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	return n, nil
}

// WriteStr writes a string to the writer, handling the byte conversion internally
func (m *MultiWriterWithWebsocket) WriteStr(s string) (n int, err error) {
	return m.Write([]byte(s))
}

func (w *Worker) Start(ctx context.Context) error {
	// Create writer at the start
	writer := NewMultiWriterWithWebsocket(os.Stdout, "")
	writer.WriteStr(fmt.Sprintf("Worker %s starting. Will poll for jobs at %s every %v\n", w.workerID, w.endpoint, retryInterval))

	for {
		select {
		case <-ctx.Done():
			writer.WriteStr("Worker shutting down...\n")
			return ctx.Err()
		default:
			// Try to bind a job
			job, err := w.bindJob()
			if err != nil {
				time.Sleep(retryInterval)
				continue
			}

			// Connect to websocket for logging
			// Convert http:// to ws:// for the websocket URL
			wsEndpoint := strings.Replace(w.endpoint, "http://", "ws://", 1)
			wsURL := fmt.Sprintf("%s/api/v1/builds/%s/logs/write?worker_id=%s", wsEndpoint, job.JobID, w.workerID)
			ws, err := websocket.Dial(wsURL, "", w.endpoint)
			if err != nil {
				writer.WriteStr(fmt.Sprintf("Failed to connect to websocket: %v\n", err))
				continue
			}
			writer.SetWebsocket(ws) // Set the websocket in our writer

			// Let the client know which worker is processing the job
			writer.WriteStr(fmt.Sprintf("Job %s bound by worker %s\n", job.JobID, w.workerID))

			// Update status to running
			if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusRunning); err != nil {
				writer.WriteStr(fmt.Sprintf("Failed to update job status to running: %v\n", err))
				continue
			}
			writer.WriteStr("Updated job status to running\n")

			writer.WriteStr("Starting job\n")
			// Process the job
			if err := w.processJob(job.JobID, job.Job.JobData, writer); err != nil {
				writer.WriteStr(fmt.Sprintf("Failed to process job: %v\n", err))
				if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusFailed); err != nil {
					writer.WriteStr(fmt.Sprintf("Failed to update job status to failed: %v\n", err))
				}
				writer.WriteStr("Updated job status to failed\n")
				ws.Close()
				continue
			}

			// Update status to complete
			if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusComplete); err != nil {
				writer.WriteStr(fmt.Sprintf("Failed to update job status to complete: %v\n", err))
				ws.Close()
				continue
			}
			writer.WriteStr("Updated job status to completed\n")

			// Close the websocket connection
			ws.Close()
		}
	}
}

func (w *Worker) processJob(jobID string, jobData jobstorage.JobData, writer *MultiWriterWithWebsocket) error {
	// Redirect all output to the multiwriter (set logger ONCE)
	internal.Log.Logger = internal.Log.Logger.Output(writer)

	// Log the start of the process
	logMessage := fmt.Sprintf("Starting process with data: %+v\n", jobData)
	if _, err := writer.WriteStr(logMessage); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	// Log the cloud config value for debugging
	if _, err := writer.WriteStr(fmt.Sprintf("[DEBUG] jobData.CloudConfig: %q\n", jobData.CloudConfig)); err != nil {
		return fmt.Errorf("failed to send cloud config debug log: %v", err)
	}

	// Create temporary directory for build
	tempdir, err := os.MkdirTemp("", "build")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// Prepare Dockerfile
	if err := prepareDockerfile(jobData, tempdir); err != nil {
		return fmt.Errorf("failed to prepare image: %v", err)
	}

	// Build container image (always needed for raw and ISO)
	if _, err := writer.WriteStr("Building container image...\n"); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	imageName := fmt.Sprintf("build-%s", jobID)
	if err := runBashProcessWithOutput(writer, buildOCI(tempdir, imageName)); err != nil {
		return fmt.Errorf("failed to build image: %v", err)
	}

	// Create temporary output directory
	jobOutputDir := filepath.Join(tempdir, "artifacts")
	if err := os.MkdirAll(jobOutputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// If cloud config is provided, write it to a persistent file in the job output dir
	var cloudConfigPath string
	if jobData.CloudConfig != "" {
		cloudConfigPath = filepath.Join(jobOutputDir, "cloud-config.yaml")
		if err := os.WriteFile(cloudConfigPath, []byte(jobData.CloudConfig), 0644); err != nil {
			return fmt.Errorf("failed to write cloud config file: %v", err)
		}
		if _, err := writer.WriteStr(fmt.Sprintf("[DEBUG] Wrote persistent cloud config to: %s\n", cloudConfigPath)); err != nil {
			return fmt.Errorf("failed to send cloud config file log: %v", err)
		}
		content, _ := os.ReadFile(cloudConfigPath)
		if _, err := writer.WriteStr(fmt.Sprintf("[DEBUG] Persistent cloud config contents:\n%s\n", string(content))); err != nil {
			return fmt.Errorf("failed to send cloud config file content log: %v", err)
		}
	}

	// Generate tarball if requested
	var tarballPath string
	if jobData.Artifacts.ContainerFile {
		if _, err := writer.WriteStr("Generating tarball...\n"); err != nil {
			return fmt.Errorf("failed to send log message: %v", err)
		}
		tarballPath = filepath.Join(jobOutputDir, "image.tar")
		if err := runBashProcessWithOutput(writer, saveOCI(tarballPath, imageName)); err != nil {
			return fmt.Errorf("failed to save image: %v", err)
		}
	}

	// Generate raw image (temporarily a requirement because we use it to name the artifact)
	if _, err := writer.WriteStr("Generating raw image...\n"); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	if err := buildRawDisk(imageName, jobOutputDir, writer, cloudConfigPath); err != nil {
		return fmt.Errorf("failed to generate raw image: %v", err)
	}

	// Find the raw image file first to get the base name
	rawImage, err := searchFileByExtensionInDirectory(jobOutputDir, ".raw")
	if err != nil {
		return fmt.Errorf("failed to find raw disk image: %v", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(rawImage), filepath.Ext(rawImage))

	// Generate ISO if requested
	if jobData.Artifacts.ISO {
		if _, err := writer.WriteStr("Generating ISO...\n"); err != nil {
			return fmt.Errorf("failed to send log message: %v", err)
		}
		if err := buildISO(imageName, jobOutputDir, baseName, writer, cloudConfigPath); err != nil {
			return fmt.Errorf("failed to generate ISO: %v", err)
		}
	}

	// Upload artifacts to server
	if _, err := writer.WriteStr("Uploading artifacts to server...\n"); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	// Move and upload selected artifacts
	artifacts := []string{}
	if jobData.Artifacts.ContainerFile {
		if err := os.Rename(filepath.Join(jobOutputDir, "image.tar"), filepath.Join(jobOutputDir, fmt.Sprintf("%s.tar", baseName))); err != nil {
			return fmt.Errorf("failed to rename image.tar to %s.tar: %v", baseName, err)
		}
		artifacts = append(artifacts, fmt.Sprintf("%s.tar", baseName))
	}
	if jobData.Artifacts.ISO {
		artifacts = append(artifacts, fmt.Sprintf("%s.iso", baseName))
	}
	artifacts = append(artifacts, filepath.Base(rawImage)) // Always upload raw image

	for _, artifact := range artifacts {
		destPath := filepath.Join(jobOutputDir, artifact)
		if err := w.uploadArtifact(jobID, destPath, artifact); err != nil {
			return fmt.Errorf("failed to upload artifact %s: %v", artifact, err)
		}
	}

	// Send completion message
	if _, err := writer.WriteStr("Build complete. Download links are ready.\n"); err != nil {
		return fmt.Errorf("failed to send completion message: %v", err)
	}

	// Give the client time to receive all messages before closing
	time.Sleep(1 * time.Second)

	// Clean up persistent cloud config file if it was written
	if cloudConfigPath != "" {
		os.Remove(cloudConfigPath)
	}

	return nil
}

func (w *Worker) uploadArtifact(jobID, filePath, fileName string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	url := fmt.Sprintf("%s/api/v1/builds/%s/artifacts/%s?worker_id=%s", w.endpoint, jobID, fileName, w.workerID)
	req, err := http.NewRequest(http.MethodPost, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

type bindResponse struct {
	JobID string              `json:"job_id"`
	Job   jobstorage.BuildJob `json:"job"`
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

func (w *Worker) updateJobStatus(jobID string, status jobstorage.JobStatus) error {
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

func prepareDockerfile(job jobstorage.JobData, tempdir string) error {
	// Create a Dockerfile from a template
	tmpl := `FROM quay.io/kairos/kairos-init:v0.4.9 AS kairos-init

FROM {{.Image}} AS base

COPY --from=kairos-init /kairos-init /kairos-init
RUN /kairos-init -l debug -s install --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN /kairos-init -l debug -s init --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN /kairos-init -l debug --validate --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN rm /kairos-init`

	t, err := template.New("Interpolate Dockerfile content").Parse(tmpl)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, job)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "Dockerfile"), buf.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func buildOCI(contextDir, image string) string {
	return fmt.Sprintf(`docker buildx build --load --progress=plain %s -t %s`, contextDir, image)
}

func saveOCI(dst, image string) string {
	return fmt.Sprintf("docker save -o %s %s", dst, image)
}

func runBashProcessWithOutput(ws io.Writer, command string) error {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = ws
	cmd.Stderr = ws
	return cmd.Run()
}

func buildRawDisk(containerImage, outputDir string, writer io.Writer, cloudConfigPath string) error {
	artifact := schema.ReleaseArtifact{
		ContainerImage: fmt.Sprintf("docker://%s", containerImage),
	}

	config := schema.Config{
		State: outputDir,
		Disk: schema.Disk{
			EFI: true,
		},
		DisableHTTPServer: true,
		DisableNetboot:    true,
	}

	if cloudConfigPath != "" {
		content, err := os.ReadFile(cloudConfigPath)
		if err != nil {
			fmt.Fprintf(writer, "[ERROR] Failed to read cloud config file: %v\n", err)
			return err
		}
		config.CloudConfig = string(content)
		fmt.Fprintf(writer, "[DEBUG] buildRawDisk using cloudConfigPath: %s\n", cloudConfigPath)
	}
	fmt.Fprintf(writer, "[DEBUG] config.CloudConfig: %q\n", config.CloudConfig)

	d := deployer.NewDeployer(config, artifact, herd.EnableInit)
	if err := deployer.RegisterAll(d); err != nil {
		fmt.Fprintf(writer, "Error registering steps: %v\n", err)
		return fmt.Errorf("error registering steps: %v", err)
	}
	d.WriteDag()
	if err := d.Run(context.Background()); err != nil {
		fmt.Fprintf(writer, "Error running deployer: %v\n", err)
		return fmt.Errorf("error running deployer: %v", err)
	}
	if err := d.CollectErrors(); err != nil {
		fmt.Fprintf(writer, "Error collecting errors: %v\n", err)
		return fmt.Errorf("error collecting errors: %v", err)
	}
	return nil
}

func buildISO(containerImage, outputDir, artifactName string, writer io.Writer, cloudConfigPath string) error {
	artifact := schema.ReleaseArtifact{
		ContainerImage: fmt.Sprintf("docker://%s", containerImage),
	}

	config, _, err := config.ReadConfig("", "", nil)
	if err != nil {
		fmt.Fprintf(writer, "Error reading config: %v\n", err)
		return fmt.Errorf("error reading config: %v", err)
	}

	config.State = outputDir
	config.ISO = schema.ISO{
		OverrideName: artifactName,
	}
	config.DisableNetboot = true
	config.DisableHTTPServer = true
	if cloudConfigPath != "" {
		content, err := os.ReadFile(cloudConfigPath)
		if err != nil {
			fmt.Fprintf(writer, "[ERROR] Failed to read cloud config file: %v\n", err)
			return err
		}
		config.CloudConfig = string(content)
		fmt.Fprintf(writer, "[DEBUG] buildISO using cloudConfigPath: %s\n", cloudConfigPath)
	}
	fmt.Fprintf(writer, "[DEBUG] config.CloudConfig: %q\n", config.CloudConfig)

	d := deployer.NewDeployer(*config, artifact, herd.EnableInit)
	if err := deployer.RegisterAll(d); err != nil {
		fmt.Fprintf(writer, "Error registering steps: %v\n", err)
		return fmt.Errorf("error registering steps: %v", err)
	}
	d.WriteDag()
	if err := d.Run(context.Background()); err != nil {
		fmt.Fprintf(writer, "Error running deployer: %v\n", err)
		return fmt.Errorf("error running deployer: %v", err)
	}
	if err := d.CollectErrors(); err != nil {
		fmt.Fprintf(writer, "Error collecting errors: %v\n", err)
		return fmt.Errorf("error collecting errors: %v", err)
	}
	return nil
}

func searchFileByExtensionInDirectory(artifactDir, ext string) (string, error) {
	filesInArtifactDir := []string{}
	err := filepath.Walk(artifactDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		filesInArtifactDir = append(filesInArtifactDir, path)
		return nil
	})
	if err != nil {
		return "", err
	}

	file := ""
	for _, f := range filesInArtifactDir {
		if filepath.Ext(f) == ext {
			file = f
			break
		}
	}

	if file == "" {
		return "", fmt.Errorf("no file found with extension %s", ext)
	}

	return file, nil
}
