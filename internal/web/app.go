package web

import (
	"embed"
	"encoding/json"
	"fmt"
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
		mu.Lock()
		uuid := c.Param("uuid")
		job, err := jobstorage.ReadJob(uuid)
		mu.Unlock()
		if err != nil {
			websocket.Message.Send(ws, "Job not found")
			return
		}

		defer ws.Close()

		// Log the start of the process
		websocket.Message.Send(ws, fmt.Sprintf("Starting process with data: %+v\n", job.JobData))

		tempdir, err := os.MkdirTemp("", "build")
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to create temp dir: %v", err))
			return
		}

		defer os.RemoveAll(tempdir)

		if err := prepareDockerfile(job.JobData, tempdir); err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to prepare image: %v", err))
			return
		}

		websocket.Message.Send(ws, "Building container image...")

		err = runBashProcessWithOutput(ws,
			buildOCI(
				tempdir,
				"my-image",
			))

		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to build image: %v", err))
			return
		}

		// Create the output dir for the job
		jobOutputDir := filepath.Join(artifactDir, uuid)
		os.MkdirAll(jobOutputDir, os.ModePerm)

		websocket.Message.Send(ws, "Generating tarball...")

		err = runBashProcessWithOutput(
			ws,
			saveOCI(filepath.Join(jobOutputDir, "image.tar"), "my-image"),
		)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
			return
		}

		websocket.Message.Send(ws, "Generating raw image...")
		if err := buildRawDisk("my-image", jobOutputDir, ws); err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to generate raw image: %v", err))
			return
		}

		websocket.Message.Send(ws, "Generating ISO...")
		if err := buildISO("my-image", jobOutputDir, "custom-kairos", ws); err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to generate ISO: %v", err))
			return
		}

		type Link struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}

		rawImage, err := searchFileByExtensionInDirectory(jobOutputDir, ".raw")
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to find raw disk image: %v", err))
			return
		}

		websocket.Message.Send(ws, "Generating download links...")
		links := []Link{
			{Name: "Container image", URL: "/artifacts/" + uuid + "/image.tar"},
			{Name: "Raw disk image", URL: "/artifacts/" + uuid + "/" + filepath.Base(rawImage)},
			{Name: "ISO image", URL: "/artifacts/" + uuid + "/custom-kairos.iso"},
		}

		dat, err := json.Marshal(links)
		if err != nil {
			websocket.Message.Send(ws, fmt.Sprintf("Failed to marshal links: %v", err))
			return
		}

		websocket.Message.Send(ws, string(dat))
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}
