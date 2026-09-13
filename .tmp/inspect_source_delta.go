package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

func tileSummary(m *world.WorldModel, x, y int) string {
	t, _ := m.TileAt(x, y)
	if t == nil || t.Build == nil {
		return "nil"
	}
	return fmt.Sprintf("%s rot=%d center=(%d,%d) items=%v", m.BlockNames[int16(t.Block)], t.Rotation, t.Build.X, t.Build.Y, t.Build.Items)
}

func run(delta time.Duration) {
	reg := protocol.NewContentRegistry()
	ids, _ := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	vanilla.ApplyContentIDs(reg, ids)
	m, _ := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	w := world.New(world.Config{TPS: 60})
	w.SetModel(m)
	_ = w.LoadVanillaProfiles(filepath.Join("data", "vanilla", "profiles.json"))

	width := w.Model().Width
	srcPos := int32(492*width + 318)
	projPos := int32(490*width + 317)
	corePos := int32(494*width + 317)
	w.Step(delta)

	v := reflect.ValueOf(w).Elem()
	cfg := v.FieldByName("itemSourceCfg")
	if delta == time.Millisecond {
		fmt.Print("itemSourceCfg phase positions:")
		iter := cfg.MapRange()
		for iter.Next() {
			pos := int(iter.Key().Int())
			item := iter.Value().Int()
			if item == 11 {
				fmt.Printf(" (%d,%d)", pos%width, pos/width)
			}
		}
		fmt.Println()
	}
	accum := v.FieldByName("itemSourceAccum")
	boosts := v.FieldByName("buildingBoostStates")
	dumpIdx := v.FieldByName("blockDumpIndex")
	direct := w.Model()
	clone := w.CloneModelForWorldStream()
	src, _ := clone.TileAt(318, 492)
	proj, _ := clone.TileAt(317, 490)
	core, _ := clone.TileAt(317, 494)
	fmt.Printf("delta=%s clone srcItems=%v projItems=%v coreItems=%v", delta, src.Build.Items, proj.Build.Items, core.Build.Items)
	fmt.Printf(" direct{%s | %s | %s}", tileSummary(direct, 318, 492), tileSummary(direct, 317, 490), tileSummary(direct, 317, 494))
	if a := accum.MapIndex(reflect.ValueOf(srcPos)); a.IsValid() {
		fmt.Printf(" accum=%.3f", float32(a.Float()))
	}
	if d := dumpIdx.MapIndex(reflect.ValueOf(srcPos)); d.IsValid() {
		fmt.Printf(" dumpIdx=%d", d.Int())
	}
	for _, p := range []int32{srcPos, projPos, corePos} {
		if b := boosts.MapIndex(reflect.ValueOf(p)); b.IsValid() {
			fmt.Printf(" boost[%d]={%.3f,%.3f}", p, float32(b.FieldByName("TimeScale").Float()), float32(b.FieldByName("Duration").Float()))
		}
	}
	fmt.Println()
}

func main() {
	run(time.Millisecond)
	run(10 * time.Millisecond)
	run(16 * time.Millisecond)
	run(time.Second / 60)
}
