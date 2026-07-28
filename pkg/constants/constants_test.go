package constants

import (
	"slices"
	"strings"
	"testing"
)

func TestUkiCmdlineEnablesInitramfsNetworking(t *testing.T) {
	if !slices.Contains(strings.Fields(UkiCmdline), "rd.neednet=1") {
		t.Fatal("UkiCmdline does not enable initramfs networking")
	}
}
