package cmd

import (
	"fmt"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/urfave/cli/v2"
)

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
		// Create RedFish client
		client, err := redfish.NewClient(
			c.String("endpoint"),
			c.String("username"),
			c.String("password"),
			c.Bool("verify-ssl"),
			c.Duration("timeout"),
		)
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

		// Validate requirements
		reqs := &hardware.Requirements{
			MinMemoryGiB:     c.Int("min-memory"),
			MinCPUs:          c.Int("min-cpus"),
			RequiredFeatures: c.StringSlice("required-features"),
		}

		if err := inspector.ValidateRequirements(sysInfo, reqs); err != nil {
			return fmt.Errorf("validating requirements: %w", err)
		}

		// Get ISO path from arguments
		isoPath := c.Args().Get(0)
		if isoPath == "" {
			return fmt.Errorf("ISO path is required")
		}

		// Deploy ISO
		status, err := client.DeployISO(isoPath)
		if err != nil {
			return fmt.Errorf("deploying ISO: %w", err)
		}

		fmt.Printf("Deployment started: %s\n", status.Message)
		fmt.Printf("System: %s %s (SN: %s)\n",
			sysInfo.Manufacturer, sysInfo.Model, sysInfo.SerialNumber)

		// Monitor deployment
		for {
			status, err := client.GetDeploymentStatus()
			if err != nil {
				return fmt.Errorf("getting deployment status: %w", err)
			}

			fmt.Printf("Deployment status: %s (Progress: %d%%)\n",
				status.State, status.Progress)

			if status.State == "Completed" {
				fmt.Println("Deployment completed successfully")
				break
			}

			if status.State == "Failed" {
				return fmt.Errorf("deployment failed: %s", status.Message)
			}

			time.Sleep(10 * time.Second)
		}

		return nil
	},
}
