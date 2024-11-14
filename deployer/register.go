package deployer

import (
	"fmt"
	"io"
	"os"
)

const (
	opDownloadISO     = "download-iso"
	opCopyCloudConfig = "copy-cloud-config"
	opPrepareISO      = "prepare-iso"
	opStartHTTPServer = "start-httpserver"
	opInjectCC        = "inject-cloud-config"

	opDownloadInitrd   = "download-initrd"
	opDownloadKernel   = "download-kernel"
	opDownloadSquashFS = "download-squashfs"
	opPrepareNetboot   = "prepare-netboot"
	opStartNetboot     = "start-netboot"

	opDumpSource     = "dump-source"
	opGenISO         = "gen-iso"
	opPreparetmproot = "prepare-temp"
	opExtractNetboot = "extract-netboot"

	opGenRawDisk    = "gen-raw-disk"
	opGenMBRRawDisk = "gen-raw-mbr-disk"

	opExtractSquashFS = "extract-squashfs"

	opConvertGCE       = "convert-gce"
	opConvertVHD       = "convert-vhd"
	opGenARMImages     = "build-arm-image"
	opPrepareARMImages = "prepare_arm"
)

// RegisterAll registers the op dag based on the configuration and the artifact wanted.
// This registers all steps for the top level Auroraboot command.
func RegisterAll(d *Deployer) error {
	for _, step := range []func() error{
		d.StepPrepTmpRootDir,
		d.StepPrepNetbootDir,
		d.StepPrepISODir,
		d.StepCopyCloudConfig,
		d.StepDumpSource,
		d.StepGenISO,
		d.StepExtractNetboot,
		//TODO: add Validate step
		// Ops to download from releases
		d.StepDownloadInitrd,
		d.StepDownloadKernel,
		d.StepDownloadSquashFS,
		d.StepDownloadISO,
		// Ops to generate disk images
		d.StepExtractSquashFS,
		d.StepGenRawDisk,
		d.StepGenMBRRawDisk,
		d.StepConvertGCE,
		d.StepConvertVHD,
		// ARM
		d.StepGenARMImages,
		d.StepPrepareARMImages,
		// Inject the data into the ISO
		d.StepInjectCC,
		// Start servers
		d.StepStartHTTPServer,
		d.StepStartNetboot,
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
