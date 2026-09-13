package world

import (
	"testing"
	"time"
)

func hasBlockItemSyncForPos(events []EntityEvent, pos int32) bool {
	for _, ev := range events {
		if ev.Kind == EntityEventBlockItemSync && ev.BuildPos == pos {
			return true
		}
	}
	return false
}

func TestConveyorTransferEmitsBlockItemSyncForSourceAndTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 3, 2, 257, 1, 0)

	sourcePos := int32(2*model.Width + 1)
	firstPos := int32(2*model.Width + 2)
	secondPos := int32(2*model.Width + 3)

	w.mu.Lock()
	if !w.tryInsertItemLocked(sourcePos, firstPos, 5, 0) {
		w.mu.Unlock()
		t.Fatalf("expected insert into first conveyor to succeed")
	}
	w.mu.Unlock()
	_ = w.DrainEntityEvents()

	foundFirst := false
	foundSecond := false
	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
		evs := w.DrainEntityEvents()
		foundFirst = foundFirst || hasBlockItemSyncForPos(evs, packTilePos(2, 2))
		foundSecond = foundSecond || hasBlockItemSyncForPos(evs, packTilePos(3, 2))
	}

	if st := w.conveyorStates[firstPos]; st != nil && st.Len != 0 {
		t.Fatalf("expected first conveyor runtime to empty after transfer, len=%d", st.Len)
	}
	if st := w.conveyorStates[secondPos]; st == nil || st.Len == 0 {
		t.Fatalf("expected second conveyor runtime to receive item")
	}
	if !foundFirst {
		t.Fatalf("expected source conveyor inventory change to emit block item sync")
	}
	if !foundSecond {
		t.Fatalf("expected target conveyor inventory change to emit block item sync")
	}
}

func TestRouterInsertEmitsBlockItemSync(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 257, 1, 0)
	router := placeTestBuilding(t, w, 2, 3, 418, 1, 0)

	sourcePos := int32(3*model.Width + 1)
	routerPos := int32(3*model.Width + 2)

	w.mu.Lock()
	if !w.tryInsertItemLocked(sourcePos, routerPos, 5, 0) {
		w.mu.Unlock()
		t.Fatalf("expected insert into router to succeed")
	}
	w.mu.Unlock()

	evs := w.DrainEntityEvents()
	if !hasBlockItemSyncForPos(evs, packTilePos(router.X, router.Y)) {
		t.Fatalf("expected router insert to emit block item sync")
	}
}

func TestPlastaniumConveyorTransferEmitsBlockItemSync(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		447: "plastanium-conveyor",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 2, 3, 447, 1, 0)
	dst := placeTestBuilding(t, w, 3, 3, 447, 1, 0)
	src.Build.AddItem(5, 10)

	foundSrc := false
	foundDst := false
	for i := 0; i < 80; i++ {
		w.Step(time.Second / 60)
		evs := w.DrainEntityEvents()
		foundSrc = foundSrc || hasBlockItemSyncForPos(evs, packTilePos(src.X, src.Y))
		foundDst = foundDst || hasBlockItemSyncForPos(evs, packTilePos(dst.X, dst.Y))
	}

	if totalBuildingItems(dst.Build) == 0 {
		t.Fatalf("expected plastanium conveyor to transfer stacked items")
	}
	if totalBuildingItems(src.Build) != 0 {
		t.Fatalf("expected source plastanium conveyor to hand off full stack")
	}
	if !foundSrc {
		t.Fatalf("expected source plastanium conveyor change to emit block item sync")
	}
	if !foundDst {
		t.Fatalf("expected target plastanium conveyor change to emit block item sync")
	}
}
