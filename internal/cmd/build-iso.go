package cmd

import (
	"fmt"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

var BuildISOCmd = cli.Command{
	Name:    "build-iso",
	Aliases: []string{"b"},
	Usage:   "Builds an ISO from a container image or github release",
	Flags: []cli.Flag{
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
			// hack to prevent ShowAppHelpAndExit from checking only subcommands
			// (in this case 'help')
			ctx.Command.Subcommands = nil
			cli.ShowCommandHelpAndExit(ctx, ctx.Command.Name, 1)
		}

		// TODO: Read these from command line args and build one config to by used
		// by all actions
		c, r, err := ReadConfig(ctx.Args().First(), ctx.String("cloud-config"), ctx.StringSlice("set"))
		if err != nil {
			return err
		}

		// TODO: Move to a RegisterBuildIso function
		d := deployer.NewDeployer(*c, *r, herd.EnableInit)
		for _, step := range []func() error{
			// TODO: Add more steps
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
