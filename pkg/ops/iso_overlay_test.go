package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeISOOverlayDereferencesAtomicWriterSymlinks(t *testing.T) {
	overlay := t.TempDir()
	version := filepath.Join(overlay, "..2026_08_20_12_00_00.0000000000")
	if err := os.MkdirAll(filepath.Join(version, "boot", "grub2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(version, "boot", "grub2", "grub.cfg"), []byte("custom grub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(version), filepath.Join(overlay, "..data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..data", "boot"), filepath.Join(overlay, "boot")); err != nil {
		t.Fatal(err)
	}

	materialized, cleanup, err := materializeISOOverlay(overlay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	info, err := os.Lstat(filepath.Join(materialized, "boot"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("boot must be a real directory, got mode %s", info.Mode())
	}
	content, err := os.ReadFile(filepath.Join(materialized, "boot", "grub2", "grub.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "custom grub" {
		t.Fatalf("unexpected grub.cfg content %q", content)
	}
	if _, err := os.Lstat(filepath.Join(materialized, "..data")); !os.IsNotExist(err) {
		t.Fatalf("atomic-writer metadata was copied: %v", err)
	}
}

func TestMaterializeISOOverlayRejectsEscapingSymlinks(t *testing.T) {
	overlay := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "secret"), []byte("do not copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "secret"), filepath.Join(overlay, "secret")); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := materializeISOOverlay(overlay)
	if cleanup != nil {
		cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "escapes overlay root") {
		t.Fatalf("expected escaping symlink error, got %v", err)
	}
}

func TestMaterializeISOOverlayPreservesOrdinaryEntries(t *testing.T) {
	overlay := t.TempDir()
	if err := os.WriteFile(filepath.Join(overlay, "target"), []byte("target"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(overlay, "target"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "..keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(overlay, "link")); err != nil {
		t.Fatal(err)
	}

	materialized, cleanup, err := materializeISOOverlay(overlay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	link, err := os.Lstat(filepath.Join(materialized, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if link.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ordinary link was dereferenced: mode %s", link.Mode())
	}
	if target, err := os.Readlink(filepath.Join(materialized, "link")); err != nil || target != "target" {
		t.Fatalf("ordinary link target = %q, %v", target, err)
	}
	if content, err := os.ReadFile(filepath.Join(materialized, "..keep")); err != nil || string(content) != "keep" {
		t.Fatalf("ordinary dot entry was not preserved: %q, %v", content, err)
	}
	info, err := os.Stat(filepath.Join(materialized, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o777 {
		t.Fatalf("target mode = %o, want 777", info.Mode().Perm())
	}
}

func TestMaterializeISOOverlayPreservesLinkIntoOrdinaryDotDataDirectory(t *testing.T) {
	overlay := t.TempDir()
	if err := os.Mkdir(filepath.Join(overlay, "..data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "..data", "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..data", "file"), filepath.Join(overlay, "link")); err != nil {
		t.Fatal(err)
	}

	materialized, cleanup, err := materializeISOOverlay(overlay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	info, err := os.Lstat(filepath.Join(materialized, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ordinary ..data link was dereferenced: mode %s", info.Mode())
	}
}

func TestMaterializeISOOverlayPopulatesReadOnlyDirectories(t *testing.T) {
	overlay := t.TempDir()
	readOnly := filepath.Join(overlay, "read-only")
	if err := os.Mkdir(readOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(readOnly, "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })

	materialized, cleanup, err := materializeISOOverlay(overlay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if content, err := os.ReadFile(filepath.Join(materialized, "read-only", "file")); err != nil || string(content) != "data" {
		t.Fatalf("read-only directory content = %q, %v", content, err)
	}
	info, err := os.Stat(filepath.Join(materialized, "read-only"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("read-only directory mode = %o, want 555", info.Mode().Perm())
	}
}
