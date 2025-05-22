package cmd

import (
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
	"os"
)

var UkiPXECmd = cli.Command{
	Name:  "uki-pxe",
	Usage: "Serve PXE boot files using a specified ISO file and key directory",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "key-dir",
			Usage:    "Directory containing the keys",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "iso-file",
			Usage:    "Iso file to use",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "loglevel",
			Aliases: []string{"l"},
			Usage:   "Set the log level",
			Value:   "info",
		},
	},
	Before: func(c *cli.Context) error {
		isoFile := c.String("iso-file")
		if _, err := os.Stat(isoFile); err != nil {
			if os.IsNotExist(err) {
				return cli.Exit("iso file does not exist", 1)
			}
			return cli.Exit("error checking iso file: "+err.Error(), 1)
		}
		return nil
	},
	Action: func(context *cli.Context) error {
		log := types.NewKairosLogger("pxe", context.String("loglevel"), false)
		return utils.ServeUkiPXE(context.String("key-dir"), context.String("iso-file"), log)
	},
}
