// Package imageref validates the container image references an operator
// supplies over the AuroraBoot API, before they are spliced into a generated
// Dockerfile.
//
// The builder writes `FROM <ref>` lines from fields the caller controls
// (baseImage, kairosInitImage, hadronBase, and an extension's source image).
// A Dockerfile instruction can only start on a line of its own, so a
// reference carrying a newline turns one FROM into FROM plus whatever the
// caller wrote next, including RUN. That is the hole these helpers close.
package imageref

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/google/go-containerregistry/pkg/name"
)

// registrySchemes are the kairos-sdk NewSrcFromURI prefixes that introduce an
// OCI reference. AuroraBoot passes baseImage through to the UKI build
// unchanged, so "oci:quay.io/kairos/ubuntu:latest" is a value an operator can
// legitimately send and must keep working.
var registrySchemes = []string{"oci:", "docker:"}

// pathSchemes are the NewSrcFromURI prefixes that introduce a path on the
// server rather than an image. They are refused here, in line with the
// catalog's existing refusal of file:// extension sources over the API, and
// they have to be named explicitly: go-containerregistry parses "dir:/etc"
// without complaint.
var pathSchemes = []string{"dir:", "file:", "ocifile:"}

// NoControlChars rejects a reference carrying whitespace or a control
// character.
//
// This is the rule that stops Dockerfile instruction injection, and it holds
// for every reference shape, including the schemed source URIs. Use it at the
// point a Dockerfile is rendered, where the value may be a locally built tag
// or a resolved artifact image rather than something a caller typed.
func NoControlChars(field, value string) error {
	for i, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("invalid %s %q: contains %q at offset %d; an image reference cannot carry whitespace or control characters", field, value, r, i)
		}
	}
	return nil
}

// Validate is NoControlChars plus a parse: with an optional oci: or docker:
// scheme stripped, the value must be a reference go-containerregistry
// accepts. Use it at the API boundary, where the value is whatever the caller
// sent.
//
// An empty value is accepted: whether a field is required is the caller's
// business, and every caller already answers that separately.
func Validate(field, value string) error {
	if value == "" {
		return nil
	}
	if err := NoControlChars(field, value); err != nil {
		return err
	}
	for _, scheme := range pathSchemes {
		if strings.HasPrefix(value, scheme) {
			return fmt.Errorf("invalid %s %q: %s names a path on the server, which is not an image reference", field, value, strings.TrimSuffix(scheme, ":"))
		}
	}
	ref := value
	for _, scheme := range registrySchemes {
		if rest, found := strings.CutPrefix(ref, scheme); found {
			ref = strings.TrimPrefix(rest, "//")
			break
		}
	}
	if _, err := name.ParseReference(ref); err != nil {
		return fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	return nil
}
