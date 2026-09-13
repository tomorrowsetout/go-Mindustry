package main

import (
	"fmt"
	"path/filepath"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

func main() {
	reg := protocol.NewContentRegistry()
	ids, _ := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	vanilla.ApplyContentIDs(reg, ids)
	m, _ := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	w := world.New(world.Config{TPS: 60})
	w.SetModel(m)
	_ = w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json"))
	printArea("initial", w.CloneModelForWorldStream())
	w.Step(time.Second / 60)
	printArea("after", w.CloneModelForWorldStream())
}

func printArea(label string, m *world.WorldModel) {
	fmt.Println(label)
	for y := 483; y <= 490; y++ {
		for x := 309; x <= 326; x++ {
			t, _ := m.TileAt(x, y)
			if t.Block == 0 {
				continue
			}
			name := m.BlockNames[int16(t.Block)]
			fmt.Printf("(%d,%d) %s rot=%d", x, y, name, t.Rotation)
			if t.Build != nil {
				fmt.Printf(" center=(%d,%d) items=%v liquids=%v current=%d/%v", t.Build.X, t.Build.Y, t.Build.Items, t.Build.Liquids, t.Build.CurrentLiquid, t.Build.CurrentLiquidSet)
			}
			fmt.Println()
		}
	}
}
