package main

import (
	"fmt"
	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
	"path/filepath"
	"time"
)

func main() {
	reg := protocol.NewContentRegistry()
	ids, _ := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	vanilla.ApplyContentIDs(reg, ids)
	m, _ := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	w := world.New(world.Config{TPS: 60})
	w.SetModel(m)
	_ = w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json"))
	w.Step(time.Second / 60)
	clone := w.CloneModelForWorldStream()
	for y := 482; y <= 497; y++ {
		for x := 374; x <= 383; x++ {
			t, _ := clone.TileAt(x, y)
			name := clone.BlockNames[int16(t.Block)]
			team := t.Team
			build := false
			bx, by := 0, 0
			if t.Build != nil {
				build = true
				bx = t.Build.X
				by = t.Build.Y
				team = t.Build.Team
			}
			if t.Block != 0 {
				fmt.Printf("(%d,%d) %s team=%d rot=%d build=%v center=(%d,%d)", x, y, name, team, t.Rotation, build, bx, by)
				if t.Build != nil {
					fmt.Printf(" liquids=%v items=%v", t.Build.Liquids, t.Build.Items)
				}
				fmt.Println()
			}
		}
	}
}
