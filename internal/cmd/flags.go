package cmd

import "github.com/urfave/cli/v2"

// InsecureFlag is shared by every command that pulls container images, so the
// flag is defined once and behaves consistently. urfave/cli v2 has no
// persistent-flag mechanism; defining it as a global app flag would force it to
// be passed before the subcommand (e.g. "auroraboot --insecure build-iso ..."),
// breaking the natural "build-iso --insecure ..." form, so we share a single
// flag value across the commands that need it instead.
var InsecureFlag = &cli.BoolFlag{
	Name:  "insecure",
	Usage: "Allow pulling container images from registries over plain HTTP or with untrusted/self-signed TLS certificates",
}
