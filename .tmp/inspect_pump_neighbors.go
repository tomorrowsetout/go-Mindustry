package main

import (
	"fmt"
	"path/filepath"

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
	w := world.New(world.Config{TPS: 60})
	w.SetModel(m)
	if err := w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json")); err != nil {
		panic(err)
	}
	for _, pt := range [][2]int{{315, 486}, {316, 486}, {319, 486}, {320, 486}, {380, 484}, {381, 484}} {
		pos := int32(pt[1]*m.Width + pt[0])
		fmt.Printf("pump %d,%d pos=%d\n", pt[0], pt[1], pos)
		for _, n := range dumpProximity(m, pt[0], pt[1]) {
			t := &m.Tiles[n]
			fmt.Printf("  -> %d,%d %s center=%d,%d\n", t.X, t.Y, m.BlockNames[int16(t.Block)], t.Build.X, t.Build.Y)
		}
	}
	_ = w
}

func dumpProximity(m *world.WorldModel, x, y int) []int32 {
	offsets := [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	var out []int32
	for _, off := range offsets {
		tx, ty := x+off[0], y+off[1]
		if !m.InBounds(tx, ty) {
			continue
		}
		t := &m.Tiles[ty*m.Width+tx]
		if t.Build == nil || t.Build.Team == 0 {
			continue
		}
		center := int32(t.Build.Y*m.Width + t.Build.X)
		if center == int32(y*m.Width+x) {
			continue
		}
		dup := false
		for _, existing := range out {
			if existing == center {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, center)
		}
	}
	return out
}
