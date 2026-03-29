package world

import (
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func placeTestBuilding(t *testing.T, w *World, x, y int, block int16, team TeamID, rotation int8) *Tile {
	t.Helper()
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
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
	w.rebuildBlockOccupancyLocked()
	return tile
}

func mustEncodeConfig(t *testing.T, value any) []byte {
	t.Helper()
	w := protocol.NewWriter()
	if err := protocol.WriteObject(w, value, nil); err != nil {
		t.Fatalf("encode config failed: %v", err)
	}
	return append([]byte(nil), w.Bytes()...)
}

func setTestPayload(t *testing.T, w *World, x, y int, payload *payloadData) int32 {
	t.Helper()
	pos := int32(y*w.Model().Width + x)
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("payload tile lookup failed at (%d,%d): %v", x, y, err)
	}
	st := w.payloadStateLocked(pos)
	st.Payload = payload
	st.Move = 0
	st.Work = 0
	st.Exporting = false
	w.syncPayloadTileLocked(tile, payload)
	return pos
}

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

func TestBuildPlanSnapshotClearsOnlyCurrentOwner(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45: "duo",
		46: "duo",
	}
	w.SetModel(model)

	ownerA := int32(101)
	ownerB := int32(202)
	team := TeamID(1)

	w.ApplyBuildPlanSnapshotForOwner(ownerA, team, []BuildPlanOp{{
		X: 1, Y: 1, BlockID: 45,
	}})
	w.ApplyBuildPlanSnapshotForOwner(ownerB, team, []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 46,
	}})

	// Owner A sends an empty authoritative snapshot, equivalent to Q-clearing plans.
	w.ApplyBuildPlanSnapshotForOwner(ownerA, team, nil)

	// Owner B's plan must remain and continue progressing.
	placed := false
	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
		tileA, _ := w.Model().TileAt(1, 1)
		if tileA.Block != 0 || tileA.Build != nil {
			t.Fatalf("owner A plan should have been cleared, got block=%d build=%v", tileA.Block, tileA.Build != nil)
		}
		tileB, _ := w.Model().TileAt(2, 2)
		if tileB.Block == 46 && tileB.Build != nil {
			placed = true
			break
		}
	}
	if !placed {
		tileB, _ := w.Model().TileAt(2, 2)
		t.Fatalf("owner B plan should remain active, got block=%d build=%v", tileB.Block, tileB.Build != nil)
	}
}

func TestCancelBuildAtForOwnerDoesNotTouchOtherOwner(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45: "duo",
		46: "duo",
	}
	w.SetModel(model)

	ownerA := int32(101)
	ownerB := int32(202)
	team := TeamID(1)

	w.ApplyBuildPlansForOwner(ownerA, team, []BuildPlanOp{{
		X: 1, Y: 1, BlockID: 45,
	}})
	w.ApplyBuildPlansForOwner(ownerB, team, []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 46,
	}})

	w.CancelBuildAtForOwner(ownerA, 1, 1, false)

	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
	}

	tileA, _ := w.Model().TileAt(1, 1)
	if tileA.Block != 0 || tileA.Build != nil {
		t.Fatalf("owner A tile should remain empty after cancel, got block=%d build=%v", tileA.Block, tileA.Build != nil)
	}
	tileB, _ := w.Model().TileAt(2, 2)
	if tileB.Block != 46 || tileB.Build == nil {
		t.Fatalf("owner B tile should still build successfully, got block=%d build=%v", tileB.Block, tileB.Build != nil)
	}
}

func TestNuclearReactorOverheatsFromItemSource(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	reactor, _ := w.Model().TileAt(3, 3)
	reactor.Block = 315
	reactor.Team = 1
	reactor.Build = &Building{
		Block:     315,
		Team:      1,
		X:         3,
		Y:         3,
		Health:    1000,
		MaxHealth: 1000,
	}

	source, _ := w.Model().TileAt(2, 3)
	source.Block = 412
	source.Team = 1
	source.Build = &Building{
		Block:     412,
		Team:      1,
		X:         2,
		Y:         3,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.rebuildBlockOccupancyLocked()
	w.ConfigureItemSource(int32(3*model.Width+2), 5)

	for i := 0; i < 300; i++ {
		w.Step(time.Second / 60)
	}

	reactor, _ = w.Model().TileAt(3, 3)
	if reactor.Block != 0 || reactor.Build != nil {
		t.Fatalf("expected thorium reactor to explode and be destroyed, got block=%d build=%v", reactor.Block, reactor.Build != nil)
	}
}

func TestItemLogisticsMovesThroughConveyorChainToReactor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 257, 1, 0)
	placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	placeTestBuilding(t, w, 4, 3, 315, 1, 0)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)

	for i := 0; i < 420; i++ {
		w.Step(time.Second / 60)
	}

	reactor, _ := w.Model().TileAt(4, 3)
	if reactor.Block != 0 || reactor.Build != nil {
		t.Fatalf("expected thorium reactor to explode after conveyor-fed thorium, got block=%d build=%v", reactor.Block, reactor.Build != nil)
	}
}

func TestSorterRoutesMatchingItemForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	east := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	north := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureSorter(int32(3*model.Width+2), 5)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected matching item to go forward through sorter")
	}
	if north.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected matching item not to route sideways, got north=%d", north.Build.ItemAmount(5))
	}
}

func TestJunctionPassesCrossedFlows(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		501: "junction",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 2, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 501, 1, 0)
	east := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	south := placeTestBuilding(t, w, 2, 4, 257, 1, 1)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureItemSource(int32(2*model.Width+2), 0)

	for i := 0; i < 240; i++ {
		w.Step(time.Second / 60)
	}

	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected west->east flow to pass through junction")
	}
	if south.Build.ItemAmount(0) == 0 {
		t.Fatalf("expected north->south flow to pass through junction")
	}
}

func TestPendingBuildAppliesConfigOnCompletion(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
	}
	w.SetModel(model)

	pos := int32(3*model.Width + 2)
	w.ApplyBuildPlansForOwner(1, 1, []BuildPlanOp{{
		X:       2,
		Y:       3,
		BlockID: 412,
		Config:  protocol.ItemRef{ItmID: 5},
	}})

	for i := 0; i < 30; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(2, 3)
		if tile.Build != nil {
			break
		}
	}

	if got := w.itemSourceCfg[pos]; got != 5 {
		t.Fatalf("expected pending build config to apply item-source item=5, got=%d", got)
	}
}

func TestRestoreSavedBridgeAndItemSourceConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		420: "bridge-conveyor",
	}

	w.SetModel(model)
	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	bridgeA := placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 4, 3, 420, 1, 0)
	out := placeTestBuilding(t, w, 5, 3, 257, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridgeA.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 2, Y: 0})

	w.SetModel(model)

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	if out.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected saved item-source + bridge config to restore and move items across bridge")
	}
}

func TestRestoreSavedSorterConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}

	w.SetModel(model)
	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	sorter := placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	forward := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	side := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	sorter.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})

	w.SetModel(model)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if forward.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected saved sorter config to restore matching forward route")
	}
	if side.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected saved sorter config not to route matching item sideways, got=%d", side.Build.ItemAmount(5))
	}
}

func TestSorterIntConfigFallbackSyncPath(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}

	w.SetModel(model)
	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	sorter := placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	forward := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	side := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureBuilding(int32(3*model.Width+2), int32(5))

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if got, ok := w.BuildingConfigPacked(protocol.PackPoint2(2, 3)); !ok {
		t.Fatalf("expected sorter config to persist")
	} else if item, ok := got.(protocol.ItemRef); !ok || item.ItmID != 5 {
		t.Fatalf("expected sorter normalized config item=5, got=%T %#v", got, got)
	}
	if forward.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected int-based sorter config to route matching item forward")
	}
	if side.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected int-based sorter config not to route matching item sideways, got=%d", side.Build.ItemAmount(5))
	}
	if sorter.Build == nil || len(sorter.Build.Config) == 0 {
		t.Fatalf("expected sorter config bytes to be stored")
	}
}

func TestUnlinkedBridgeDoesNotAcceptItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	bridge := placeTestBuilding(t, w, 2, 3, 420, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	w.SetModel(model)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if bridge.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected unlinked bridge not to accept items, got=%d", bridge.Build.ItemAmount(5))
	}
}

func TestLinkedBridgeRejectsInputFromLinkedSide(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 3, 3, 412, 1, 0)
	bridge := placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 5, 3, 420, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridge.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 3, Y: 0})
	w.SetModel(model)

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	bridge, _ = w.Model().TileAt(2, 3)
	if totalBuildingItems(bridge.Build) != 0 {
		t.Fatalf("expected linked bridge to reject input from its link-facing side, got=%d", totalBuildingItems(bridge.Build))
	}
}

func TestUnlinkedBridgeDoesNotDumpBackTowardIncomingBridge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 0, 3, 412, 1, 0)
	bridgeA := placeTestBuilding(t, w, 1, 3, 420, 1, 0)
	placeTestBuilding(t, w, 4, 3, 420, 1, 2)
	west := placeTestBuilding(t, w, 3, 3, 418, 1, 0)
	east := placeTestBuilding(t, w, 5, 3, 418, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridgeA.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 3, Y: 0})
	w.SetModel(model)

	for i := 0; i < 420; i++ {
		w.Step(time.Second / 60)
	}

	if west.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected unlinked bridge not to dump back toward incoming side, got west=%d", west.Build.ItemAmount(5))
	}
	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected unlinked bridge to dump to a non-incoming side")
	}
}

func TestConveyorRuntimeTracksPerItemPositions(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 1, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 1, 418, 1, 0)

	conveyorPos := int32(2*model.Width + 2)
	behindPos := int32(2*model.Width + 1)
	northPos := int32(1*model.Width + 2)

	if !w.tryInsertItemLocked(behindPos, conveyorPos, 5, 0) {
		t.Fatalf("expected rear insert into conveyor to succeed")
	}
	state := w.conveyorStates[conveyorPos]
	if state == nil || state.Len != 1 || state.YS[0] != 0 {
		t.Fatalf("expected first item at conveyor origin, got state=%+v", state)
	}

	w.Step(500 * time.Millisecond)
	state = w.conveyorStates[conveyorPos]
	if state == nil || state.MinItem <= 0.7 {
		t.Fatalf("expected conveyor item to advance enough for side insert, got minitem=%v", state.MinItem)
	}

	if !w.tryInsertItemLocked(northPos, conveyorPos, 6, 0) {
		t.Fatalf("expected side insert into conveyor to succeed after spacing opens")
	}
	state = w.conveyorStates[conveyorPos]
	if state.Len != 2 {
		t.Fatalf("expected 2 runtime items, got=%d", state.Len)
	}
	if state.YS[state.LastInserted] != 0.5 {
		t.Fatalf("expected side inserted item at y=0.5, got=%v", state.YS[state.LastInserted])
	}
	if totalBuildingItems(w.Model().Tiles[conveyorPos].Build) != 2 {
		t.Fatalf("expected mirrored building inventory total=2, got=%d", totalBuildingItems(w.Model().Tiles[conveyorPos].Build))
	}
}

func TestConveyorRuntimePassesItemsForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 3, 2, 257, 1, 0)
	placeTestBuilding(t, w, 1, 2, 418, 1, 0)

	firstPos := int32(2*model.Width + 2)
	secondPos := int32(2*model.Width + 3)
	sourcePos := int32(2*model.Width + 1)

	if !w.tryInsertItemLocked(sourcePos, firstPos, 5, 0) {
		t.Fatalf("expected rear insert into first conveyor to succeed")
	}

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	first := w.conveyorStates[firstPos]
	second := w.conveyorStates[secondPos]
	if first != nil && first.Len != 0 {
		t.Fatalf("expected first conveyor to pass item onward, len=%d", first.Len)
	}
	if second == nil || second.Len == 0 {
		t.Fatalf("expected second conveyor runtime to receive item")
	}
	if totalBuildingItems(w.Model().Tiles[secondPos].Build) == 0 {
		t.Fatalf("expected mirrored inventory on second conveyor to contain item")
	}
}

func TestRouterImmediatelyPassesToConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 418, 1, 0)
	placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	router.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	conveyorPos := int32(3*model.Width + 3)
	state := w.conveyorStates[conveyorPos]
	if state == nil || state.Len == 0 {
		t.Fatalf("expected router to immediately pass item into conveyor on first frame")
	}
	if totalBuildingItems(router.Build) != 0 {
		t.Fatalf("expected router inventory to be empty after immediate pass, got=%d", totalBuildingItems(router.Build))
	}
}

func TestSorterRejectsInstantTransferThreeChain(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		500: "sorter",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 502, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	placeTestBuilding(t, w, 3, 3, 502, 1, 0)

	if w.tryInsertItemLocked(int32(3*model.Width+1), int32(3*model.Width+2), 5, 0) {
		t.Fatalf("expected sorter to reject instantTransfer three-chain forward path")
	}
}

func TestOverflowRejectsInstantTransferForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		500: "sorter",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 500, 1, 0)
	placeTestBuilding(t, w, 2, 3, 502, 1, 0)
	placeTestBuilding(t, w, 3, 3, 500, 1, 0)

	if w.tryInsertItemLocked(int32(3*model.Width+1), int32(3*model.Width+2), 5, 0) {
		t.Fatalf("expected overflow gate to reject instantTransfer forward chain")
	}
}

func TestSorterCanAcceptDoesNotFlipRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		500: "sorter",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 4, 418, 1, 0)

	sorterPos := int32(3*model.Width + 2)
	sourcePos := int32(3*model.Width + 1)
	w.routerRotation[sorterPos] = 0

	if !w.canAcceptItemLocked(sourcePos, sorterPos, 5, 0) {
		t.Fatalf("expected sorter accept probe to succeed")
	}
	if w.routerRotation[sorterPos] != 0 {
		t.Fatalf("expected sorter accept probe not to flip rotation, got=%d", w.routerRotation[sorterPos])
	}
}

func TestOverflowCanAcceptDoesNotFlipRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 502, 1, 0)
	placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 4, 418, 1, 0)

	gatePos := int32(3*model.Width + 2)
	sourcePos := int32(3*model.Width + 1)
	w.routerRotation[gatePos] = 0

	if !w.canAcceptItemLocked(sourcePos, gatePos, 5, 0) {
		t.Fatalf("expected overflow accept probe to succeed")
	}
	if w.routerRotation[gatePos] != 0 {
		t.Fatalf("expected overflow accept probe not to flip rotation, got=%d", w.routerRotation[gatePos])
	}
}

func TestArmoredConveyorRejectsSideInputFromNonConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
		259: "armored-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 259, 1, 0)
	placeTestBuilding(t, w, 2, 1, 418, 1, 0)

	armoredPos := int32(2*model.Width + 2)
	sourcePos := int32(1*model.Width + 2)
	if w.tryInsertItemLocked(sourcePos, armoredPos, 5, 0) {
		t.Fatalf("expected armored conveyor to reject side input from non-conveyor")
	}
}

func TestArmoredConveyorAcceptsSideInputFromConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		259: "armored-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 259, 1, 0)
	placeTestBuilding(t, w, 2, 1, 257, 1, 1)

	armoredPos := int32(2*model.Width + 2)
	sourcePos := int32(1*model.Width + 2)
	if !w.tryInsertItemLocked(sourcePos, armoredPos, 5, 0) {
		t.Fatalf("expected armored conveyor to accept side input from conveyor")
	}
}

func TestUnlinkedBridgeDumpRotatesTargets(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 2, 420, 1, 0)
	placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 418, 1, 0)

	bridgePos := int32(2*model.Width + 2)
	first, ok := w.bridgeDumpTargetLocked(bridgePos, bridge, 5)
	if !ok {
		t.Fatalf("expected first dump target for unlinked bridge")
	}
	second, ok := w.bridgeDumpTargetLocked(bridgePos, bridge, 5)
	if !ok {
		t.Fatalf("expected second dump target for unlinked bridge")
	}
	if first == second {
		t.Fatalf("expected dump rotation to advance to a different target, got same=%d", first)
	}
}

func TestItemSourceDumpsOnePathPerUpdate(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 412, 1, 0)
	east := placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	south := placeTestBuilding(t, w, 2, 3, 418, 1, 0)
	w.ConfigureItemSource(int32(2*model.Width+2), 5)

	w.Step(time.Second / 60)

	total := totalBuildingItems(east.Build) + totalBuildingItems(south.Build)
	if total != 1 {
		t.Fatalf("expected item source to dump through exactly one path on first update, got total=%d", total)
	}
}

func TestUnlinkedBridgeDumpsOnFirstFrame(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 2, 420, 1, 0)
	out := placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	bridge.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	if totalBuildingItems(out.Build) != 1 {
		t.Fatalf("expected unlinked bridge to dump on first frame, got=%d", totalBuildingItems(out.Build))
	}
	if totalBuildingItems(bridge.Build) != 0 {
		t.Fatalf("expected bridge inventory empty after first-frame dump, got=%d", totalBuildingItems(bridge.Build))
	}
}

func TestDistributorUsesMultiBlockProximity(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		600: "distributor",
	}
	w.SetModel(model)

	distributor := placeTestBuilding(t, w, 3, 3, 600, 1, 0)
	placeTestBuilding(t, w, 2, 3, 257, 1, 0)
	placeTestBuilding(t, w, 2, 4, 257, 1, 0)
	placeTestBuilding(t, w, 5, 3, 257, 1, 0)
	placeTestBuilding(t, w, 5, 4, 257, 1, 0)

	distributor.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	moved := 0
	for _, pos := range []int32{
		int32(3*model.Width + 2),
		int32(4*model.Width + 2),
		int32(3*model.Width + 5),
		int32(4*model.Width + 5),
	} {
		if st := w.conveyorStates[pos]; st != nil && st.Len > 0 {
			moved++
		}
	}
	if moved != 1 {
		t.Fatalf("expected distributor to route exactly one item into adjacent edge conveyor, moved=%d", moved)
	}
	if totalBuildingItems(distributor.Build) != 0 {
		t.Fatalf("expected distributor inventory empty after routing, got=%d", totalBuildingItems(distributor.Build))
	}
}

func TestItemSourceFeedsThoriumReactorAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 4, 4, 315, 1, 0)
	placeTestBuilding(t, w, 6, 4, 412, 1, 0)
	w.ConfigureItemSource(int32(4*model.Width+6), 5)

	w.Step(time.Second / 60)

	if reactor.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected item source to feed thorium reactor through multiblock edge")
	}
}

func TestItemSourceFeedsCoreAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 1)

	w.Step(time.Second / 60)

	if core.Build.ItemAmount(1) == 0 {
		t.Fatalf("expected item source to feed core through multiblock edge")
	}
}

func TestConveyorFeedsCoreAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(14, 14)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	conveyor := placeTestBuilding(t, w, 8, 5, 257, 1, 2)
	conveyor.Build.AddItem(0, 1)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
		if core.Build.ItemAmount(0) > 0 {
			break
		}
	}

	if core.Build.ItemAmount(0) == 0 {
		t.Fatalf("expected conveyor to feed core through multiblock edge")
	}
	if conveyor.Build.ItemAmount(0) != 0 {
		t.Fatalf("expected conveyor inventory drained into core, got=%d", conveyor.Build.ItemAmount(0))
	}
}

func TestTeamCoreItemSnapshotsUseRealCoreInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	core.Build.AddItem(0, 7)
	core.Build.AddItem(5, 3)
	w.teamItems[1] = map[ItemID]int32{
		0: 2900,
		1: 1900,
	}

	snapshots := w.TeamCoreItemSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 core snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Team != 1 {
		t.Fatalf("expected team 1 snapshot, got team %d", snapshots[0].Team)
	}
	if len(snapshots[0].Items) != 2 {
		t.Fatalf("expected 2 real core items, got %d", len(snapshots[0].Items))
	}
	if snapshots[0].Items[0].Item != 0 || snapshots[0].Items[0].Amount != 7 {
		t.Fatalf("expected copper amount 7, got item=%d amount=%d", snapshots[0].Items[0].Item, snapshots[0].Items[0].Amount)
	}
	if snapshots[0].Items[1].Item != 5 || snapshots[0].Items[1].Amount != 3 {
		t.Fatalf("expected sand amount 3, got item=%d amount=%d", snapshots[0].Items[1].Item, snapshots[0].Items[1].Amount)
	}
}

func TestTeamItemsReflectRealCoreInput(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 0)

	w.Step(time.Second / 60)

	items := w.TeamItems(1)
	if items[0] == 0 {
		t.Fatalf("expected team item view to reflect real core input, got copper=%d", items[0])
	}
}

func TestCoreFeedEmitsTeamItemEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 0)

	w.Step(time.Second / 60)

	evs := w.DrainEntityEvents()
	for _, ev := range evs {
		if ev.Kind == EntityEventTeamItems && ev.BuildTeam == 1 && ev.ItemID == 0 && ev.ItemAmount > 0 {
			return
		}
	}
	t.Fatalf("expected feeding a core to emit a team item sync event")
}

func TestLinkedStorageMergesIntoCoreInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 12)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		431: "vault",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	store := placeTestBuilding(t, w, 8, 5, 431, 1, 0)
	store.Build.AddItem(0, 4)
	w.rebuildBlockOccupancyLocked()

	if got := w.TeamItems(1)[0]; got != 4 {
		t.Fatalf("expected linked vault items to merge into core inventory view, got %d", got)
	}
	if core.Build.ItemAmount(0) != 4 {
		t.Fatalf("expected primary core to hold merged linked inventory, got %d", core.Build.ItemAmount(0))
	}
	if store.Build.ItemAmount(0) != 0 {
		t.Fatalf("expected linked vault local inventory cleared after merge, got %d", store.Build.ItemAmount(0))
	}
	positions := w.TeamItemSyncPositions(1)
	if len(positions) != 2 {
		t.Fatalf("expected core and linked vault sync positions, got %d", len(positions))
	}
}

func TestBuildCostConsumesCoreInventoryWhenCorePresent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	core.Build.AddItem(0, 40)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	w.Step(3 * time.Second)

	if core.Build.ItemAmount(0) != 5 {
		t.Fatalf("expected duo build to consume real core copper, got %d", core.Build.ItemAmount(0))
	}
}

func TestPhaseConveyorTransfersLinkedItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		421: "phase-conveyor",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 2, 3, 421, 1, 0)
	dst := placeTestBuilding(t, w, 6, 3, 421, 1, 0)
	out := placeTestBuilding(t, w, 7, 3, 418, 1, 0)
	w.ConfigureBuilding(int32(3*model.Width+2), protocol.Point2{X: 4, Y: 0})

	src.Build.AddItem(5, 1)

	for i := 0; i < 6; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) != 1 {
		t.Fatalf("expected linked phase conveyor to transfer item, got=%d", totalBuildingItems(out.Build))
	}
	if totalBuildingItems(dst.Build) != 0 {
		t.Fatalf("expected target phase conveyor inventory drained after transfer, got=%d", totalBuildingItems(dst.Build))
	}
}

func TestConfigureBuildingPackedBridgeUsesOfficialAbsolutePos(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 8)
	model.BlockNames = map[int16]string{
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 6, 3, 420, 1, 0)

	srcPacked := protocol.PackPoint2(2, 3)
	dstPacked := protocol.PackPoint2(6, 3)

	w.ConfigureBuildingPacked(srcPacked, dstPacked)

	cfg, ok := w.BuildingConfigPacked(srcPacked)
	if !ok {
		t.Fatalf("expected packed building config to be readable")
	}
	point, ok := cfg.(protocol.Point2)
	if !ok {
		t.Fatalf("expected normalized bridge config to be Point2, got %T", cfg)
	}
	if point.X != 4 || point.Y != 0 {
		t.Fatalf("expected relative bridge config (4,0), got (%d,%d)", point.X, point.Y)
	}
}

func TestUnloaderMovesConfiguredItemBetweenAdjacentBlocks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		430: "unloader",
		431: "vault",
	}
	w.SetModel(model)

	store := placeTestBuilding(t, w, 2, 3, 431, 1, 0)
	placeTestBuilding(t, w, 3, 3, 430, 1, 0)
	placeTestBuilding(t, w, 4, 3, 257, 1, 0)
	store.Build.AddItem(5, 3)
	w.ConfigureUnloader(int32(3*model.Width+3), 5)

	for i := 0; i < 8; i++ {
		w.Step(time.Second / 60)
	}

	if st := w.conveyorStates[int32(3*model.Width+4)]; st == nil || st.Len == 0 {
		t.Fatalf("expected unloader to move configured item into conveyor")
	}
	if store.Build.ItemAmount(5) >= 3 {
		t.Fatalf("expected unloader to remove item from source storage")
	}
}

func TestMassDriverTransfersBatchToLinkedTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 12)
	model.BlockNames = map[int16]string{
		432: "mass-driver",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 4, 4, 432, 1, 0)
	dst := placeTestBuilding(t, w, 14, 4, 432, 1, 0)
	w.ConfigureBuilding(int32(4*model.Width+4), protocol.Point2{X: 10, Y: 0})
	src.Build.AddItem(5, 20)

	for i := 0; i < 260; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(dst.Build) == 0 {
		t.Fatalf("expected mass driver to transfer batch to linked target")
	}
	if totalBuildingItems(src.Build) >= 20 {
		t.Fatalf("expected mass driver source inventory to decrease")
	}
}

func TestDuctMovesItemForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		440: "duct",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 440, 1, 0)
	placeTestBuilding(t, w, 3, 3, 440, 1, 0)
	out := placeTestBuilding(t, w, 4, 3, 257, 1, 0)
	w.ConfigureItemSource(int32(3*model.Width+1), 5)

	for i := 0; i < 40; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct chain to move item into output")
	}
}

func TestDuctBridgeTransfersBetweenLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		440: "duct",
		445: "duct-bridge",
	}
	w.SetModel(model)

	in := placeTestBuilding(t, w, 1, 3, 440, 1, 0)
	placeTestBuilding(t, w, 2, 3, 445, 1, 0)
	placeTestBuilding(t, w, 5, 3, 445, 1, 0)
	out := placeTestBuilding(t, w, 6, 3, 257, 1, 0)
	in.Build.AddItem(5, 1)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct bridge pair to deliver item forward")
	}
}

func TestDirectionalDuctUnloaderMovesConfiguredItem(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		431: "vault",
		446: "duct-unloader",
	}
	w.SetModel(model)

	store := placeTestBuilding(t, w, 1, 3, 431, 1, 0)
	placeTestBuilding(t, w, 2, 3, 446, 1, 0)
	out := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	store.Build.AddItem(5, 3)
	w.ConfigureUnloader(int32(3*model.Width+2), 5)

	for i := 0; i < 20; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct-unloader to push configured item forward")
	}
	if store.Build.ItemAmount(5) >= 3 {
		t.Fatalf("expected duct-unloader to remove item from rear storage")
	}
}

func TestPlastaniumConveyorTransfersWholeStack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		447: "plastanium-conveyor",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 2, 3, 447, 1, 0)
	dst := placeTestBuilding(t, w, 3, 3, 447, 1, 0)
	src.Build.AddItem(5, 10)

	for i := 0; i < 80; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(dst.Build) == 0 {
		t.Fatalf("expected plastanium conveyor to transfer stacked items")
	}
	if totalBuildingItems(src.Build) != 0 {
		t.Fatalf("expected source plastanium conveyor to hand off full stack")
	}
}

func TestSurgeRouterUnloadsBatchToForwardSide(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		448: "surge-router",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 448, 1, 0)
	out := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	router.Build.AddItem(5, 10)
	w.ConfigureSorter(int32(3*model.Width+2), 5)

	for i := 0; i < 80; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected surge router to unload batch to forward output")
	}
}

func TestLiquidRouterMovesLiquidForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		450: "liquid-router",
		315: "thorium-reactor",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 450, 1, 0)
	store := placeTestBuilding(t, w, 3, 3, 315, 1, 0)
	router.Build.AddLiquid(1, 10)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(store.Build) <= 0 {
		t.Fatalf("expected liquid router to move liquid into container")
	}
}

func TestLiquidBridgeTransfersLinkedLiquid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		452: "bridge-conduit",
		315: "thorium-reactor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 3, 452, 1, 0)
	placeTestBuilding(t, w, 5, 3, 452, 1, 0)
	tank := placeTestBuilding(t, w, 6, 3, 315, 1, 0)
	bridge.Build.AddLiquid(1, 20)
	w.ConfigureBuilding(int32(3*model.Width+2), protocol.Point2{X: 3, Y: 0})

	for i := 0; i < 40; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(tank.Build) <= 0 {
		t.Fatalf("expected linked liquid bridge to move liquid into tank")
	}
}

func TestConduitMovesLiquidForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		454: "conduit",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 1, 3, 454, 1, 0)
	placeTestBuilding(t, w, 2, 3, 454, 1, 0)
	dst := placeTestBuilding(t, w, 3, 3, 315, 1, 0)
	src.Build.AddLiquid(1, 20)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(dst.Build) <= 0 {
		t.Fatalf("expected conduit chain to move liquid into destination")
	}
}

func TestPlatedConduitAcceptsRearLiquid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		455: "plated-conduit",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 1, 3, 455, 1, 0)
	dst := placeTestBuilding(t, w, 2, 3, 315, 1, 0)
	src.Build.AddLiquid(1, 30)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(dst.Build) <= 0 {
		t.Fatalf("expected plated conduit to move liquid forward")
	}
}

func TestPayloadConveyorTransfersBlockPayloadForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 12)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		700: "payload-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 4, 700, 1, 0)
	placeTestBuilding(t, w, 7, 4, 700, 1, 0)

	sourcePos := setTestPayload(t, w, 4, 4, &payloadData{Kind: payloadKindBlock, BlockID: 257})
	targetPos := int32(4*model.Width + 7)

	for i := 0; i < 60; i++ {
		w.Step(time.Second / 60)
	}

	if w.payloadStateLocked(sourcePos).Payload != nil {
		t.Fatalf("expected source payload conveyor to hand off payload")
	}
	if got := w.payloadStateLocked(targetPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected target payload conveyor to receive block payload, got=%+v", got)
	}
}

func TestPayloadRouterRoutesMatchingBlockForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		700: "payload-conveyor",
		701: "payload-router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 8, 8, 701, 1, 1)
	placeTestBuilding(t, w, 11, 8, 700, 1, 0)
	placeTestBuilding(t, w, 8, 11, 700, 1, 1)

	routerPos := int32(8*model.Width + 8)
	w.ConfigureBuildingPacked(protocol.PackPoint2(8, 8), protocol.BlockRef{BlkID: 257})
	cfg, ok := w.BuildingConfigPacked(protocol.PackPoint2(8, 8))
	if !ok {
		t.Fatalf("expected payload router config to persist")
	}
	filter, ok := cfg.(protocol.Content)
	if !ok || filter.ContentType() != protocol.ContentBlock || filter.ID() != 257 {
		t.Fatalf("expected payload router block filter config, got=%T %+v", cfg, cfg)
	}

	st := w.payloadStateLocked(routerPos)
	st.Payload = &payloadData{Kind: payloadKindBlock, BlockID: 257}
	st.RecDir = 0

	for i := 0; i < 60; i++ {
		w.Step(time.Second / 60)
	}

	forwardPos := int32(8*model.Width + 11)
	sidePos := int32(11*model.Width + 8)
	if got := w.payloadStateLocked(forwardPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected matching payload to route forward, got=%+v", got)
	}
	if got := w.payloadStateLocked(sidePos).Payload; got != nil {
		t.Fatalf("expected side target to stay empty, got=%+v", got)
	}
}

func TestPayloadMassDriverTransfersPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		702: "payload-mass-driver",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 6, 8, 702, 1, 0)
	placeTestBuilding(t, w, 16, 8, 702, 1, 0)

	sourcePos := int32(8*model.Width + 6)
	targetPos := int32(8*model.Width + 16)
	w.ConfigureBuilding(sourcePos, protocol.Point2{X: 10, Y: 0})
	setTestPayload(t, w, 6, 8, &payloadData{Kind: payloadKindBlock, BlockID: 257})

	for i := 0; i < 260; i++ {
		w.Step(time.Second / 60)
	}

	if w.payloadStateLocked(sourcePos).Payload != nil {
		t.Fatalf("expected source payload mass driver to launch payload")
	}
	if got := w.payloadStateLocked(targetPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected target payload mass driver to receive payload, got=%+v", got)
	}
}

func TestPayloadLoaderAndUnloaderTransferItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 16)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		703: "payload-loader",
		704: "payload-unloader",
	}
	w.SetModel(model)

	loader := placeTestBuilding(t, w, 8, 8, 703, 1, 0)
	unloader := placeTestBuilding(t, w, 11, 8, 704, 1, 0)
	loader.Build.AddItem(5, 6)
	setTestPayload(t, w, 8, 8, &payloadData{Kind: payloadKindBlock, BlockID: 315})

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	if unloader.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected payload loader/unloader pair to transfer items into unloader inventory")
	}
}
