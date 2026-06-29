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

	// bin/sh must be a symlink pointing to busybox
	for _, link := range []string{"sh", "ash", "bash"} {
		target, err := os.Readlink(filepath.Join(dir, "bin", link))
		if err != nil {
			t.Fatalf("bin/%s symlink missing: %v", link, err)
		}
		if target != "busybox" {
			t.Fatalf("bin/%s -> %q, want busybox", link, target)
		}
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
}
