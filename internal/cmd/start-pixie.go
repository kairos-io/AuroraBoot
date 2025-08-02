package cmd

import (
	"fmt"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
)

var StartPixieCmd = cli.Command{
	Name:    "start-pixie",
	Aliases: []string{"sp"},
	Usage:   "Start the Pixiecore netboot server and serve custom PXE files (kernel, initrd, squashfs) with a cloud-config.",
	Description: `Start a Pixiecore-based PXE server to serve a kernel, initrd, and squashfs image for network booting. 

Arguments:
  cloud-config-file   Path to the cloud-init or cloud-config YAML file.
  squashfs-file       Path to the root filesystem squashfs image.
  address             IP address to bind the server (e.g., 0.0.0.0).
  port                Port for the netboot server (e.g., 8080).
  initrd-file         Path to the initrd image.
  kernel-file         Path to the kernel image.

Options:
  --debug             Enable debug logging for troubleshooting.

Example:
  start-pixie user-data.yaml rootfs.squashfs 0.0.0.0 8080 initrd.img vmlinuz --debug
`,
	ArgsUsage: "<cloud-config-file> <squashfs-file> <address> <port> <initrd-file> <kernel-file>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug logging",
		},
		// Add more flags for optional NetBoot struct fields as needed
	},
	Action: func(c *cli.Context) error {
		cloudConfigFile := c.Args().Get(0)
		squashFSfile := c.Args().Get(1)
		address := c.Args().Get(2)
		netbootPort := c.Args().Get(3)
		initrdFile := c.Args().Get(4)
		kernelFile := c.Args().Get(5)

		// Simple argument validation
		if cloudConfigFile == "" || squashFSfile == "" || address == "" || netbootPort == "" || initrdFile == "" || kernelFile == "" {
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("all arguments are required")
		}

		loglevel := "info"
		if c.Bool("debug") {
			loglevel = "debug"
		}
		internal.Log = types.NewKairosLogger("AuroraBoot", loglevel, false)

		// Optionally parse NetBoot from flags here if desired
		nb := schema.NetBoot{} // Use defaults, or parse from CLI flags

		f := ops.StartPixiecore(
			cloudConfigFile,
			address,
			netbootPort,
			func() string { return squashFSfile }, // Wrap squashFSfile in a function
			func() string { return initrdFile },   // Wrap initrdFile in a function
			func() string { return kernelFile },   // Wrap kernelFile in a function
			nb,
		)

		return f(c.Context)
	},
}
