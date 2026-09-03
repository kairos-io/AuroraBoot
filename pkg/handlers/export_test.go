package handlers

import (
	"context"
	"io"

	"github.com/kairos-io/AuroraBoot/pkg/redfish"
)

// RedfishFinalizer is the test-visible alias of the unexported redfishFinalizer
// interface, so external (handlers_test) tests can supply a fake BMC client to the
// eject/finalize path without standing up a real gofish connection.
type RedfishFinalizer = redfishFinalizer

// WithTestFinalizerFactory injects a fake redfishFinalizer factory keyed on the
// redfish.Config the deploy/finalize path would otherwise pass to NewDeployer. It
// returns the handler for chaining. Test-only.
func (h *DeployHandler) WithTestFinalizerFactory(f func(cfg redfish.Config) RedfishFinalizer) *DeployHandler {
	return h.WithFinalizerFactory(func(cfg redfish.Config) redfishFinalizer { return f(cfg) })
}

// MarkEjectPendingForTest exposes the unexported markEjectPending so a test can arm
// a deployment's eject lifecycle without driving the full async deploy goroutine.
func (h *DeployHandler) MarkEjectPendingForTest(id string) { h.markEjectPending(id) }

// ImageURLUsesHTTPS exposes the unexported imageURLUsesHTTPS helper to external
// (handlers_test) tests so the InsertMedia transfer-protocol derivation can be
// exercised directly.
var ImageURLUsesHTTPS = imageURLUsesHTTPS

// ResolveOperatorImageURL exposes the unexported image-URL precedence helper so
// the per-deploy > per-BMC > global-default selection can be unit-tested
// directly, without driving the async deploy goroutine.
var ResolveOperatorImageURL = resolveOperatorImageURL

// WithTestImageExporter substitutes the docker create/export/import/save
// pipeline behind GET /api/v1/artifacts/:id/image, so the endpoint's queueing,
// header and error behavior can be tested without a docker daemon. It returns
// the handler for chaining. Test-only.
func (h *ArtifactHandler) WithTestImageExporter(f func(ctx context.Context, containerImage string, w io.Writer) error) *ArtifactHandler {
	h.exportImage = imageExportFunc(f)
	return h
}

// ExportLockCountForTest reports how many artifacts currently hold an export
// lock entry, so a test can prove the map is emptied as requests finish rather
// than growing for the life of the server. Test-only.
func (h *ArtifactHandler) ExportLockCountForTest() int { return h.exportLocks.count() }

// ExportObjectNamesForTest exposes the per-export docker container name and flat
// image tag, so their uniqueness - the property whose absence caused the
// concurrent-export failures - can be asserted directly. Test-only.
var ExportObjectNamesForTest = exportObjectNames
