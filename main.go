package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/kairos/sdk/unstructured"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/urfave/cli"
)

func main() {

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	app := &cli.App{
		Name:    "AuroraBoot",
		Version: "0.1",
		Author:  "Kairos authors",
		Usage:   "auroraboot",
		Flags: []cli.Flag{
			cli.StringSliceFlag{
				Name: "set",
			},
			cli.StringFlag{
				Name: "cloud-config",
			},
		},
		Description: `
`,
		UsageText: ``,
		Copyright: "kairos authors",
		Action: func(ctx *cli.Context) error {

			c := &deployer.Config{}
			r := &deployer.ReleaseArtifact{}

			file := ctx.Args().First()
			if file != "" {
				var err error
				c, r, err = deployer.LoadFile(file)
				if err != nil {
					return err
				}
			}

			setCommands := ctx.StringSlice("set")
			cloudConfig := ctx.String("cloud-config")

			m := map[string]interface{}{}
			for _, c := range setCommands {
				dat := strings.Split(c, "=")
				if len(dat) != 2 {
					return fmt.Errorf("Invalid arguments for set")
				}
				m[dat[0]] = dat[1]
			}

			y, err := unstructured.ToYAML(m)
			if err != nil {
				return err
			}

			yaml.Unmarshal(y, c)
			yaml.Unmarshal(y, r)

			if cloudConfig != "" {
				if _, err := os.Stat(cloudConfig); err == nil {
					dat, err := os.ReadFile(cloudConfig)
					if err == nil {
						c.CloudConfig = string(dat)
					}
				} else {
					c.CloudConfig = cloudConfig
				}
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
