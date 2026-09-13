package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/worldstream"
)

func main() {
	reg := protocol.NewContentRegistry()
	ids, err := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	if err != nil {
		panic(err)
	}
	vanilla.ApplyContentIDs(reg, ids)
	m, err := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	if err != nil {
		panic(err)
	}
	for _, tile := range m.Tiles {
		name := strings.ToLower(strings.TrimSpace(m.BlockNames[int16(tile.Overlay)]))
		if tile.Overlay == 1 || name == "spawn" {
			fmt.Printf("spawn tile=(%d,%d) world=(%.1f,%.1f) overlay=%d name=%q pos=%d\n",
				tile.X, tile.Y, float32(tile.X*8+4), float32(tile.Y*8+4), tile.Overlay, name, tile.Y*m.Width+tile.X)
		}
	}
}
