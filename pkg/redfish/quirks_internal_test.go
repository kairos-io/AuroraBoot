package redfish

import (
	"errors"
	"strings"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
)

// TestGenericProfileHasNoHooks is the regression guard for the seam: the generic
// (and DMTF, and empty/unknown) profile must have every hook nil, which is what
// guarantees the default path is byte-for-byte unchanged through the new seam.
func TestGenericProfileHasNoHooks(t *testing.T) {
	for _, v := range []VendorType{VendorGeneric, VendorDMTF, "", "totally-unknown"} {
		q := quirksFor(v)
		if q.mediaSearch != nil || q.mediaType != nil || q.resetType != nil ||
			q.tuneInsertParams != nil || q.pushMedia != nil {
			t.Fatalf("vendor %q resolved to a profile with non-nil hooks; the default path must stay spec-default", v)
		}
	}
}

// TestVendorProfilesSelected confirms the vendor selector wires the right named
// profile (so a future hook lands on the intended vendor).
func TestVendorProfilesSelected(t *testing.T) {
	cases := map[VendorType]string{
		VendorHPE:        "ilo",
		VendorSuperMicro: "supermicro",
		VendorGeneric:    "generic",
	}
	for v, want := range cases {
		if got := quirksFor(v).name; got != want {
			t.Fatalf("quirksFor(%q).name = %q, want %q", v, got, want)
		}
	}
}

// TestILOPrefersManagerMedia documents/locks the one safe iLO divergence at the
// hook level: given Manager-hosted media it is searched ahead of System media.
func TestILOPrefersManagerMedia(t *testing.T) {
	q := iloQuirks()
	if q.mediaSearch == nil {
		t.Fatal("iLO profile must define a mediaSearch hook")
	}
	// The hook needs a *Deployer to read managers; we cannot easily fake the
	// gofish service here, so the end-to-end ordering is asserted in the
	// fake-BMC-driven spec (deployer_test.go). This test just guards the hook's
	// presence and the supermicro profile's deliberate absence of hooks.
	if iloQuirks().tuneInsertParams != nil {
		t.Fatal("iLO must not silently tweak InsertMedia params (no verified need)")
	}
	if supermicroQuirks().mediaSearch != nil || supermicroQuirks().tuneInsertParams != nil {
		t.Fatal("supermicro profile must stay spec-default until verified on real HW (#7)")
	}
}

// TestExtendedInfoSummary verifies the ExtendedInfo extraction renders the
// actionable fields and skips empty entries.
func TestExtendedInfoSummary(t *testing.T) {
	infos := []schemas.ErrExtendedInfo{
		{Message: "Image unreachable", MessageID: "Base.1.0.X", Resolution: "Check the URL"},
		{}, // empty entry must be skipped
	}
	got := extendedInfoSummary(infos)
	for _, want := range []string{"Image unreachable", "Base.1.0.X", "Check the URL"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
	if strings.Contains(got, ";;") {
		t.Fatalf("empty ExtendedInfo entry was not skipped: %q", got)
	}
	if extendedInfoSummary(nil) != "" {
		t.Fatal("nil ExtendedInfo must summarise to empty string")
	}
}

// TestEnrichRedfishError appends the ExtendedInfo summary to a gofish *Error and
// leaves non-Redfish errors untouched.
func TestEnrichRedfishError(t *testing.T) {
	plain := errors.New("boom")
	if enrichRedfishError(plain) != plain {
		t.Fatal("non-Redfish error must be returned unchanged")
	}
	if enrichRedfishError(nil) != nil {
		t.Fatal("nil must stay nil")
	}

	rfErr := &schemas.Error{
		Code:          "Base.1.0.GeneralError",
		Message:       "failed",
		ExtendedInfos: []schemas.ErrExtendedInfo{{Message: "deep detail", Resolution: "do X"}},
	}
	enriched := enrichRedfishError(rfErr)
	if !strings.Contains(enriched.Error(), "deep detail") || !strings.Contains(enriched.Error(), "do X") {
		t.Fatalf("enriched error missing ExtendedInfo detail: %v", enriched)
	}
	// Must remain unwrappable back to the underlying *schemas.Error.
	var as *schemas.Error
	if !errors.As(enriched, &as) {
		t.Fatal("enriched error must still unwrap to *schemas.Error")
	}
}

// TestKnownTaskState locks the enum membership check that pollTask relies on to
// reject garbage states.
func TestKnownTaskState(t *testing.T) {
	known := []schemas.TaskState{
		schemas.NewTaskState, schemas.RunningTaskState, schemas.CompletedTaskState,
		schemas.ExceptionTaskState, schemas.CancelledTaskState, schemas.PendingTaskState,
	}
	for _, s := range known {
		if !isKnownTaskState(s) {
			t.Fatalf("state %q should be known", s)
		}
	}
	for _, s := range []schemas.TaskState{"", "Frobnicating", "garbage", "running"} {
		if isKnownTaskState(s) {
			t.Fatalf("state %q should be unknown", s)
		}
	}
}
