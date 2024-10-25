package cmd

import (
	"fmt"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

var BuildISOCmd = cli.Command{
	Name:    "build-iso",
	Aliases: []string{"b"},
	Usage:   "Builds an ISO from a container image or github release",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "cloud-config",
			Aliases: []string{"c"},
			Usage:   "The cloud config to embed in the ISO",
		},
		&cli.StringFlag{
			Name:    "name",
			Aliases: []string{"n"},
			Usage:   "Basename of the generated ISO file",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output directory (defaults to current directory)",
		},
		&cli.BoolFlag{
			Name:  "date",
			Value: false,
			Usage: "Adds a date suffix into the generated ISO file",
		},
		&cli.StringFlag{
			Name:  "overlay-rootfs",
			Usage: "Path of the overlayed rootfs data",
		},
		&cli.StringFlag{
			Name:  "overlay-uefi",
			Usage: "Path of the overlayed uefi data",
		},
		&cli.StringFlag{
			Name:  "overlay-iso",
			Usage: "Path of the overlayed iso data",
		},
		&cli.StringFlag{
			Name:  "label",
			Usage: "Label of the ISO volume",
		},
		&cli.BoolFlag{
			Name:  "squash-no-compression",
			Value: true,
			Usage: "Disable squashfs compression",
		},
		// TODO: Validate that only one of the allowed values is used
		// archType := newEnumFlag([]string{"x86_64", "arm64"}, "x86_64")
		&cli.StringFlag{
			Name:    "arch",
			Aliases: []string{"a"},
			Usage:   "Arch to build the image for",
		},
	},
	ArgsUsage: "<source>",
	Action: func(ctx *cli.Context) error {
		source := ctx.Args().Get(0)
		if source == "" {
			fmt.Println("\nNo source defined\n")
			// Hack to prevent ShowAppHelpAndExit from checking only subcommands.
			// (in this case there is only the 'help' subcommand). We are exiting
			// anyway, so no harm from setting it to nil.
			ctx.Command.Subcommands = nil
			cli.ShowCommandHelpAndExit(ctx, ctx.Command.Name, 1)
		}

		// TODO: Read these from command line args and build one config to by used
		// by all actions
		cloudConfig := ""
		var err error
		if ctx.String("cloud-config") != "" {
			// we don't allow templating in this command (like we do at the top level one)
			// TODO: Should we allow it?
			cloudConfig, err = readCloudConfig(ctx.String("cloud-config"), map[string]interface{}{})
			if err != nil {
				return fmt.Errorf("reading cloud config: %w", err)
			}
		}
		r := schema.ReleaseArtifact{ContainerImage: source, Name: ctx.String("name")}
		c := schema.Config{State: ctx.String("output"), CloudConfig: cloudConfig}

		d := deployer.NewDeployer(c, r, herd.EnableInit)
		for _, step := range []func() error{
			d.StepPrepNetbootDir,
			d.StepPrepTmpRootDir,
			d.StepPrepISODir,
			d.StepCopyCloudConfig,
			d.StepPullContainer,
			d.StepGenISO,
		} {
			if err := step(); err != nil {
				return err
			}
		}

		if err := d.Run(ctx.Context); err != nil {
			return err
		}

		return d.CollectErrors()
	},
}
