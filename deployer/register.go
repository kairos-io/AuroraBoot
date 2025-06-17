package deployer

import (
	"fmt"
	"io"
	"os"
)

// RegisterAll registers the op dag based on the configuration and the artifact wanted.
// This registers all steps for the top level Auroraboot command.
func RegisterAll(d *Deployer) error {
	for _, step := range []func() error{
		d.StepPrepDestination,
		d.StepPrepTmpRootDir,
		d.StepPrepNetbootDir,
		d.StepCopyCloudConfig,
		d.StepDumpSource,
		d.StepGenISO,
		d.StepDownloadISO,
		d.StepExtractNetboot,
		// Ops to generate RAW disk images
		d.StepGenRawDisk,
		d.StepGenMBRRawDisk,
		d.StepConvertGCE,
		d.StepConvertVHD,
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
