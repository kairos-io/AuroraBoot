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

func App(listenAddr, artifactDir string) error {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Ensure artifact directory exists
	os.MkdirAll(artifactDir, os.ModePerm)

	assetHandler := http.FileServer(getFileSystem(false))
	e.GET("/", echo.WrapHandler(assetHandler))

	// Store the last submitted form data
	var lastFormData map[string]string
	var mu sync.Mutex

	// Handle form submission
	e.POST("/start", func(c echo.Context) error {
		mu.Lock()
		defer mu.Unlock()
		// Collect form data
		lastFormData = map[string]string{
			"variant":             c.FormValue("variant"),
			"model":               c.FormValue("model"),
			"trusted_boot":        c.FormValue("trusted_boot"),
			"kubernetes_provider": c.FormValue("kubernetes_provider"),
			"kubernetes_version":  c.FormValue("kubernetes_version"),
			"image":               c.FormValue("image"),
		}

		fmt.Printf("Received form data: %+v\n", lastFormData)

		return c.String(http.StatusOK, "Form submitted successfully. Connect to WebSocket to view the process.")
	})

	// Handle WebSocket connection
	e.GET("/ws", func(c echo.Context) error {
		websocket.Handler(func(ws *websocket.Conn) {
			mu.Lock()
			defer mu.Unlock()

			defer ws.Close()

			if lastFormData == nil {
				websocket.Message.Send(ws, "No form data submitted. Please submit the form first.")
				return
			}

			// Log the start of the process
			websocket.Message.Send(ws, fmt.Sprintf("Starting process with data: %+v\n", lastFormData))

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
					lastFormData["image"],
					lastFormData["variant"],
					lastFormData["model"],
					lastFormData["trusted_boot"] == "true",
					lastFormData["kubernetes_provider"],
					lastFormData["kubernetes_version"],
				))
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to build image: %v", err))
				return
			}

			websocket.Message.Send(ws, "Saving container image...")

			err = runBashProcessWithOutput(
				ws,
				saveOCI(filepath.Join(artifactDir, "image.tar"), "my-image"),
			)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
				return
			}

			err = runBashProcessWithOutput(
				ws,
				buildRawDisk("my-image", artifactDir),
			)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
				return
			}

			err = runBashProcessWithOutput(
				ws,
				buildISO("my-image", artifactDir, "custom-kairos"),
			)
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to save image: %v", err))
				return
			}

			type Link struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}

			rawImage, err := searchFileByExtensionInDirectory(artifactDir, ".raw")
			if err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to find raw disk image: %v", err))
				return
			}

			links := []Link{
				{Name: "Container image", URL: "/artifacts/image.tar"},
				{Name: "Raw disk image", URL: "/artifacts/" + filepath.Base(rawImage)},
				{Name: "ISO image", URL: "/artifacts/custom-kairos.iso"},
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
