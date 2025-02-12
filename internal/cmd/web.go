package cmd

import (
	"os"

	"github.com/kairos-io/AuroraBoot/internal/web"
	"github.com/urfave/cli/v2"
)

var WebCMD = cli.Command{
	Name:    "web",
	Aliases: []string{"w"},
	Usage:   "Starts a ui",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "address",
			Usage: "Listen address",
			Value: ":8080",
		},
		&cli.StringFlag{
			Name:  "artifact-dir",
			Usage: "Artifact directory",
			Value: "/tmp/artifacts",
		},
	},
	Action: func(c *cli.Context) error {
		os.MkdirAll(c.String("artifact-dir"), os.ModePerm)
		return web.App(c.String("address"), c.String("artifact-dir"))
	},
}
