package main

import (
	"encoding/binary"
	"fmt"
	"math"
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
	if t, err := model.TileAt(101, 230); err == nil && t != nil && t.Build != nil {
		fmt.Printf("arc mapsync revision=%d dataLen=%d tailLen=%d tail=% x\n", t.Build.MapSyncRevision, len(t.Build.MapSyncData), len(t.Build.MapSyncTail), t.Build.MapSyncTail)
		if len(t.Build.MapSyncTail) >= 8 {
			reload := math.Float32frombits(binary.BigEndian.Uint32(t.Build.MapSyncTail[:4]))
			rot := math.Float32frombits(binary.BigEndian.Uint32(t.Build.MapSyncTail[4:8]))
			fmt.Printf("tail floats reload=%.3f rotation=%.3f\n", reload, rot)
		}
	}
	for _, pt := range [][2]int{{311, 311}, {311, 322}} {
		if t, err := model.TileAt(pt[0], pt[1]); err == nil && t != nil && t.Build != nil {
			fmt.Printf("%d,%d %s mapsync revision=%d dataLen=%d tailLen=%d tail=% x\n", pt[0], pt[1], model.BlockNames[int16(t.Block)], t.Build.MapSyncRevision, len(t.Build.MapSyncData), len(t.Build.MapSyncTail), t.Build.MapSyncTail)
			if len(t.Build.MapSyncTail) >= 8 {
				reload := math.Float32frombits(binary.BigEndian.Uint32(t.Build.MapSyncTail[:4]))
				rot := math.Float32frombits(binary.BigEndian.Uint32(t.Build.MapSyncTail[4:8]))
				fmt.Printf("tail floats reload=%.3f rotation=%.3f\n", reload, rot)
			}
		}
	}
	fmt.Println("entities near scatter:")
	ssx, ssy := float32(311*8+4), float32(311*8+4)
	for _, e := range model.Entities {
		dx, dy := e.X-ssx, e.Y-ssy
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist <= 200 {
			fmt.Printf("id=%d type=%d name=%s team=%d pos=(%.1f,%.1f) dist=%.1f flying=%v health=%.1f\n", e.ID, e.TypeID, model.UnitNames[e.TypeID], e.Team, e.X, e.Y, dist, e.Flying, e.Health)
		}
	}
	w := world.New(world.Config{TPS: 60})
	w.SetModel(model)
	if err := w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json")); err != nil {
		panic(err)
	}
	w.Step(time.Second / 60)
	stepped := w.Model()
	fmt.Println("entities near scatter after one Go step:")
	for _, e := range stepped.Entities {
		dx, dy := e.X-ssx, e.Y-ssy
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist <= 200 {
			fmt.Printf("id=%d type=%d name=%s team=%d pos=(%.1f,%.1f) dist=%.1f flying=%v health=%.1f\n", e.ID, e.TypeID, stepped.UnitNames[e.TypeID], e.Team, e.X, e.Y, dist, e.Flying, e.Health)
		}
	}
	sx, sy := float32(101*8+4), float32(230*8+4)
	rangeLimit := float32(90)
	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 || t.Build.Team == 0 || t.Build.Team == 1 {
			continue
		}
		tx, ty := float32(t.X*8+4), float32(t.Y*8+4)
		dx, dy := tx-sx, ty-sy
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		name := model.BlockNames[int16(t.Block)]
		hit := float32(8)
		if name == "copper-wall-large" {
			hit = 16
		}
		if dist <= rangeLimit+hit/2+24 {
			fmt.Printf("pos=(%d,%d) team=%d block=%s dist=%.2f edge=%.2f rot=%.2f\n", t.X, t.Y, t.Build.Team, name, dist, dist-hit/2, math.Atan2(float64(dy), float64(dx))*180/math.Pi)
		}
	}
}
