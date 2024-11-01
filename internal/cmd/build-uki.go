package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/enki/pkg/constants"
	enkiconstants "github.com/kairos-io/enki/pkg/constants"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
)

// Use:   "build-uki SourceImage",
// Short: "Build a UKI artifact from a container image",
var BuildUKICmd = cli.Command{
	Name:      "build-uki",
	Aliases:   []string{"bu"},
	Usage:     "Builds a UKI artifact from a container image",
	ArgsUsage: "<source>",
	Description: "Build a UKI artifact from a container image\n\n" +
		"SourceImage - should be provided as uri in following format <sourceType>:<sourceName>\n" +
		"    * <sourceType> - might be [\"dir\", \"file\", \"oci\", \"docker\"], as default is \"docker\"\n" +
		"    * <sourceName> - is path to file or directory, image name with tag version\n" +
		"The following files are expected inside the keys directory:\n" +
		"    - DB.crt\n" +
		"    - DB.der\n" +
		"    - DB.key\n" +
		"    - DB.auth\n" +
		"    - KEK.der\n" +
		"    - KEK.auth\n" +
		"    - PK.der\n" +
		"    - PK.auth\n" +
		"    - tpm2-pcr-private.pem\n",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "name",
			Aliases: []string{"n"},
			Usage:   "Basename of the generated artifact (ignored for uki output type)",
		},
		&cli.StringFlag{
			Name:    "output-dir",
			Aliases: []string{"d"},
			Value:   ".",
			Usage:   "Output dir for artifact",
		},
		&cli.StringFlag{
			Name:    "output-type",
			Aliases: []string{"t"},
			Value:   string(enkiconstants.DefaultOutput),
			Usage:   fmt.Sprintf("Artifact output type [%s]", strings.Join(enkiconstants.OutPutTypes(), ", ")),
		},
		&cli.StringFlag{
			Name:    "overlay-rootfs",
			Aliases: []string{"o"},
			Usage:   "Dir with files to be applied to the system rootfs. All the files under this dir will be copied into the rootfs of the uki respecting the directory structure under the dir.",
		},
		&cli.StringFlag{
			Name:    "overlay-iso",
			Aliases: []string{"i"},
			Usage:   "Dir with files to be copied to the ISO rootfs.",
		},
		&cli.StringFlag{
			Name:  "boot-branding",
			Value: "Kairos",
			Usage: "Boot title branding",
		},
		&cli.BoolFlag{
			Name:  "include-version-in-config",
			Value: false,
			Usage: "Include the OS version in the .config file",
		},
		&cli.BoolFlag{
			Name:  "include-cmdline-in-config",
			Value: false,
			Usage: "Include the cmdline in the .config file. Only the extra values are included.",
		},
		&cli.StringSliceFlag{
			Name:    "extra-cmdline",
			Aliases: []string{"c"},
			Usage:   "Add extra efi files with this cmdline for the default 'norole' artifacts. This creates efi files with the default cmdline and extra efi files with the default+provided cmdline.",
		},
		&cli.StringFlag{
			Name:    "extend-cmdline",
			Aliases: []string{"x"},
			Usage:   "Extend the default cmdline for the default 'norole' artifacts. This creates efi files with the default+provided cmdline.",
		},
		&cli.StringSliceFlag{
			Name:    "single-efi-cmdline",
			Aliases: []string{"s"},
			Usage:   "Add one extra efi file with the default+provided cmdline. The syntax is '--single-efi-cmdline \"My Entry: cmdline,options,here\"'. The boot entry name is the text under which it appears in systemd-boot menu.",
		},
		&cli.StringFlag{
			Name:     "keys",
			Aliases:  []string{"k"},
			Usage:    "Directory with the signing keys",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "default-entry",
			Aliases: []string{"e"},
			Usage:   "Default entry selected in the boot menu. Supported glob wildcard patterns are \"?\", \"*\", and \"[...]\". If not selected, the default entry with install-mode is selected.",
		},
		&cli.Int64Flag{
			Name:  "efi-size-warn",
			Value: 1024,
			Usage: "EFI file size warning threshold in megabytes",
		},
		&cli.StringFlag{
			Name:  "secure-boot-enroll",
			Value: "if-safe",
			Usage: "The value of secure-boot-enroll option of systemd-boot. Possible values: off|manual|if-safe|force. Minimum systemd version: 253. Docs: https://manpages.debian.org/experimental/systemd-boot/loader.conf.5.en.html. !! Danger: this feature might soft-brick your device if used improperly !!",
		},
		&cli.StringFlag{
			Name:  "splash",
			Usage: "Path to the custom logo splash BMP file.",
		},
		// // Mark some flags as mutually exclusive
		// c.MarkFlagsMutuallyExclusive([]string{"extra-cmdline", "extend-cmdline"}...)
	},
	Before: func(ctx *cli.Context) error {
		artifact := ctx.String("output-type")
		if artifact != string(constants.DefaultOutput) && artifact != string(constants.IsoOutput) && artifact != string(constants.ContainerOutput) {
			return fmt.Errorf("invalid output type: %s", artifact)
		}

		overlayRootfs := ctx.String("overlay-rootfs")
		if overlayRootfs != "" {
			// Check if overlay dir exists by doing an os.stat
			// If it does not exist, return an error
			ol, err := os.Stat(overlayRootfs)
			if err != nil {
				return fmt.Errorf("overlay-rootfs directory does not exist: %s", overlayRootfs)
			}
			if !ol.IsDir() {
				return fmt.Errorf("overlay-rootfs is not a directory: %s", overlayRootfs)
			}

			// Transform it into absolute path
			absolutePath, err := filepath.Abs(overlayRootfs)
			if err != nil {
				viper.Set("overlay-rootfs", absolutePath)
			}
		}
		overlayIso := ctx.String("overlay-iso")
		if overlayIso != "" {
			// Check if overlay dir exists by doing an os.stat
			// If it does not exist, return an error
			ol, err := os.Stat(overlayIso)
			if err != nil {
				return fmt.Errorf("overlay directory does not exist: %s", overlayIso)
			}
			if !ol.IsDir() {
				return fmt.Errorf("overlay is not a directory: %s", overlayIso)
			}

			// Check if we are setting a different artifact and overlay-iso is set
			if artifact != string(constants.IsoOutput) {
				return fmt.Errorf("overlay-iso is only supported for iso artifacts")
			}

			// Transform it into absolute path
			absolutePath, err := filepath.Abs(overlayIso)
			if err != nil {
				viper.Set("overlay-iso", absolutePath)
			}
		}

		// Check if the keys directory exists
		keysDir := ctx.String("keys")
		_, err := os.Stat(keysDir)
		if err != nil {
			return fmt.Errorf("keys directory does not exist: %s", keysDir)
		}
		// Check if the keys directory contains the required files
		requiredFiles := []string{"db.der", "db.key", "db.auth", "KEK.der", "KEK.auth", "PK.der", "PK.auth", "tpm2-pcr-private.pem"}
		for _, file := range requiredFiles {
			_, err = os.Stat(filepath.Join(keysDir, file))
			if err != nil {
				return fmt.Errorf("keys directory does not contain required file: %s", file)
			}
		}
		return CheckRoot()
	},
}
