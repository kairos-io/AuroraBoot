package hardware

import (
	"fmt"

	"github.com/kairos-io/AuroraBoot/pkg/redfish"
)

// Requirements defines the minimum hardware requirements for deployment
type Requirements struct {
	MinMemoryGiB     int
	MinStorageGiB    int
	MinCPUs          int
	RequiredFeatures []string
}

// Inspector handles hardware inspection and validation
type Inspector struct {
	client redfish.VendorClient
}

// NewInspector creates a new hardware inspector
func NewInspector(client redfish.VendorClient) *Inspector {
	return &Inspector{
		client: client,
	}
}

// InspectSystem performs a comprehensive hardware inspection
func (i *Inspector) InspectSystem() (*SystemInfo, error) {
	sysInfo, err := i.client.GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}

	// Convert to our internal SystemInfo type
	info := &SystemInfo{
		MemoryGiB:      sysInfo.MemorySize,
		ProcessorCount: sysInfo.ProcessorCount,
		Model:          sysInfo.Model,
		Manufacturer:   sysInfo.Manufacturer,
		SerialNumber:   sysInfo.SerialNumber,
	}

	return info, nil
}

// SystemInfo represents the inspected system information
type SystemInfo struct {
	MemoryGiB      int
	ProcessorCount int
	Model          string
	Manufacturer   string
	SerialNumber   string
}

// ValidateRequirements checks if the system meets the minimum requirements
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

// hasFeature checks if the system has a specific feature
func (i *Inspector) hasFeature(info *SystemInfo, feature string) bool {
	// Add feature detection logic here
	// For now, we'll assume all systems have UEFI
	return true
}
