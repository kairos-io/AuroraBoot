package hardware

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/pkg/redfish"
)

// Requirements defines the minimum hardware requirements for deployment.
type Requirements struct {
	MinMemoryGiB     int
	MinStorageGiB    int
	MinCPUs          int
	RequiredFeatures []string
}

// systemInspector is the subset of the Redfish Deployer the inspector needs. It
// keeps gofish types out of pkg/hardware (only our own redfish.SystemInfo crosses
// the boundary) and lets tests supply a fake.
type systemInspector interface {
	Inspect(ctx context.Context) (*redfish.SystemInfo, error)
}

// Inspector handles hardware inspection and validation.
type Inspector struct {
	client systemInspector
}

// NewInspector creates a new hardware inspector backed by a Redfish Deployer (or
// any value implementing the inspection contract).
func NewInspector(client systemInspector) *Inspector {
	return &Inspector{
		client: client,
	}
}

// InspectSystem performs a comprehensive hardware inspection.
func (i *Inspector) InspectSystem(ctx context.Context) (*SystemInfo, error) {
	sysInfo, err := i.client.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}

	// Convert to our internal SystemInfo type. Memory/CPU now arrive populated
	// from the Deployer's typed ComputerSystem read (no longer 0/0).
	info := &SystemInfo{
		MemoryGiB:      sysInfo.MemoryGiB,
		ProcessorCount: sysInfo.ProcessorCount,
		Model:          sysInfo.Model,
		Manufacturer:   sysInfo.Manufacturer,
		SerialNumber:   sysInfo.SerialNumber,
	}

	return info, nil
}

// SystemInfo represents the inspected system information.
type SystemInfo struct {
	MemoryGiB      int
	ProcessorCount int
	Model          string
	Manufacturer   string
	SerialNumber   string
}

// ValidateRequirements checks if the system meets the minimum requirements.
func (i *Inspector) ValidateRequirements(info *SystemInfo, reqs *Requirements) error {
	if info.MemoryGiB < reqs.MinMemoryGiB {
		return fmt.Errorf("insufficient memory: %d GiB (minimum: %d GiB)",
			info.MemoryGiB, reqs.MinMemoryGiB)
	}

	if info.ProcessorCount < reqs.MinCPUs {
		return fmt.Errorf("insufficient CPUs: %d (minimum: %d)",
			info.ProcessorCount, reqs.MinCPUs)
	}

	for _, feature := range reqs.RequiredFeatures {
		if !i.hasFeature(info, feature) {
			return fmt.Errorf("missing required feature: %s", feature)
		}
	}

	return nil
}

// hasFeature checks if the system has a specific feature.
//
// NOTE: this is still a stub that always returns true. A real feature gate is
// Phase 4 (#4114); it is intentionally left untouched here so the package keeps
// compiling.
func (i *Inspector) hasFeature(info *SystemInfo, feature string) bool {
	// Add feature detection logic here.
	// For now, we'll assume all systems have UEFI.
	return true
}
