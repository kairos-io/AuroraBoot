package redfish

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVendorClient(t *testing.T) {
	testCases := []struct {
		name        string
		vendor      VendorType
		wantErr     bool
		errContains string
	}{
		{
			name:    "Generic Vendor",
			vendor:  VendorGeneric,
			wantErr: false,
		},
		{
			name:    "SuperMicro Vendor",
			vendor:  VendorSuperMicro,
			wantErr: false,
		},
		{
			name:    "HPE iLO Vendor",
			vendor:  VendorHPE,
			wantErr: false,
		},
		{
			name:        "Unknown Vendor",
			vendor:      "unknown",
			wantErr:     true,
			errContains: "unsupported vendor: unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Mock authentication endpoint
				if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
					w.Header().Set("X-Auth-Token", "test-token")
					w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
					w.WriteHeader(http.StatusCreated)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			// Test client creation
			client, err := NewVendorClient(tc.vendor, server.URL, "admin", "password", true, 10*time.Second)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, client)
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
