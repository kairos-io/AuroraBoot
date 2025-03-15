package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/robert-nix/ansihtml"
	"golang.org/x/net/websocket"
)

//go:embed app
var staticFiles embed.FS

//go:embed assets
var assets embed.FS

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

type JobData struct {
	Variant                string `json:"variant"`
	Model                  string `json:"model"`
	TrustedBoot            bool   `json:"trusted_boot"`
	KubernetesDistribution string `json:"kubernetes_distribution"`
	KubernetesVersion      string `json:"kubernetes_version"`
	Image                  string `json:"image"`
}

func App(listenAddr, artifactDir string) error {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Ensure artifact directory exists
	os.MkdirAll(artifactDir, os.ModePerm)

	assetHandler := http.FileServer(getFileSystem(false))
	e.GET("/*", echo.WrapHandler(assetHandler))

	// Store the last submitted form data
	jobsData := map[string]JobData{}
	var mu sync.Mutex

	// Handle form submission
	e.POST("/start", func(c echo.Context) error {
		mu.Lock()
		defer mu.Unlock()

		image := c.FormValue("base_image")
		if image == "byoi" {
			image = c.FormValue("byoi_image")
		}
		// Collect job data
		job := JobData{
			Variant:                c.FormValue("variant"),
			Model:                  c.FormValue("model"),
			TrustedBoot:            c.FormValue("trusted_boot") == "true",
			KubernetesDistribution: c.FormValue("kubernetes_distribution"),
			KubernetesVersion:      c.FormValue("kubernetes_version"),
			Image:                  image,
		}

		id := uuid.NewString()

		jobsData[id] = job

		return c.JSON(http.StatusOK, map[string]string{"uuid": id})
	})

	// Handle WebSocket connection
	e.GET("/ws/:uuid", func(c echo.Context) error {
		websocket.Handler(func(ws *websocket.Conn) {
			mu.Lock()
			uuid := c.Param("uuid")
			jobData, exists := jobsData[uuid]
			mu.Unlock()
			if !exists {
				websocket.Message.Send(ws, "Job not found")
				return
			}

			defer ws.Close()

			// Log the start of the process
			websocket.Message.Send(ws, fmt.Sprintf("Starting process with data: %+v\n", jobData))

			tempdir, err := os.MkdirTemp("", "build")
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to create temp dir: %v", err))
				return
			}

			defer os.RemoveAll(tempdir)

			if err := prepareDockerfile(tempdir); err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to prepare image: %v", err))
				return
			}

			websocket.Message.Send(ws, "Building container image...")

			err = runBashProcessWithOutput(ws,
				buildOCI(
					tempdir,
					"my-image",
					"v0.2.3",
					jobData.Image,
					jobData.Variant,
					jobData.Model,
					jobData.TrustedBoot,
					jobData.KubernetesDistribution,
					jobData.KubernetesVersion,
				))

			fmt.Println("Finished building image")
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
			err = runBashProcessWithOutput(
				ws,
				buildRawDisk("my-image", jobOutputDir),
			)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
				return
			}

			websocket.Message.Send(ws, "Generating ISO...")
			err = runBashProcessWithOutput(
				ws,
				buildISO("my-image", jobOutputDir, "custom-kairos"),
			)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
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
	})

	// Serve static artifact files
	e.Static("/artifacts", artifactDir)

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

func ansiToHTML(r io.Reader) io.Reader {

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// Read from the reader and write to the pipe
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if err != nil {
				if err != io.EOF {
					pw.CloseWithError(err)
				}
				break
			}
			pw.Write(ansihtml.ConvertToHTML(buf[:n]))
		}
	}()

	return pr
}
