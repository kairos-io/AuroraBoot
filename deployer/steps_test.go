package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
)

// isUnder reports whether path is the dir itself or lives inside it, comparing
// on path-segment boundaries so that "/tmp-rootfs" is not considered under
// "/tmp".
func isUnder(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

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

	if isUnder(tmp, stateDir) {
		t.Fatalf("tmpRootFs() must not live under state_dir %q, got %q", stateDir, tmp)
	}
	if !isUnder(tmp, os.TempDir()) {
		t.Fatalf("tmpRootFs() must live under the local temp dir %q, got %q", os.TempDir(), tmp)
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

// TestTmpRootFsNormalizesStateDir ensures superficial differences in state_dir
// (a trailing slash, an unclean path) map to the same unpack directory, so the
// same logical build always resolves to one temp rootfs.
func TestTmpRootFsNormalizesStateDir(t *testing.T) {
	a := &Deployer{Config: schema.Config{State: "/output"}}
	b := &Deployer{Config: schema.Config{State: "/output/"}}
	c := &Deployer{Config: schema.Config{State: "/output/../output"}}

	if a.tmpRootFs() != b.tmpRootFs() || a.tmpRootFs() != c.tmpRootFs() {
		t.Fatalf("equivalent state_dirs must map to the same temp rootfs: %q, %q, %q",
			a.tmpRootFs(), b.tmpRootFs(), c.tmpRootFs())
	}
}

// TestTmpRootFsUniquePerStateDir guards the concurrency invariant: the internal
// builder runs multiple builds in parallel in the same process, each with a
// distinct state_dir. Their unpack directories must not collide, otherwise one
// build's PrepDirs RemoveAll would wipe another build's rootfs mid-unpack.
func TestTmpRootFsUniquePerStateDir(t *testing.T) {
	a := &Deployer{Config: schema.Config{State: "/builds/aaaa"}}
	b := &Deployer{Config: schema.Config{State: "/builds/bbbb"}}

	if a.tmpRootFs() == b.tmpRootFs() {
		t.Fatalf("distinct state_dirs must map to distinct temp rootfs dirs, both got %q", a.tmpRootFs())
	}
}
