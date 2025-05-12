package cmd

import (
	"fmt"
	"os"

	"github.com/kairos-io/AuroraBoot/internal/web"
	"github.com/kairos-io/AuroraBoot/internal/worker"
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
		&cli.StringFlag{
			Name:  "logs-dir",
			Usage: "Directory to store build logs",
			Value: "/tmp/build-logs",
		},
		&cli.BoolFlag{
			Name:  "create-worker",
			Usage: "Start a local worker in a goroutine",
			Value: false,
		},
	},
	Action: func(c *cli.Context) error {
		os.MkdirAll(c.String("artifact-dir"), os.ModePerm)
		os.MkdirAll(c.String("logs-dir"), os.ModePerm)

		// If create-worker flag is set, start a worker in a goroutine
		if c.Bool("create-worker") {
			workerID := "local-worker"
			w := worker.NewWorker("http://localhost"+c.String("address"), workerID)
			go func() {
				if err := w.Start(); err != nil {
					// Log error but don't exit - the web server should keep running
					fmt.Printf("Worker error: %v\n", err)
				}
			}()
		}

		return web.App(c.String("address"), c.String("artifact-dir"), c.String("logs-dir"))
	},
}
