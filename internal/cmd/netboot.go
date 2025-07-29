package cmd

import (
	"fmt"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
	 "github.com/kairos-io/AuroraBoot/pkg/schema"
)

var NetBootCmd = cli.Command{
	Name:      "netboot",
	Aliases:   []string{"nb"},
	Usage:     "Extract artifacts for netboot from a given ISO",
	ArgsUsage: "<iso-file> <output-dir> <output-artifact-prefix>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		iso := c.Args().Get(0)
		if iso == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("iso-file is required")
		}
		output := c.Args().Get(1)
		if output == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("output-dir is required")
		}

		name := c.Args().Get(2)
		if name == "" {
			c.Command.Subcommands = nil
			cli.ShowCommandHelp(c, c.Command.Name)
			fmt.Println("")
			return fmt.Errorf("output-artifact-prefix is required")
		}
		loglevel := "info"
		if c.Bool("debug") {
			loglevel = "debug"
		}
		internal.Log = types.NewKairosLogger("AuroraBoot", loglevel, false)
		isoGet := func() string {
			return iso
		}
		outputGet := func() string {
			return output
		}
		f := ops.ExtractNetboot(isoGet, outputGet, name)
		return f(c.Context)
	},
}

var StartPixieCmd = cli.Command{
    Name:      "start-pixie",
    Usage:     "Start the Pixiecore netboot server and custom server PXE Files.",
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
            func() string { return initrdFile },    // Wrap initrdFile in a function
            func() string { return kernelFile },     // Wrap kernelFile in a function
            nb,
        )

        return f(c.Context)
    },
}