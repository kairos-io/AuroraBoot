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

// SysextCmd generates a sysextension from the last layer of the given CONTAINER
var SysextCmd = cli.Command{
	Name:      "sysext",
	Usage:     "Generate a sysextension from the last layer of the given CONTAINER",
	ArgsUsage: "<name> <container>",

	Flags: append(
		commonFlagsSysextConfext(),
		&cli.BoolFlag{
			Name:    "service-reload",
			Aliases: []string{"service-load"},
			Value:   false,
			Usage:   "Make systemctl reload the service when loading the sysext. This is useful for sysext that provide systemd service files.",
		},
	),
	Before: validateSysextConfextArgs,
	Action: generateSysextConfext,
}

// ConfextCmd generates a confextension from the last layer of the given CONTAINER
var ConfextCmd = cli.Command{
	Name:      "confext",
	Usage:     "Generate a confextension from the last layer of the given CONTAINER",
	ArgsUsage: "<name> <container>",

	Flags:  commonFlagsSysextConfext(),
	Before: validateSysextConfextArgs,
	Action: generateSysextConfext,
}

// commonFlagsSysextConfext returns the common flags for both sysext and confext commands
func commonFlagsSysextConfext() []cli.Flag {
	return []cli.Flag{
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
	}
}

// validateSysextConfextArgs validates the arguments for both sysext and confext commands
func validateSysextConfextArgs(ctx *cli.Context) error {
	arch := ctx.String("arch")
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture: %s", arch)
	}
	if ctx.NArg() < 2 {
		return fmt.Errorf("missing required arguments: <name> <container>")
	}
	return nil
}

// generateSysextConfext generates a sysext or confext based on the command name and the last layer of the given container image
func generateSysextConfext(ctx *cli.Context) error {
	buildType := ctx.Command.Name
	level := "info"
	if ctx.Bool("debug") {
		level = "debug"
	}
	logger := logger.NewKairosLogger("auroraboot", level, false)
	args := ctx.Args()

	name := args.Get(0)
	if _, err := os.Stat(fmt.Sprintf("%s.%s.raw", name, buildType)); err == nil {
		_ = os.Remove(fmt.Sprintf("%s.%s.raw", name, buildType))
	}
	logger.Infof("🚀 Start %s creation", buildType)

	dir, err := os.MkdirTemp("", fmt.Sprintf("auroraboot-%s-", buildType))
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

	var allowList *regexp.Regexp
	if buildType == "sysext" {
		allowList = regexp.MustCompile(`^usr/*|^/usr/*`)
	} else {
		allowList = regexp.MustCompile(`^etc/*|^/etc/*`)
	}
	// extract the files into the temp dir
	logger.Info("📤 Extracting archives from image layer")
	err = sysext.ExtractFilesFromLastLayer(image, dir, logger, allowList)
	if err != nil {
		logger.Logger.Error().Str("image", args.Get(1)).Err(err).Msg("⛔ extracting layer")
		return err
	}

	// Now create the file that tells systemd that this is a sysext/confext!
	var extensionReleaseDir string
	if buildType == "sysext" {
		extensionReleaseDir = filepath.Join(dir, "/usr/lib/extension-release.d/")
	} else {
		extensionReleaseDir = filepath.Join(dir, "/etc/extension-release.d/")
	}
	err = os.MkdirAll(extensionReleaseDir, os.ModeDir|os.ModePerm)
	if err != nil {
		logger.Logger.Error().Str("dir", extensionReleaseDir).Err(err).Msg("⛔ creating dir")
		return err
	}

	arch := "x86-64"
	if ctx.String("arch") == "arm64" {
		arch = "arm64"
	}

	extensionData := fmt.Sprintf("ID=_any\nARCHITECTURE=%s", arch)

	// If the extension ships any service files, we want this so systemd is reloaded and the service available immediately
	if ctx.Bool("service-reload") && buildType == "sysext" {
		extensionData = fmt.Sprintf("%s\nEXTENSION_RELOAD_MANAGER=1", extensionData)
	}
	logger.Logger.Debug().Str("file", fmt.Sprintf("extension-release.%s", name)).Str("content", extensionData).Msg("creating release file")
	err = os.WriteFile(filepath.Join(extensionReleaseDir, fmt.Sprintf("extension-release.%s", name)), []byte(extensionData), os.ModePerm)
	if err != nil {
		logger.Logger.Error().Str("file", fmt.Sprintf("extension-release.%s", name)).Err(err).Msg("⛔ creating releasefile")
		return err
	}

	logger.Logger.Info().Msgf("📦 Packing %s into raw image", buildType)
	// Call systemd-repart to create the sysext based off the files
	outputFile := fmt.Sprintf("%s.%s.raw", name, buildType)
	if outputDir := ctx.String("output"); outputDir != "" {
		outputFile = filepath.Join(outputDir, outputFile)
	}
	// Call systemd-repart to create the sysext/confext based off the files
	cmdArgs := []string{
		fmt.Sprintf("--make-ddi=%s", buildType),
		"--image-policy=root=verity+signed+absent:usr=verity+signed+absent",
		fmt.Sprintf("--architecture=%s", arch),
		// Having a fixed predictable seed makes the Image UUID be always the same if the inputs are the same,
		// so its a reproducible image. So getting the same files and same cert/key should produce a reproducible image always
		// Another layer to verify images, even if its a manual check, we make it easier
		fmt.Sprintf("--seed=%s", uuid.NewV5(uuid.NamespaceDNS, fmt.Sprintf("kairos-%s", buildType))),
		fmt.Sprintf("--copy-source=%s", dir),
		outputFile, // output file
	}
	// Add signing flags or exclude partitions based on whether key/cert are provided
	cmdArgs = append(cmdArgs, aurorabootUtils.GetSysextSigningFlags(ctx.String("private-key"), ctx.String("certificate"))...)
	command := exec.Command("systemd-repart", cmdArgs...)
	out, err := command.CombinedOutput()
	logger.Logger.Debug().Str("output", string(out)).Msgf("building %s", buildType)
	if err != nil {
		logger.Logger.Error().Err(err).
			Str("command", strings.Join(command.Args, " ")).
			Str("output", string(out)).
			Msgf("⛔ building %s failed", buildType)
		return err
	}

	logger.Logger.Info().Str("output", outputFile).Msgf("🎉 Done %s creation", buildType)
	return nil
}
