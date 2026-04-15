package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// WebCMD is a temporary stub: the old Alpine.js web UI and jobstorage
// worker pipeline have been removed. The real fleet management server
// is being ported in from github.com/kairos-io/daedalus and will be
// wired into this command in the next commit.
var WebCMD = cli.Command{
	Name:    "web",
	Aliases: []string{"w"},
	Usage:   "Run the fleet management web UI and REST API (under reconstruction)",
	Action: func(c *cli.Context) error {
		return fmt.Errorf("auroraboot web is being rewired — the new server lands in the next commit on this branch")
	},
}
