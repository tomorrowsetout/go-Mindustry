package mdtserver

import "embed"

// BundledFiles contains the startup resources released into a fresh workspace.
//
//go:embed configs/*.toml configs/*.md configs/json/*.json assets/worlds/*.msav data/vanilla/*.json
var BundledFiles embed.FS
