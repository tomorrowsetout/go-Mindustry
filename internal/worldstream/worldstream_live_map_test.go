package worldstream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func TestBuildWorldStreamFromActualLiveMapParses(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "0.msav"),
		filepath.Join("..", "..", "bin", "data", "snapshots", "0-snapshot.msav"),
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					t.Skip("snapshot not present in workspace")
				}
				t.Fatalf("stat world model path: %v", err)
			}

			model, err := LoadWorldModelFromMSAV(path, nil)
			if err != nil {
				t.Fatalf("load world model: %v", err)
			}

			wld := world.New(world.Config{TPS: 60})
			wld.SetModel(model)

			liveModel := wld.CloneModelForWorldStream()
			if liveModel == nil {
				t.Fatal("expected live world model clone")
			}

			payload, err := BuildWorldStreamFromModelSnapshot(liveModel, 7, world.Snapshot{
				Wave:     1,
				WaveTime: 60,
				Tick:     1,
			})
			if err != nil {
				t.Fatalf("build live world stream: %v", err)
			}

			raw := decompressWorldStream(t, payload)
			_, contentStart, err := contentStartFromWorldStreamRaw(raw)
			if err != nil {
				t.Fatalf("locate content start: %v", err)
			}
			assertContentStartsCleanly(t, raw, contentStart)

			_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
			if _, err := decodeMapChunk(mapChunk); err != nil {
				t.Fatalf("decode live map chunk: %v", err)
			}
		})
	}
}

func TestBuildWorldStreamFromActualMapWithBufferedBridgeParses(t *testing.T) {
	path := filepath.Join("..", "..", "bin", "data", "snapshots", "0-snapshot.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("runtime snapshot not present in workspace")
		}
		t.Fatalf("stat runtime snapshot: %v", err)
	}

	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load world model: %v", err)
	}

	wld := world.New(world.Config{TPS: 60})
	wld.SetModel(model)

	found := false
	for i := range wld.Model().Tiles {
		tile := &wld.Model().Tiles[i]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		name := model.BlockNames[int16(tile.Block)]
		if name != "bridge-conveyor" {
			continue
		}
		cfg, ok := wld.BuildingConfigPacked(protocol.PackPoint2(int32(tile.X), int32(tile.Y)))
		if !ok {
			continue
		}
		if _, ok := cfg.(protocol.Point2); !ok {
			continue
		}
		tile.Build.AddItem(0, 1)
		wld.Step(time.Second / 60)
		found = true
		break
	}
	if !found {
		t.Skip("no linked bridge-conveyor found in runtime snapshot")
	}

	liveModel := wld.CloneModelForWorldStream()
	if liveModel == nil {
		t.Fatal("expected live world model clone")
	}
	payload, err := BuildWorldStreamFromModelSnapshot(liveModel, 7, wld.Snapshot())
	if err != nil {
		t.Fatalf("build live world stream: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	assertContentStartsCleanly(t, raw, contentStart)
}

func TestBuildWorldStreamFromHiddenMapPreservesKnownEnemyTiles(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "0.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("hidden/0.msav not present in workspace")
		}
		t.Fatalf("stat hidden/0 map: %v", err)
	}

	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load hidden/0 model: %v", err)
	}

	wld := world.New(world.Config{TPS: 60})
	wld.SetModel(model)
	liveModel := wld.CloneModelForWorldStream()
	if liveModel == nil {
		t.Fatal("expected live world model clone")
	}
	payload, err := BuildWorldStreamFromModelSnapshot(liveModel, 7, wld.Snapshot())
	if err != nil {
		t.Fatalf("build live world stream: %v", err)
	}

	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	decoded, err := decodeMapChunk(mapChunk)
	if err != nil {
		t.Fatalf("decode hidden/0 live map chunk: %v", err)
	}
	decoded.BlockNames = model.BlockNames

	checks := []struct {
		x    int
		y    int
		name string
	}{
		{x: 59, y: 221, name: "blast-drill"},
		{x: 84, y: 212, name: "armored-conveyor"},
		{x: 93, y: 229, name: "graphite-press"},
		{x: 135, y: 162, name: "titanium-conveyor"},
		{x: 180, y: 246, name: "silicon-smelter"},
		{x: 208, y: 224, name: "pyratite-mixer"},
		{x: 241, y: 219, name: "silicon-crucible"},
		{x: 271, y: 85, name: "titanium-conveyor"},
		{x: 283, y: 54, name: "silicon-crucible"},
	}

	for _, tc := range checks {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			origTile, err := model.TileAt(tc.x, tc.y)
			if err != nil || origTile == nil {
				t.Fatalf("lookup original tile (%d,%d): %v", tc.x, tc.y, err)
			}
			decodedTile, err := decoded.TileAt(tc.x, tc.y)
			if err != nil || decodedTile == nil {
				t.Fatalf("lookup decoded tile (%d,%d): %v", tc.x, tc.y, err)
			}

			gotName := strings.ToLower(strings.TrimSpace(decoded.BlockNames[int16(decodedTile.Block)]))
			wantName := strings.ToLower(strings.TrimSpace(tc.name))
			if gotName != wantName {
				t.Fatalf("expected decoded tile (%d,%d) block=%q, got %q", tc.x, tc.y, wantName, gotName)
			}
			if origTile.Build == nil {
				t.Fatalf("expected original tile (%d,%d) to have build", tc.x, tc.y)
			}
			if decodedTile.Build == nil {
				t.Fatalf("expected decoded tile (%d,%d) to have build", tc.x, tc.y)
			}
		})
	}
}

func TestBuildWorldStreamFromHiddenMapPreservesCoreTeams(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "0.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("hidden/0.msav not present in workspace")
		}
		t.Fatalf("stat hidden/0 map: %v", err)
	}

	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load hidden/0 model: %v", err)
	}

	wld := world.New(world.Config{TPS: 60})
	wld.SetModel(model)
	liveModel := wld.CloneModelForWorldStream()
	if liveModel == nil {
		t.Fatal("expected live world model clone")
	}
	payload, err := BuildWorldStreamFromModelSnapshot(liveModel, 7, wld.Snapshot())
	if err != nil {
		t.Fatalf("build live world stream: %v", err)
	}

	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	decoded, err := decodeMapChunk(mapChunk)
	if err != nil {
		t.Fatalf("decode hidden/0 live map chunk: %v", err)
	}
	decoded.BlockNames = model.BlockNames

	checks := []struct {
		x    int
		y    int
		team world.TeamID
		name string
	}{
		{x: 185, y: 71, team: 1, name: "core-foundation"},
		{x: 164, y: 79, team: 1, name: "core-foundation"},
		{x: 268, y: 71, team: 2, name: "core-shard"},
		{x: 50, y: 119, team: 2, name: "core-shard"},
		{x: 82, y: 214, team: 2, name: "core-foundation"},
		{x: 235, y: 222, team: 2, name: "core-foundation"},
		{x: 172, y: 261, team: 2, name: "core-nucleus"},
	}

	for _, tc := range checks {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			decodedTile, err := decoded.TileAt(tc.x, tc.y)
			if err != nil || decodedTile == nil {
				t.Fatalf("lookup decoded core tile (%d,%d): %v", tc.x, tc.y, err)
			}
			if decodedTile.Build == nil {
				t.Fatalf("expected decoded core tile (%d,%d) to have build", tc.x, tc.y)
			}
			if decodedTile.Build.X != decodedTile.X || decodedTile.Build.Y != decodedTile.Y {
				t.Fatalf("expected decoded core tile (%d,%d) to stay center-owned, got build center=(%d,%d)", tc.x, tc.y, decodedTile.Build.X, decodedTile.Build.Y)
			}
			gotName := strings.ToLower(strings.TrimSpace(decoded.BlockNames[int16(decodedTile.Block)]))
			if gotName != tc.name {
				t.Fatalf("expected decoded core tile (%d,%d) block=%q, got %q", tc.x, tc.y, tc.name, gotName)
			}
			if decodedTile.Build.Team != tc.team || decodedTile.Team != tc.team {
				t.Fatalf("expected decoded core tile (%d,%d) team=%d, got build=%d tile=%d", tc.x, tc.y, tc.team, decodedTile.Build.Team, decodedTile.Team)
			}
		})
	}
}

func TestHiddenMapRulesTagPreservesCriticalRules(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "0.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("hidden/0.msav not present in workspace")
		}
		t.Fatalf("stat hidden/0 map: %v", err)
	}

	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load hidden/0 model: %v", err)
	}

	rm := world.NewRulesManager(nil)
	if err := rm.FromTags(model.Tags); err != nil {
		t.Fatalf("parse hidden/0 rules tags: %v", err)
	}

	rules := rm.Get()
	if !rules.AttackMode {
		t.Fatal("expected hidden/0 attackMode=true")
	}
	if !rules.InfiniteResources {
		t.Fatal("expected hidden/0 infiniteResources=true")
	}
	if rules.WaveTimer {
		t.Fatal("expected hidden/0 waveTimer=false")
	}
	if rules.StaticFog {
		t.Fatal("expected hidden/0 staticFog=false")
	}
	team2, ok := rules.TeamInfiniteResources(2), false
	if tr, exists := rules.Teams["2"]; exists {
		ok = true
		if !tr.FillItems {
			t.Fatal("expected hidden/0 teams[2].fillItems=true")
		}
	}
	if !ok {
		t.Fatal("expected hidden/0 to include explicit team 2 rules")
	}
	if !team2 {
		t.Fatal("expected hidden/0 teams[2].infiniteResources=true")
	}
}

func checkWorldStreamMatchesOriginalModel(t *testing.T, label string, original *world.WorldModel, payload []byte) {
	t.Helper()
	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	decoded, err := decodeMapChunk(mapChunk)
	if err != nil {
		t.Fatalf("%s: decode worldstream map chunk: %v", label, err)
	}
	decoded.BlockNames = original.BlockNames

	if decoded.Width != original.Width || decoded.Height != original.Height {
		t.Fatalf("%s: expected map size %dx%d, got %dx%d", label, original.Width, original.Height, decoded.Width, decoded.Height)
	}
	if len(decoded.Tiles) != len(original.Tiles) {
		t.Fatalf("%s: expected %d tiles, got %d", label, len(original.Tiles), len(decoded.Tiles))
	}

	for i := range original.Tiles {
		want := original.Tiles[i]
		got := decoded.Tiles[i]
		if want.Floor != got.Floor || want.Overlay != got.Overlay || want.Block != got.Block || want.Team != got.Team || want.Rotation != got.Rotation {
			x := i % original.Width
			y := i / original.Width
			t.Fatalf("%s: tile mismatch at (%d,%d): want floor=%d overlay=%d block=%d team=%d rot=%d got floor=%d overlay=%d block=%d team=%d rot=%d",
				label, x, y, want.Floor, want.Overlay, want.Block, want.Team, want.Rotation,
				got.Floor, got.Overlay, got.Block, got.Team, got.Rotation)
		}
		wantBuild := want.Build != nil
		gotBuild := got.Build != nil
		if wantBuild != gotBuild {
			x := i % original.Width
			y := i / original.Width
			t.Fatalf("%s: build presence mismatch at (%d,%d): want=%v got=%v", label, x, y, wantBuild, gotBuild)
		}
		if wantBuild && gotBuild {
			if want.Build.X != got.Build.X || want.Build.Y != got.Build.Y || want.Build.Team != got.Build.Team || want.Build.Rotation != got.Build.Rotation {
				x := i % original.Width
				y := i / original.Width
				t.Fatalf("%s: build mismatch at (%d,%d): want center=(%d,%d) team=%d rot=%d got center=(%d,%d) team=%d rot=%d",
					label, x, y,
					want.Build.X, want.Build.Y, want.Build.Team, want.Build.Rotation,
					got.Build.X, got.Build.Y, got.Build.Team, got.Build.Rotation)
			}
		}
	}
}

func loadWeatheredChannelsModel(t *testing.T) *world.WorldModel {
	t.Helper()
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "weatheredChannels.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("weatheredChannels.msav not present in workspace")
		}
		t.Fatalf("stat weatheredChannels map: %v", err)
	}

	original, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load original model: %v", err)
	}
	return original
}

func TestBuildWorldStreamFromWeatheredChannelsPreservesMapTiles(t *testing.T) {
	original := loadWeatheredChannelsModel(t)
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "weatheredChannels.msav")
	rawPayload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build raw world stream: %v", err)
	}
	checkWorldStreamMatchesOriginalModel(t, "raw-msav", original, rawPayload)
}

func TestBuildWorldStreamFromWeatheredChannelsModelPreservesMapTiles(t *testing.T) {
	original := loadWeatheredChannelsModel(t)
	modelPayload, err := BuildWorldStreamFromModel(original, 1)
	if err != nil {
		t.Fatalf("build model world stream: %v", err)
	}
	checkWorldStreamMatchesOriginalModel(t, "model", original, modelPayload)
}
