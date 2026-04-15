package ui

import "embed"

// Assets holds the compiled React frontend.
// During development, this will be empty until `npm run build` is run in ui/.
//
//go:embed all:dist
var Assets embed.FS
