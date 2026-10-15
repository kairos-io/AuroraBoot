package auroraboot

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/AuroraBoot/pkg/uki"
)

const (
	scaffoldISO = "kairos-netboot-scaffold.iso"
	ukiISO      = "kairos-hadron-v0.4.0-core-amd64-generic-v4.1.2-uki.iso"
)

// runISOBuild runs a build whose deployer writes a plain ISO and a raw disk
// and whose UKI step writes a UKI ISO, then returns the build folder.
func runISOBuild(t *testing.T, outputs builder.OutputOptions) string {
	t.Helper()
	baseDir := t.TempDir()

	writeFiles := func(dir string, names ...string) error {
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	deploy := func(_ context.Context, _ schema.Config, _ schema.ReleaseArtifact, outputDir string, _ io.Writer) error {
		return writeFiles(outputDir, scaffoldISO, scaffoldISO+".sha256", "kairos.raw")
	}
	b := New(baseDir, deploy, nil).WithUKIBuildFunc(func(o uki.Options) error {
		return writeFiles(o.OutputDir, ukiISO, ukiISO+".sha256")
	})

	const id = "iso-cleanup"
	if _, err := b.Build(context.Background(), builder.BuildOptions{
		ID:        id,
		BaseImage: "quay.io/kairos/ubuntu:latest",
		Outputs:   outputs,
		Signing: builder.SigningOptions{
			UKISecureBootKey:  "/keys/db.key",
			UKISecureBootCert: "/keys/db.pem",
			UKITPMPCRKey:      "/keys/tpm.pem",
		},
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		s, _ := b.Status(context.Background(), id)
		if s != nil && s.Phase == builder.BuildReady {
			break
		}
		if s != nil && s.Phase == builder.BuildError {
			t.Fatalf("build failed: %s", s.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("build did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return filepath.Join(baseDir, id)
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// Without an ISO output, the ISO the deployer wrote as netboot scaffolding is
// removed, while the ISO the UKI step produced is the build's output and stays.
func TestISOCleanupKeepsUKIISO(t *testing.T) {
	dir := runISOBuild(t, builder.OutputOptions{Netboot: true, UKI: true})

	if fileExists(dir, scaffoldISO) || fileExists(dir, scaffoldISO+".sha256") {
		t.Fatalf("scaffolding ISO should be removed")
	}
	if !fileExists(dir, ukiISO) || !fileExists(dir, ukiISO+".sha256") {
		t.Fatalf("UKI ISO and its checksum must be kept")
	}
	if !fileExists(dir, "kairos.raw") {
		t.Fatalf("non-ISO file must be untouched")
	}
}

// With an ISO output requested, both the plain ISO and the UKI ISO are kept.
func TestISOCleanupSkippedWhenISORequested(t *testing.T) {
	dir := runISOBuild(t, builder.OutputOptions{ISO: true, UKI: true})

	for _, n := range []string{scaffoldISO, scaffoldISO + ".sha256", ukiISO, ukiISO + ".sha256", "kairos.raw"} {
		if !fileExists(dir, n) {
			t.Fatalf("%s should be kept when an ISO is requested", n)
		}
	}
}
