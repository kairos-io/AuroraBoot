package internal

import sdkTypes "github.com/kairos-io/kairos-sdk/types"

var Log sdkTypes.KairosLogger

func init() {
	Log = sdkTypes.NewKairosLogger("aurora", "info", false)
}
