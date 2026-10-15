package auroraboot

import (
	"os"
	"path/filepath"
	"testing"
)

// Captures issue kairos-io/kairos#5495 as it behaves today: the Step 2.7
// cleanup deletes every ISO in the build folder, including the one the UKI
// step produced. The scaffolding ISO written only for netboot is removed too
// (that part is correct and must keep working after the fix).
func TestRemoveUnrequestedISOsRemovesUKIISOCurrentBug(t *testing.T) {
	dir := t.TempDir()
	scaffold := "kairos-netboot-scaffold.iso"
	uki := "kairos-hadron-v0.4.0-core-amd64-generic-v4.1.2-uki.iso"
	for _, f := range []string{scaffold, scaffold + ".sha256", uki, uki + ".sha256", "kairos.raw"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	removeUnrequestedISOs(dir, nil)

	exists := func(n string) bool { _, err := os.Stat(filepath.Join(dir, n)); return err == nil }
	if exists(scaffold) || exists(scaffold+".sha256") {
		t.Fatalf("scaffolding ISO should be removed")
	}
	// BUG: the UKI ISO and its checksum are deleted as well.
	if exists(uki) || exists(uki+".sha256") {
		t.Fatalf("expected current buggy behaviour (UKI ISO removed), but it survived")
	}
	if !exists("kairos.raw") {
		t.Fatalf("non-ISO file must be untouched")
	}
}
