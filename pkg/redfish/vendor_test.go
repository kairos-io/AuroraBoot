package redfish

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewVendorClient(t *testing.T) {
	testCases := []struct {
		name        string
		vendor      VendorType
		expectError bool
	}{
		{
			name:        "Generic Vendor",
			vendor:      VendorGeneric,
			expectError: false,
		},
		{
			name:        "SuperMicro Vendor",
			vendor:      VendorSuperMicro,
			expectError: false,
		},
		{
			name:        "HPE iLO Vendor",
			vendor:      VendorHPE,
			expectError: false,
		},
		{
			name:        "Unknown Vendor",
			vendor:      "unknown",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewVendorClient(
				tc.vendor,
				"https://example.com",
				"admin",
				"password",
				true,
				10*time.Second,
			)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)

				// Verify the client type based on vendor
				switch tc.vendor {
				case VendorGeneric:
					_, ok := client.(*GenericClient)
					assert.True(t, ok, "Expected GenericClient")
				case VendorSuperMicro:
					_, ok := client.(*SuperMicroClient)
					assert.True(t, ok, "Expected SuperMicroClient")
				case VendorHPE:
					_, ok := client.(*HPEClient)
					assert.True(t, ok, "Expected HPEClient")
				}
			}
		})
	}
}

func TestVendorType_String(t *testing.T) {
	testCases := []struct {
		name     string
		vendor   VendorType
		expected string
	}{
		{
			name:     "Generic Vendor",
			vendor:   VendorGeneric,
			expected: "generic",
		},
		{
			name:     "SuperMicro Vendor",
			vendor:   VendorSuperMicro,
			expected: "supermicro",
		},
		{
			name:     "HPE iLO Vendor",
			vendor:   VendorHPE,
			expected: "ilo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, string(tc.vendor))
		})
	}
}
