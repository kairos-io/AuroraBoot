package cmd

import "github.com/urfave/cli/v2"

// AllowInsecureRegistriesFlag is shared by every command that pulls container
// images, so the flag is defined once and behaves consistently. urfave/cli v2
// has no persistent-flag mechanism; defining it as a global app flag would force
// it to be passed before the subcommand (e.g.
// "auroraboot --allow-insecure-registries build-iso ..."), breaking the natural
// "build-iso --allow-insecure-registries ..." form, so we share a single flag
// value across the commands that need it instead.
//
// The legacy "insecure" name is kept as a deprecated alias so scripts written
// against v0.22.0 keep working.
var AllowInsecureRegistriesFlag = &cli.BoolFlag{
	Name:    "allow-insecure-registries",
	Aliases: []string{"insecure"},
	Usage:   "Allow pulling container images from registries over plain HTTP or with untrusted/self-signed TLS certificates. The --insecure alias is deprecated; use --allow-insecure-registries instead",
}
