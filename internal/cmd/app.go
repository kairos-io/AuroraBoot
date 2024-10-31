package cmd

import (
	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

func GetApp(version string) *cli.App {
	return &cli.App{
		Name:     "AuroraBoot",
		Version:  version,
		Authors:  []*cli.Author{{Name: "Kairos authors", Email: "members@kairos.io"}},
		Usage:    "auroraboot",
		Commands: []*cli.Command{&BuildISOCmd},
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
		Description: "Auroraboot is a tool that builds various Kairos artifacts suitable to run Kairos on Vms, bare metal, public cloud or single board computers (SBCs).\nIt also provides functionality like network booting to install Kairos. Read more in the docs: https://kairos.io/docs/reference/auroraboot/",
		UsageText:   ``,
		Copyright:   "Kairos authors",
		Action: func(ctx *cli.Context) error {
			internal.Log = sdkTypes.NewKairosLogger("aurora", "info", false)

			if ctx.Bool("debug") {
				internal.Log.SetLevel("debug")
			}
			c, r, err := ReadConfig(ctx.Args().First(), ctx.String("cloud-config"), ctx.StringSlice("set"))
			if err != nil {
				return err
			}
			c.ISO.Name = KairosDefaultArtifactName

			d := deployer.NewDeployer(*c, *r, herd.CollectOrphans)
			err = deployer.RegisterAll(d)
			if err != nil {
				return err
			}

			d.WriteDag()
			if err := d.Run(ctx.Context); err != nil {
				return err
			}

			return d.CollectErrors()
		},
	}
}