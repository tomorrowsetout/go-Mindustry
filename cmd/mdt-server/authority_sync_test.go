package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

func TestBuildSyncSnapshotFromModelUsesBuildTeamWhenPresent(t *testing.T) {
	model := world.NewWorldModel(4, 4)
	model.BlockNames = map[int16]string{
		322: "container",
	}
	tile, err := model.TileAt(1, 1)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Block = 322
	tile.Team = 1
	tile.Rotation = 3
	tile.Build = &world.Building{
		Block:     322,
		Team:      4,
		Rotation:  3,
		X:         1,
		Y:         1,
		Health:    90,
		MaxHealth: 100,
	}

	snaps := buildSyncSnapshotFromModel(model)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 build sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Team != 4 {
		t.Fatalf("expected snapshot team to use build team=4, got %d", snaps[0].Team)
	}
}

func TestFallbackSpawnPosFromModelUsesAnyOwnedCore(t *testing.T) {
	model := world.NewWorldModel(6, 6)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	tile, err := model.TileAt(4, 3)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Block = 343
	tile.Team = 0
	tile.Build = &world.Building{
		Block:     343,
		Team:      2,
		X:         4,
		Y:         3,
		Health:    1200,
		MaxHealth: 1200,
	}

	pos, ok := fallbackSpawnPosFromModel(model)
	if !ok {
		t.Fatal("expected fallback spawn position to resolve")
	}
	if pos != (protocol.Point2{X: 4, Y: 3}) {
		t.Fatalf("unexpected fallback spawn position: %+v", pos)
	}
}

func TestResolveTeamCoreTileHiddenMapMatchesIndexedTeamCores(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "0.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("hidden/0.msav not present in workspace")
		}
		t.Fatalf("stat hidden/0 map: %v", err)
	}

	model, err := worldstream.LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load hidden/0 model: %v", err)
	}

	wld := world.New(world.Config{TPS: 60})
	wld.SetModel(model)

	if team := resolveDefaultPlayerTeam(wld); team != 1 {
		t.Fatalf("expected hidden/0 default player team=1, got %d", team)
	}

	got, ok := resolveTeamCoreTile(wld, 1, protocol.Point2{})
	if !ok {
		t.Fatal("expected hidden/0 to resolve a default team core")
	}
	want := protocol.Point2{X: 164, Y: 79}
	if got != want {
		t.Fatalf("expected hidden/0 default core at %+v, got %+v", want, got)
	}

	team1 := wld.TeamCorePositions(1)
	if len(team1) != 2 {
		t.Fatalf("expected hidden/0 team 1 to have 2 cores, got %d", len(team1))
	}
	team2 := wld.TeamCorePositions(2)
	if len(team2) != 5 {
		t.Fatalf("expected hidden/0 team 2 to have 5 cores, got %d", len(team2))
	}

	checkCore := func(pos protocol.Point2, wantTeam world.TeamID, wantName string) {
		t.Helper()
		tile, err := model.TileAt(int(pos.X), int(pos.Y))
		if err != nil || tile == nil {
			t.Fatalf("lookup core tile %+v failed: %v", pos, err)
		}
		if tile.Build == nil {
			t.Fatalf("expected core tile %+v to have build", pos)
		}
		if tile.Build.X != tile.X || tile.Build.Y != tile.Y {
			t.Fatalf("expected %+v to be a center core tile, got build center=(%d,%d)", pos, tile.Build.X, tile.Build.Y)
		}
		if tile.Build.Team != wantTeam {
			t.Fatalf("expected core %+v team=%d, got %d", pos, wantTeam, tile.Build.Team)
		}
		gotName := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Block)]))
		if gotName != wantName {
			t.Fatalf("expected core %+v block=%q, got %q", pos, wantName, gotName)
		}
	}

	checkCore(protocol.Point2{X: 185, Y: 71}, 1, "core-foundation")
	checkCore(protocol.Point2{X: 164, Y: 79}, 1, "core-foundation")
	checkCore(protocol.Point2{X: 268, Y: 71}, 2, "core-shard")
	checkCore(protocol.Point2{X: 50, Y: 119}, 2, "core-shard")
	checkCore(protocol.Point2{X: 82, Y: 214}, 2, "core-foundation")
	checkCore(protocol.Point2{X: 235, Y: 222}, 2, "core-foundation")
	checkCore(protocol.Point2{X: 172, Y: 261}, 2, "core-nucleus")
}
