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
	client *redfish.Client
}

// NewInspector creates a new hardware inspector
func NewInspector(client *redfish.Client) *Inspector {
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

// ValidateRequirements checks if the system meets the specified requirements
func (i *Inspector) ValidateRequirements(info *SystemInfo, reqs *Requirements) error {
	if info.MemoryGiB < reqs.MinMemoryGiB {
		return fmt.Errorf("insufficient memory: got %d GiB, need %d GiB",
			info.MemoryGiB, reqs.MinMemoryGiB)
	}

	if info.ProcessorCount < reqs.MinCPUs {
		return fmt.Errorf("insufficient CPUs: got %d, need %d",
			info.ProcessorCount, reqs.MinCPUs)
	}

	// Additional feature checks could be added here
	for _, feature := range reqs.RequiredFeatures {
		if !i.hasFeature(info, feature) {
			return fmt.Errorf("missing required feature: %s", feature)
		}
	}

	return nil
}

// hasFeature checks if the system has a specific feature
func (i *Inspector) hasFeature(info *SystemInfo, feature string) bool {
	// This is a placeholder implementation
	// In a real implementation, this would check specific hardware features
	// based on the manufacturer and model
	switch feature {
	case "UEFI":
		return true // Most modern systems support UEFI
	case "IPMI":
		return true // Most server hardware supports IPMI
	default:
		return false
	}
}
