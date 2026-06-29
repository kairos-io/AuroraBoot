package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageMaasCurtinHook(t *testing.T) {
	raw := NewEFIRawImage("", "", "", 0, 0, true)
	raw.maas = true
	dir := t.TempDir()
	if err := raw.stageMaasCurtinHook(dir); err != nil {
		t.Fatalf("stageMaasCurtinHook: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "curtin", "curtin-hooks"))
	if err != nil {
		t.Fatalf("hook not staged: %v", err)
	}
	if len(got) == 0 || string(got) != string(maasCurtinHook) {
		t.Fatalf("staged hook does not match embedded asset (len=%d)", len(got))
	}
}

func TestStageMaasCurtinHookSkippedWhenNotMaas(t *testing.T) {
	raw := NewEFIRawImage("", "", "", 0, 0, true)
	raw.maas = false
	dir := t.TempDir()
	if err := raw.stageMaasCurtinHook(dir); err != nil {
		t.Fatalf("stageMaasCurtinHook: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "curtin")); !os.IsNotExist(err) {
		t.Fatalf("curtin dir should not exist when maas=false (err=%v)", err)
	}
}
