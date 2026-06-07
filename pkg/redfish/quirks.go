package redfish

import "github.com/stmcginnis/gofish/schemas"

// quirks is a per-vendor profile of optional hook functions that influence the
// genuine deltas between BMC implementations. The shared auth/discovery/insert/
// boot/reset/task machinery lives in the core Deployer; a quirk only customises
// the small set of decisions where vendors actually diverge.
//
// Every field is optional. A nil hook means "use the spec-default behaviour", so
// the generic profile (genericQuirks) is the zero value and the default path is
// byte-for-byte identical to having no quirk seam at all. This is the contract
// the regression tests assert: VendorGeneric must never change behaviour.
type quirks struct {
	// name identifies the profile (for diagnostics only).
	name string

	// mediaSearch returns the ordered list of VirtualMedia collections to search
	// for a CD/DVD device. The core supplies the spec-default collections (System
	// first, then each Manager). A profile may reorder or filter them — e.g. HPE
	// iLO hosts virtual media under the Manager, so the iLO profile prefers the
	// Manager collections. Returning nil falls back to the core default.
	mediaSearch func(d *Deployer, system *schemas.ComputerSystem, def mediaCollections) mediaCollections

	// mediaType selects the VirtualMedia MediaType sent on InsertMedia. nil uses
	// the spec default (CD).
	mediaType func(vm *schemas.VirtualMedia) schemas.VirtualMediaType

	// resetType lets a profile override the chosen ResetType. It receives the
	// spec-default choice (chooseResetType) and may return a different one. nil
	// keeps the default.
	resetType func(system *schemas.ComputerSystem, def schemas.ResetType) schemas.ResetType

	// tuneInsertParams adjusts the InsertMedia parameters after the core has built
	// the spec-default set. A profile may, for example, drop TransferProtocolType
	// or WriteProtected for BMCs that reject them. nil leaves the params untouched.
	tuneInsertParams func(params *schemas.VirtualMediaInsertMediaParameters, req DeployRequest)

	// pushMedia is an optional hook point for a future multipart media push (some
	// modern BMCs accept a multipart HTTP POST of the ISO bytes instead of a
	// URL-pull InsertMedia). It is a STUB seam only: no profile implements it yet
	// and the core never calls it. It exists so the multipart work (a later phase)
	// has a defined extension point rather than re-plumbing the flow. When a
	// profile returns (handled=true), it has performed the media insertion itself.
	pushMedia func(d *Deployer, media *schemas.VirtualMedia, req DeployRequest) (handled bool, info *schemas.TaskMonitorInfo, err error)
}

// mediaCollections is an ordered list of VirtualMedia collections (each itself a
// slice of media members). Search order is significant: the first CD/DVD-capable
// member wins.
type mediaCollections [][]*schemas.VirtualMedia

// quirksFor returns the quirk profile for a VendorType. Unknown/empty vendors map
// to the generic (spec-default) profile so a typo can never silently enable a
// vendor workaround.
func quirksFor(v VendorType) quirks {
	switch v {
	case VendorHPE:
		return iloQuirks()
	case VendorSuperMicro:
		return supermicroQuirks()
	case VendorGeneric, VendorDMTF, "":
		return genericQuirks()
	default:
		return genericQuirks()
	}
}

// genericQuirks is the spec-default profile: all hooks nil. DMTF/sushy-tools/
// PiKVM use this. Keeping it the zero value (plus a name) guarantees the default
// path is unchanged.
func genericQuirks() quirks {
	return quirks{name: "generic"}
}
