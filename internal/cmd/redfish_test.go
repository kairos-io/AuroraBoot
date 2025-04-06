package cmd

import (
	"os"
	"path/filepath"
	"strings"
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

	type TestCase struct {
		name        string
		vendor      string
		expectError bool
	}

	// Test cases for different vendors
	testCases := []TestCase{
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

			if err != nil && strings.Contains(err.Error(), "failed with status: 403") {
				assert.Error(t, err)
				return
			}

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
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
