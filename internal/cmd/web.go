package cmd

import (
	"fmt"
	"net"
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
			Name:  "builds-dir",
			Usage: "Directory to store build jobs and their artifacts",
			Value: "/tmp/kairos-builds",
		},
		&cli.BoolFlag{
			Name:  "create-worker",
			Usage: "Start a local worker in a goroutine",
			Value: false,
		},
	},
	Action: func(c *cli.Context) error {
		os.MkdirAll(c.String("artifact-dir"), os.ModePerm)
		os.MkdirAll(c.String("builds-dir"), os.ModePerm)

		// If create-worker flag is set, start a worker in a goroutine
		if c.Bool("create-worker") {
			workerID := "local-worker"
			// Extract just the port from the address for the worker connection
			// The server might be listening on 0.0.0.0:port, but we need to connect to localhost:port
			addr := c.String("address")
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return fmt.Errorf("invalid address format: %v", err)
			}
			workerAddr := "http://localhost:" + port
			w := worker.NewWorker(workerAddr, workerID)
			go func() {
				if err := w.Start(); err != nil {
					// Log error but don't exit - the web server should keep running
					fmt.Printf("Worker error: %v\n", err)
				}
			}()
		}

		return web.App(web.AppConfig{
			EnableLogger: true,
			ListenAddr:   c.String("address"),
			OutDir:       c.String("artifact-dir"),
			BuildsDir:    c.String("builds-dir"),
		})
	},
}
