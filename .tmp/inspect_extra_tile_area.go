package main

import (
	"fmt"
	"path/filepath"
	"time"

	"mdt-server/internal/oracle"
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
	w.Step(time.Second / 60)
	printModelArea("go", w.CloneModelForWorldStream())

	official, err := oracle.ReadTrace(filepath.Join(".tmp", "official-go-one-tick", "trace-official.json"))
	if err != nil {
		panic(err)
	}
	fmt.Println("official trace")
	for _, t := range official.Ticks[1].Tiles {
		if t.X >= 438 && t.X <= 460 && t.Y >= 232 && t.Y <= 241 {
			fmt.Printf("(%d,%d) block=%d team=%d rot=%d items=%v liquids=%v powerStored=%.3f powerBalance=%.3f health=%.1f reload=%.3f\n",
				t.X, t.Y, t.BlockID, t.TeamID, t.Rotation, t.Items, t.Liquids, t.PowerStored, t.PowerBalance, t.BuildHealth, t.Reload)
		}
	}
}

func printModelArea(label string, m *world.WorldModel) {
	fmt.Println(label)
	for y := 232; y <= 241; y++ {
		for x := 438; x <= 460; x++ {
			t, _ := m.TileAt(x, y)
			if t.Block == 0 {
				continue
			}
			name := m.BlockNames[int16(t.Block)]
			fmt.Printf("(%d,%d) %s rot=%d team=%d", x, y, name, t.Rotation, t.Team)
			if t.Build != nil {
				fmt.Printf(" center=(%d,%d) hp=%.1f items=%v liquids=%v", t.Build.X, t.Build.Y, t.Build.Health, t.Build.Items, t.Build.Liquids)
			}
			fmt.Println()
		}
	}
}
