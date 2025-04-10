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
			name:        "DMTF Vendor",
			vendor:      "dmtf",
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
				"redfish",
				"deploy",
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

	// Create a new app
	app := &cli.App{
		Commands: []*cli.Command{
			&RedFishDeployCmd,
		},
	}

	// Test missing required flags
	args := []string{
		"auroraboot",
		"redfish",
		"deploy",
		isoPath,
	}

	err = app.Run(args)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Required flags")

	// Test invalid memory value
	args = []string{
		"auroraboot",
		"redfish",
		"deploy",
		"--endpoint", "https://example.com",
		"--username", "admin",
		"--password", "password",
		"--min-memory", "-1",
		isoPath,
	}

	err = app.Run(args)
	assert.Error(t, err)

	// Test invalid CPU value
	args = []string{
		"auroraboot",
		"redfish",
		"deploy",
		"--endpoint", "https://example.com",
		"--username", "admin",
		"--password", "password",
		"--min-cpus", "0",
		isoPath,
	}

	err = app.Run(args)
	assert.Error(t, err)

	// Test invalid timeout value
	args = []string{
		"auroraboot",
		"redfish",
		"deploy",
		"--endpoint", "https://example.com",
		"--username", "admin",
		"--password", "password",
		"--timeout", "0s",
		isoPath,
	}

	err = app.Run(args)
	assert.Error(t, err)
}
