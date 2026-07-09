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

// Sysext/confext info: https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html
// System extension images may – dynamically at runtime — extend the /usr/ and /opt/ directory hierarchies with additional files.
// Configuration extension images may – dynamically at runtime — extend the /etc/ directory hierarchy with additional files.

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
		// Deprecated: prefer --include-path=/opt. Kept as an indefinite alias —
		// the cost of carrying it is negligible; breaking scripts isn't.
		&cli.BoolFlag{
			Name:  "with-opt",
			Value: false,
			Usage: "Deprecated: prefer --include-path=/opt. Include files from /opt in the sysext.",
		},
		&cli.StringSliceFlag{
			Name:  "include-path",
			Usage: "Filesystem path to extract from the image layer (repeatable). /usr is always included.",
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
			Usage: "Arch to get the image from and build the sysext for. Accepts amd64, arm64, and riscv64 values.",
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
	if arch != "amd64" && arch != "arm64" && arch != "riscv64" {
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

	// We only want to extract files from /usr for sysext and /etc for confext, so we create a regex allowlist based on the build type.
	// Operators can extend the allowlist via --include-path (repeatable) or the
	// legacy --with-opt alias. Paths flow into SYSTEMD_SYSEXT_HIERARCHIES at
	// boot time (AuroraBoot bakes the drop-in via extensionHierarchies on the
	// artifact create payload).
	allowList := regexp.MustCompile(`^usr/*|^/usr/*`)
	if buildType == "sysext" {
		includes := includePathsFromFlags(ctx)
		if len(includes) > 0 {
			parts := []string{`^usr/*`, `^/usr/*`}
			for _, p := range includes {
				p = strings.TrimPrefix(strings.TrimSpace(p), "/")
				if p == "" {
					continue
				}
				parts = append(parts, "^"+regexp.QuoteMeta(p)+"/*", "^/"+regexp.QuoteMeta(p)+"/*")
			}
			logger.Logger.Debug().Strs("includes", includes).Msg("extending sysext allowlist")
			allowList = regexp.MustCompile(strings.Join(parts, "|"))
		}
	}
	// The directory where the extension-release file will be created, based on the build type
	extensionReleaseDir := filepath.Join(dir, "/usr/lib/extension-release.d/")

	// If its a confext, we change the allowlist and the extension release dir to match /etc instead of /usr
	if buildType == "confext" {
		allowList = regexp.MustCompile(`^etc/*|^/etc/*`)
		extensionReleaseDir = filepath.Join(dir, "/etc/extension-release.d/")
	}
	// extract the files into the temp dir
	logger.Info("📤 Extracting archives from image layer")
	err = sysext.ExtractFilesFromLastLayer(image, dir, logger, allowList)
	if err != nil {
		logger.Logger.Error().Str("image", args.Get(1)).Err(err).Msg("⛔ extracting layer")
		return err
	}

	// Now create the file that tells systemd that this is a sysext/confext!
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
	// Call systemd-repart to create the sysext/confext based off the files
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

// includePathsFromFlags merges --include-path entries with the legacy
// --with-opt boolean (which is equivalent to --include-path=/opt) and
// dedupes the result. A one-time deprecation warning fires on stderr when
// --with-opt is set so existing scripts keep working without log spam.
func includePathsFromFlags(ctx *cli.Context) []string {
	out := append([]string(nil), ctx.StringSlice("include-path")...)
	if ctx.Bool("with-opt") {
		fmt.Fprintln(os.Stderr,
			"auroraboot sysext: --with-opt is deprecated; use --include-path=/opt instead.")
		out = append(out, "/opt")
	}
	seen := map[string]struct{}{}
	dedup := out[:0]
	for _, p := range out {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		dedup = append(dedup, p)
	}
	return dedup
}
