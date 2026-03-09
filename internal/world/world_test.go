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
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected immediate preview placement, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health <= 0 || tile.Build.Health >= 1000 {
		t.Fatalf("expected low initial build health, got=%.2f", tile.Build.Health)
	}

	w.Step(300 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected pending build at 0.3s, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health <= 1 || tile.Build.Health >= 1000 {
		t.Fatalf("expected intermediate build health at 0.3s, got=%.2f", tile.Build.Health)
	}

	w.Step(600 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected placed block after progress, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health < 999 {
		t.Fatalf("expected completed build health, got=%.2f", tile.Build.Health)
	}
}

func TestRotateBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		Breaking: false,
		X:        3,
		Y:        4,
		Rotation: 0,
		BlockID:  45,
	}})
	w.Step(2 * time.Second)

	pos := int32((3 << 16) | 4)
	blockID, rot, team, ok := w.RotateBuilding(pos, true)
	if !ok {
		t.Fatalf("expected rotate success")
	}
	if blockID != 45 || team != TeamID(1) || rot != 1 {
		t.Fatalf("unexpected rotate result: block=%d team=%d rot=%d", blockID, team, rot)
	}

	_, rot, _, ok = w.RotateBuilding(pos, false)
	if !ok {
		t.Fatalf("expected rotate success on reverse")
	}
	if rot != 0 {
		t.Fatalf("expected rotation to return to 0, got=%d", rot)
	}
}

func TestRemovePendingBuild(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		Breaking: false,
		X:        2,
		Y:        2,
		Rotation: 0,
		BlockID:  45,
	}})
	if w.PendingBuildCount() != 1 {
		t.Fatalf("expected 1 pending build, got=%d", w.PendingBuildCount())
	}
	if !w.RemovePendingBuild(2, 2, false) {
		t.Fatalf("expected remove pending build success")
	}
	if w.PendingBuildCount() != 0 {
		t.Fatalf("expected 0 pending build after remove, got=%d", w.PendingBuildCount())
	}
}

func TestSnapshotBuildingInventoriesIncludesEmptyAndNonEmpty(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	w.SetModel(m)

	ta, _ := m.TileAt(1, 1)
	ta.Block = 45
	ta.Build = &Building{
		Block:     45,
		Team:      TeamID(1),
		X:         1,
		Y:         1,
		Items:     []ItemStack{{Item: ItemID(2), Amount: 30}},
		Health:    100,
		MaxHealth: 100,
	}

	tb, _ := m.TileAt(2, 1)
	tb.Block = 46
	tb.Build = &Building{
		Block:     46,
		Team:      TeamID(1),
		X:         2,
		Y:         1,
		Items:     nil,
		Health:    100,
		MaxHealth: 100,
	}

	inv := w.SnapshotBuildingInventories()
	posA := int32((1 << 16) | 1)
	posB := int32((2 << 16) | 1)

	itemsA, okA := inv[posA]
	if !okA || len(itemsA) != 1 || itemsA[0].Amount != 30 || itemsA[0].Item != ItemID(2) {
		t.Fatalf("unexpected inventory for posA: %#v", itemsA)
	}
	itemsB, okB := inv[posB]
	if !okB {
		t.Fatalf("expected empty inventory entry for posB")
	}
	if len(itemsB) != 0 {
		t.Fatalf("expected empty inventory for posB, got %#v", itemsB)
	}
}

func TestBuildingTeam(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	w.SetModel(m)

	tt, _ := m.TileAt(1, 2)
	tt.Block = 45
	tt.Team = TeamID(2)
	tt.Build = &Building{Block: 45, Team: TeamID(2), X: 1, Y: 2}

	pos := int32((1 << 16) | 2)
	team, ok := w.BuildingTeam(pos)
	if !ok {
		t.Fatalf("expected BuildingTeam ok=true")
	}
	if team != TeamID(2) {
		t.Fatalf("expected team=2, got=%d", team)
	}
}

func TestCanDepositToBuilding_OnlyDepositCore(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		45: "router",
		78: "core-shard",
	}
	w.SetModel(m)

	ta, _ := m.TileAt(1, 1)
	ta.Block = 45
	ta.Team = TeamID(1)
	ta.Build = &Building{Block: 45, Team: TeamID(1), X: 1, Y: 1}

	tb, _ := m.TileAt(2, 1)
	tb.Block = 78
	tb.Team = TeamID(1)
	tb.Build = &Building{Block: 78, Team: TeamID(1), X: 2, Y: 1}

	posA := int32((1 << 16) | 1)
	posB := int32((2 << 16) | 1)

	if !w.CanDepositToBuilding(posA) || !w.CanDepositToBuilding(posB) {
		t.Fatalf("expected deposit allowed by default rules")
	}

	r := w.GetRulesManager().Get()
	r.OnlyDepositCore = true
	w.GetRulesManager().Set(r)

	if w.CanDepositToBuilding(posA) {
		t.Fatalf("expected non-core deposit blocked when onlyDepositCore=true")
	}
	if !w.CanDepositToBuilding(posB) {
		t.Fatalf("expected core deposit allowed when onlyDepositCore=true")
	}
}

func TestAcceptBuildingItem_CapacityClamp(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		78: "core-shard",
	}
	w.SetModel(m)

	tile, _ := m.TileAt(1, 1)
	tile.Block = 78
	tile.Team = TeamID(1)
	tile.Build = &Building{
		Block: 78, Team: TeamID(1), X: 1, Y: 1,
		Items: []ItemStack{{Item: ItemID(1), Amount: 3990}},
	}
	pos := int32((1 << 16) | 1)

	if got := w.AcceptBuildingItem(pos, 1, 50); got != 10 {
		t.Fatalf("expected accepted=10, got=%d", got)
	}
	inv := w.SnapshotBuildingInventories()
	items := inv[pos]
	if len(items) != 1 || items[0].Amount != 4000 {
		t.Fatalf("expected total=4000 after clamp, got=%#v", items)
	}
	if got := w.AcceptBuildingItem(pos, 1, 1); got != 0 {
		t.Fatalf("expected no space left, accepted=%d", got)
	}
}

func TestStepLogisticsUnloaderMovesItems(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		10: "unloader",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	unl, _ := m.TileAt(1, 1)
	unl.Block = 10
	unl.Team = TeamID(1)
	unl.Build = &Building{Block: 10, Team: TeamID(1), X: 1, Y: 1}
	dst, _ := m.TileAt(2, 1)
	dst.Block = 11
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 1}

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	posSrc := int32((0 << 16) | 1)
	posDst := int32((2 << 16) | 1)
	if got := inv[posSrc][0].Amount; got != 4 {
		t.Fatalf("expected source amount=4, got=%d", got)
	}
	if got := inv[posDst][0].Amount; got != 1 {
		t.Fatalf("expected dest amount=1, got=%d", got)
	}
}

func TestStepLogisticsUnloaderFilterByTileConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		10: "unloader",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}, {Item: ItemID(3), Amount: 5}},
	}
	unl, _ := m.TileAt(1, 1)
	unl.Block = 10
	unl.Team = TeamID(1)
	unl.Build = &Building{Block: 10, Team: TeamID(1), X: 1, Y: 1}
	dst, _ := m.TileAt(2, 1)
	dst.Block = 11
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 1}

	unlPos := int32((1 << 16) | 1)
	_ = w.SetBuildingConfigValue(unlPos, int32(3))

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	posSrc := int32((0 << 16) | 1)
	posDst := int32((2 << 16) | 1)
	srcItems := inv[posSrc]
	dstItems := inv[posDst]
	if len(srcItems) < 2 {
		t.Fatalf("expected source to keep two item types, got %#v", srcItems)
	}
	var src2, src3 int32
	for _, it := range srcItems {
		if it.Item == ItemID(2) {
			src2 = it.Amount
		}
		if it.Item == ItemID(3) {
			src3 = it.Amount
		}
	}
	if src2 != 5 || src3 != 4 {
		t.Fatalf("expected source item2=5 item3=4, got item2=%d item3=%d", src2, src3)
	}
	if len(dstItems) != 1 || dstItems[0].Item != ItemID(3) || dstItems[0].Amount != 1 {
		t.Fatalf("expected destination to receive only item3 amount=1, got %#v", dstItems)
	}
}

func TestStepLogisticsUnloaderNoMoveWhenBalanced(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		10: "unloader",
		11: "container",
	}
	w.SetModel(m)

	left, _ := m.TileAt(0, 1)
	left.Block = 11
	left.Team = TeamID(1)
	left.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	unl, _ := m.TileAt(1, 1)
	unl.Block = 10
	unl.Team = TeamID(1)
	unl.Build = &Building{Block: 10, Team: TeamID(1), X: 1, Y: 1}
	right, _ := m.TileAt(2, 1)
	right.Block = 11
	right.Team = TeamID(1)
	right.Build = &Building{
		Block: 11, Team: TeamID(1), X: 2, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	leftPos := int32((0 << 16) | 1)
	rightPos := int32((2 << 16) | 1)
	if got := inv[leftPos][0].Amount; got != 5 {
		t.Fatalf("expected left amount unchanged=5, got=%d", got)
	}
	if got := inv[rightPos][0].Amount; got != 5 {
		t.Fatalf("expected right amount unchanged=5, got=%d", got)
	}
}

func TestStepLogisticsUnloaderRotatesItemsWithoutFilter(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		10: "unloader",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 4}, {Item: ItemID(3), Amount: 4}},
	}
	unl, _ := m.TileAt(1, 1)
	unl.Block = 10
	unl.Team = TeamID(1)
	unl.Build = &Building{Block: 10, Team: TeamID(1), X: 1, Y: 1}
	dst, _ := m.TileAt(2, 1)
	dst.Block = 11
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 1}

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	dstPos := int32((2 << 16) | 1)
	var got2, got3 int32
	for _, it := range inv[dstPos] {
		if it.Item == ItemID(2) {
			got2 = it.Amount
		}
		if it.Item == ItemID(3) {
			got3 = it.Amount
		}
	}
	if got2 != 1 || got3 != 1 {
		t.Fatalf("expected rotated outputs item2=1 item3=1, got item2=%d item3=%d", got2, got3)
	}
}

func TestStepLogisticsSorterFilterByTileConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(4, 4)
	m.BlockNames = map[int16]string{
		20: "sorter",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}, {Item: ItemID(3), Amount: 5}},
	}
	srt, _ := m.TileAt(1, 1)
	srt.Block = 20
	srt.Team = TeamID(1)
	srt.Build = &Building{Block: 20, Team: TeamID(1), X: 1, Y: 1}
	dst, _ := m.TileAt(2, 1)
	dst.Block = 11
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 1}

	srtPos := int32((1 << 16) | 1)
	_ = w.SetBuildingConfigValue(srtPos, int32(2))

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	posSrc := int32((0 << 16) | 1)
	posDst := int32((2 << 16) | 1)
	srcItems := inv[posSrc]
	dstItems := inv[posDst]
	var src2, src3 int32
	for _, it := range srcItems {
		if it.Item == ItemID(2) {
			src2 = it.Amount
		}
		if it.Item == ItemID(3) {
			src3 = it.Amount
		}
	}
	if src2 != 4 || src3 != 5 {
		t.Fatalf("expected source item2=4 item3=5, got item2=%d item3=%d", src2, src3)
	}
	if len(dstItems) != 1 || dstItems[0].Item != ItemID(2) || dstItems[0].Amount != 1 {
		t.Fatalf("expected destination to receive only item2 amount=1, got %#v", dstItems)
	}
}

func TestStepLogisticsSorterNoFilterGoesSide(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(5, 5)
	m.BlockNames = map[int16]string{
		20: "sorter",
		11: "container",
	}
	w.SetModel(m)

	// source west -> sorter center; forward is east, side includes north.
	src, _ := m.TileAt(1, 2)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 1, Y: 2,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	srt, _ := m.TileAt(2, 2)
	srt.Block = 20
	srt.Team = TeamID(1)
	srt.Build = &Building{Block: 20, Team: TeamID(1), X: 2, Y: 2}
	forward, _ := m.TileAt(3, 2)
	forward.Block = 11
	forward.Team = TeamID(1)
	forward.Build = &Building{Block: 11, Team: TeamID(1), X: 3, Y: 2}
	sideNorth, _ := m.TileAt(2, 3)
	sideNorth.Block = 11
	sideNorth.Team = TeamID(1)
	sideNorth.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 3}

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	if got := inv[int32((3<<16)|2)]; len(got) > 0 {
		t.Fatalf("expected forward target unchanged for sorter without filter, got %#v", got)
	}
	sidePos := int32((2 << 16) | 3)
	if got := inv[sidePos][0].Amount; got != 1 {
		t.Fatalf("expected side output amount=1, got=%d", got)
	}
}

func TestStepLogisticsBridgeConveyorMovesToConfiguredTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(5, 4)
	m.BlockNames = map[int16]string{
		30: "bridge-conveyor",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	br, _ := m.TileAt(1, 1)
	br.Block = 30
	br.Team = TeamID(1)
	br.Build = &Building{Block: 30, Team: TeamID(1), X: 1, Y: 1}
	dst, _ := m.TileAt(4, 1)
	dst.Block = 30
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 30, Team: TeamID(1), X: 4, Y: 1}

	brPos := int32((1 << 16) | 1)
	dstPos := int32((4 << 16) | 1)
	_ = w.SetBuildingConfigValue(brPos, dstPos)

	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	posSrc := int32((0 << 16) | 1)
	if got := inv[posSrc][0].Amount; got != 4 {
		t.Fatalf("expected source amount=4, got=%d", got)
	}
	if got := inv[dstPos][0].Amount; got != 1 {
		t.Fatalf("expected destination amount=1, got=%d", got)
	}
}

func TestBridgeConveyorLinkRangeAndAxis(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(8, 6)
	m.BlockNames = map[int16]string{
		30: "bridge-conveyor",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	br, _ := m.TileAt(1, 1)
	br.Block = 30
	br.Team = TeamID(1)
	br.Build = &Building{Block: 30, Team: TeamID(1), X: 1, Y: 1}

	// Out-of-range straight link: distance 5 (>4), should not move.
	dstFar, _ := m.TileAt(6, 1)
	dstFar.Block = 30
	dstFar.Team = TeamID(1)
	dstFar.Build = &Building{Block: 30, Team: TeamID(1), X: 6, Y: 1}
	brPos := int32((1 << 16) | 1)
	_ = w.SetBuildingConfigValue(brPos, int32((6<<16)|1))
	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}
	inv := w.SnapshotBuildingInventories()
	if got := inv[int32((0<<16)|1)][0].Amount; got != 5 {
		t.Fatalf("expected no transfer for out-of-range link, source=%d", got)
	}

	// Diagonal link (within manhattan but not axial), should not move.
	dstDiag, _ := m.TileAt(3, 3)
	dstDiag.Block = 30
	dstDiag.Team = TeamID(1)
	dstDiag.Build = &Building{Block: 30, Team: TeamID(1), X: 3, Y: 3}
	_ = w.SetBuildingConfigValue(brPos, int32((3<<16)|3))
	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}
	inv = w.SnapshotBuildingInventories()
	if got := inv[int32((0<<16)|1)][0].Amount; got != 5 {
		t.Fatalf("expected no transfer for diagonal link, source=%d", got)
	}
}

func TestBridgeConveyorRejectsNonBridgeTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(5, 4)
	m.BlockNames = map[int16]string{
		30: "bridge-conveyor",
		11: "container",
	}
	w.SetModel(m)

	src, _ := m.TileAt(0, 1)
	src.Block = 11
	src.Team = TeamID(1)
	src.Build = &Building{
		Block: 11, Team: TeamID(1), X: 0, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 5}},
	}
	br, _ := m.TileAt(1, 1)
	br.Block = 30
	br.Team = TeamID(1)
	br.Build = &Building{Block: 30, Team: TeamID(1), X: 1, Y: 1}
	dstContainer, _ := m.TileAt(4, 1)
	dstContainer.Block = 11
	dstContainer.Team = TeamID(1)
	dstContainer.Build = &Building{Block: 11, Team: TeamID(1), X: 4, Y: 1}

	brPos := int32((1 << 16) | 1)
	_ = w.SetBuildingConfigValue(brPos, int32((4<<16)|1))
	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	if got := inv[int32((0<<16)|1)][0].Amount; got != 5 {
		t.Fatalf("expected no transfer when target is non-bridge, source=%d", got)
	}
	if _, ok := inv[int32((4<<16)|1)]; ok && len(inv[int32((4<<16)|1)]) > 0 {
		t.Fatalf("expected non-bridge target inventory unchanged, got %#v", inv[int32((4<<16)|1)])
	}
}

func TestBridgeConveyorInvalidLinkDoDump(t *testing.T) {
	w := New(Config{TPS: 60})
	m := NewWorldModel(5, 4)
	m.BlockNames = map[int16]string{
		30: "bridge-conveyor",
		11: "container",
	}
	w.SetModel(m)

	br, _ := m.TileAt(1, 1)
	br.Block = 30
	br.Team = TeamID(1)
	br.Build = &Building{
		Block: 30, Team: TeamID(1), X: 1, Y: 1,
		Items: []ItemStack{{Item: ItemID(2), Amount: 3}},
	}
	dst, _ := m.TileAt(2, 1)
	dst.Block = 11
	dst.Team = TeamID(1)
	dst.Build = &Building{Block: 11, Team: TeamID(1), X: 2, Y: 1}

	// invalid link (out of range), bridge should fallback to doDump behavior.
	brPos := int32((1 << 16) | 1)
	_ = w.SetBuildingConfigValue(brPos, int32((7<<16)|1))
	for i := 0; i < 15; i++ {
		w.Step(time.Second / 60)
	}

	inv := w.SnapshotBuildingInventories()
	if got := inv[brPos][0].Amount; got != 2 {
		t.Fatalf("expected bridge dump one item, remaining=2 got=%d", got)
	}
	dstPos := int32((2 << 16) | 1)
	if got := inv[dstPos][0].Amount; got != 1 {
		t.Fatalf("expected dump target amount=1, got=%d", got)
	}
}
