package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	containerdCompression "github.com/containerd/containerd/v2/pkg/archive/compression"
	"github.com/google/go-containerregistry/pkg/name"
	container "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	sdkUtils "github.com/kairos-io/kairos-sdk/utils"
	"github.com/spf13/viper"
)

type BootEntry struct {
	FileName string
	Cmdline  string
	Title    string
}

// CreateSquashFS creates a squash file at destination from a source, with options
// TODO: Check validity of source maybe?
func CreateSquashFS(runner v1.Runner, logger sdkTypes.KairosLogger, source string, destination string, options []string) error {
	// create args
	args := []string{source, destination}
	// append options passed to args in order to have the correct order
	// protect against options passed together in the same string , i.e. "-x add" instead of "-x", "add"
	var optionsExpanded []string
	for _, op := range options {
		optionsExpanded = append(optionsExpanded, strings.Split(op, " ")...)
	}
	args = append(args, optionsExpanded...)
	out, err := runner.Run("mksquashfs", args...)
	if err != nil {
		logger.Debugf("Error running squashfs creation, stdout: %s", out)
		logger.Errorf("Error while creating squashfs from %s to %s: %s", source, destination, err)
		return err
	}
	return nil
}

func GolangArchToArch(arch string) (string, error) {
	switch strings.ToLower(arch) {
	case constants.ArchAmd64:
		return constants.Archx86, nil
	case constants.ArchArm64:
		return constants.ArchArm64, nil
	default:
		return "", fmt.Errorf("invalid arch")
	}
}

// GetUkiCmdline returns the cmdline to be used for the kernel.
// The cmdline can be overridden by the user using the cmdline flag.
// For each cmdline passed, we generate a uki file with that cmdline
// extend-cmdline will just extend the default cmdline so we only create one efi file
// extra-cmdline will create a new efi file for each cmdline passed
func GetUkiCmdline() []BootEntry {
	defaultCmdLine := constants.UkiCmdline + " " + constants.UkiCmdlineInstall

	// Extend only
	cmdlineExtend := viper.GetString("extend-cmdline")
	if cmdlineExtend != "" {
		cmdline := defaultCmdLine + " " + cmdlineExtend
		return []BootEntry{{
			Cmdline:  cmdline,
			Title:    viper.GetString("boot-branding"),
			FileName: NameFromCmdline(constants.ArtifactBaseName, cmdline),
		}}
	}

	// default entry
	result := []BootEntry{{
		Cmdline:  defaultCmdLine,
		Title:    viper.GetString("boot-branding"),
		FileName: NameFromCmdline(constants.ArtifactBaseName, defaultCmdLine),
	}}

	// extra
	for _, extra := range viper.GetStringSlice("extra-cmdline") {
		cmdline := defaultCmdLine + " " + extra
		result = append(result, BootEntry{
			Cmdline:  cmdline,
			Title:    viper.GetString("boot-branding"),
			FileName: NameFromCmdline(constants.ArtifactBaseName, cmdline),
		})
	}

	return result
}

// GetUkiSingleCmdlines returns the single-efi-cmdline as passed by the user.
func GetUkiSingleCmdlines(logger sdkTypes.KairosLogger) []BootEntry {
	result := []BootEntry{}
	// extra
	defaultCmdLine := constants.UkiCmdline + " " + constants.UkiCmdlineInstall

	cmdlines := viper.GetStringSlice("single-efi-cmdline")
	for _, userValue := range cmdlines {
		bootEntry := BootEntry{}

		before, after, hasTitle := strings.Cut(userValue, ":")
		if hasTitle {
			bootEntry.Title = fmt.Sprintf("%s (%s)", viper.GetString("boot-branding"), before)
			bootEntry.Cmdline = defaultCmdLine + " " + after
			bootEntry.FileName = strings.ReplaceAll(before, " ", "_")
		} else {
			bootEntry.Title = viper.GetString("boot-branding")
			bootEntry.Cmdline = defaultCmdLine + " " + before
			bootEntry.FileName = NameFromCmdline("single_entry", before)
		}
		result = append(result, bootEntry)
	}

	return result
}

// Tar takes a source and variable writers and walks 'source' writing each file
// found to the tar writer; the purpose for accepting multiple writers is to allow
// for multiple outputs (for example a file, or md5 hash)
func Tar(src string, writers ...io.Writer) error {
	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("Unable to tar files - %v", err.Error())
	}

	mw := io.MultiWriter(writers...)

	gzw := gzip.NewWriter(mw)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// walk path
	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fi.Mode().IsRegular() {
			return nil
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		f.Close()

		return nil
	})
}

// CreateTar a imagetarball from a standard tarball
func CreateTar(log sdkTypes.KairosLogger, srctar, dstimageTar, imagename, architecture, OS string) error {

	dstFile, err := os.Create(dstimageTar)
	if err != nil {
		return fmt.Errorf("Cannot create %s: %s", dstimageTar, err)
	}
	defer dstFile.Close()

	newRef, img, err := imageFromTar(imagename, architecture, OS, func() (io.ReadCloser, error) {
		f, err := os.Open(srctar)
		if err != nil {
			return nil, fmt.Errorf("cannot open %s: %s", srctar, err)
		}
		decompressed, err := containerdCompression.DecompressStream(f)
		if err != nil {
			return nil, fmt.Errorf("cannot open %s: %s", srctar, err)
		}

		return decompressed, nil
	})
	if err != nil {
		return err
	}

	// Lets try to load it into the docker daemon?
	// Code left here in case we want to use it in the future
	/*
		tag, err := name.NewTag(imagename)

		if err != nil {
			log.Warnf("Cannot create tag for %s: %s", imagename, err)
		}
		if err == nil {
			// Best effort only, just try and forget
			out, err := daemon.Write(tag, img)
			if err != nil {
				log.Warnf("Cannot write image %s to daemon: %s\noutput: %s", imagename, err, out)
			} else {
				log.Infof("Image %s written to daemon", tag.String())
			}
		}
	*/

	return tarball.Write(newRef, img, dstFile)

}

func imageFromTar(imagename, architecture, OS string, opener func() (io.ReadCloser, error)) (name.Reference, container.Image, error) {
	newRef, err := name.ParseReference(imagename)
	if err != nil {
		return nil, nil, err
	}

	layer, err := tarball.LayerFromOpener(opener)
	if err != nil {
		return nil, nil, err
	}

	baseImage := empty.Image
	cfg, err := baseImage.ConfigFile()
	if err != nil {
		return nil, nil, err
	}

	cfg.Architecture = architecture
	cfg.OS = OS

	baseImage, err = mutate.ConfigFile(baseImage, cfg)
	if err != nil {
		return nil, nil, err
	}
	img, err := mutate.Append(baseImage, mutate.Addendum{
		Layer: layer,
		History: container.History{
			CreatedBy: "Enki",
			Comment:   "Custom image",
			Created:   container.Time{Time: time.Now()},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	return newRef, img, nil
}

func IsAmd64(arch string) bool {
	return arch == constants.ArchAmd64 || arch == constants.Archx86
}

func IsArm64(arch string) bool {
	return arch == constants.ArchArm64 || arch == constants.Archaarch64
}

// NameFromCmdline returns the name of the efi/conf file based on the cmdline
// we want to have at least 1 efi file that its the default, that is the one we ship with the iso/media/whatever install medium
// that one has the default cmdline + the install cmdline
// For that one, we use it as the BASE one, configs will only trigger for that install stanza if we are on install media
// so we dont have to worry about it, but we want to provide a clean name for it
// so in that case we dont add anything to the efi name/conf name/cmdline inside the config
// For the other ones, we add the cmdline to the efi name and the cmdline to the conf file
// so you get
// - norole.efi
// - norole.conf
// - norole_interactive-install.efi
// - norole_interactive-install.conf
// This is mostly for convenience in generating the names as the real data is stored in the config file
// but it can easily be used to identify the efi file and the conf file.
func NameFromCmdline(basename, cmdline string) string {
	// Remove the default cmdline from the current cmdline
	cmdlineForEfi := strings.TrimSpace(strings.TrimPrefix(cmdline, constants.UkiCmdline))
	// For the default install entry, do not add anything on the efi name
	if cmdlineForEfi == constants.UkiCmdlineInstall {
		cmdlineForEfi = ""
	}
	// Although only slashes are truly forbidden, we also replace other characters,
	// as they can be problematic when interpreted by the shell (e.g. &, |, etc.)
	allowedChars := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	cleanCmdline := allowedChars.ReplaceAllString(cmdlineForEfi, "_")
	name := basename + "_" + cleanCmdline
	// If the cmdline is empty, we remove the underscore as to not get a dangling one
	finalName := strings.TrimSuffix(name, "_")
	return finalName
}

// GetArchFromRootfs returns the architecture from the rootfs of a Kairos image
func GetArchFromRootfs(rootfs string, l sdkTypes.KairosLogger) (string, error) {
	var arch string
	var ok bool
	releaseFilename := filepath.Join("etc", "kairos-release")
	if _, ok := os.Stat(filepath.Join(rootfs, releaseFilename)); os.IsNotExist(ok) {
		// Try to fall back to os-release as we used that before
		releaseFilename = filepath.Join("etc", "os-release")
	}
	l.Logger.Debug().Str("file", releaseFilename).Str("rootfs", rootfs).Msg("Checking for architecture in rootfs")

	kairosRelease, err := godotenv.Read(filepath.Join(rootfs, releaseFilename))
	if err != nil {
		return "", err
	}
	arch, ok = kairosRelease["KAIROS_ARCH"]
	if ok && arch != "" {
		l.Logger.Debug().Str("file", releaseFilename).Str("arch", arch).Str("rootfs", rootfs).Msg("Found KAIROS_ARCH in rootfs")
		return arch, nil
	}

	// Fall back to target arch, this was used before kairos-init
	archFallback, ok := kairosRelease["KAIROS_TARGETARCH"]
	if ok && archFallback != "" {
		l.Logger.Debug().Str("file", releaseFilename).Str("arch", archFallback).Str("rootfs", rootfs).Msg("Found KAIROS_TARGETARCH in rootfs")
		return archFallback, nil
	}
	l.Logger.Debug().Str("file", releaseFilename).Str("rootfs", rootfs).Msg("Could not find KAIROS_ARCH/KAIROS_TARGETARCH in rootfs")
	return "", fmt.Errorf("KAIROS_ARCH/KAIROS_TARGETARCH not found in %s", releaseFilename)
}

// NameFromRootfs This generates the artifact name based on the rootfs kairos-release files
// name of isos for example so we store them equally:
// kairos-ubuntu-24.04-core-amd64-generic-v3.2.4.iso
// Raw images
// kairos-ubuntu-24.04-core-amd64-generic-v3.2.4.raw
// Containers
// 24.10-core-amd64-generic-v3.3.1
// 22.04-core-arm64-rpi4-v3.3.1
// UKI containers
// 24.04-core-amd64-generic-v3.3.1-uki
// raw images for boards
// 22.04-core-arm64-rpi4-v3.3.1-img
// So basically for iso/raw images we append kairos and the distro name
// for containers we store them under the distro name (ubuntu, opensuse, etc) as the repo name
// and then the rest is for the tag
// quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.2.4
// so in here we only return the shared part of the name
// its the callers responsibility to add the rest of the name if its building an iso or raw image
// also, no extension is added to the name, so its up to the caller to add it
func NameFromRootfs(rootfs string) string {
	var label string
	var flavor string
	var err error
	if _, ok := os.Stat(filepath.Join(rootfs, "etc/kairos-release")); ok == nil {
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(rootfs, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
		}
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(rootfs, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image flavor")
		}
	} else {
		// Before 3.2.x the kairos info was in /etc/os-release
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(rootfs, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
		}
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(rootfs, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
		}
	}

	return fmt.Sprintf("%s-%s", flavor, label)
}
