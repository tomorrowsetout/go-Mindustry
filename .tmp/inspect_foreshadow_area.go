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
	for y := 488; y <= 496; y++ {
		for x := 485; x <= 496; x++ {
			t, _ := m.TileAt(x, y)
			if t.Block == 0 {
				continue
			}
			name := m.BlockNames[int16(t.Block)]
			fmt.Printf("(%d,%d) %s rot=%d", x, y, name, t.Rotation)
			if t.Build != nil {
				reload, rotation, ok := decodeTail(t)
				fmt.Printf(" center=(%d,%d) hp=%.1f items=%v liquids=%v current=%d/%v tail=%v reload=%.3f tailRot=%.3f",
					t.Build.X, t.Build.Y, t.Build.Health, t.Build.Items, t.Build.Liquids, t.Build.CurrentLiquid, t.Build.CurrentLiquidSet, ok, reload, rotation)
			}
			fmt.Println()
		}
	}
}

func decodeTail(t *world.Tile) (float32, float32, bool) {
	if t == nil || t.Build == nil || t.Build.MapSyncRevision < 1 || len(t.Build.MapSyncTail) < 8 {
		return 0, 0, false
	}
	r := protocol.NewReader(t.Build.MapSyncTail)
	reload, err := r.ReadFloat32()
	if err != nil {
		return 0, 0, false
	}
	rotation, err := r.ReadFloat32()
	if err != nil {
		return 0, 0, false
	}
	return reload, rotation, true
}
