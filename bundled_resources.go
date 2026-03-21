package mdtserver

import "embed"

// BundledFiles includes built-in worlds that are released on startup.
//
//go:embed all:assets/worlds
//go:embed all:configs
var BundledFiles embed.FS
