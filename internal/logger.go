package internal

import sdkTypes "github.com/kairos-io/kairos-sdk/types"

var Log sdkTypes.KairosLogger

// The init function initializes the default logger for the package.
// This ensures that logging is configured and ready for use throughout the package.
func init() {
	Log = sdkTypes.NewKairosLogger("aurora", "info", false)
}
