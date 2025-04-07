package redfish

import (
	"fmt"
	"time"
)

// VendorType represents the type of hardware vendor
type VendorType string

const (
	VendorGeneric    VendorType = "generic"
	VendorSuperMicro VendorType = "supermicro"
	VendorHPE        VendorType = "ilo"
	VendorDMTF       VendorType = "dmtf"
)

// VendorClient interface defines vendor-specific RedFish operations
type VendorClient interface {
	// GetSystemInfo retrieves basic system information
	GetSystemInfo() (*SystemInfo, error)
	// DeployISO deploys an ISO image to the target system
	DeployISO(isoPath string) (*DeploymentStatus, error)
	// GetDeploymentStatus retrieves the current status of the deployment
	GetDeploymentStatus() (*DeploymentStatus, error)
}

// NewVendorClient creates a new vendor-specific RedFish client
func NewVendorClient(vendor VendorType, baseURL, username, password string, verifySSL bool, timeout time.Duration) (VendorClient, error) {
	switch vendor {
	case VendorSuperMicro:
		return NewSuperMicroClient(baseURL, username, password, verifySSL, timeout)
	case VendorHPE:
		return NewHPEClient(baseURL, username, password, verifySSL, timeout)
	case VendorDMTF:
		return NewDMTFClient(baseURL, username, password, verifySSL, timeout)
	case VendorGeneric:
		return NewGenericClient(baseURL, username, password, verifySSL, timeout)
	default:
		return nil, fmt.Errorf("unsupported vendor: %s", vendor)
	}
}
