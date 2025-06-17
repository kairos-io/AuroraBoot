package cmd

import (
	"fmt"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
)

var NetBootCmd = cli.Command{
	Name:      "netboot",
	Aliases:   []string{"nb"},
	Usage:     "Extract artifacts for netboot from a given ISO",
	ArgsUsage: "<iso-file> <output-dir> <output-artifact-prefix>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		iso := c.Args().Get(0)
		if iso == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("iso-file is required")
		}
		output := c.Args().Get(1)
		if output == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("output-dir is required")
		}

		name := c.Args().Get(2)
		if name == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("output-artifact-prefix is required")
		}
		loglevel := "info"
		if c.Bool("debug") {
			loglevel = "debug"
		}
		internal.Log = types.NewKairosLogger("AuroraBoot", loglevel, false)
		isoGet := func() string {
			return iso
		}
		outputGet := func() string {
			return output
		}
		f := ops.ExtractNetboot(isoGet, outputGet, name)
		return f(c.Context)
	},
}
