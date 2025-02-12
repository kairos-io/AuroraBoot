package web

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

			if err := prepareImage(tempdir); err != nil {
				websocket.Message.Send(ws, fmt.Sprintf("Failed to prepare image: %v", err))
				return
			}

			err = runBashProcessWithOutput(ws,
				dockerCommand(
					tempdir,
					"my-image",
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

			err = runBashProcessWithOutput(
				ws,
				"docker save --quiet -o "+filepath.Join(artifactDir, "image.tar")+" my-image",
			)

			// Send download links
			artifactLinks := []string{
				genLink("image.tar", "Download container image"),
			}

			websocket.Message.Send(ws, strings.Join(artifactLinks, "\n"))
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// Serve static artifact files
	e.Static("/artifacts", artifactDir)

	e.Logger.Fatal(e.Start(listenAddr))

	return nil
}

func genLink(artifactName, message string) string {
	return fmt.Sprintf("<a href=\"/artifacts/%s\" download>%s</a>", artifactName, message)
}

func runBashProcessWithOutput(ws io.Writer, command string) error {
	// Simulate a background process
	cmd := exec.Command("bash", "-c", command)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	out := io.MultiReader(stdout, stderr)

	if err := cmd.Start(); err != nil {
		return err
	}

	// Stream process output to writer
	reader := io.TeeReader(out, ws)
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}
