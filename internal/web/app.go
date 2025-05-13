package web

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/net/websocket"
)

//go:embed app
var staticFiles embed.FS

var mu sync.Mutex
var artifactDir string

//go:embed assets
var assets embed.FS

type AppConfig struct {
	EnableLogger bool
}

func getFileSystem(useOS bool) http.FileSystem {
	if useOS {
		return http.FS(os.DirFS("app"))
	}

	fsys, err := fs.Sub(staticFiles, "app")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}

func App(listenAddr, outDir, buildsDirectory string, config ...AppConfig) error {
	artifactDir = outDir
	jobstorage.BuildsDir = buildsDirectory
	e := echo.New()

	// Only enable logger if not explicitly disabled
	enableLogger := true
	if len(config) > 0 {
		enableLogger = config[0].EnableLogger
	}

	if enableLogger {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())

	// Ensure directories exist
	os.MkdirAll(artifactDir, os.ModePerm)
	os.MkdirAll(jobstorage.BuildsDir, os.ModePerm)

	assetHandler := http.FileServer(getFileSystem(false))
	e.GET("/*", echo.WrapHandler(assetHandler))

	// Handle form submission
	e.POST("/start", buildHandler)

	// Handle WebSocket connection
	e.GET("/ws/:uuid", webSocketHandler)

	// API routes
	api := e.Group("/api/v1")
	api.POST("/builds", HandleQueueBuild)
	api.POST("/builds/bind", HandleBindBuildJob)
	api.PUT("/builds/:job_id/status", HandleUpdateJobStatus)
	api.GET("/builds/:job_id", HandleGetBuild)
	api.GET("/builds/:job_id/logs", HandleGetBuildLogs)
	api.GET("/builds/:job_id/logs/write", HandleWriteBuildLogs)
	api.POST("/builds/:job_id/artifacts/:filename", HandleUploadArtifact)
	api.GET("/builds/:job_id/artifacts", HandleGetArtifacts)

	// Serve static artifact files
	e.Static("/artifacts", artifactDir)
	e.Static("/builds", jobstorage.BuildsDir)

	e.Logger.Fatal(e.Start(listenAddr))

	return nil
}

func searchFileByExtensionInDirectory(artifactDir, ext string) (string, error) {
	// list all files in the artifact directory, search for one ending in .raw
	// and one ending in .iso
	filesInArtifactDir := []string{}
	err := filepath.Walk(artifactDir, func(path string, info fs.FileInfo, err error) error {
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

func buildHandler(c echo.Context) error {
	mu.Lock()
	defer mu.Unlock()

	image := c.FormValue("base_image")
	if image == "byoi" {
		image = c.FormValue("byoi_image")
	}

	variant := c.FormValue("variant")

	kubernetesDistribution := c.FormValue("kubernetes_distribution")
	kubernetesVersion := c.FormValue("kubernetes_version")
	if variant == "core" {
		kubernetesDistribution = ""
		kubernetesVersion = ""
	}

	// Collect job data
	job := jobstorage.BuildJob{
		JobData: jobstorage.JobData{
			Variant:                variant,
			Model:                  c.FormValue("model"),
			TrustedBoot:            c.FormValue("trusted_boot") == "true",
			KubernetesDistribution: kubernetesDistribution,
			KubernetesVersion:      kubernetesVersion,
			Image:                  image,
			Version:                c.FormValue("version"),
		},
		Status:    jobstorage.JobStatusQueued,
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	id, err := uuid.NewV4()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate UUID"})
	}

	// Create job directory
	jobPath := jobstorage.GetJobPath(id.String())
	if err := os.MkdirAll(jobPath, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create job directory"})
	}

	// Write job metadata
	if err := jobstorage.WriteJob(id.String(), job); err != nil {
		os.RemoveAll(jobPath) // Clean up on error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write job metadata"})
	}

	return c.JSON(http.StatusOK, map[string]string{"uuid": id.String()})
}

func webSocketHandler(c echo.Context) error {
	websocket.Handler(func(ws *websocket.Conn) {
		uuid := c.Param("uuid")
		_, err := jobstorage.ReadJob(uuid)
		if err != nil {
			websocket.Message.Send(ws, "Job not found")
			return
		}

		defer func() {
			// Send a final message before closing
			websocket.Message.Send(ws, "Connection closing...")
			time.Sleep(1 * time.Second) // Give time for the final message to be sent
			ws.Close()
		}()

		// Get the job's build directory
		jobPath := jobstorage.GetJobPath(uuid)
		buildLogPath := filepath.Join(jobPath, "build.log")

		// Check if build.log exists
		if _, err := os.Stat(buildLogPath); os.IsNotExist(err) {
			websocket.Message.Send(ws, "Waiting for worker to pick up the job...")

			// Wait for the file to appear
			for {
				time.Sleep(1 * time.Second)
				if _, err := os.Stat(buildLogPath); err == nil {
					break
				}
			}
		}

		// Open the log file
		file, err := os.Open(buildLogPath)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to open build log: %v", err))
			return
		}
		defer file.Close()

		// Create a buffer for reading
		buf := make([]byte, 1024)
		for {
			// Check job status
			job, err := jobstorage.ReadJob(uuid)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Error reading job status: %v", err))
				return
			}

			// If job is complete or failed, close the connection
			if job.Status == jobstorage.JobStatusComplete || job.Status == jobstorage.JobStatusFailed {
				return // Connection will be closed by defer with sleep
			}

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
			time.Sleep(100 * time.Millisecond)
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}
