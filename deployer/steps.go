package deployer

import (
	"context"
	"fmt"
	"os"

	"github.com/spectrocloud-labs/herd"
)

func (d *Deployer) StepPrepNetbootDir() error {
	dstNetboot := d.config.StateDir("netboot")

	return d.Add(opPrepareNetboot, herd.WithCallback(
		func(ctx context.Context) error {
			fmt.Println("creating another dir")
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepTmpRootDir() error {
	dstNetboot := d.config.StateDir("netboot")

	return d.Add(opPreparetmproot, herd.WithCallback(
		func(ctx context.Context) error {
			fmt.Println("creating a dir")
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepDestDir() error {
	dst := d.config.StateDir("build")

	return d.Add(opPrepareISO, herd.WithCallback(func(ctx context.Context) error {
		fmt.Println("creating yet another dir")
		return os.MkdirAll(dst, 0700)
	}))
}
