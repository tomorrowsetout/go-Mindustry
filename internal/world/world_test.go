package world

import (
	"testing"
	"time"
)

func TestWorldSnapshot(t *testing.T) {
	w := New(Config{TPS: 60})
	before := w.Snapshot()
	w.Step(500 * time.Millisecond)
	after := w.Snapshot()
	if after.WaveTime <= before.WaveTime {
		t.Fatalf("expected wavetime to increase, before=%v after=%v", before.WaveTime, after.WaveTime)
	}
	if after.Tps != 60 {
		t.Fatalf("expected tps=60, got=%d", after.Tps)
	}
}

func TestApplyBuildPlansIsAsync(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	ops := []BuildPlanOp{{
		Breaking: false,
		X:        2,
		Y:        3,
		Rotation: 1,
		BlockID:  45,
	}}
	w.ApplyBuildPlans(TeamID(1), ops)

	tile, err := w.Model().TileAt(2, 3)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected no immediate placement, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	w.Step(500 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected still pending build at 0.5s, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	placed := false
	for i := 0; i < 16; i++ { // up to 3.2s
		w.Step(200 * time.Millisecond)
		tile, _ = w.Model().TileAt(2, 3)
		if tile.Block == 45 && tile.Build != nil {
			placed = true
			break
		}
	}
	if !placed {
		tile, _ = w.Model().TileAt(2, 3)
		t.Fatalf("expected placed block after progress, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestDeconstructRefund(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{45: "duo"}
	w.SetModel(model)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	w.Step(3 * time.Second)
	mid := w.TeamItems(TeamID(1))[0]
	if mid >= 3000 {
		t.Fatalf("expected build to consume copper from starter inventory, mid=%d", mid)
	}

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		Breaking: true, X: 2, Y: 2,
	}})
	w.Step(3 * time.Second)
	after := w.TeamItems(TeamID(1))[0]
	if after <= mid {
		t.Fatalf("expected deconstruct refund, mid=%d after=%d", mid, after)
	}
}

func TestFactoryProductionSpawnsUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{100: "ground-factory"}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)

	ops := []BuildPlanOp{{
		X: 3, Y: 3, BlockID: 100, Rotation: 0,
	}}
	w.ApplyBuildPlans(TeamID(1), ops)
	w.Step(3 * time.Second)
	if len(w.Model().Entities) != 0 {
		t.Fatalf("expected no unit before factory cycle, got=%d", len(w.Model().Entities))
	}
	w.Step(11 * time.Second)
	if len(w.Model().Entities) == 0 {
		t.Fatalf("expected produced unit, got=%d", len(w.Model().Entities))
	}
}

func TestApplyBuildPlanSnapshotDoesNotRemoveMissingOps(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{
		{X: 1, Y: 1, BlockID: 45},
		{X: 2, Y: 2, BlockID: 46},
	})
	if got := len(w.pendingBuilds); got != 2 {
		t.Fatalf("expected 2 pending builds before snapshot, got=%d", got)
	}

	changed := w.ApplyBuildPlanSnapshot(TeamID(1), []BuildPlanOp{
		{X: 2, Y: 2, BlockID: 46},
	})
	_ = changed
	if got := len(w.pendingBuilds); got != 2 {
		t.Fatalf("expected snapshot not to remove missing plans, got=%d", got)
	}
}

func TestUnitBuildSpeedByNameOfficialValues(t *testing.T) {
	cases := map[string]float32{
		"alpha":   0.5,
		"beta":    0.75,
		"gamma":   1.0,
		"evoke":   1.2,
		"incite":  1.4,
		"emanate": 1.5,
		"oct":     4.0,
		"navanax": 3.5,
		"vela":    3.0,
	}
	for name, want := range cases {
		if got := unitBuildSpeedByName(name); got != want {
			t.Fatalf("unit %s buildSpeed mismatch: got=%v want=%v", name, got, want)
		}
	}
}

func TestFallbackContentNamesAppliedToModel(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetFallbackContentNames(map[int16]string{
		257: "surge-router",
	}, map[int16]string{
		1: "alpha",
	})
	w.SetModel(NewWorldModel(4, 4))

	if got := w.BlockNameByID(257); got != "surge-router" {
		t.Fatalf("expected fallback block name surge-router, got=%q", got)
	}
	if got := w.UnitNameByTypeID(1); got != "alpha" {
		t.Fatalf("expected fallback unit name alpha, got=%q", got)
	}
}

func TestAdjustTeamItem(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(4, 4))

	if ok := w.AdjustTeamItem(TeamID(1), ItemID(0), 120); !ok {
		t.Fatalf("expected add item success")
	}
	got := w.TeamItems(TeamID(1))[ItemID(0)]
	if got != 3120 {
		t.Fatalf("expected copper=3120 after add, got=%d", got)
	}
	if ok := w.AdjustTeamItem(TeamID(1), ItemID(0), -4000); ok {
		t.Fatalf("expected subtract too much to fail")
	}
	got = w.TeamItems(TeamID(1))[ItemID(0)]
	if got != 3120 {
		t.Fatalf("expected copper unchanged after failed subtract, got=%d", got)
	}
}

func TestTeamCorePositionsFallbackToBuildTeam(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	tile, err := model.TileAt(1, 1)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Block = BlockID(339)
	tile.Team = 0
	tile.Build = &Building{
		Block: BlockID(339),
		Team:  TeamID(1),
		X:     1,
		Y:     1,
	}
	w.SetModel(model)

	cores := w.TeamCorePositions(TeamID(1))
	if len(cores) != 1 || cores[0] != packTilePos(1, 1) {
		t.Fatalf("expected one core position at (1,1), got=%v", cores)
	}
	_, team, isCore, ok := w.TileInfoByPackedPos(packTilePos(1, 1))
	if !ok || !isCore {
		t.Fatalf("expected tile info to identify core tile")
	}
	if team != TeamID(1) {
		t.Fatalf("expected tile core team=1, got=%d", team)
	}
}

func TestTeamCorePositionsFallbackByKnownID(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	tileA, err := model.TileAt(1, 1)
	if err != nil || tileA == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tileA.Block = BlockID(316) // official content-id core-shard
	tileA.Team = TeamID(1)
	tileB, err := model.TileAt(2, 2)
	if err != nil || tileB == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tileB.Block = BlockID(339) // world-content core-shard id
	tileB.Team = TeamID(1)
	tileC, err := model.TileAt(3, 3)
	if err != nil || tileC == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tileC.Block = BlockID(344) // world-content core-acropolis id
	tileC.Team = TeamID(1)
	w.SetModel(model)

	cores := w.TeamCorePositions(TeamID(1))
	got := map[int32]struct{}{}
	for _, pos := range cores {
		got[pos] = struct{}{}
	}
	for _, want := range []int32{packTilePos(1, 1), packTilePos(2, 2), packTilePos(3, 3)} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected core fallback by known IDs, missing pos=%v, got=%v", want, cores)
		}
	}
}

func TestEnsureTeamItemsInitializesInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(4, 4))
	items := w.EnsureTeamItems(TeamID(1))
	if len(items) == 0 {
		t.Fatalf("expected initialized team inventory")
	}
	if items[0] <= 0 {
		t.Fatalf("expected copper seed > 0, got=%d", items[0])
	}
}
