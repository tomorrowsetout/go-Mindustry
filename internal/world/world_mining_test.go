package world

import "testing"

func TestResolveMineTileUsesOverlayOreForFloorMining(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		2: "ore-copper",
	}
	model.Tiles[0].Floor = 1
	model.Tiles[0].Overlay = 2
	model.Tiles[0].Block = 0
	w.SetModel(model)

	got, ok := w.ResolveMineTile(packTilePos(0, 0), true, false)
	if !ok {
		t.Fatalf("expected mine tile to resolve")
	}
	if got.Item != 0 {
		t.Fatalf("expected copper item id 0, got %d", got.Item)
	}
	if got.Hardness != 1 {
		t.Fatalf("expected hardness 1, got %d", got.Hardness)
	}
}

func TestAcceptItemAtRespectsCoreCapacity(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.Tiles[0].Block = 339
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &Building{
		Block: 339,
		Team:  1,
		X:     0,
		Y:     0,
	}
	model.Tiles[0].Build.AddItem(0, 3999)
	w.SetModel(model)

	accepted := w.AcceptItemAt(packTilePos(0, 0), 0, 5)
	if accepted != 1 {
		t.Fatalf("expected exactly 1 item accepted, got %d", accepted)
	}
	if total := model.Tiles[0].Build.ItemAmount(0); total != 4000 {
		t.Fatalf("expected core copper total 4000, got %d", total)
	}
}
