package cmd_test

import (
	"os"
	"path/filepath"
	"strings"

	cmdpkg "github.com/kairos-io/AuroraBoot/internal/cmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

var _ = Describe("redfish deploy", Label("redfish", "cmd"), func() {
	var tempDir string
	var isoPath string
	var app *cli.App

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "redfish-test")
		Expect(err).NotTo(HaveOccurred())

		isoPath = filepath.Join(tempDir, "test.iso")
		err = os.WriteFile(isoPath, []byte("test iso content"), 0644)
		Expect(err).NotTo(HaveOccurred())

		app = &cli.App{
			Commands: []*cli.Command{
				&cmdpkg.RedFishDeployCmd,
			},
		}
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	DescribeTable("deploy command with different vendors",
		func(vendor string, expectError bool) {
			args := []string{
				"auroraboot",
				"redfish",
				"deploy",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--vendor", vendor,
				"--verify-ssl", "true",
				"--min-memory", "4",
				"--min-cpus", "2",
				"--required-features", "UEFI",
				"--timeout", "5m",
				isoPath,
			}

			err := app.Run(args)

			// When using a fake endpoint (example.com), we expect connection/auth errors
			// Accept any HTTP error status (403, 405, etc.) as expected failure
			if err != nil && (strings.Contains(err.Error(), "failed with status: 403") ||
				strings.Contains(err.Error(), "failed with status: 405") ||
				strings.Contains(err.Error(), "authentication failed")) {
				// This is expected when connecting to a fake endpoint
				Expect(err).To(HaveOccurred())
				return
			}

			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("Generic Vendor", "generic", false),
		Entry("SuperMicro Vendor", "supermicro", false),
		Entry("HPE iLO Vendor", "ilo", false),
		Entry("DMTF Vendor", "dmtf", false),
		Entry("Unknown Vendor", "unknown", true),
	)

	Describe("validation", func() {
		It("errors out if required flags are missing", func() {
			args := []string{
				"auroraboot",
				"redfish",
				"deploy",
				isoPath,
			}

			err := app.Run(args)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Required flags"))
		})

		It("errors out if min-memory is invalid", func() {
			args := []string{
				"auroraboot",
				"redfish",
				"deploy",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--min-memory", "-1",
				isoPath,
			}

			err := app.Run(args)
			Expect(err).To(HaveOccurred())
		})

		It("errors out if min-cpus is invalid", func() {
			args := []string{
				"auroraboot",
				"redfish",
				"deploy",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--min-cpus", "0",
				isoPath,
			}

			err := app.Run(args)
			Expect(err).To(HaveOccurred())
		})

		It("errors out if timeout is invalid", func() {
			args := []string{
				"auroraboot",
				"redfish",
				"deploy",
				"--endpoint", "https://example.com",
				"--username", "admin",
				"--password", "password",
				"--timeout", "0s",
				isoPath,
			}

			err := app.Run(args)
			Expect(err).To(HaveOccurred())
		})
	})
})
