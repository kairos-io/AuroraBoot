package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	agentconstants "github.com/kairos-io/kairos/v4/agent/pkg/constants"
	"github.com/kairos-io/kairos/v4/sdk/iso"
	extensiontypes "github.com/kairos-io/kairos/v4/sdk/types/extensions"
	"gopkg.in/yaml.v3"
)

// valueGetOnCall is a function type that returns a string value.
// It is used to defer the retrieval of a value until the function is called,
// allowing for dynamic values to be fetched at the time of the call.
// This is mainly due to the fact that the deployer might not have the values available at the time of the registration,
// but rather at the time of the call, e.g. when the ISO file is downloaded or the destination directory is created.
type valueGetOnCall func() string

// ExtractNetboot extracts all the required netbooting artifacts
// isoFunc is a function that returns the path to the ISO file
// we need the function to be passed so its executed in the context of the deployer as otherwise
// the ISO file might not be available at the time of the call
func ExtractNetboot(isoFunc, dstFunc valueGetOnCall, prefix string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		src := isoFunc()
		dst := dstFunc()

		if prefix == "" {
			prefix = "kairos"
		}

		if _, err := os.Stat(dst); err != nil && os.IsNotExist(err) {
			return fmt.Errorf("destination directory %s does not exist: %w", dst, err)
		}
		internal.Log.Logger.Info().Str("prefix", prefix).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")

		artifact := filepath.Join(dst, fmt.Sprintf("%s.squashfs", prefix))
		err := iso.ExtractFileFromIso("/rootfs.squashfs", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-kernel", prefix))
		err = iso.ExtractFileFromIso("/boot/kernel", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-initrd", prefix))
		err = iso.ExtractFileFromIso("/boot/initrd", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		carried, err := extractNetbootExtensions(src, dst)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("source", src).Str("destination", dst).Msg("Failed extracting the bundled extensions")
			return err
		}
		if len(carried) > 0 {
			internal.Log.Logger.Info().Strs("extensions", carried).Str("destination", dst).Msg("Extracted the bundled system extensions")
		}
		internal.Log.Logger.Info().Msg("Artifacts extracted")

		return nil
	}
}

func StartPixiecore(cloudConfigFile, address, netbootPort string, squashFSfileGet, initrdFileGet, kernelFileGet valueGetOnCall, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Start pixiecore")
		// do them in the context of the deployer so we can use the functions at the time of the call
		squashFSfile := squashFSfileGet()
		initrdFile := initrdFileGet()
		kernelFile := kernelFileGet()

		configFile := cloudConfigFile

		cmdLine := `rd.live.overlay.overlayfs rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

		if nb.Cmdline != "" {
			cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
		}

		return netboot.Server(kernelFile, fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, initrdFile, true)
	}
}

// isoRootEntries lists the names of the plain files in the ISO root.
func isoRootEntries(src string) ([]string, error) {
	img, err := diskfs.Open(src, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return nil, err
	}
	defer img.Close()
	fsys, err := img.GetFilesystem(0)
	if err != nil {
		return nil, err
	}
	// io/fs paths carry no leading slash, so the ISO root is ".".
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// isoName normalizes an ISO directory entry for comparison. A plain ISO 9660
// name carries a ";1" version suffix and is upper case; a Rock Ridge one, which
// is what the Kairos ISOs are built with, does not. Comparing normalized names
// reads both.
func isoName(name string) string {
	if i := strings.LastIndex(name, ";"); i > 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
}

// extractNetbootExtensions carries the bundled system extensions out of the
// ISO root and into the netboot directory.
//
// A resolved extension image lands at the ISO root rather than inside
// rootfs.squashfs: GenISO appends the materialized images to spec.Image, while
// spec.RootFS is what becomes the squashfs. The three paths above are all a
// netboot extraction used to read, so every bundled extension was dropped on
// the way to the netboot artifacts. Refs kairos-io/kairos#5040.
//
// Which root files are extensions is recorded in the extensions.yaml that
// materializeISOExtensions writes next to them, so that declaration is what is
// read here. An ISO carrying no such file carries no bundled extension and
// this is a no-op, and an --overlay-iso that happens to ship a .raw of its own
// is not mistaken for one.
//
// It returns the names it wrote into dst.
func extractNetbootExtensions(src, dst string) ([]string, error) {
	entries, err := isoRootEntries(src)
	if err != nil {
		return nil, fmt.Errorf("listing the root of %s: %w", src, err)
	}
	onISO := make(map[string]string, len(entries))
	for _, entry := range entries {
		onISO[isoName(entry)] = entry
	}
	declaration, ok := onISO[isoName(isoExtensionsConfig)]
	if !ok {
		return nil, nil
	}

	tmp, err := os.MkdirTemp("", "auroraboot-netboot-extensions")
	if err != nil {
		return nil, fmt.Errorf("creating the declaration staging directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	local := filepath.Join(tmp, isoExtensionsConfig)
	if err := iso.ExtractFileFromIso("/"+declaration, src, local, &internal.Log); err != nil {
		return nil, fmt.Errorf("extracting %s: %w", isoExtensionsConfig, err)
	}
	declared, err := declaredExtensionImages(local)
	if err != nil {
		return nil, err
	}

	carried := make([]string, 0, len(declared))
	for _, name := range declared {
		entry, ok := onISO[isoName(name)]
		if !ok {
			// AuroraBoot wrote both the declaration and the image, so a
			// declaration naming a file the root does not carry means the ISO
			// is inconsistent. Say so rather than shipping a netboot tree that
			// silently lacks an extension the ISO install gets.
			return carried, fmt.Errorf("%s declares %s but the ISO root does not carry it", isoExtensionsConfig, name)
		}
		if err := iso.ExtractFileFromIso("/"+entry, src, filepath.Join(dst, name), &internal.Log); err != nil {
			return carried, fmt.Errorf("extracting the bundled extension %s: %w", name, err)
		}
		carried = append(carried, name)
	}
	return carried, nil
}

// declaredExtensionImages reads the file names install.extensions points at on
// the live media.
//
// Only an entry sitting directly under the live media directory names a file
// on this ISO, because that is where materializeISOExtensions puts the images
// it resolves: at the root. Everything else is the installed system's to fetch
// rather than this build's to carry, whether it is an oci:// or https:// image
// or a nested live-media path from an extensions.yaml AuroraBoot did not write,
// so it is skipped rather than looked for at a root that does not hold it.
func declaredExtensionImages(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", isoExtensionsConfig, err)
	}
	var config struct {
		Install struct {
			Extensions extensiontypes.Extensions `yaml:"extensions"`
		} `yaml:"install"`
	}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", isoExtensionsConfig, err)
	}
	images := make([]string, 0, len(config.Install.Extensions))
	for _, extension := range config.Install.Extensions {
		if filepath.Dir(extension.Name) != agentconstants.LiveDir {
			continue
		}
		images = append(images, filepath.Base(extension.Name))
	}
	return images, nil
}
