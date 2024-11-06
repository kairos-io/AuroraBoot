package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	enkiconfig "github.com/kairos-io/enki/pkg/config"
	enkiconstants "github.com/kairos-io/enki/pkg/constants"
	enkiutils "github.com/kairos-io/enki/pkg/utils"
	"github.com/kairos-io/go-ukify/pkg/uki"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"
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
			Value:   KairosDefaultArtifactName,
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
	},
	Before: func(ctx *cli.Context) error {
		// // Mark flags as mutually exclusive
		// TODO: Use MutuallyExclusiveFlags when urfave/cli v3 is stable:
		// https://github.com/urfave/cli/blob/7ec374fe2abd3e9c75369f6bb4191fe7866bd89c/command.go#L128
		if ctx.String("extra-cmdline") != "" && ctx.String("extend-cmdline") != "" {
			return errors.New("extra-cmdline and extend-cmdline flags are mutually exclusive")
		}

		artifact := ctx.String("output-type")
		if artifact != string(enkiconstants.DefaultOutput) && artifact != string(enkiconstants.IsoOutput) && artifact != string(enkiconstants.ContainerOutput) {
			return fmt.Errorf("invalid output type: %s", artifact)
		}

		// TODO: Not used anywhere in enki?
		//
		// overlayRootfs := ctx.String("overlay-rootfs")
		// if overlayRootfs != "" {
		// 	// Check if overlay dir exists by doing an os.stat
		// 	// If it does not exist, return an error
		// 	ol, err := os.Stat(overlayRootfs)
		// 	if err != nil {
		// 		return fmt.Errorf("overlay-rootfs directory does not exist: %s", overlayRootfs)
		// 	}
		// 	if !ol.IsDir() {
		// 		return fmt.Errorf("overlay-rootfs is not a directory: %s", overlayRootfs)
		// 	}

		// 	// Transform it into absolute path
		// 	absolutePath, err := filepath.Abs(overlayRootfs)
		// 	if err != nil {
		// 		viper.Set("overlay-rootfs", absolutePath)
		// 	}
		// }
		// overlayIso := ctx.String("overlay-iso")
		// if overlayIso != "" {
		// 	// Check if overlay dir exists by doing an os.stat
		// 	// If it does not exist, return an error
		// 	ol, err := os.Stat(overlayIso)
		// 	if err != nil {
		// 		return fmt.Errorf("overlay directory does not exist: %s", overlayIso)
		// 	}
		// 	if !ol.IsDir() {
		// 		return fmt.Errorf("overlay is not a directory: %s", overlayIso)
		// 	}

		// 	// Check if we are setting a different artifact and overlay-iso is set
		// 	if artifact != string(enkiconstants.IsoOutput) {
		// 		return fmt.Errorf("overlay-iso is only supported for iso artifacts")
		// 	}

		// 	// Transform it into absolute path
		// 	absolutePath, err := filepath.Abs(overlayIso)
		// 	if err != nil {
		// 		viper.Set("overlay-iso", absolutePath)
		// 	}
		// }

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
	Action: func(ctx *cli.Context) error {
		args := ctx.Args()
		if args.Len() < 1 {
			return errors.New("no image provided")
		}

		// TODO: Implement log-level flag
		logLevel := "debug"
		if ctx.String("log-level") != "" {
			logLevel = ctx.String("log-level")
		}
		logger := sdkTypes.NewKairosLogger("auroraboot", logLevel, false)

		// TODO: Get rid of "configs".
		config := enkiconfig.NewConfig(
			enkiconfig.WithImageExtractor(v1.OCIImageExtractor{}),
			enkiconfig.WithLogger(logger),
		)

		if err := checkBuildUKIDeps(config.Arch); err != nil {
			return err
		}

		// artifactsTempDir Is where we copy the kernel and initramfs files
		// So only artifacts that are needed to build the efi, so we dont pollute the sourceDir
		artifactsTempDir, err := os.MkdirTemp("", "enki-build-uki-artifacts-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(artifactsTempDir)

		logger.Info("Extracting image to a temporary directory")
		// Source dir is the directory where we extract the image
		// It should only contain the image files and whatever changes we add or remove like creating dir or removing leftover
		// lets not pollute it

		// TODO: if img is a dir, we should not copy or rsync anything and just use that dir as source?
		sourceDir, err := os.MkdirTemp("", "enki-build-uki-")
		if err != nil {
			return err
		}

		if err = os.Chmod(sourceDir, 0755); err != nil { // We need 755 permissions to allow all users to access the sockets.
			return err
		}

		imgSource, err := v1.NewSrcFromURI(args.Get(0))
		if err != nil {
			return fmt.Errorf("not a valid rootfs source image argument: %s", args.Get(0))
		}

		e := elemental.NewElemental(config)
		_, err = e.DumpSource(sourceDir, imgSource)
		defer os.RemoveAll(sourceDir)

		if ctx.String("overlay-rootfs") != "" {
			logger.Infof("Adding files from %s to rootfs", ctx.String("overlay-rootfs"))
			overlay, err := v1.NewSrcFromURI(fmt.Sprintf("dir:%s", ctx.String("overlay-rootfs")))
			if err != nil {
				return fmt.Errorf("error creating overlay image: %s", err)
			}
			if _, err = e.DumpSource(sourceDir, overlay); err != nil {
				return fmt.Errorf("error copying overlay image: %s", err)
			}
		}

		// Store the version so we only need to check it once
		kairosVersion, err := findKairosVersion(sourceDir)
		if err != nil {
			return err
		}

		logger.Info("Creating additional directories in the rootfs")
		if err := setupDirectoriesAndFiles(sourceDir); err != nil {
			return err
		}

		logger.Info("Copying kernel")
		if err := copyKernel(sourceDir, artifactsTempDir); err != nil {
			return fmt.Errorf("Copying kernel: %w", err)
		}

		// Remove the boot directory as we already copied the kernel and we dont need the initrd files
		if err := os.RemoveAll(filepath.Join(sourceDir, "boot")); err != nil {
			return fmt.Errorf("Cleaning up the source directory: %w", err)
		}

		logger.Info("Creating an initramfs file")
		if err := createInitramfs(sourceDir, artifactsTempDir); err != nil {
			return err
		}

		entries := append(
			GetUkiCmdline(ctx.String("extend-cmdline"), ctx.String("boot-branding"), ctx.StringSlice("extra-cmdline")),
			GetUkiSingleCmdlines(ctx.String("boot-branding"), ctx.StringSlice("single-efi-cmdline"), logger)...)

		for _, entry := range entries {
			logger.Info(fmt.Sprintf("Running ukify for cmdline: %s: %s", entry.Title, entry.Cmdline))

			logger.Infof("Generating: " + entry.FileName + ".efi")

			// New ukifier !!
			// Create Builder instance
			stub, err := getEfiStub(config.Arch)
			if err != nil {
				return err
			}
			// Get systemd-boot info (we can sign it at the same time)
			var systemdBoot string
			var outputSystemdBootEfi string
			if enkiutils.IsAmd64(config.Arch) {
				systemdBoot = enkiconstants.UkiSystemdBootx86
				outputSystemdBootEfi = enkiconstants.EfiFallbackNamex86
			} else if enkiutils.IsArm64(config.Arch) {
				systemdBoot = enkiconstants.UkiSystemdBootArm
				outputSystemdBootEfi = enkiconstants.EfiFallbackNameArm
			} else {
				return fmt.Errorf("unsupported arch: %s", config.Arch)
			}

			if logger.GetLevel().String() == "debug" {
				slog.SetLogLoggerLevel(slog.LevelDebug)
			}
			builder := &uki.Builder{
				Arch:          config.Arch,
				Version:       kairosVersion,
				SdStubPath:    stub,
				KernelPath:    filepath.Join(artifactsTempDir, "vmlinuz"),
				InitrdPath:    filepath.Join(artifactsTempDir, "initrd"),
				Cmdline:       entry.Cmdline,
				OsRelease:     filepath.Join(sourceDir, "etc/os-release"),
				OutUKIPath:    entry.FileName + ".efi",
				PCRKey:        filepath.Join(ctx.String("keys"), "tpm2-pcr-private.pem"),
				SBKey:         filepath.Join(ctx.String("keys"), "db.key"),
				SBCert:        filepath.Join(ctx.String("keys"), "db.pem"),
				SdBootPath:    systemdBoot,
				OutSdBootPath: outputSystemdBootEfi,
				Splash:        ctx.String("splash"),
			}

			if err := os.Chdir(sourceDir); err != nil {
				return fmt.Errorf("changing to %s directory: %w", sourceDir, err)
			}

			if err := builder.Build(); err != nil {
				return err
			}

			logger.Info("Creating kairos and loader conf files")
			if err := createConfFiles(sourceDir, entry.Cmdline, entry.Title, entry.FileName, kairosVersion, ctx.Bool("include-version-in-config"), ctx.Bool("include-cmdline-in-config")); err != nil {
				return err
			}
		}

		if err = createSystemdConf(sourceDir, ctx.String("default-entry"), ctx.String("secure-boot-enroll")); err != nil {
			return err
		}

		switch ctx.String("output-type") {
		case string(enkiconstants.IsoOutput):
			err = createISO(e, sourceDir, ctx.String("output-dir"), ctx.String("overlay-iso"), ctx.String("keys"), kairosVersion, ctx.String("name"), entries, logger)
			logger.Infof("Done building %s at: %s", ctx.String("output-type"), ctx.String("output-dir"))
		case string(enkiconstants.ContainerOutput):
			// First create the files
			if err = createArtifact(sourceDir, ctx.String("output-dir"), ctx.String("keys"), entries, logger); err != nil {
				return err
			}
			// Then build the image
			if err = createContainer(sourceDir, ctx.String("output-dir"), ctx.String("name"), kairosVersion, logger); err != nil {
				return err
			}
			logger.Infof("Done building %s", ctx.String("output-type"))

			//Then remove the output dir files as we dont need them, the container has been loaded
			if err = removeUkiFiles(ctx.String("output-dir"), ctx.String("keys"), entries, logger); err != nil {
				return err
			}
		case string(enkiconstants.DefaultOutput):
			if err = createArtifact(sourceDir, ctx.String("output-dir"), ctx.String("keys"), entries, logger); err != nil {
				return err
			}
			logger.Infof("Done building %s at: %s", ctx.String("output-type"), ctx.String("output-dir"))
		}

		return nil
	},
}

func checkBuildUKIDeps(arch string) error {
	neededBinaries := []string{
		"dd",
		"mkfs.msdos",
		"mmd",
		"mcopy",
		"xorriso",
	}

	for _, b := range neededBinaries {
		_, err := exec.LookPath(b)
		if err != nil {
			return err
		}
	}

	neededFiles, err := getEfiNeededFiles(arch)
	if err != nil {
		return err
	}

	for _, b := range neededFiles {
		_, err := os.Stat(b)
		if err != nil {
			return err
		}
	}

	return nil
}

func getEfiNeededFiles(arch string) ([]string, error) {
	if enkiutils.IsAmd64(arch) {
		return []string{
			enkiconstants.UkiSystemdBootStubx86,
			enkiconstants.UkiSystemdBootx86,
		}, nil
	} else if enkiutils.IsArm64(arch) {
		return []string{
			enkiconstants.UkiSystemdBootStubArm,
			enkiconstants.UkiSystemdBootArm,
		}, nil
	} else {
		return nil, fmt.Errorf("unsupported arch: %s", arch)
	}
}

func findKairosVersion(sourceDir string) (string, error) {
	var osReleaseBytes []byte
	osReleaseBytes, err := os.ReadFile(filepath.Join(sourceDir, "etc", "kairos-release"))
	if err != nil {
		// fallback to os-release
		osReleaseBytes, err = os.ReadFile(filepath.Join(sourceDir, "etc", "os-release"))
		if err != nil {
			return "", fmt.Errorf("reading kairos-release file: %w", err)
		}
	}

	re := regexp.MustCompile("(?m)^KAIROS_RELEASE=\"(.*)\"")
	match := re.FindStringSubmatch(string(osReleaseBytes))

	if len(match) != 2 {
		return "", fmt.Errorf("unexpected number of matches for KAIROS_RELEASE in os-release: %d", len(match))
	}

	return match[1], nil
}

func setupDirectoriesAndFiles(workDir string) error {
	if err := os.Symlink("/usr/bin/immucore", filepath.Join(workDir, "init")); err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}

	// able to mount oem under here if found
	if err := os.MkdirAll(filepath.Join(workDir, "oem"), os.ModeDir); err != nil {
		return fmt.Errorf("error creating /oem dir: %w", err)
	}

	// mount the esp under here if found
	if err := os.MkdirAll(filepath.Join(workDir, "efi"), os.ModeDir); err != nil {
		return fmt.Errorf("error creating /oem dir: %w", err)
	}

	// for install/upgrade they copy stuff there
	if err := os.MkdirAll(filepath.Join(workDir, "usr/local/cloud-config"), os.ModeDir); err != nil {
		return fmt.Errorf("error creating /oem dir: %w", err)
	}

	return nil
}

func copyKernel(sourceDir, targetDir string) error {
	linkTarget, err := os.Readlink(filepath.Join(sourceDir, "boot", "vmlinuz"))
	if err != nil {
		return err
	}

	kernelFile := filepath.Base(linkTarget)
	sourceFile, err := os.Open(filepath.Join(sourceDir, "boot", kernelFile))
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(filepath.Join(targetDir, "vmlinuz"))
	if err != nil {
		return err
	}
	defer destinationFile.Close()
	_, err = io.Copy(destinationFile, sourceFile)

	return err
}

// createInitramfs creates a compressed initramfs file (cpio format, gzipped).
// The resulting file is named "initrd" and is saved inthe sourceDir.
func createInitramfs(sourceDir, artifactsTempDir string) error {
	format := "newc"
	archiver, err := cpio.Format(format)
	if err != nil {
		return fmt.Errorf("format %q not supported: %w", format, err)
	}

	cpioFileName := filepath.Join(artifactsTempDir, "initramfs.cpio")
	cpioFile, err := os.Create(cpioFileName)
	if err != nil {
		return fmt.Errorf("creating cpio file: %w", err)
	}
	defer cpioFile.Close()

	rw := archiver.Writer(cpioFile)
	cr := cpio.NewRecorder()

	// List of directories to exclude
	excludeDirs := map[string]bool{
		"sys":  true,
		"run":  true,
		"dev":  true,
		"tmp":  true,
		"proc": true,
	}

	if err = os.Chdir(sourceDir); err != nil {
		return fmt.Errorf("changing to %s directory: %w", sourceDir, err)
	}

	// Walk through the source directory and add files to the cpio archive
	err = filepath.Walk(".", func(filePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if the current directory should be excluded
		if fileInfo.IsDir() && excludeDirs[filePath] {
			return filepath.SkipDir
		}

		if strings.Contains(filePath, "initramfs.cpio") {
			return nil
		}

		rec, err := cr.GetRecord(filePath)
		if err != nil {
			return fmt.Errorf("getting record of %q failed: %w", filePath, err)
		}

		// In case the record contains the sourceDir we want to remove it as its not part of the cpio initramfs
		// All files should have the proper path for the initramfs so SOURCEDIR/usr/bin needs to be stored as /usr/bin
		// in the cpio image
		rec.Name = strings.TrimPrefix(rec.Name, sourceDir)

		// MakeReproducible is not working as expected so we canno use it yet
		// as that breaks hardlinks
		// See upstream https://github.com/u-root/u-root/issues/3031
		// When its fixed we should use rw.WriteRecord(cpio.MakeReproducible(rec)) to generate reproducible initrds
		// Meanwhile we can try to make it as reproducible as possible
		rec.MTime = 0
		rec.UID = 0
		rec.GID = 0
		rec.Dev = 0
		rec.Major = 0
		rec.Minor = 0
		if err := rw.WriteRecord(rec); err != nil {
			return fmt.Errorf("writing record %q failed: %w", filePath, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking the source dir: %w", err)
	}

	if err := cpio.WriteTrailer(rw); err != nil {
		return fmt.Errorf("error writing trailer record: %w", err)
	}

	if err := ZstdFile(cpioFileName, filepath.Join(artifactsTempDir, "initrd")); err != nil {
		return err
	}

	if err := os.RemoveAll(cpioFileName); err != nil {
		return fmt.Errorf("error deleting cpio file: %w", err)
	}

	return nil
}

func ZstdFile(sourcePath, targetPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("error opening initramfs file: %w", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("error creating compressed initramfs file: %w", err)
	}
	defer outputFile.Close()

	// SpeedBetterCompression is heavier, takes 36 seconds in my 24core cpu but generates a 919MB file
	// SpeedBestCompression is really fast, takes 6 seconds but generates a 950Mb file
	// If we need we can use the heavier one if we need to gain those 30 extra Mb
	zstdWriter, _ := zstd.NewWriter(outputFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	defer zstdWriter.Close()

	if _, err = io.Copy(zstdWriter, inputFile); err != nil {
		return fmt.Errorf("error writing data to the compress initramfs file: %w", err)
	}

	return nil
}

func getEfiStub(arch string) (string, error) {
	if enkiutils.IsAmd64(arch) {
		return enkiconstants.UkiSystemdBootStubx86, nil
	} else if enkiutils.IsArm64(arch) {
		return enkiconstants.UkiSystemdBootStubArm, nil
	} else {
		return "", nil
	}
}

func createConfFiles(sourceDir, cmdline, title, finalEfiName, version string, includeVersion, includeCmdline bool) error {
	// This is stored in the config
	var extraCmdline string
	// For the config title we get only the extra cmdline we added, no replacement of spaces with underscores needed
	extraCmdline = strings.TrimSpace(strings.TrimPrefix(cmdline, enkiconstants.UkiCmdline))
	// For the default install entry, do not add anything on the config
	if extraCmdline == enkiconstants.UkiCmdlineInstall {
		extraCmdline = ""
	}

	// You can add entries into the config files, they will be ignored by systemd-boot
	// So we store the cmdline in a key cmdline for easy tracking of what was added to the uki cmdline

	configData := fmt.Sprintf("title %s\nefi /EFI/kairos/%s.efi\n", title, finalEfiName)

	if includeVersion {
		configData = fmt.Sprintf("%sversion %s\n", configData, version)
	}

	if includeCmdline {
		configData = fmt.Sprintf("%scmdline %s\n", configData, strings.Trim(extraCmdline, " "))
	}

	err := os.WriteFile(filepath.Join(sourceDir, finalEfiName+".conf"), []byte(configData), os.ModePerm)
	if err != nil {
		return fmt.Errorf("creating the %s.conf file", finalEfiName)
	}

	return nil
}

// createSystemdConf creates the generic conf that systemd-boot uses
func createSystemdConf(dir, defaultEntry, secureBootEnroll string) error {
	var finalEfiConf string
	if defaultEntry != "" {
		if !strings.HasSuffix(defaultEntry, ".conf") {
			finalEfiConf = strings.TrimSuffix(defaultEntry, " ") + ".conf"
		} else {
			finalEfiConf = defaultEntry
		}

	} else {
		// Get the generic efi file that we produce from the default cmdline
		// This is the one name that has nothing added, just the version
		finalEfiConf = enkiutils.NameFromCmdline(enkiconstants.ArtifactBaseName, enkiconstants.UkiCmdline+" "+enkiconstants.UkiCmdlineInstall) + ".conf"
	}

	// Set that as default selection for booting
	data := fmt.Sprintf("default %s\ntimeout 5\nconsole-mode max\neditor no\nsecure-boot-enroll %s\n", finalEfiConf, secureBootEnroll)
	err := os.WriteFile(filepath.Join(dir, "loader.conf"), []byte(data), os.ModePerm)
	if err != nil {
		return fmt.Errorf("creating the loader.conf file: %s", err)
	}
	return nil
}

func createISO(e *elemental.Elemental, sourceDir, outputDir, overlayISO, keysDir, kairosVersion, artifactName string, entries []enkiutils.BootEntry, logger sdkTypes.KairosLogger) error {
	// isoDir is where we generate the img file. We pass this dir to xorriso.
	isoDir, err := os.MkdirTemp("", "enki-iso-dir-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(isoDir)

	filesMap, err := imageFiles(sourceDir, keysDir, entries, logger)
	if err != nil {
		return err
	}

	logger.Info("Calculating the size of the img file")
	artifactSize, err := sumFileSizes(filesMap)
	if err != nil {
		return err
	}

	// Create just the size we need + 50MB just in case
	imgSize := artifactSize + 50
	imgFile := filepath.Join(isoDir, "efiboot.img")
	logger.Info(fmt.Sprintf("Creating the img file with size: %dMb", imgSize))
	if err = createImgWithSize(imgFile, imgSize); err != nil {
		return err
	}
	defer os.Remove(imgFile)

	logger.Info(fmt.Sprintf("Created image: %s", imgFile))

	logger.Info("Creating directories in the img file")
	if err := createImgDirs(imgFile, filesMap); err != nil {
		return err
	}

	logger.Info("Copying files in the img file")
	if err := copyFilesToImg(imgFile, filesMap); err != nil {
		return err
	}

	if overlayISO != "" {
		logger.Infof("Adding files from %s to iso", overlayISO)
		overlay, err := v1.NewSrcFromURI(fmt.Sprintf("dir:%s", overlayISO))
		if err != nil {
			logger.Errorf("error creating overlay image: %s", err)
			return err
		}
		_, err = e.DumpSource(isoDir, overlay)

		if err != nil {
			logger.Errorf("error copying overlay image: %s", err)
			return err
		}
	}

	isoName := fmt.Sprintf("kairos_%s.iso", kairosVersion)
	if artifactName != "" {
		isoName = fmt.Sprintf("%s.iso", artifactName)
	}

	logger.Info("Creating the iso files with xorriso")
	cmd := exec.Command("xorriso", "-as", "mkisofs", "-V", "UKI_ISO_INSTALL", "-isohybrid-gpt-basdat",
		"-e", filepath.Base(imgFile), "-no-emul-boot", "-o", filepath.Join(outputDir, isoName), isoDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error creating iso file: %w\n%s", err, string(out))
	}

	return nil
}

func imageFiles(sourceDir, keysDir string, entries []enkiutils.BootEntry, logger sdkTypes.KairosLogger) (map[string][]string, error) {
	// the keys are the target dirs
	// the values are the source files that should be copied into the target dir
	data := map[string][]string{
		"EFI":            {},
		"EFI/BOOT":       {filepath.Join(sourceDir, "BOOTX64.EFI")},
		"EFI/kairos":     {},
		"EFI/tools":      {},
		"loader":         {filepath.Join(sourceDir, "loader.conf")},
		"loader/entries": {},
		"loader/keys":    {},
		"loader/keys/auto": {
			filepath.Join(keysDir, "PK.der"),
			filepath.Join(keysDir, "KEK.der"),
			filepath.Join(keysDir, "db.der"),
			filepath.Join(keysDir, "PK.auth"),
			filepath.Join(keysDir, "KEK.auth"),
			filepath.Join(keysDir, "db.auth")},
	}

	// Add the kairos efi files and the loader conf files for each cmdline
	for _, entry := range entries {
		data["EFI/kairos"] = append(data["EFI/kairos"], filepath.Join(sourceDir, entry.FileName+".efi"))
		data["loader/entries"] = append(data["loader/entries"], filepath.Join(sourceDir, entry.FileName+".conf"))
	}
	return data, nil
}

func sumFileSizes(filesMap map[string][]string) (int64, error) {
	total := int64(0)
	for _, files := range maps.Values(filesMap) {
		for _, f := range files {
			fileInfo, err := os.Stat(f)
			if err != nil {
				return total, fmt.Errorf("finding file info for file %s: %w", f, err)
			}
			total += fileInfo.Size()
		}
	}

	totalInMB := int64(math.Round(float64(total) / (1024 * 1024)))

	return totalInMB, nil
}

func createImgWithSize(imgFile string, size int64) error {
	cmd := exec.Command("dd",
		"if=/dev/zero", fmt.Sprintf("of=%s", imgFile),
		"bs=1M", fmt.Sprintf("count=%d", size),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating the img file: %w\n%s", err, out)
	}

	return nil
}

func createImgDirs(imgFile string, filesMap map[string][]string) error {
	cmd := exec.Command("mkfs.msdos", "-F", "32", imgFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("formating the img file to fat: %w\n%s", err, string(out))
	}

	dirs := maps.Keys(filesMap)
	sort.Strings(dirs) // Make sure we create outer dirs first
	for _, dir := range dirs {
		// Dirs in MSDOS are marked with ::DIR
		cmd := exec.Command("mmd", "-i", imgFile, fmt.Sprintf("::%s", dir))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("creating directory %s on the img file: %w\n%s\nThe failed command was: %s", dir, err, string(out), cmd.String())
		}
	}

	return nil
}

func copyFilesToImg(imgFile string, filesMap map[string][]string) error {
	for dir, files := range filesMap {
		for _, f := range files {
			cmd := exec.Command("mcopy", "-i", imgFile, f, filepath.Join(fmt.Sprintf("::%s", dir), filepath.Base(f)))
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("copying %s in img file: %w\n%s", f, err, string(out))
			}
		}
	}

	return nil
}

// Create artifact just outputs the files from the sourceDir to the outputDir
// Maintains the same structure as the sourceDir which is the final structure we want
func createArtifact(sourceDir, outputDir, keysDir string, entries []enkiutils.BootEntry, logger sdkTypes.KairosLogger) error {
	filesMap, err := imageFiles(sourceDir, keysDir, entries, logger)
	if err != nil {
		return err
	}
	for dir, files := range filesMap {
		logger.Debugf(fmt.Sprintf("creating dir %s", filepath.Join(outputDir, dir)))
		err = os.MkdirAll(filepath.Join(outputDir, dir), os.ModeDir|os.ModePerm)
		if err != nil {
			logger.Errorf("creating dir %s: %s", dir, err)
			return err
		}
		for _, f := range files {
			logger.Debugf(fmt.Sprintf("copying %s to %s", f, filepath.Join(outputDir, dir, filepath.Base(f))))
			source, err := os.Open(f)
			if err != nil {
				logger.Errorf("opening file %s: %s", f, err)
				return err
			}
			defer func(source *os.File) {
				err := source.Close()
				if err != nil {
					logger.Errorf("closing file %s: %s", f, err)
				}
			}(source)

			destination, err := os.Create(filepath.Join(outputDir, dir, filepath.Base(f)))
			if err != nil {
				logger.Errorf("creating file %s: %s", filepath.Join(outputDir, dir, filepath.Base(f)), err)
				return err
			}
			defer func(destination *os.File) {
				err := destination.Close()
				if err != nil {
					logger.Errorf("closing file %s: %s", filepath.Join(outputDir, dir, filepath.Base(f)), err)
				}
			}(destination)
			_, err = io.Copy(destination, source)
			if err != nil {
				logger.Errorf("copying file %s: %s", f, err)
				return err
			}
		}
	}
	return nil
}

func createContainer(sourceDir, outputDir, artifactName, version string, logger sdkTypes.KairosLogger) error {
	temp, err := os.CreateTemp("", "image.tar")
	if err != nil {
		return err
	}
	// Create tarball from sourceDir
	err = enkiutils.Tar(sourceDir, temp)
	if err != nil {
		return err
	}
	_ = temp.Close()
	defer os.RemoveAll(temp.Name())
	finalImage := filepath.Join(outputDir, fmt.Sprintf("kairos_uki_%s.tar", version))
	// TODO: get the arch from the running system or by flag? Config.Arch has this value on it
	arch := "amd64"
	os := "linux"
	// Build imageTar from normal tar
	tarName := fmt.Sprintf("kairos_uki_%s.tar", version)
	if artifactName != "" {
		tarName = fmt.Sprintf("%s.tar", artifactName)
	}
	err = enkiutils.CreateTar(logger, temp.Name(), finalImage, tarName, arch, os)
	if err != nil {
		return err
	}

	return err
}

// removeUkiFiles removes all the files and directories inside the output directory that match our filesMap
// so this should only remove the generated intermediate artifacts that we use to build the container
func removeUkiFiles(outputDir, keysDir string, entries []enkiutils.BootEntry, logger sdkTypes.KairosLogger) error {
	filesMap, _ := imageFiles(outputDir, keysDir, entries, logger)
	for dir, files := range filesMap {
		for _, f := range files {
			err := os.Remove(filepath.Join(outputDir, dir, filepath.Base(f)))
			if err != nil {
				return err
			}
		}
	}
	for dir := range filesMap {
		err := os.RemoveAll(filepath.Join(outputDir, dir))
		if err != nil {
			return err
		}
	}
	return nil
}

// GetUkiCmdline returns the cmdline to be used for the kernel.
// The cmdline can be overridden by the user using the cmdline flag.
// For each cmdline passed, we generate a uki file with that cmdline
// extend-cmdline will just extend the default cmdline so we only create one efi file
// extra-cmdline will create a new efi file for each cmdline passed
func GetUkiCmdline(cmdlineExtend, bootBranding string, extraCmdlines []string) []enkiutils.BootEntry {
	defaultCmdLine := enkiconstants.UkiCmdline + " " + enkiconstants.UkiCmdlineInstall

	// Extend only
	if cmdlineExtend != "" {
		cmdline := defaultCmdLine + " " + cmdlineExtend
		return []enkiutils.BootEntry{{
			Cmdline:  cmdline,
			Title:    bootBranding,
			FileName: enkiutils.NameFromCmdline(enkiconstants.ArtifactBaseName, cmdline),
		}}
	}

	// default entry
	result := []enkiutils.BootEntry{{
		Cmdline:  defaultCmdLine,
		Title:    bootBranding,
		FileName: enkiutils.NameFromCmdline(enkiconstants.ArtifactBaseName, defaultCmdLine),
	}}

	// extra
	for _, extra := range extraCmdlines {
		cmdline := defaultCmdLine + " " + extra
		result = append(result, enkiutils.BootEntry{
			Cmdline:  cmdline,
			Title:    bootBranding,
			FileName: enkiutils.NameFromCmdline(enkiconstants.ArtifactBaseName, cmdline),
		})
	}

	return result
}

// GetUkiSingleCmdlines returns the single-efi-cmdline as passed by the user.
func GetUkiSingleCmdlines(bootBranding string, cmdlines []string, logger sdkTypes.KairosLogger) []enkiutils.BootEntry {
	result := []enkiutils.BootEntry{}
	// extra
	defaultCmdLine := enkiconstants.UkiCmdline + " " + enkiconstants.UkiCmdlineInstall

	for _, userValue := range cmdlines {
		bootEntry := enkiutils.BootEntry{}

		before, after, hasTitle := strings.Cut(userValue, ":")
		if hasTitle {
			bootEntry.Title = fmt.Sprintf("%s (%s)", bootBranding, before)
			bootEntry.Cmdline = defaultCmdLine + " " + after
			bootEntry.FileName = strings.ReplaceAll(before, " ", "_")
		} else {
			bootEntry.Title = bootBranding
			bootEntry.Cmdline = defaultCmdLine + " " + before
			bootEntry.FileName = enkiutils.NameFromCmdline("single_entry", before)
		}
		result = append(result, bootEntry)
	}

	return result
}
