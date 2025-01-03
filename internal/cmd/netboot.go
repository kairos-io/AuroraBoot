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
	ArgsUsage: "<iso> <output> <name>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		iso := c.Args().Get(0)
		if iso == "" {
			return fmt.Errorf("iso is required")
		}
		output := c.Args().Get(1)
		if output == "" {
			return fmt.Errorf("output is required")
		}

		name := c.Args().Get(2)
		if name == "" {
			return fmt.Errorf("name is required")
		}
		loglevel := "info"
		if c.Bool("debug") {
			loglevel = "debug"
		}
		internal.Log = types.NewKairosLogger("AuroraBoot", loglevel, false)

		f := ops.ExtractNetboot(iso, output, name)
		return f(c.Context)
	},
}
