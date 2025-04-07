package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestRedFishDeployCmd(t *testing.T) {
	// Create a temporary ISO file
	tempDir, err := os.MkdirTemp("", "redfish-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	isoPath := filepath.Join(tempDir, "test.iso")
	err = os.WriteFile(isoPath, []byte("test iso content"), 0644)
	require.NoError(t, err)

	// Test cases for different vendors
	testCases := []struct {
		name        string
		vendor      string
		expectError bool
	}{
		{
			name:        "Generic Vendor",
			vendor:      "generic",
			expectError: false,
		},
		{
			name:        "SuperMicro Vendor",
			vendor:      "supermicro",
			expectError: false,
		},
		{
			name:        "HPE iLO Vendor",
			vendor:      "ilo",
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
			// Create a new app for each test case
			app := &cli.App{
				Commands: []*cli.Command{
					&RedFishDeployCmd,
				},
			}

			// Set up test arguments
			args := []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", tc.vendor,
				"--verify-ssl", "true",
				"--min-memory", "4",
				"--min-cpus", "2",
				"--required-features", "UEFI",
				"--timeout", "5m",
				isoPath,
			}

			// Run the command
			err := app.Run(args)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				// We expect an error here because we're not actually connecting to a real server
				// But we want to verify that the command at least parsed correctly
				assert.Error(t, err)
				// The error should not be about invalid vendor
				assert.NotContains(t, err.Error(), "unsupported vendor")
			}
		})
	}
}

func TestRedFishDeployCmd_Validation(t *testing.T) {
	// Create a temporary ISO file
	tempDir, err := os.MkdirTemp("", "redfish-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	isoPath := filepath.Join(tempDir, "test.iso")
	err = os.WriteFile(isoPath, []byte("test iso content"), 0644)
	require.NoError(t, err)

	// Test cases for validation
	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Missing Endpoint",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--username", "admin",
				"--password", "password",
				"--vendor", "generic",
				isoPath,
			},
			expectError: true,
			errorMsg:    "required flag",
		},
		{
			name: "Missing Username",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--password", "password",
				"--vendor", "generic",
				isoPath,
			},
			expectError: true,
			errorMsg:    "required flag",
		},
		{
			name: "Missing Password",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--vendor", "generic",
				isoPath,
			},
			expectError: true,
			errorMsg:    "required flag",
		},
		{
			name: "Missing ISO Path",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", "generic",
			},
			expectError: true,
			errorMsg:    "ISO path is required",
		},
		{
			name: "Invalid Memory Value",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", "generic",
				"--min-memory", "invalid",
				isoPath,
			},
			expectError: true,
			errorMsg:    "invalid value",
		},
		{
			name: "Invalid CPU Value",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", "generic",
				"--min-cpus", "invalid",
				isoPath,
			},
			expectError: true,
			errorMsg:    "invalid value",
		},
		{
			name: "Invalid Timeout Value",
			args: []string{
				"auroraboot",
				"deploy-redfish",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", "generic",
				"--timeout", "invalid",
				isoPath,
			},
			expectError: true,
			errorMsg:    "invalid value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new app for each test case
			app := &cli.App{
				Commands: []*cli.Command{
					&RedFishDeployCmd,
				},
			}

			// Run the command
			err := app.Run(tc.args)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
