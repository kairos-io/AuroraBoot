package cmd

import (
	"fmt"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/urfave/cli/v2"
)

type RedFishDeployConfig struct {
	Endpoint         string        `arg:"--endpoint" help:"RedFish endpoint URL"`
	Username         string        `arg:"--username" help:"RedFish username"`
	Password         string        `arg:"--password" help:"RedFish password"`
	Vendor           string        `arg:"--vendor" help:"Hardware vendor (generic, supermicro, ilo)" default:"generic"`
	VerifySSL        bool          `arg:"--verify-ssl" help:"Verify SSL certificates" default:"true"`
	MinMemory        int           `arg:"--min-memory" help:"Minimum required memory in GB" default:"4"`
	MinCPUs          int           `arg:"--min-cpus" help:"Minimum required CPUs" default:"2"`
	RequiredFeatures []string      `arg:"--required-features" help:"Required hardware features"`
	Timeout          time.Duration `arg:"--timeout" help:"Operation timeout" default:"5m"`
	ISO              string        `arg:"positional" help:"Path to ISO file"`
}

var RedFishDeployCmd = cli.Command{
	Name:  "deploy-redfish",
	Usage: "Deploy ISO to server via RedFish",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "endpoint",
			Usage:    "RedFish endpoint URL",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "username",
			Usage:    "RedFish username",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "password",
			Usage:    "RedFish password",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "vendor",
			Usage: "Hardware vendor (generic, supermicro)",
			Value: "generic",
		},
		&cli.BoolFlag{
			Name:  "verify-ssl",
			Usage: "Verify SSL certificates",
			Value: true,
		},
		&cli.IntFlag{
			Name:  "min-memory",
			Usage: "Minimum required memory in GiB",
			Value: 4,
		},
		&cli.IntFlag{
			Name:  "min-cpus",
			Usage: "Minimum required CPUs",
			Value: 2,
		},
		&cli.StringSliceFlag{
			Name:  "required-features",
			Usage: "Required hardware features",
			Value: cli.NewStringSlice("UEFI"),
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Operation timeout",
			Value: 30 * time.Minute,
		},
	},
	Action: func(c *cli.Context) error {
		endpoint := c.String("endpoint")
		username := c.String("username")
		password := c.String("password")
		vendor := c.String("vendor")
		verifySSL := c.Bool("verify-ssl")
		minMemory := c.Int("min-memory")
		minCPUs := c.Int("min-cpus")
		requiredFeatures := c.StringSlice("required-features")
		timeout := c.Duration("timeout")
		isoPath := c.Args().First()

		if isoPath == "" {
			return fmt.Errorf("ISO path is required")
		}

		// Create vendor-specific RedFish client
		vendorType := redfish.VendorType(vendor)
		client, err := redfish.NewVendorClient(vendorType, endpoint, username, password, verifySSL, timeout)
		if err != nil {
			return fmt.Errorf("creating RedFish client: %w", err)
		}

		// Create hardware inspector
		inspector := hardware.NewInspector(client)

		// Inspect system
		sysInfo, err := inspector.InspectSystem()
		if err != nil {
			return fmt.Errorf("inspecting system: %w", err)
		}

		fmt.Printf("System: %s %s (SN: %s)\n",
			sysInfo.Manufacturer, sysInfo.Model, sysInfo.SerialNumber)

		// Validate requirements
		reqs := &hardware.Requirements{
			MinMemoryGiB:     minMemory,
			MinCPUs:          minCPUs,
			RequiredFeatures: requiredFeatures,
		}
		if err := inspector.ValidateRequirements(sysInfo, reqs); err != nil {
			return fmt.Errorf("validating requirements: %w", err)
		}

		// Deploy ISO
		status, err := client.DeployISO(isoPath)
		if err != nil {
			return fmt.Errorf("deploying ISO: %w", err)
		}

		fmt.Printf("Deployment started: %s\n", status.Message)

		// Monitor deployment
		for {
			status, err := client.GetDeploymentStatus()
			if err != nil {
				return fmt.Errorf("getting deployment status: %w", err)
			}

			fmt.Printf("Deployment status: %s (%.0f%%)\n", status.State, status.Progress)

			if status.State == "Completed" {
				fmt.Println("Deployment completed successfully")
				break
			} else if status.State == "Failed" {
				return fmt.Errorf("deployment failed: %s", status.Message)
			}

			time.Sleep(5 * time.Second)
		}

		return nil
	},
}
