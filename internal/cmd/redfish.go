package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/urfave/cli/v2"
)

// redfishPasswordEnv is the environment variable consulted for the RedFish
// password when neither --password nor --password-file is set.
const redfishPasswordEnv = "AURORABOOT_REDFISH_PASSWORD"

// resolveRedfishPassword resolves the RedFish password from, in precedence order:
// the explicit --password flag, --password-file, the AURORABOOT_REDFISH_PASSWORD
// environment variable, then --password-stdin. It errors if none is provided.
// stdin is injected for testability.
func resolveRedfishPassword(flagPassword, passwordFile string, passwordStdin bool, stdin io.Reader) (string, error) {
	switch {
	case flagPassword != "":
		return flagPassword, nil
	case passwordFile != "":
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("reading --password-file: %w", err)
		}
		pw := strings.TrimRight(string(data), "\r\n")
		if pw == "" {
			return "", fmt.Errorf("--password-file %q is empty", passwordFile)
		}
		return pw, nil
	case os.Getenv(redfishPasswordEnv) != "":
		return os.Getenv(redfishPasswordEnv), nil
	case passwordStdin:
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("reading password from stdin: %w", err)
		}
		pw := strings.TrimRight(string(data), "\r\n")
		if pw == "" {
			return "", fmt.Errorf("no password read from stdin")
		}
		return pw, nil
	default:
		return "", fmt.Errorf("no RedFish password provided: set --password (insecure), --password-file, %s, or --password-stdin", redfishPasswordEnv)
	}
}

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
					Name:  "password",
					Usage: "RedFish password (INSECURE: visible in the process list/argv; prefer --password-file, AURORABOOT_REDFISH_PASSWORD, or --password-stdin)",
				},
				&cli.StringFlag{
					Name:  "password-file",
					Usage: "Read the RedFish password from this file (trailing newline trimmed)",
				},
				&cli.BoolFlag{
					Name:  "password-stdin",
					Usage: "Read the RedFish password from standard input (trailing newline trimmed)",
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
				&cli.StringFlag{
					Name:  "serve-tls-cert",
					Usage: "TLS certificate file for the local ISO-serve (used with --serve-tls)",
				},
				&cli.StringFlag{
					Name:  "serve-tls-key",
					Usage: "TLS key file for the local ISO-serve (used with --serve-tls)",
				},
				&cli.StringFlag{
					Name:  "redfish-serve-url",
					Usage: "Advertised base URL the BMC fetches a local ISO from (e.g. http://10.0.0.5:8090). Required when deploying a local ISO path without --image-url",
				},
				&cli.StringFlag{
					Name:  "redfish-serve-addr",
					Usage: "Bind address for the local ISO-serve (e.g. 10.0.0.5:8090). Defaults to the host:port of --redfish-serve-url",
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
					Usage: "Hardware features the system must support; the deploy aborts if any is missing or cannot be verified. Detectable features: UEFI, SecureBoot (default: UEFI)",
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
				password, err := resolveRedfishPassword(
					c.String("password"),
					c.String("password-file"),
					c.Bool("password-stdin"),
					os.Stdin,
				)
				if err != nil {
					return err
				}
				imageURL := c.String("image-url")
				vendor := c.String("vendor")
				verifySSL := c.Bool("verify-ssl")
				serveTLS := c.Bool("serve-tls")
				serveTLSCert := c.String("serve-tls-cert")
				serveTLSKey := c.String("serve-tls-key")
				serveURL := c.String("redfish-serve-url")
				serveAddr := c.String("redfish-serve-addr")
				minMemory := c.Int("min-memory")
				minCPUs := c.Int("min-cpus")
				requiredFeatures := c.StringSlice("required-features")
				timeout := c.Duration("timeout")
				isoPath := c.Args().First()

				// SSRF-guard the operator-supplied BMC endpoint regardless of mode.
				if err := isoserve.ValidateMediaURL(endpoint); err != nil {
					return fmt.Errorf("validating endpoint: %w", err)
				}

				// Resolve the image URL. InsertMedia is URL-pull, so the BMC needs a
				// URL it can fetch. Two modes:
				//   1. --image-url given: SSRF-validate and use it directly.
				//   2. a local ISO path given: start a one-shot tokenized ISO-serve,
				//      Register the file, deploy with the returned URL, then
				//      Revoke/Shutdown after the deploy returns.
				var (
					serve      *isoserve.Server
					serveToken string
				)
				switch {
				case imageURL != "":
					if err := isoserve.ValidateMediaURL(imageURL); err != nil {
						return fmt.Errorf("validating --image-url: %w", err)
					}
				case isoPath != "":
					absISO, err := filepath.Abs(isoPath)
					if err != nil {
						return fmt.Errorf("resolving ISO path: %w", err)
					}
					if serveURL == "" {
						return fmt.Errorf("serving a local ISO requires --redfish-serve-url (the base URL the BMC fetches the ISO from)")
					}
					if serveTLS && (serveTLSCert == "" || serveTLSKey == "") {
						return fmt.Errorf("--serve-tls with a local ISO requires --serve-tls-cert and --serve-tls-key (the BMC fetches over HTTPS)")
					}
					if serveAddr == "" {
						// Derive the bind address from the serve URL host:port rather
						// than silently binding 0.0.0.0.
						parsed, err := url.Parse(serveURL)
						if err != nil {
							return fmt.Errorf("parsing --redfish-serve-url: %w", err)
						}
						serveAddr = parsed.Host
						if serveAddr == "" {
							return fmt.Errorf("--redfish-serve-url has no host; set --redfish-serve-addr explicitly")
						}
					}

					serve = isoserve.New(isoserve.Config{
						BaseURL:  serveURL,
						BindAddr: serveAddr,
						CertFile: serveTLSCert,
						KeyFile:  serveTLSKey,
					})
					if err := serve.Start(ctx); err != nil {
						return fmt.Errorf("starting ISO-serve: %w", err)
					}
					defer func() {
						shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						_ = serve.Shutdown(shutdownCtx)
					}()

					url, token, err := serve.Register(absISO, timeout+5*time.Minute)
					if err != nil {
						return fmt.Errorf("registering ISO for serving: %w", err)
					}
					imageURL = url
					serveToken = token
					defer serve.Revoke(serveToken)
				default:
					return fmt.Errorf("provide --image-url with a URL the BMC can reach, or a local ISO path with --redfish-serve-url")
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
				defer func() { _ = deployer.Close() }()

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
