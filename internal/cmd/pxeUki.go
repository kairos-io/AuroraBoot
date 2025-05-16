package cmd

import (
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
)

var UkiPXECmd = cli.Command{
	Name:  "uki-pxe",
	Usage: "",
	Action: func(context *cli.Context) error {
		log := types.NewKairosLogger("pxe", "debug", false)
		return utils.ServeUkiPXE(log)
	},
}
