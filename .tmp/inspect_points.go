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
	model, err := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	if err != nil {
		panic(err)
	}
	w := world.New(world.Config{TPS: 60})
	w.SetModel(model)
	if err := w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json")); err != nil {
		panic(err)
	}
	pts := [][2]int{{311, 311}, {312, 311}, {311, 322}, {312, 323}, {293, 322}, {309, 486}}
	dumpModel := func(label string, clone *world.WorldModel) {
		fmt.Println(label)
		for _, pt := range pts {
			t, _ := clone.TileAt(pt[0], pt[1])
			fmt.Printf("%d,%d block=%d name=%s team=%d rot=%d build=%v", pt[0], pt[1], t.Block, clone.BlockNames[int16(t.Block)], t.Team, t.Rotation, t.Build != nil)
			if t.Build != nil {
				fmt.Printf(" buildRot=%d hp=%.3f items=%v liquids=%v center=(%d,%d) syncRev=%d tailLen=%d", t.Build.Rotation, t.Build.Health, t.Build.Items, t.Build.Liquids, t.Build.X, t.Build.Y, t.Build.MapSyncRevision, len(t.Build.MapSyncTail))
				if len(t.Build.MapSyncTail) >= 8 {
					r := protocol.NewReader(t.Build.MapSyncTail)
					reload, rerr := r.ReadFloat32()
					rotation, roterr := r.ReadFloat32()
					fmt.Printf(" tailReload=%.6f/%v tailRotation=%.6f/%v", reload, rerr, rotation, roterr)
				}
			}
			fmt.Println()
		}
	}
	dump := func(label string) {
		dumpModel(label+" direct", w.Model())
		dumpModel(label+" clone", w.CloneModelForWorldStream())
	}
	dump("initial")
	w.Step(time.Second / 60)
	dump("after")
}
