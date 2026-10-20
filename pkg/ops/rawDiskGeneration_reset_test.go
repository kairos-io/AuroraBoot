package ops

import (
	"os"
	"path/filepath"
	"testing"

	sdkConstants "github.com/kairos-io/kairos/v4/sdk/constants"
	"github.com/mudler/yip/pkg/schema"
	"gopkg.in/yaml.v3"
)

// The first-boot reset config creates COS_STATE and COS_PERSISTENT on a raw
// disk. They are partitions holding live, writable data (k3s/etcd, containerd,
// kubelet), so they have to get the partition filesystem that a regular
// install uses, not the journal-less one meant for the active/passive/recovery
// image files.
func TestResetConfigCreatesStateAndPersistentWithThePartitionFilesystem(t *testing.T) {
	work := t.TempDir()
	oem := filepath.Join(work, "oem")
	if err := os.MkdirAll(oem, 0o755); err != nil {
		t.Fatalf("creating the OEM staging dir: %v", err)
	}
	cloudConfig := filepath.Join(work, "config.yaml")
	if err := os.WriteFile(cloudConfig, []byte("#cloud-config\n"), 0o644); err != nil {
		t.Fatalf("writing the cloud config: %v", err)
	}
	recovery := filepath.Join(work, "recovery.img")
	if err := os.WriteFile(recovery, make([]byte, 1024), 0o644); err != nil {
		t.Fatalf("writing the fake recovery image: %v", err)
	}

	image := newTestRawImage(nil)
	image.CloudConfig = cloudConfig
	if _, err := image.stageOemContents(oem, recovery); err != nil {
		t.Fatalf("stageOemContents: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(oem, "01_reset.yaml"))
	if err != nil {
		t.Fatalf("reading the reset cloud config: %v", err)
	}
	var cfg schema.YipConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("parsing the reset cloud config: %v", err)
	}

	got := map[string]string{}
	for _, stage := range cfg.Stages["rootfs.before"] {
		for _, part := range stage.Layout.Parts {
			got[part.FSLabel] = part.FileSystem
		}
	}
	for _, label := range []string{sdkConstants.StateLabel, sdkConstants.PersistentLabel} {
		fs, ok := got[label]
		if !ok {
			t.Fatalf("the reset config does not create %s", label)
		}
		if fs != sdkConstants.LinuxFs {
			t.Errorf("%s is created as %q, want %q", label, fs, sdkConstants.LinuxFs)
		}
	}
}
