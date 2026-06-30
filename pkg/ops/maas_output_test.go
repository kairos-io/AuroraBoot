package ops

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// MAAS consumes a custom image with the "ddgz" filetype: a gzip-compressed raw
// disk. Raw2MAAS gzips the raw produced by the EFI build into <raw>.gz so the
// artifact is directly uploadable to MAAS without a manual compression step.
func TestRaw2MAAS(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "kairos-maas.raw")
	payload := []byte("this is a fake raw disk image payload, repeated for some size\n")
	want := bytes.Repeat(payload, 1000)
	if err := os.WriteFile(raw, want, 0o644); err != nil {
		t.Fatalf("writing fake raw: %v", err)
	}

	out, err := Raw2MAAS(raw)
	if err != nil {
		t.Fatalf("Raw2MAAS: %v", err)
	}

	if out != raw+".gz" {
		t.Fatalf("output name = %q, want %q", out, raw+".gz")
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("gzip output not created: %v", err)
	}

	// The .gz must decompress back to the exact original raw bytes.
	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("opening output: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not a valid gzip stream: %v", err)
	}
	got, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading gzip stream: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("decompressed content does not match original raw (got %d bytes, want %d)", len(got), len(want))
	}

	// The original raw must still be present (mirrors the gce/vhd converters,
	// which leave the raw alongside the converted artifact).
	if _, err := os.Stat(raw); err != nil {
		t.Fatalf("original raw should be left in place: %v", err)
	}
}
