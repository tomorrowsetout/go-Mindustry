package world

import (
	"bytes"
	"testing"
)

func TestNewFogInitializesDimensionsAndTeamBuffers(t *testing.T) {
	fog := NewFog(8, 6)
	if fog.Width != 8 || fog.Height != 6 {
		t.Fatalf("unexpected fog dimensions: %dx%d", fog.Width, fog.Height)
	}
	if len(fog.Visible) != 48 {
		t.Fatalf("expected global fog buffer size 48, got %d", len(fog.Visible))
	}
	fog.AddTeamFog(3)
	teamFog, ok := fog.Teams[3]
	if !ok || teamFog == nil {
		t.Fatal("expected team fog to be created")
	}
	if len(teamFog.Visible) != 48 {
		t.Fatalf("expected team fog buffer size 48, got %d", len(teamFog.Visible))
	}
	fog.SetFog(2, 4, 3, false)
	if fog.GetFog(2, 4, 3) {
		t.Fatal("expected SetFog to update per-team visibility")
	}
}

func TestRaycastHitsBlockingBuilding(t *testing.T) {
	model := NewWorldModel(8, 1)
	blocker, err := model.TileAt(4, 0)
	if err != nil {
		t.Fatalf("tileAt blocker: %v", err)
	}
	blocker.Block = 1
	blocker.Team = 2
	blocker.Build = &Building{
		Block:     1,
		Team:      2,
		X:         4,
		Y:         0,
		Health:    100,
		MaxHealth: 100,
	}

	w := New(Config{})
	w.SetModel(model)

	result := w.Raycast(4, 4, 60, 4, 1, 0)
	if result == nil || !result.Hit {
		t.Fatal("expected raycast to hit blocking building")
	}
	if result.Pos.X != 4 || result.Pos.Y != 0 {
		t.Fatalf("expected hit at tile (4,0), got (%d,%d)", result.Pos.X, result.Pos.Y)
	}
	if result.Team != 2 {
		t.Fatalf("expected hit team 2, got %d", result.Team)
	}
	if result.Building == nil || result.Tile == nil {
		t.Fatal("expected hit result to include tile and building")
	}
}

func TestLineBuildReturnsTilePath(t *testing.T) {
	points := lineTilesForWorldCoords(4, 4, 28, 4, 0)
	if len(points) != 4 {
		t.Fatalf("expected 4 path tiles, got %d", len(points))
	}
	for i, point := range points {
		if point.X != int32(i) || point.Y != 0 {
			t.Fatalf("unexpected path point %d: %+v", i, point)
		}
	}
}

func TestBehaviorTileVisibilityAndTargets(t *testing.T) {
	model := NewWorldModel(8, 1)
	obstacle, _ := model.TileAt(2, 0)
	obstacle.Block = 2
	obstacle.Team = 2
	obstacle.Build = &Building{Block: 2, Team: 2, X: 2, Y: 0, Health: 50, MaxHealth: 50}

	core, _ := model.TileAt(4, 0)
	core.Block = 316
	core.Team = 2
	core.Build = &Building{Block: 316, Team: 2, X: 4, Y: 0, Health: 300, MaxHealth: 300}

	model.AddEntity(RawEntity{ID: 1, Team: 2, X: 44, Y: 4, Health: 40, MaxHealth: 40})

	w := New(Config{})
	w.SetModel(model)
	w.blockNamesByID = map[int16]string{
		2:   "duo",
		316: "core-shard",
	}

	source := NewBehaviorTile(0, 0, 0, 1)
	source.World = w

	if target := source.TargetEnemy(1); target == nil || target.Tile.X != 2 {
		t.Fatalf("expected nearest enemy building at x=2, got %+v", target)
	}

	if coreTarget := source.TargetCore(); coreTarget == nil || coreTarget.Tile.X != 4 {
		t.Fatalf("expected core target at x=4, got %+v", coreTarget)
	}

	unitOnly := NewBehaviorTile(6, 0, 0, 1)
	unitOnly.World = w
	if target := unitOnly.TargetUnit(); target == nil || target.Block != 0 || target.Team != 2 {
		t.Fatalf("expected nearest enemy unit target, got %+v", target)
	}

	targetBehindObstacle := NewBehaviorTile(4, 0, 316, 2)
	targetBehindObstacle.World = w
	if source.CanSee(targetBehindObstacle) {
		t.Fatal("expected obstacle to block visibility to target")
	}
}

func TestBehaviorTilePowerAndPayloadLifecycle(t *testing.T) {
	tile := NewBehaviorTile(1, 1, 1, 1)
	tile.Power = 5
	tile.PowerCapacity = 10

	if !tile.ConsumePower(3) {
		t.Fatal("expected power consumption to succeed")
	}
	if tile.Power != 2 {
		t.Fatalf("expected remaining power 2, got %f", tile.Power)
	}
	if tile.ConsumePower(3) {
		t.Fatal("expected power over-consumption to fail")
	}

	payload := []byte{1, 2, 3}
	if !tile.LoadPayload(payload) {
		t.Fatal("expected payload load to succeed")
	}
	if !tile.IsLoaded() {
		t.Fatal("expected tile to report loaded payload")
	}
	dumped := tile.DumpPayload()
	if !bytes.Equal(dumped, payload) {
		t.Fatalf("unexpected dumped payload: %v", dumped)
	}
	if tile.IsLoaded() {
		t.Fatal("expected payload to be cleared after dump")
	}
}
