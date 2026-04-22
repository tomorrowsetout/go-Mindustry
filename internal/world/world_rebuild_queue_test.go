package world

import "testing"

func TestDestroyedBuildingQueuesRebuildPlanWhenGhostBlocksEnabled(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	rules.GhostBlocks = true

	placeTestBuilding(t, w, 5, 5, 45, 1, 2)

	if !w.DamageBuildingPacked(packTilePos(5, 5), 2000) {
		t.Fatalf("expected destroyed building to queue rebuild plan")
	}

	tile, err := w.Model().TileAt(5, 5)
	if err != nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected destroyed tile to be empty, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	plans := w.teamRebuildPlans[1]
	if len(plans) != 1 {
		t.Fatalf("expected exactly one queued rebuild plan, got %d", len(plans))
	}
	if plans[0].X != 5 || plans[0].Y != 5 || plans[0].Rotation != 2 || plans[0].BlockID != 45 {
		t.Fatalf("unexpected queued rebuild plan: %+v", plans[0])
	}

	op, ok := w.AcquireNextRebuildPlan(1)
	if !ok {
		t.Fatalf("expected queued rebuild plan to be acquirable")
	}
	if op.X != 5 || op.Y != 5 || op.Rotation != 2 || op.BlockID != 45 {
		t.Fatalf("unexpected acquired rebuild plan: %+v", op)
	}
	if len(w.teamRebuildPlans[1]) != 1 {
		t.Fatalf("expected acquired rebuild plan to remain queued for reuse, got %d", len(w.teamRebuildPlans[1]))
	}
}

func TestInstantDeconstructDoesNotQueueRebuildPlan(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	rules.GhostBlocks = true
	rules.InstantBuild = true

	placeTestBuilding(t, w, 5, 5, 45, 1, 0)

	w.ApplyBuildPlans(1, []BuildPlanOp{{
		Breaking: true,
		X:        5,
		Y:        5,
	}})

	tile, err := w.Model().TileAt(5, 5)
	if err != nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected instant deconstruct to remove building, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if _, ok := w.teamRebuildPlans[1]; ok {
		t.Fatalf("expected instant deconstruct to avoid broken-block rebuild queue, got %+v", w.teamRebuildPlans[1])
	}
}

func TestPlacedBuildingClearsOverlappingRebuildPlans(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		45:  "duo",
		100: "ground-factory",
	}
	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	rules.GhostBlocks = true
	rules.InstantBuild = true

	placeTestBuilding(t, w, 6, 6, 100, 1, 0)

	if !w.DamageBuildingPacked(packTilePos(6, 6), 2000) {
		t.Fatalf("expected destroyed factory to queue rebuild plan")
	}
	if len(w.teamRebuildPlans[1]) != 1 {
		t.Fatalf("expected queued factory rebuild plan, got %d", len(w.teamRebuildPlans[1]))
	}

	w.ApplyBuildPlans(1, []BuildPlanOp{{
		X:       5,
		Y:       5,
		BlockID: 45,
	}})

	tile, err := w.Model().TileAt(5, 5)
	if err != nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 45 || tile.Build == nil || tile.Team != 1 {
		t.Fatalf("expected overlapping placement to succeed, got block=%d build=%v team=%d", tile.Block, tile.Build != nil, tile.Team)
	}
	if _, ok := w.teamRebuildPlans[1]; ok {
		t.Fatalf("expected overlapping placement to clear broken-block queue, got %+v", w.teamRebuildPlans[1])
	}
}

func TestAcquireNextRebuildPlanInRangeScansForFirstValidPlanAndRotatesItToTail(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	w.SetModel(model)

	w.teamRebuildPlans[1] = []rebuildBlockPlan{
		{X: 20, Y: 20, BlockID: 45},
		{X: 5, Y: 5, BlockID: 45},
		{X: 8, Y: 8, BlockID: 45},
	}

	op, ok := w.AcquireNextRebuildPlanInRange(1, 5*8+4, 5*8+4, 40)
	if !ok {
		t.Fatal("expected in-range rebuild plan to be acquirable")
	}
	if op.X != 5 || op.Y != 5 || op.BlockID != 45 {
		t.Fatalf("expected first valid in-range rebuild plan at (5,5), got %+v", op)
	}

	plans := w.teamRebuildPlans[1]
	if len(plans) != 3 {
		t.Fatalf("expected rebuild queue size to stay unchanged, got %d", len(plans))
	}
	if plans[2].X != 5 || plans[2].Y != 5 {
		t.Fatalf("expected selected in-range rebuild plan to rotate to tail, queue=%+v", plans)
	}
}
