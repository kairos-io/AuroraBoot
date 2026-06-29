package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageCurtinLanding(t *testing.T) {
	dir := t.TempDir()
	// Use a fake busybox payload.
	fakeBusybox := []byte("fake-busybox-binary")
	if err := stageCurtinLanding(dir, fakeBusybox); err != nil {
		t.Fatalf("stageCurtinLanding: %v", err)
	}

	// bin/busybox must exist with the right content
	bbPath := filepath.Join(dir, "bin", "busybox")
	got, err := os.ReadFile(bbPath)
	if err != nil {
		t.Fatalf("bin/busybox not written: %v", err)
	}
	if string(got) != string(fakeBusybox) {
		t.Fatalf("bin/busybox content mismatch")
	}

	// sh/ash are real busybox applets -> symlinks. bash is not an applet, so it
	// must be a wrapper script forwarding to busybox sh (curtin runs bash -c).
	for _, link := range []string{"sh", "ash"} {
		target, err := os.Readlink(filepath.Join(dir, "bin", link))
		if err != nil {
			t.Fatalf("bin/%s symlink missing: %v", link, err)
		}
		if target != "busybox" {
			t.Fatalf("bin/%s -> %q, want busybox", link, target)
		}
	}
	bashWrapper, err := os.ReadFile(filepath.Join(dir, "bin", "bash"))
	if err != nil {
		t.Fatalf("bin/bash wrapper missing: %v", err)
	}
	if string(bashWrapper) != "#!/bin/sh\nexec /bin/busybox sh \"$@\"\n" {
		t.Fatalf("bin/bash wrapper content wrong: %q", string(bashWrapper))
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "bin", "bash")); fi != nil && fi.Mode()&0o111 == 0 {
		t.Fatalf("bin/bash not executable")
	}

	// stub executables
	for _, s := range []string{"cloud-init", "netplan"} {
		p := filepath.Join(dir, "usr/bin", s)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("usr/bin/%s not written: %v", s, err)
		}
		if string(b) != "#!/bin/sh\nexit 0\n" {
			t.Fatalf("usr/bin/%s content wrong: %q", s, b)
		}
	}

	// curtin/curtin-hooks must match the embedded asset
	hookPath := filepath.Join(dir, "curtin", "curtin-hooks")
	hookBytes, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("curtin/curtin-hooks not written: %v", err)
	}
	if len(hookBytes) == 0 || string(hookBytes) != string(maasCurtinHook) {
		t.Fatalf("curtin/curtin-hooks does not match embedded asset (len=%d)", len(hookBytes))
	}

	// curtin's ChrootableTarget bind-mounts these into the target before the
	// in-target validation; they must exist as empty dirs or the deploy fails.
	for _, d := range []string{"proc", "sys", "dev", "run", "tmp"} {
		fi, err := os.Stat(filepath.Join(dir, d))
		if err != nil || !fi.IsDir() {
			t.Fatalf("mountpoint dir %q missing: %v", d, err)
		}
	}
}
