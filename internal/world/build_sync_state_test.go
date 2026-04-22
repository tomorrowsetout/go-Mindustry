package world

import "testing"

func TestBuildSyncSnapshotUsesBuildTeamWhenPresent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	model.BlockNames = map[int16]string{
		322: "container",
	}
	tile, err := model.TileAt(1, 2)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Block = 322
	tile.Team = 1
	tile.Rotation = 2
	tile.Build = &Building{
		Block:     322,
		Team:      3,
		Rotation:  2,
		X:         1,
		Y:         2,
		Health:    80,
		MaxHealth: 100,
	}
	w.SetModel(model)

	snaps := w.BuildSyncSnapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 build sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Team != 3 {
		t.Fatalf("expected snapshot team to use build team=3, got %d", snaps[0].Team)
	}
}

func TestBuildSyncSnapshotSkipsNonCenterSharedBuildTiles(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(6, 6)
	model.BlockNames = map[int16]string{
		703: "payload-loader",
	}
	center, err := model.TileAt(3, 3)
	if err != nil || center == nil {
		t.Fatalf("center tile lookup failed: %v", err)
	}
	center.Block = 703
	center.Team = 1
	center.Rotation = 0
	center.Build = &Building{
		Block:     703,
		Team:      1,
		Rotation:  0,
		X:         3,
		Y:         3,
		Health:    100,
		MaxHealth: 100,
	}
	edge, err := model.TileAt(2, 3)
	if err != nil || edge == nil {
		t.Fatalf("edge tile lookup failed: %v", err)
	}
	edge.Block = 703
	edge.Team = 1
	edge.Rotation = 0
	edge.Build = center.Build

	w.SetModel(model)

	snaps := w.BuildSyncSnapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected only center tile to produce a build sync snapshot, got %d", len(snaps))
	}
	if snaps[0].X != 3 || snaps[0].Y != 3 {
		t.Fatalf("expected center snapshot at (3,3), got (%d,%d)", snaps[0].X, snaps[0].Y)
	}
}

func TestBlockSyncSnapshotsForPackedNormalizeSharedBuildEdgesToCenter(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		344: "vault",
	}
	center, err := model.TileAt(5, 5)
	if err != nil || center == nil {
		t.Fatalf("center tile lookup failed: %v", err)
	}
	center.Block = 344
	center.Team = 1
	center.Build = &Building{
		Block:     344,
		Team:      1,
		X:         5,
		Y:         5,
		Health:    600,
		MaxHealth: 600,
	}
	edge, err := model.TileAt(4, 5)
	if err != nil || edge == nil {
		t.Fatalf("edge tile lookup failed: %v", err)
	}
	edge.Block = 344
	edge.Team = 1
	edge.Build = center.Build

	w.SetModel(model)

	edgePacked := packTilePos(4, 5)
	centerPacked := packTilePos(5, 5)

	info, ok := w.BuildingInfoPacked(edgePacked)
	if !ok {
		t.Fatal("expected edge tile to resolve to shared center building")
	}
	if info.Pos != centerPacked || info.X != 5 || info.Y != 5 {
		t.Fatalf("expected building info to normalize to center (5,5), got pos=%d x=%d y=%d", info.Pos, info.X, info.Y)
	}

	snaps := w.BlockSyncSnapshotsForPackedLiveOnly([]int32{edgePacked})
	if len(snaps) != 1 {
		t.Fatalf("expected one normalized block snapshot, got %d", len(snaps))
	}
	if snaps[0].Pos != centerPacked {
		t.Fatalf("expected normalized snapshot position %d, got %d", centerPacked, snaps[0].Pos)
	}

	related := w.RelatedBlockSyncPackedPositions(edgePacked)
	if len(related) != 1 || related[0] != centerPacked {
		t.Fatalf("expected related block sync positions to contain only center %d, got %v", centerPacked, related)
	}
}
