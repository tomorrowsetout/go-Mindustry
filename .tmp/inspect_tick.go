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
	ids, err := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	if err != nil {
		panic(err)
	}
	vanilla.ApplyContentIDs(reg, ids)
	model, err := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	if err != nil {
		panic(err)
	}
	w := world.New(world.Config{TPS: 60})
	w.SetModel(model)
	if err := w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json")); err != nil {
		panic(err)
	}
	points := [][2]int{{101, 230}, {102, 249}, {11, 230}, {285, 405}}
	print := func(label string) {
		clone := w.CloneModelForWorldStream()
		fmt.Println(label)
		for _, pt := range points {
			t, _ := clone.TileAt(pt[0], pt[1])
			fmt.Printf("%v block=%d name=%s team=%d rot=%d build=%v\n", pt, t.Block, clone.BlockNames[int16(t.Block)], t.Team, t.Rotation, t.Build != nil)
			if t.Build != nil {
				fmt.Printf("  build rot=%d health=%g center=(%d,%d)\n", t.Build.Rotation, t.Build.Health, t.Build.X, t.Build.Y)
			}
		}
	}
	print("initial")
	w.Step(time.Duration(16) * time.Millisecond)
	print("after")
}
