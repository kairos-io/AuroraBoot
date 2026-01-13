package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofrs/uuid"
	aurorabootUtils "github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos-sdk/sysext"
	"github.com/kairos-io/kairos-sdk/types/logger"
	sdkImage "github.com/kairos-io/kairos-sdk/utils/image"
	"github.com/urfave/cli/v2"
)

// Use:   "build-uki SourceImage",
// Short: "Build a UKI artifact from a container image",
var SysextCmd = cli.Command{
	Name:      "sysext",
	Usage:     "Generate a sysextension from the last layer of the given CONTAINER",
	ArgsUsage: "<name> <container>",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "private-key",
			Value:    "",
			Usage:    "Private key to sign the sysext with",
			Required: false,
		},
		&cli.StringFlag{
			Name:     "certificate",
			Usage:    "Certificate to sign the sysext with",
			Required: false,
		},
		&cli.BoolFlag{
			Name:  "service-load",
			Value: false,
			Usage: "Make systemctl reload the service when loading the sysext. This is useful for sysext that provide systemd service files.",
		},
		&cli.StringFlag{
			Name:  "output",
			Usage: "Output dir",
		},
		&cli.StringFlag{
			Name:  "arch",
			Value: "amd64",
			Usage: "Arch to get the image from and build the sysext for. Accepts amd64 and arm64 values.",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Value: false,
			Usage: "Enable debug logging",
		},
	},
	Before: func(ctx *cli.Context) error {
		arch := ctx.String("arch")
		if arch != "amd64" && arch != "arm64" {
			return fmt.Errorf("unsupported architecture: %s", arch)
		}
		if ctx.NArg() < 2 {
			return fmt.Errorf("missing required arguments: <name> <container>")
		}
		return nil
	},
	Action: func(ctx *cli.Context) error {
		level := "warn"
		if ctx.Bool("debug") {
			level = "debug"
		}
		logger := logger.NewKairosLogger("auroraboot", level, false)
		args := ctx.Args()

		name := args.Get(0)
		if _, err := os.Stat(fmt.Sprintf("%s.sysext.raw", name)); err == nil {
			_ = os.Remove(fmt.Sprintf("%s.sysext.raw", name))
		}
		logger.Info("🚀 Start sysext creation")

		dir, err := os.MkdirTemp("", "auroraboot-sysext-")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
		defer func(path string) {
			err := os.RemoveAll(path)
			if err != nil {
				logger.Logger.Error().Str("dir", dir).Err(err).Msg("⛔ removing dir")
			}
		}(dir)
		logger.Logger.Debug().Str("dir", dir).Msg("creating directory")

		// Get the image struct
		logger.Info("💿 Getting image info")
		platform := fmt.Sprintf("linux/%s", ctx.String("arch"))
		image, err := sdkImage.GetImage(args.Get(1), platform, nil, nil)
		if err != nil {
			logger.Logger.Error().Str("image", args.Get(1)).Err(err).Msg("⛔ getting image")
			return err
		}
		// Only for sysext, confext not supported yet
		AllowList := regexp.MustCompile(`^usr/*|^/usr/*`)
		// extract the files into the temp dir
		logger.Info("📤 Extracting archives from image layer")
		err = sysext.ExtractFilesFromLastLayer(image, dir, logger, AllowList)
		if err != nil {
			logger.Logger.Error().Str("image", args.Get(1)).Err(err).Msg("⛔ extracting layer")
		}

		// Now create the file that tells systemd that this is a sysext!
		err = os.MkdirAll(filepath.Join(dir, "/usr/lib/extension-release.d/"), os.ModeDir|os.ModePerm)
		if err != nil {
			logger.Logger.Error().Str("dir", filepath.Join(dir, "/usr/lib/extension-release.d/")).Err(err).Msg("⛔ creating dir")
			return err
		}

		arch := "x86-64"
		if ctx.String("arch") == "arm64" {
			arch = "arm64"
		}

		extensionData := fmt.Sprintf("ID=_any\nARCHITECTURE=%s", arch)

		// If the extension ships any service files, we want this so systemd is reloaded and the service available immediately
		if ctx.Bool("service-reload") {
			extensionData = fmt.Sprintf("%s\nEXTENSION_RELOAD_MANAGER=1", extensionData)
		}
		err = os.WriteFile(filepath.Join(dir, "/usr/lib/extension-release.d/", fmt.Sprintf("extension-release.%s", name)), []byte(extensionData), os.ModePerm)
		if err != nil {
			logger.Logger.Error().Str("file", fmt.Sprintf("extension-release.%s", name)).Err(err).Msg("⛔ creating releasefile")
			return err
		}

		logger.Logger.Info().Msg("📦 Packing sysext into raw image")
		// Call systemd-repart to create the sysext based off the files
		outputFile := fmt.Sprintf("%s.sysext.raw", name)
		if outputDir := ctx.String("output"); outputDir != "" {
			outputFile = filepath.Join(outputDir, outputFile)
		}
		// Call systemd-repart to create the sysext based off the files
		cmdArgs := []string{
			"--make-ddi=sysext",
			"--image-policy=root=verity+signed+absent:usr=verity+signed+absent",
			fmt.Sprintf("--architecture=%s", arch),
			// Having a fixed predictable seed makes the Image UUID be always the same if the inputs are the same,
			// so its a reproducible image. So getting the same files and same cert/key should produce a reproducible image always
			// Another layer to verify images, even if its a manual check, we make it easier
			fmt.Sprintf("--seed=%s", uuid.NewV5(uuid.NamespaceDNS, "kairos-sysext")),
			fmt.Sprintf("--copy-source=%s", dir),
			outputFile, // output sysext image
		}
		// Add signing flags or exclude partitions based on whether key/cert are provided
		cmdArgs = append(cmdArgs, aurorabootUtils.GetSysextSigningFlags(ctx.String("private-key"), ctx.String("certificate"))...)
		command := exec.Command("systemd-repart", cmdArgs...)
		out, err := command.CombinedOutput()
		logger.Logger.Debug().Str("output", string(out)).Msg("building sysext")
		if err != nil {
			logger.Logger.Error().Err(err).
				Str("command", strings.Join(command.Args, " ")).
				Str("output", string(out)).
				Msg("⛔ building sysext")
			return err
		}

		logger.Logger.Info().Str("output", outputFile).Msg("🎉 Done sysext creation")
		return nil
	},
}
