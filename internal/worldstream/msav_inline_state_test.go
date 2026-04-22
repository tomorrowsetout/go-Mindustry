package worldstream

import (
	"path/filepath"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func TestHydrateInlineBuildingConfigsRestoresBridgeLink(t *testing.T) {
	model := world.NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		257: "bridge-conveyor",
	}
	src, err := model.TileAt(10, 10)
	if err != nil || src == nil {
		t.Fatalf("source tile lookup failed: %v", err)
	}
	dst, err := model.TileAt(14, 10)
	if err != nil || dst == nil {
		t.Fatalf("target tile lookup failed: %v", err)
	}
	src.Block = 257
	src.Team = 1
	src.Build = &world.Building{
		Block:     257,
		Team:      1,
		X:         10,
		Y:         10,
		Health:    100,
		MaxHealth: 100,
	}
	dst.Block = 257
	dst.Team = 1
	dst.Build = &world.Building{
		Block:     257,
		Team:      1,
		X:         14,
		Y:         10,
		Health:    100,
		MaxHealth: 100,
	}
	w := protocol.NewWriter()
	if err := w.WriteInt32(protocol.PackPoint2(14, 10)); err != nil {
		t.Fatalf("write link failed: %v", err)
	}
	if err := w.WriteFloat32(0.5); err != nil {
		t.Fatalf("write warmup failed: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write incoming count failed: %v", err)
	}
	if err := w.WriteBool(false); err != nil {
		t.Fatalf("write moved failed: %v", err)
	}
	src.Build.MapSyncTail = append([]byte(nil), w.Bytes()...)

	hydrateInlineBuildingConfigs(model)

	value, err := protocol.ReadObject(protocol.NewReader(src.Build.Config), false, nil)
	if err != nil {
		t.Fatalf("decode hydrated config failed: %v", err)
	}
	point, ok := value.(protocol.Point2)
	if !ok {
		t.Fatalf("expected bridge config as Point2, got %T", value)
	}
	if point != (protocol.Point2{X: 4, Y: 0}) {
		t.Fatalf("expected bridge relative link (4,0), got %+v", point)
	}
}

func TestLoadWorldModelFromMSAVHydratesPowerNodeConfig(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "127.msav")
	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load world model failed: %v", err)
	}

	found := false
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		name := model.BlockNames[int16(tile.Block)]
		if name != "power-node" && name != "power-node-large" && name != "surge-tower" && name != "beam-link" && name != "power-source" {
			continue
		}
		if len(tile.Build.Config) == 0 {
			continue
		}
		value, err := protocol.ReadObject(protocol.NewReader(tile.Build.Config), false, nil)
		if err != nil {
			t.Fatalf("decode power-node config failed at (%d,%d): %v", tile.X, tile.Y, err)
		}
		points, ok := value.([]protocol.Point2)
		if !ok || len(points) == 0 {
			continue
		}
		found = true
		break
	}
	if !found {
		t.Fatal("expected at least one map power node to hydrate non-empty []Point2 config from inline MSAV state")
	}
}
