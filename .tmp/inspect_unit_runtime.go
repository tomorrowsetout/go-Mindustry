package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
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
	rulesRaw := m.Tags["rules"]
	fmt.Printf("tags wave=%q wavetime=%q tick=%q rules.len=%d hasSpawns=%v rules.prefix=%q\n", m.Tags["wave"], m.Tags["wavetime"], m.Tags["tick"], len(rulesRaw), strings.Contains(rulesRaw, "spawns"), prefix(rulesRaw, 4000))
	fmt.Println("model entities")
	for _, e := range m.Entities {
		fmt.Printf("id=%d type=%d name=%s team=%d pos=(%.6f,%.6f) rot=%.6f vel=(%.6f,%.6f) hp=%.3f max=%.3f shield=%.6f command=%d behavior=%q runtime=%v\n",
			e.ID, e.TypeID, m.UnitNames[e.TypeID], e.Team, e.X, e.Y, e.Rotation, e.VelX, e.VelY, e.Health, e.MaxHealth, e.Shield, e.CommandID, e.Behavior, e.RuntimeInit)
	}

	w := world.New(world.Config{TPS: 60})
	w.SetModel(m)
	if err := w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json")); err != nil {
		panic(err)
	}
	snap := w.Snapshot()
	fmt.Printf("snapshot before wave=%d wavetime=%.6f tick=%d\n", snap.Wave, snap.WaveTime, snap.Tick)
	fmt.Println("world before")
	for _, e := range w.CloneModelForWorldStream().Entities {
		fmt.Printf("id=%d type=%d name=%s team=%d pos=(%.6f,%.6f) rot=%.6f vel=(%.6f,%.6f) hp=%.3f max=%.3f shield=%.6f command=%d behavior=%q runtime=%v move=%.3f fly=%v\n",
			e.ID, e.TypeID, m.UnitNames[e.TypeID], e.Team, e.X, e.Y, e.Rotation, e.VelX, e.VelY, e.Health, e.MaxHealth, e.Shield, e.CommandID, e.Behavior, e.RuntimeInit, e.MoveSpeed, e.Flying)
	}
	w.Step(time.Second / 60)
	snap = w.Snapshot()
	fmt.Printf("snapshot after wave=%d wavetime=%.6f tick=%d\n", snap.Wave, snap.WaveTime, snap.Tick)
	fmt.Println("world after")
	for _, e := range w.CloneModelForWorldStream().Entities {
		fmt.Printf("id=%d type=%d name=%s team=%d pos=(%.6f,%.6f) rot=%.6f vel=(%.6f,%.6f) hp=%.3f max=%.3f shield=%.6f command=%d behavior=%q runtime=%v move=%.3f fly=%v\n",
			e.ID, e.TypeID, m.UnitNames[e.TypeID], e.Team, e.X, e.Y, e.Rotation, e.VelX, e.VelY, e.Health, e.MaxHealth, e.Shield, e.CommandID, e.Behavior, e.RuntimeInit, e.MoveSpeed, e.Flying)
	}
}

func prefix(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
