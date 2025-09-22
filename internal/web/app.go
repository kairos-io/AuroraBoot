package web

import (
	"embed"
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
)

//go:embed app
var staticFiles embed.FS

var mu sync.Mutex
var appConfig AppConfig

type AppConfig struct {
	EnableLogger             bool
	ListenAddr               string
	OutDir                   string
	BuildsDir                string
	DefaultKairosInitVersion string
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

func App(config AppConfig) error {
	jobstorage.BuildsDir = config.BuildsDir
	appConfig = config
	e := echo.New()

	if config.EnableLogger {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())

	// Ensure directories exist
	os.MkdirAll(config.OutDir, os.ModePerm)
	os.MkdirAll(config.BuildsDir, os.ModePerm)

	assetHandler := http.FileServer(getFileSystem(false))
	e.GET("/*", echo.WrapHandler(assetHandler))

	// Handle form submission
	e.POST("/start", buildHandler)

	// API routes
	api := e.Group("/api/v1")
	api.GET("/config", HandleGetConfig)
	api.GET("/builds", HandleListBuilds)
	api.POST("/builds", HandleQueueBuild)
	api.POST("/builds/bind", HandleBindBuildJob)
	api.PUT("/builds/:job_id/status", HandleUpdateJobStatus)
	api.GET("/builds/:job_id", HandleGetBuild)
	api.GET("/builds/:job_id/logs", HandleGetBuildLogs)
	api.GET("/builds/:job_id/logs/write", HandleWriteBuildLogs)
	api.POST("/builds/:job_id/artifacts/:filename", HandleUploadArtifact)
	api.GET("/builds/:job_id/artifacts", HandleGetArtifacts)

	// Serve static artifact files
	e.Static("/artifacts", config.OutDir)
	e.Static("/builds", config.BuildsDir)

	e.Logger.Fatal(e.Start(config.ListenAddr))

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
	architecture := c.FormValue("architecture")
	if architecture == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Architecture is required"})
	}

	kubernetesDistribution := c.FormValue("kubernetes_distribution")
	kubernetesVersion := c.FormValue("kubernetes_version")
	if variant == "core" {
		kubernetesDistribution = ""
		kubernetesVersion = ""
	}

	// Collect artifact selection from form
	artifacts := jobstorage.Artifacts{
		RawImage:      true, // Always true
		ISO:           c.FormValue("artifact_iso") == "on",
		ContainerFile: c.FormValue("artifact_tar") == "on",
		GCP:           c.FormValue("artifact_gcp") == "on",
		Azure:         c.FormValue("artifact_azure") == "on",
	}

	// Collect job data
	job := jobstorage.BuildJob{
		JobData: jobstorage.JobData{
			Variant:                variant,
			Model:                  c.FormValue("model"),
			Architecture:           architecture,
			TrustedBoot:            c.FormValue("trusted_boot") == "true",
			KubernetesDistribution: kubernetesDistribution,
			KubernetesVersion:      kubernetesVersion,
			Image:                  image,
			Version:                c.FormValue("version"),
			KairosInitVersion:      c.FormValue("kairos_init_version"),
			Artifacts:              artifacts,
			CloudConfig:            c.FormValue("cloud_config"),
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
	jobPath, err := jobstorage.GetJobPath(id.String())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create job directory"})
	}
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

// Legacy webSocketHandler removed - functionality moved to HandleGetBuildLogs API endpoint
