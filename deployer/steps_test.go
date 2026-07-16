package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
)

// TestTmpRootFsNotUnderStateDir is a regression test for
// https://github.com/kairos-io/kairos/issues/3922.
//
// state_dir is commonly a host bind mount. On Docker Desktop for macOS that
// mount is backed by VirtioFS, which cannot represent all the Linux metadata
// containerd's tar-apply sets while unpacking a rootfs, so unpacking there
// fails with "failed to Lchown ... permission denied". The intermediate rootfs
// must therefore stay on the local filesystem (os.TempDir()), never under
// state_dir.
func TestTmpRootFsNotUnderStateDir(t *testing.T) {
	stateDir := "/output" // a bind-mounted state_dir, as in the bug report
	d := &Deployer{Config: schema.Config{State: stateDir}}

	tmp := d.tmpRootFs()

	if strings.HasPrefix(filepath.Clean(tmp), filepath.Clean(stateDir)+string(os.PathSeparator)) {
		t.Fatalf("tmpRootFs() must not live under state_dir %q, got %q", stateDir, tmp)
	}

	if want := os.TempDir(); !strings.HasPrefix(filepath.Clean(tmp), filepath.Clean(want)) {
		t.Fatalf("tmpRootFs() must live under the local temp dir %q, got %q", want, tmp)
	}
}

// TestTmpRootFsStable ensures the path is deterministic across calls, since it
// is evaluated both eagerly at step registration and lazily inside callbacks.
func TestTmpRootFsStable(t *testing.T) {
	d := &Deployer{Config: schema.Config{State: "/output"}}

	if a, b := d.tmpRootFs(), d.tmpRootFs(); a != b {
		t.Fatalf("tmpRootFs() must be stable across calls, got %q then %q", a, b)
	}
}
