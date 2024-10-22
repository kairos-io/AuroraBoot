package main

import (
	"fmt"
	"os"

	cmd "github.com/kairos-io/AuroraBoot/internal/cmd"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func main() {

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	app := &cli.App{
		Name:    "AuroraBoot",
		Version: "0.1",
		Authors: []*cli.Author{{Name: "Kairos authors", Email: "members@kairos.io"}},
		Usage:   "auroraboot",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name: "set",
			},
			&cli.StringFlag{
				Name: "cloud-config",
			},
			&cli.BoolFlag{
				Name: "debug",
			},
		},
		Description: `
`,
		UsageText: ``,
		Copyright: "kairos authors",
		Action: func(ctx *cli.Context) error {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			if ctx.Bool("debug") {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}
			c, r, err := cmd.ReadConfig(ctx.Args().First(), ctx.String("cloud-config"), ctx.StringSlice("set"))

			if err != nil {
				return err
			}

			if err := deployer.Start(c, r); err != nil {
				return err
			}
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
