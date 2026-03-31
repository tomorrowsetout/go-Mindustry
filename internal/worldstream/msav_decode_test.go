package worldstream

import (
	"path/filepath"
	"testing"
)

func TestLoadWorldModelFromMSAVHasOwnedCoreBuild(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load world model: %v", err)
	}
	if model == nil {
		t.Fatalf("expected model")
	}
	found := false
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		if tile.Block <= 0 {
			continue
		}
		name := model.BlockNames[int16(tile.Block)]
		switch name {
		case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis":
			if tile.Build != nil && tile.Build.Team != 0 {
				if tile.Build.Team > 10 {
					t.Fatalf("expected sane core team id, got %d for %s at (%d,%d)", tile.Build.Team, name, tile.X, tile.Y)
				}
				if tile.Team != tile.Build.Team {
					t.Fatalf("expected tile team to match build team for %s at (%d,%d), tile=%d build=%d", name, tile.X, tile.Y, tile.Team, tile.Build.Team)
				}
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected at least one owned core with build state in loaded map")
	}
}
