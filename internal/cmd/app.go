package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/internal/config"
	"github.com/kairos-io/AuroraBoot/internal/worker"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

var WorkerCmd = cli.Command{
	Name:  "worker",
	Usage: "Start a build worker",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "endpoint",
			Usage:    "API endpoint URL (e.g., http://localhost:8080)",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "worker-id",
			Usage:    "Unique worker identifier",
			Required: true,
		},
	},
	Action: func(ctx *cli.Context) error {
		w := worker.NewWorker(ctx.String("endpoint"), ctx.String("worker-id"))
		fmt.Printf("Starting worker %s, connecting to %s\n", ctx.String("worker-id"), ctx.String("endpoint"))
		return w.Start(ctx.Context)
	},
}

func GetApp(version string) *cli.App {
	return &cli.App{
		Name:    "AuroraBoot",
		Version: version,
		Authors: []*cli.Author{{Name: "Kairos authors", Email: "members@kairos.io"}},
		Usage:   "auroraboot",
		Commands: []*cli.Command{
			&UkiPXECmd,
			&BuildISOCmd,
			&BuildUKICmd,
			&GenKeyCmd,
			&SysextCmd,
			&NetBootCmd,
      &StartPixieCmd,
			&WebCMD,
			&RedFishDeployCmd,
			&WorkerCmd,
		},
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
			c, r, err := config.ReadConfig(ctx.Args().First(), ctx.String("cloud-config"), ctx.StringSlice("set"))
			if err != nil {
				return err
			}

			d := deployer.NewDeployer(*c, *r, herd.CollectOrphans)
			err = deployer.RegisterAll(d)
			if err != nil {
				return err
			}

			if d.Config.State == "" {
				d.Config.State = "/tmp/auroraboot"
			}

			d.WriteDag()
			if err := d.Run(ctx.Context); err != nil {
				return err
			}

			err = d.CollectErrors()
			errCleanup := d.CleanTmpDirs()
			if errCleanup != nil {
				// Append the cleanup error to the main errors if any
				err = multierror.Append(err, errCleanup)
			}

			// If there are errors, write the DAG to help debugging
			if err != nil {
				d.WriteDag()
			}
			return err

		},
	}
}

// CheckRoot is a helper which can add it to commands that require root
func CheckRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("this command requires root privileges")
	}
	return nil
}
