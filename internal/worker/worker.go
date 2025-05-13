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
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// WebsocketWriter wraps an io.Writer to write to a websocket
type WebsocketWriter struct {
	w io.Writer
}

func NewWebsocketWriter(w io.Writer) *WebsocketWriter {
	return &WebsocketWriter{w: w}
}

func (w *WebsocketWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
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
		if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusRunning); err != nil {
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
		if err := w.processJob(job.JobID, job.Job.JobData, ws); err != nil {
			fmt.Printf("Failed to process job: %v\n", err)
			if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusFailed); err != nil {
				fmt.Printf("Failed to update job status to failed: %v\n", err)
			}
			continue
		}

		// Update status to complete
		if err := w.updateJobStatus(job.JobID, jobstorage.JobStatusComplete); err != nil {
			fmt.Printf("Failed to update job status to complete: %v\n", err)
			continue
		}
	}
}

func (w *Worker) processJob(jobID string, jobData jobstorage.JobData, ws io.Writer) error {
	// Log the start of the process
	logMessage := fmt.Sprintf("Starting process with data: %+v\n", jobData)
	if err := websocket.Message.Send(ws.(*websocket.Conn), logMessage); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
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

	// Build container image
	if err := websocket.Message.Send(ws.(*websocket.Conn), "Building container image..."); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	imageName := fmt.Sprintf("build-%s", jobID)
	if err := runBashProcessWithOutput(ws, buildOCI(tempdir, imageName)); err != nil {
		return fmt.Errorf("failed to build image: %v", err)
	}

	// Create temporary output directory
	jobOutputDir := filepath.Join(tempdir, "artifacts")
	if err := os.MkdirAll(jobOutputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Generate tarball
	if err := websocket.Message.Send(ws.(*websocket.Conn), "Generating tarball..."); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	tarballPath := filepath.Join(jobOutputDir, "image.tar")
	if err := runBashProcessWithOutput(ws, saveOCI(tarballPath, imageName)); err != nil {
		return fmt.Errorf("failed to save image: %v", err)
	}

	// Generate raw image
	if err := websocket.Message.Send(ws.(*websocket.Conn), "Generating raw image..."); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	if err := buildRawDisk(imageName, jobOutputDir, ws); err != nil {
		return fmt.Errorf("failed to generate raw image: %v", err)
	}

	// Generate ISO
	if err := websocket.Message.Send(ws.(*websocket.Conn), "Generating ISO..."); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	if err := buildISO(imageName, jobOutputDir, "custom-kairos", ws); err != nil {
		return fmt.Errorf("failed to generate ISO: %v", err)
	}

	// Upload artifacts to server
	if err := websocket.Message.Send(ws.(*websocket.Conn), "Uploading artifacts to server..."); err != nil {
		return fmt.Errorf("failed to send log message: %v", err)
	}

	// Upload each artifact
	artifacts := []string{
		"image.tar",
		"custom-kairos.iso",
	}

	// Find the raw image file
	rawImage, err := searchFileByExtensionInDirectory(jobOutputDir, ".raw")
	if err != nil {
		return fmt.Errorf("failed to find raw disk image: %v", err)
	}
	artifacts = append(artifacts, filepath.Base(rawImage))

	for _, artifact := range artifacts {
		artifactPath := filepath.Join(jobOutputDir, artifact)
		if err := w.uploadArtifact(jobID, artifactPath, artifact); err != nil {
			return fmt.Errorf("failed to upload artifact %s: %v", artifact, err)
		}
	}

	// Send completion message with artifact URLs
	links := []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}{
		{Name: "Container image", URL: "/builds/" + jobID + "/image.tar"},
		{Name: "Raw disk image", URL: "/builds/" + jobID + "/" + filepath.Base(rawImage)},
		{Name: "ISO image", URL: "/builds/" + jobID + "/custom-kairos.iso"},
	}

	dat, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("failed to marshal links: %v", err)
	}

	if err := websocket.Message.Send(ws.(*websocket.Conn), string(dat)); err != nil {
		return fmt.Errorf("failed to send links: %v", err)
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
	tmpl := `FROM quay.io/kairos/kairos-init:v0.3.0 AS kairos-init

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
	return fmt.Sprintf(`docker build %s -t %s`, contextDir, image)
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

func buildRawDisk(containerImage, outputDir string, ws io.Writer) error {
	// Create a websocket writer
	wsWriter := NewWebsocketWriter(ws)

	// Set the logger to use our websocket writer
	internal.Log.Logger = internal.Log.Logger.Output(wsWriter)

	// Create the release artifact
	artifact := schema.ReleaseArtifact{
		ContainerImage: fmt.Sprintf("docker://%s", containerImage),
	}

	// Create the config
	config := schema.Config{
		State: outputDir,
		Disk: schema.Disk{
			EFI: true, // This is what disk.raw=true maps to
		},
		DisableHTTPServer: true,
		DisableNetboot:    true,
	}

	// Create the deployer with proper initialization
	d := deployer.NewDeployer(config, artifact, herd.EnableInit)

	// Register all steps
	err := deployer.RegisterAll(d)
	if err != nil {
		fmt.Fprintf(wsWriter, "Error registering steps: %v\n", err)
		return fmt.Errorf("error registering steps: %v", err)
	}

	// Write the DAG for debugging
	d.WriteDag()

	// Run the deployer
	if err := d.Run(context.Background()); err != nil {
		fmt.Fprintf(wsWriter, "Error running deployer: %v\n", err)
		return fmt.Errorf("error running deployer: %v", err)
	}

	// Collect any errors
	if err := d.CollectErrors(); err != nil {
		fmt.Fprintf(wsWriter, "Error collecting errors: %v\n", err)
		return fmt.Errorf("error collecting errors: %v", err)
	}

	return nil
}

func buildISO(containerImage, outputDir, artifactName string, ws io.Writer) error {
	// Create a websocket writer
	wsWriter := NewWebsocketWriter(ws)

	// Set the logger to use our websocket writer
	internal.Log.Logger = internal.Log.Logger.Output(wsWriter)

	// Create the release artifact
	artifact := schema.ReleaseArtifact{
		ContainerImage: fmt.Sprintf("docker://%s", containerImage),
	}

	// Read the config using the shared config package
	config, _, err := config.ReadConfig("", "", nil)
	if err != nil {
		fmt.Fprintf(wsWriter, "Error reading config: %v\n", err)
		return fmt.Errorf("error reading config: %v", err)
	}

	// Override the state and ISO name, and ensure netboot is disabled
	config.State = outputDir
	config.ISO.OverrideName = artifactName
	config.DisableNetboot = true
	config.DisableHTTPServer = true

	// Create the deployer with proper initialization
	d := deployer.NewDeployer(*config, artifact, herd.EnableInit)

	// Register all steps
	err = deployer.RegisterAll(d)
	if err != nil {
		fmt.Fprintf(wsWriter, "Error registering steps: %v\n", err)
		return fmt.Errorf("error registering steps: %v", err)
	}

	// Write the DAG for debugging
	d.WriteDag()

	// Run the deployer
	if err := d.Run(context.Background()); err != nil {
		fmt.Fprintf(wsWriter, "Error running deployer: %v\n", err)
		return fmt.Errorf("error running deployer: %v", err)
	}

	// Collect any errors
	if err := d.CollectErrors(); err != nil {
		fmt.Fprintf(wsWriter, "Error collecting errors: %v\n", err)
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
