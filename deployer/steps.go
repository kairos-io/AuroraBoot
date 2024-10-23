package deployer

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spectrocloud-labs/herd"
)

func (d *Deployer) StepPrepNetbootDir() error {
	dstNetboot := d.config.StateDir("netboot")

	return d.Add(opPrepareNetboot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepTmpRootDir() error {
	dstNetboot := d.config.StateDir("netboot")

	return d.Add(opPreparetmproot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepDestDir() error {
	dst := d.config.StateDir("build")

	return d.Add(opPrepareISO, herd.WithCallback(func(ctx context.Context) error {
		return os.MkdirAll(dst, 0700)
	}))
}

func (d *Deployer) StepCopyCloudConfig() error {
	dst := d.config.StateDir("build")

	return d.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(func(ctx context.Context) error {
			return os.WriteFile(filepath.Join(dst, "config.yaml"), []byte(d.config.CloudConfig), 0600)
		}))
}
