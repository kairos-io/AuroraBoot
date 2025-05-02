package web

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/internal/config"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/spectrocloud-labs/herd"
)

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
