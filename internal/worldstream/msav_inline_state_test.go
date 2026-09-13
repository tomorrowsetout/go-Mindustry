package worldstream

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func placeInlineConfigHydrationTestTile(t *testing.T, model *world.WorldModel, x, y int, blockID int16, name string) *world.Tile {
	t.Helper()
	if model.BlockNames == nil {
		model.BlockNames = map[int16]string{}
	}
	model.BlockNames[blockID] = name
	tile, err := model.TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = world.BlockID(blockID)
	tile.Team = 1
	tile.Build = &world.Building{
		Block:     world.BlockID(blockID),
		Team:      1,
		X:         x,
		Y:         y,
		Health:    100,
		MaxHealth: 100,
	}
	return tile
}

func decodeInlineHydratedConfig(t *testing.T, raw []byte) any {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("expected hydrated config bytes")
	}
	value, err := protocol.ReadObject(protocol.NewReader(raw), false, nil)
	if err != nil {
		t.Fatalf("decode hydrated config failed: %v", err)
	}
	return value
}

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

func TestHydrateInlineBuildingConfigsRestoresItemConfigBlocks(t *testing.T) {
	tests := []struct {
		name    string
		blockID int16
		block   string
		itemID  int16
		extra   func(*protocol.Writer) error
	}{
		{name: "item source", blockID: 600, block: "item-source", itemID: 5},
		{name: "sorter", blockID: 601, block: "sorter", itemID: 7},
		{
			name:    "duct unloader",
			blockID: 602,
			block:   "duct-unloader",
			itemID:  9,
			extra: func(w *protocol.Writer) error {
				return w.WriteInt16(3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := world.NewWorldModel(8, 8)
			tile := placeInlineConfigHydrationTestTile(t, model, 3, 4, tt.blockID, tt.block)
			w := protocol.NewWriter()
			if err := w.WriteInt16(tt.itemID); err != nil {
				t.Fatalf("write item id failed: %v", err)
			}
			if tt.extra != nil {
				if err := tt.extra(w); err != nil {
					t.Fatalf("write extra payload failed: %v", err)
				}
			}
			tile.Build.MapSyncTail = append([]byte(nil), w.Bytes()...)

			hydrateInlineBuildingConfigs(model)

			value := decodeInlineHydratedConfig(t, tile.Build.Config)
			item, ok := value.(protocol.Content)
			if !ok {
				t.Fatalf("expected hydrated item config as Content, got %T", value)
			}
			if item.ContentType() != protocol.ContentItem || item.ID() != tt.itemID {
				t.Fatalf("expected item content id %d, got type=%v id=%d", tt.itemID, item.ContentType(), item.ID())
			}
		})
	}
}

func TestHydrateInlineBuildingConfigsRestoresLiquidSourceConfig(t *testing.T) {
	model := world.NewWorldModel(8, 8)
	tile := placeInlineConfigHydrationTestTile(t, model, 2, 5, 610, "liquid-source")
	w := protocol.NewWriter()
	if err := w.WriteInt16(4); err != nil {
		t.Fatalf("write liquid id failed: %v", err)
	}
	tile.Build.MapSyncTail = append([]byte(nil), w.Bytes()...)

	hydrateInlineBuildingConfigs(model)

	value := decodeInlineHydratedConfig(t, tile.Build.Config)
	content, ok := value.(protocol.Content)
	if !ok {
		t.Fatalf("expected hydrated liquid config as Content, got %T", value)
	}
	if content.ContentType() != protocol.ContentLiquid || content.ID() != 4 {
		t.Fatalf("expected liquid content id 4, got type=%v id=%d", content.ContentType(), content.ID())
	}
}

func TestHydrateInlineBuildingConfigsRestoresPayloadRouterConfig(t *testing.T) {
	model := world.NewWorldModel(8, 8)
	tile := placeInlineConfigHydrationTestTile(t, model, 1, 6, 620, "payload-router")
	w := protocol.NewWriter()
	if err := w.WriteByte(byte(protocol.ContentBlock)); err != nil {
		t.Fatalf("write payload content type failed: %v", err)
	}
	if err := w.WriteInt16(342); err != nil {
		t.Fatalf("write payload content id failed: %v", err)
	}
	if err := w.WriteByte(1); err != nil {
		t.Fatalf("write payload router recDir failed: %v", err)
	}
	tile.Build.MapSyncTail = append([]byte(nil), w.Bytes()...)

	hydrateInlineBuildingConfigs(model)

	value := decodeInlineHydratedConfig(t, tile.Build.Config)
	content, ok := value.(protocol.Content)
	if !ok {
		t.Fatalf("expected hydrated payload-router config as Content, got %T", value)
	}
	if content.ContentType() != protocol.ContentBlock || content.ID() != 342 {
		t.Fatalf("expected payload-router block content id 342, got type=%v id=%d", content.ContentType(), content.ID())
	}
}

func TestHydrateInlineBuildingConfigsFeedsRuntimeSorterAndItemSource(t *testing.T) {
	model := world.NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}

	source := placeInlineConfigHydrationTestTile(t, model, 1, 3, 412, "item-source")
	sorter := placeInlineConfigHydrationTestTile(t, model, 2, 3, 500, "sorter")
	forward := placeInlineConfigHydrationTestTile(t, model, 3, 3, 257, "conveyor")
	side := placeInlineConfigHydrationTestTile(t, model, 2, 2, 257, "conveyor")
	side.Rotation = 3
	side.Build.Rotation = 3

	sourceCfg := protocol.NewWriter()
	if err := sourceCfg.WriteInt16(5); err != nil {
		t.Fatalf("write item-source inline config failed: %v", err)
	}
	source.Build.MapSyncTail = append([]byte(nil), sourceCfg.Bytes()...)

	sorterCfg := protocol.NewWriter()
	if err := sorterCfg.WriteInt16(5); err != nil {
		t.Fatalf("write sorter inline config failed: %v", err)
	}
	sorter.Build.MapSyncTail = append([]byte(nil), sorterCfg.Bytes()...)

	hydrateInlineBuildingConfigs(model)

	if len(source.Build.Config) == 0 {
		t.Fatal("expected item-source inline config to hydrate into stored config bytes")
	}
	if len(sorter.Build.Config) == 0 {
		t.Fatal("expected sorter inline config to hydrate into stored config bytes")
	}

	w := world.New(world.Config{TPS: 60})
	w.SetModel(model)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if forward.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected hydrated map sorter/item-source config to move matching item forward")
	}
	if side.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected hydrated map sorter config not to route matching item sideways, got=%d", side.Build.ItemAmount(5))
	}
}

func TestLoadWorldModelFromMSAVHydratesPowerNodeConfig(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "127.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("hidden 127 map not present in workspace")
		}
		t.Fatalf("stat hidden 127 map failed: %v", err)
	}
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
