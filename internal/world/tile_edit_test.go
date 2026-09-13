package world

import "testing"

// TestTileEditPrimitives covers the server-authoritative tile mutation path
// that backs map editing and world-processor style edits.
func TestTileEditPrimitives(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	w.SetModel(model)

	if !w.SetFloorAt(1, 2, FloorID(7), OverlayID(9)) {
		t.Fatal("SetFloorAt within bounds should succeed")
	}
	tile, ok := w.TileSnapshotAt(1, 2)
	if !ok {
		t.Fatal("tile snapshot should be readable after edit")
	}
	if tile.Floor != 7 || tile.Overlay != 9 {
		t.Fatalf("floor/overlay = %d/%d, want 7/9", tile.Floor, tile.Overlay)
	}

	if !w.SetOverlayAt(0, 0, OverlayID(3)) {
		t.Fatal("SetOverlayAt should succeed")
	}
	tile, _ = w.TileSnapshotAt(0, 0)
	if tile.Overlay != 3 {
		t.Fatalf("overlay = %d, want 3", tile.Overlay)
	}

	if !w.SetBlockAt(3, 3, BlockID(15), TeamID(2), 1) {
		t.Fatal("SetBlockAt should succeed")
	}
	tile, _ = w.TileSnapshotAt(3, 3)
	if tile.Block != 15 || tile.Team != 2 || tile.Rotation != 1 {
		t.Fatalf("block/team/rotation = %d/%d/%d, want 15/2/1", tile.Block, tile.Team, tile.Rotation)
	}

	// Removing a block resets it to air but keeps floor/overlay.
	if !w.RemoveTileAt(3, 3) {
		t.Fatal("RemoveTileAt should succeed")
	}
	tile, _ = w.TileSnapshotAt(3, 3)
	if tile.Block != 0 || tile.Team != 0 || tile.Rotation != 0 {
		t.Fatalf("after remove block/team/rotation = %d/%d/%d, want 0/0/0", tile.Block, tile.Team, tile.Rotation)
	}
}

// TestTileEditRejectsOutOfBounds verifies boundary guards and the unloaded
// model case so callers can trust the boolean result.
func TestTileEditRejectsOutOfBounds(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	w.SetModel(model)

	for _, op := range []func() bool{
		func() bool { return w.SetFloorAt(-1, 0, 0, 0) },
		func() bool { return w.SetFloorAt(0, 4, 0, 0) },
		func() bool { return w.SetOverlayAt(4, 0, 0) },
		func() bool { return w.SetBlockAt(100, 100, 0, 0, 0) },
		func() bool { return w.RemoveTileAt(-5, -5) },
	} {
		if op() {
			t.Fatal("out-of-bounds tile edit must be rejected")
		}
	}
	if _, ok := w.TileSnapshotAt(9, 9); ok {
		t.Fatal("out-of-bounds snapshot must fail")
	}

	// Unloaded world: no model means every edit is rejected.
	empty := New(Config{TPS: 60})
	if empty.SetFloorAt(0, 0, 0, 0) {
		t.Fatal("edit on unloaded world must be rejected")
	}
}

// TestSetBlockAtDropsStaleBuilding ensures replacing a block discards the
// previous building runtime state, so snapshot senders cannot emit stale data.
func TestSetBlockAtDropsStaleBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(2, 2)
	model.Tiles[1].Block = BlockID(5)
	model.Tiles[1].Build = &Building{Block: BlockID(5)}
	w.SetModel(model)

	if !w.SetBlockAt(1, 0, BlockID(8), TeamID(1), 0) {
		t.Fatal("SetBlockAt should succeed")
	}
	tile, _ := w.TileSnapshotAt(1, 0)
	if tile.Build != nil {
		t.Fatal("replaced block must drop the stale building state")
	}
	if tile.Block != 8 {
		t.Fatalf("block = %d, want 8", tile.Block)
	}
}
