package cmd

import (
	"fmt"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/urfave/cli/v2"
)

var RedFishDeployCmd = cli.Command{
	Name:  "redfish",
	Usage: "Deploy ISO to server via RedFish (EXPERIMENTAL)",
	Subcommands: []*cli.Command{
		{
			Name:  "deploy",
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
					Name:  "image-url",
					Usage: "URL the BMC pulls the ISO from (InsertMedia is URL-pull; the BMC must be able to reach this URL)",
				},
				&cli.StringFlag{
					Name:  "vendor",
					Usage: "Hardware vendor (generic, supermicro, ilo, dmtf)",
					Value: "generic",
				},
				&cli.BoolFlag{
					Name:  "verify-ssl",
					Usage: "Verify SSL certificates",
					Value: true,
				},
				&cli.BoolFlag{
					Name:  "serve-tls",
					Usage: "Set the InsertMedia transfer protocol to HTTPS (requires a BMC-trusted serving cert)",
					Value: false,
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
				ctx := c.Context

				endpoint := c.String("endpoint")
				username := c.String("username")
				password := c.String("password")
				imageURL := c.String("image-url")
				vendor := c.String("vendor")
				verifySSL := c.Bool("verify-ssl")
				serveTLS := c.Bool("serve-tls")
				minMemory := c.Int("min-memory")
				minCPUs := c.Int("min-cpus")
				requiredFeatures := c.StringSlice("required-features")
				timeout := c.Duration("timeout")
				isoPath := c.Args().First()

				// D4: InsertMedia is URL-pull, so a deployment needs a URL the BMC
				// can fetch. Serving a local ISO via an ephemeral tokenized URL is
				// Phase 1b (#4111); until then, require --image-url.
				if imageURL == "" {
					if isoPath != "" {
						return fmt.Errorf("serving a local ISO is not yet implemented (Phase 1b, kairos-io/kairos#4111); pass --image-url with a URL the BMC can reach")
					}
					return fmt.Errorf("--image-url is required (the BMC pulls the ISO from this URL)")
				}

				deployer := redfish.NewDeployer(redfish.Config{
					Endpoint:  endpoint,
					Username:  username,
					Password:  password,
					Vendor:    redfish.VendorType(vendor),
					VerifySSL: verifySSL,
					Timeout:   timeout,
				})

				if err := deployer.Connect(ctx); err != nil {
					return fmt.Errorf("connecting to RedFish endpoint: %w", err)
				}
				// Always tear the session down (DELETE) on both success and error.
				defer deployer.Close()

				// Inspect the system and gate on the hardware requirements.
				inspector := hardware.NewInspector(deployer)
				sysInfo, err := inspector.InspectSystem(ctx)
				if err != nil {
					return fmt.Errorf("inspecting system: %w", err)
				}

				fmt.Printf("System: %s %s (SN: %s)\n",
					sysInfo.Manufacturer, sysInfo.Model, sysInfo.SerialNumber)

				reqs := &hardware.Requirements{
					MinMemoryGiB:     minMemory,
					MinCPUs:          minCPUs,
					RequiredFeatures: requiredFeatures,
				}
				if err := inspector.ValidateRequirements(sysInfo, reqs); err != nil {
					return fmt.Errorf("validating requirements: %w", err)
				}

				// Deploy: InsertMedia (URL-pull) -> one-time boot -> reset -> Task poll.
				result, err := deployer.Deploy(ctx, redfish.DeployRequest{
					ImageURL:              imageURL,
					BootTarget:            redfish.BootTargetCd,
					BootMode:              redfish.BootModeUEFI,
					TransferProtocolHTTPS: serveTLS,
				})
				if err != nil {
					return fmt.Errorf("deploying ISO: %w", err)
				}

				if result.TaskState != "" {
					fmt.Printf("Deployment task finished: %s\n", result.TaskState)
				}
				for _, m := range result.Messages {
					fmt.Printf("  BMC: %s\n", m)
				}
				fmt.Println("Deployment completed successfully")

				return nil
			},
		},
	},
}
