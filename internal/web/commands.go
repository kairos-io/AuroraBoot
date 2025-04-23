package web

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
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

	// Create the deployer
	d := deployer.NewDeployer(config, artifact, herd.EnableInit)

	// Register the necessary steps
	for _, step := range []func() error{
		d.StepPrepNetbootDir,
		d.StepPrepTmpRootDir,
		d.StepDumpSource,
		d.StepGenRawDisk,
	} {
		if err := step(); err != nil {
			fmt.Fprintf(wsWriter, "Error registering step: %v\n", err)
			return fmt.Errorf("error registering step: %v", err)
		}
	}

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

	// Create the config
	config := schema.Config{
		State: outputDir,
		ISO: schema.ISO{
			OverrideName: artifactName,
		},
	}

	// Create the deployer
	d := deployer.NewDeployer(config, artifact, herd.EnableInit)

	// Register the necessary steps
	for _, step := range []func() error{
		d.StepPrepNetbootDir,
		d.StepPrepTmpRootDir,
		d.StepPrepISODir,
		d.StepCopyCloudConfig,
		d.StepDumpSource,
		d.StepGenISO,
	} {
		if err := step(); err != nil {
			fmt.Fprintf(wsWriter, "Error registering step: %v\n", err)
			return fmt.Errorf("error registering step: %v", err)
		}
	}

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
