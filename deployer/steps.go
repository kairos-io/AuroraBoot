package deployer

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/spectrocloud-labs/herd"
)

func (d *Deployer) StepPrepNetbootDir() error {
	dstNetboot := d.Config.StateDir("netboot")

	return d.Add(opPrepareNetboot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepTmpRootDir() error {
	dstNetboot := d.Config.StateDir("netboot")

	return d.Add(opPreparetmproot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
}

func (d *Deployer) StepPrepISODir() error {
	dst := d.Config.StateDir("build")

	return d.Add(opPrepareISO, herd.WithCallback(func(ctx context.Context) error {
		return os.MkdirAll(dst, 0700)
	}))
}

func (d *Deployer) StepCopyCloudConfig() error {
	dst := d.Config.StateDir("build")

	return d.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(func(ctx context.Context) error {
			return os.WriteFile(filepath.Join(dst, "config.yaml"), []byte(d.Config.CloudConfig), 0600)
		}))
}

func (d *Deployer) StepPullContainer(fromImageOption func() bool) error {
	// Ops to generate from container image
	return d.Add(opContainerPull,
		herd.EnableIf(fromImageOption),
		herd.WithDeps(opPreparetmproot), herd.WithCallback(ops.PullContainerImage(d.containerImage(), d.tmpRootFs())))
}

func (d *Deployer) containerImage() string {
	// Pull local docker daemon if container image starts with docker://
	containerImage := d.Artifact.ContainerImage
	if strings.HasPrefix(containerImage, "docker://") {
		containerImage = strings.ReplaceAll(containerImage, "docker://", "")
	}

	return containerImage
}

func (d *Deployer) tmpRootFs() string {
	return d.Config.StateDir("temp-rootfs")
}
