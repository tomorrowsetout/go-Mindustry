package mdtserver

import "embed"

// BundledFiles includes built-in resources that are released on startup.
//
//go:embed all:assets/worlds
//go:embed all:configs
//go:embed all:data/vanilla
var BundledFiles embed.FS
