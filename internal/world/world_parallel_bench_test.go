package world

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/sim"
)

func placeBenchmarkBuildingRaw(tb testing.TB, w *World, x, y int, block int16, team TeamID, rotation int8) *Tile {
	tb.Helper()
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil {
		tb.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = BlockID(block)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &Building{
		Block:     BlockID(block),
		Team:      team,
		Rotation:  rotation,
		X:         x,
		Y:         y,
		Health:    1000,
		MaxHealth: 1000,
	}
	return tile
}

func placeBenchmarkBuilding(tb testing.TB, w *World, x, y int, block int16, team TeamID, rotation int8) *Tile {
	tb.Helper()
	tile := placeBenchmarkBuildingRaw(tb, w, x, y, block, team, rotation)
	w.rebuildBlockOccupancyLocked()
	return tile
}

func paintBenchmarkAreaFloor(tb testing.TB, w *World, cx, cy, size int, floor int16) {
	tb.Helper()
	low, high := blockFootprintRange(size)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			tile, err := w.Model().TileAt(cx+dx, cy+dy)
			if err != nil || tile == nil {
				tb.Fatalf("floor tile lookup failed at (%d,%d): %v", cx+dx, cy+dy, err)
			}
			tile.Floor = FloorID(floor)
		}
	}
}

func newMixedParallelBenchmarkWorld(tb testing.TB, enableScheduler bool) (*World, func()) {
	tb.Helper()
	w := New(Config{TPS: 60})
	model := NewWorldModel(96, 96)
	model.BlockNames = map[int16]string{
		1:   "water",
		2:   "sand-floor",
		3:   "ore-copper",
		257: "conveyor",
		418: "router",
		429: "mechanical-drill",
		440: "mechanical-pump",
		412: "item-source",
		500: "container",
	}
	w.SetModel(model)

	cleanup := func() {}
	if enableScheduler {
		cores := runtime.NumCPU()
		if cores < 2 {
			cores = 2
		}
		workers := cores - 1
		if workers < 1 {
			workers = 1
		}
		engine := sim.NewEngine(sim.Config{
			TPS:        60,
			Cores:      cores,
			Partitions: workers,
		})
		w.SetScheduler(engine)
		cleanup = engine.Stop
	}

	for row := 4; row < 92; row += 6 {
		source := placeBenchmarkBuilding(tb, w, 2, row, 412, 1, 0)
		sourcePos := int32(source.Y*model.Width + source.X)
		w.ConfigureItemSource(sourcePos, copperItemID)
		for x := 3; x <= 13; x++ {
			placeBenchmarkBuilding(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuilding(tb, w, 14, row, 418, 1, 0)
		for x := 15; x <= 23; x++ {
			placeBenchmarkBuilding(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuilding(tb, w, 24, row, 500, 1, 0)
	}

	for row := 6; row < 92; row += 6 {
		paintBenchmarkAreaFloor(tb, w, 38, row, 2, 2)
		drill := placeBenchmarkBuilding(tb, w, 38, row, 429, 1, 0)
		oreTile, err := w.Model().TileAt(drill.X, drill.Y)
		if err != nil || oreTile == nil {
			tb.Fatalf("drill ore tile lookup failed at (%d,%d): %v", drill.X, drill.Y, err)
		}
		oreTile.Overlay = 3
		placeBenchmarkBuilding(tb, w, 41, row, 500, 1, 0)

		paintBenchmarkAreaFloor(tb, w, 56, row, 1, 1)
		placeBenchmarkBuilding(tb, w, 56, row, 440, 1, 0)
	}

	return w, cleanup
}

func newSteadyStateBenchmarkWorld(tb testing.TB, enableScheduler bool) (*World, func()) {
	tb.Helper()
	w := New(Config{TPS: 60})
	model := NewWorldModel(128, 128)
	model.BlockNames = map[int16]string{
		1:   "water",
		2:   "sand-floor",
		3:   "ore-copper",
		257: "conveyor",
		258: "router",
		259: "item-source",
		260: "item-void",
		261: "bridge-conveyor",
		262: "container",
		263: "unloader",
		264: "mechanical-drill",
		265: "mechanical-pump",
		266: "conduit",
		267: "liquid-router",
		268: "liquid-void",
	}
	w.SetModel(model)

	cleanup := func() {}
	if enableScheduler {
		cores := runtime.NumCPU()
		if cores < 2 {
			cores = 2
		}
		workers := cores - 1
		if workers < 1 {
			workers = 1
		}
		engine := sim.NewEngine(sim.Config{
			TPS:        60,
			Cores:      cores,
			Partitions: workers,
		})
		w.SetScheduler(engine)
		cleanup = engine.Stop
	}

	var itemSources []int32
	var bridgeLinks []struct {
		pos int32
		cfg protocol.Point2
	}

	for row := 6; row < 120; row += 6 {
		source := placeBenchmarkBuildingRaw(tb, w, 2, row, 259, 1, 0)
		itemSources = append(itemSources, int32(source.Y*model.Width+source.X))
		for x := 3; x <= 9; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuildingRaw(tb, w, 10, row, 258, 1, 0)
		for x := 11; x <= 17; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuildingRaw(tb, w, 18, row, 260, 1, 0)

		source = placeBenchmarkBuildingRaw(tb, w, 24, row, 259, 1, 0)
		itemSources = append(itemSources, int32(source.Y*model.Width+source.X))
		for x := 25; x <= 28; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 257, 1, 0)
		}
		bridgeA := placeBenchmarkBuildingRaw(tb, w, 29, row, 261, 1, 0)
		bridgeB := placeBenchmarkBuildingRaw(tb, w, 34, row, 261, 1, 0)
		for x := 35; x <= 40; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuildingRaw(tb, w, 41, row, 260, 1, 0)
		bridgeLinks = append(bridgeLinks,
			struct {
				pos int32
				cfg protocol.Point2
			}{pos: int32(bridgeA.Y*model.Width + bridgeA.X), cfg: protocol.Point2{X: int32(bridgeB.X - bridgeA.X), Y: 0}},
		)

		source = placeBenchmarkBuildingRaw(tb, w, 48, row, 259, 1, 0)
		itemSources = append(itemSources, int32(source.Y*model.Width+source.X))
		placeBenchmarkBuildingRaw(tb, w, 49, row, 257, 1, 0)
		placeBenchmarkBuildingRaw(tb, w, 50, row, 262, 1, 0)
		placeBenchmarkBuildingRaw(tb, w, 51, row, 263, 1, 0)
		placeBenchmarkBuildingRaw(tb, w, 52, row, 260, 1, 0)

		paintBenchmarkAreaFloor(tb, w, 70, row, 2, 2)
		drill := placeBenchmarkBuildingRaw(tb, w, 70, row, 264, 1, 0)
		oreTile, err := w.Model().TileAt(drill.X, drill.Y)
		if err != nil || oreTile == nil {
			tb.Fatalf("drill ore tile lookup failed at (%d,%d): %v", drill.X, drill.Y, err)
		}
		oreTile.Overlay = 3
		for x := 73; x <= 77; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 257, 1, 0)
		}
		placeBenchmarkBuildingRaw(tb, w, 78, row, 260, 1, 0)

		paintBenchmarkAreaFloor(tb, w, 96, row, 1, 1)
		placeBenchmarkBuildingRaw(tb, w, 96, row, 265, 1, 0)
		for x := 97; x <= 101; x++ {
			placeBenchmarkBuildingRaw(tb, w, x, row, 266, 1, 0)
		}
		placeBenchmarkBuildingRaw(tb, w, 102, row, 267, 1, 0)
		placeBenchmarkBuildingRaw(tb, w, 103, row, 268, 1, 0)
	}

	w.rebuildBlockOccupancyLocked()
	for _, pos := range itemSources {
		w.ConfigureItemSource(pos, copperItemID)
	}
	for _, link := range bridgeLinks {
		w.ConfigureBuilding(link.pos, link.cfg)
	}

	return w, cleanup
}

func newGroundWaypointBenchmarkWorld(tb testing.TB) (*World, float32, float32, float32, []protocol.Point2) {
	tb.Helper()
	w := New(Config{TPS: 60})
	model := NewWorldModel(600, 600)
	model.BlockNames = map[int16]string{
		1:   "stone-wall",
		339: "core-shard",
	}
	w.SetModel(model)

	for wallX := 40; wallX < 560; wallX += 40 {
		gapY := 30 + ((wallX / 40) % 9 * 55)
		for y := 1; y < 599; y++ {
			if y >= gapY-4 && y <= gapY+4 {
				continue
			}
			tile, err := w.Model().TileAt(wallX, y)
			if err != nil || tile == nil {
				tb.Fatalf("wall tile lookup failed at (%d,%d): %v", wallX, y, err)
			}
			tile.Block = 1
		}
	}

	core := placeBenchmarkBuilding(tb, w, 570, 300, 339, 2, 0)
	targetX := float32(core.X*8 + 4)
	targetY := float32(core.Y*8 + 4)
	targetRadius := float32(16)
	sources := []protocol.Point2{
		{X: 20, Y: 120},
		{X: 20, Y: 220},
		{X: 20, Y: 320},
		{X: 20, Y: 420},
	}
	return w, targetX, targetY, targetRadius, sources
}

func TestParallelMixedWorldMatchesSerialSnapshots(t *testing.T) {
	serial, cleanupSerial := newMixedParallelBenchmarkWorld(t, false)
	defer cleanupSerial()
	parallel, cleanupParallel := newMixedParallelBenchmarkWorld(t, true)
	defer cleanupParallel()

	for i := 0; i < 180; i++ {
		serial.Step(time.Second / 60)
		parallel.Step(time.Second / 60)
	}

	serialBlocks := serial.BlockSyncSnapshots()
	parallelBlocks := parallel.BlockSyncSnapshots()
	if !reflect.DeepEqual(serialBlocks, parallelBlocks) {
		t.Fatalf("expected parallel block snapshots to match serial snapshots")
	}

	serialBuilds := serial.BuildSyncSnapshotWithConfig()
	parallelBuilds := parallel.BuildSyncSnapshotWithConfig()
	if !reflect.DeepEqual(serialBuilds, parallelBuilds) {
		t.Fatalf("expected parallel build sync snapshot to match serial snapshot")
	}
}

func BenchmarkWorldStepMixedHeavy(b *testing.B) {
	run := func(b *testing.B, enableScheduler bool) {
		var (
			w       *World
			cleanup func()
		)
		reset := func() {
			if cleanup != nil {
				cleanup()
			}
			w, cleanup = newMixedParallelBenchmarkWorld(b, enableScheduler)
		}
		reset()
		defer cleanup()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i > 0 && i%240 == 0 {
				b.StopTimer()
				reset()
				b.StartTimer()
			}
			w.Step(time.Second / 60)
		}
	}

	b.Run("serial", func(b *testing.B) {
		b.ReportAllocs()
		run(b, false)
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		run(b, true)
	})
}

func BenchmarkWorldStepMixedSteadyState(b *testing.B) {
	run := func(b *testing.B, enableScheduler bool) {
		w, cleanup := newSteadyStateBenchmarkWorld(b, enableScheduler)
		defer cleanup()

		for i := 0; i < 240; i++ {
			w.Step(time.Second / 60)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w.Step(time.Second / 60)
		}
	}

	b.Run("serial", func(b *testing.B) {
		b.ReportAllocs()
		run(b, false)
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		run(b, true)
	})
}

func BenchmarkGroundWaypointMaze(b *testing.B) {
	w, targetX, targetY, targetRadius, sources := newGroundWaypointBenchmarkWorld(b)
	warm := sources[0]
	if _, _, ok := w.findGroundWaypointLocked(float32(warm.X*8+4), float32(warm.Y*8+4), targetX, targetY, targetRadius); !ok {
		b.Fatal("expected warm path through maze")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := sources[i%len(sources)]
		if _, _, ok := w.findGroundWaypointLocked(float32(src.X*8+4), float32(src.Y*8+4), targetX, targetY, targetRadius); !ok {
			b.Fatal("expected path through maze")
		}
	}
}
