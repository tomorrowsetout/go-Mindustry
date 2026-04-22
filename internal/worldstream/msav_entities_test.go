package worldstream

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func buildUnitSaveEntityEntry(t *testing.T, unit *protocol.UnitEntitySync) []byte {
	t.Helper()
	writer := protocol.NewWriter()
	if err := writer.WriteByte(unit.ClassID()); err != nil {
		t.Fatalf("write class id: %v", err)
	}
	if err := writer.WriteInt32(unit.ID()); err != nil {
		t.Fatalf("write entity id: %v", err)
	}
	if err := unit.WriteEntity(writer); err != nil {
		t.Fatalf("write entity payload: %v", err)
	}
	return append([]byte(nil), writer.Bytes()...)
}

func buildMSAVEntitiesChunkForTest(t *testing.T, teamBlocks []byte, entries ...[]byte) []byte {
	t.Helper()
	if teamBlocks == nil {
		teamBlocks = []byte{0, 0, 0, 0}
	}
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt16(0); err != nil {
		t.Fatalf("write entity mapping count: %v", err)
	}
	if err := w.WriteBytes(teamBlocks); err != nil {
		t.Fatalf("write team blocks: %v", err)
	}
	if err := w.WriteInt32(int32(len(entries))); err != nil {
		t.Fatalf("write world entity count: %v", err)
	}
	for _, entry := range entries {
		if err := w.WriteInt32(int32(len(entry))); err != nil {
			t.Fatalf("write entity chunk size: %v", err)
		}
		if err := w.WriteBytes(entry); err != nil {
			t.Fatalf("write entity chunk payload: %v", err)
		}
	}
	return out.Bytes()
}

func TestDecodeEntitiesChunkReadsChunkedWorldUnits(t *testing.T) {
	unit := &protocol.UnitEntitySync{
		IDValue:        77,
		Ammo:           3,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 9},
		Health:         42,
		MineTile:       protocol.TileBox{PosValue: 11},
		Shield:         5,
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 4}, Amount: 6},
		TeamID:         2,
		TypeID:         1,
		UpdateBuilding: true,
		Vel:            protocol.Vec2{X: 1.5, Y: -2.5},
		X:              40,
		Y:              56,
	}
	unit.ApplyLayoutByName("dagger")
	raw := buildMSAVEntitiesChunkForTest(t, nil, buildUnitSaveEntityEntry(t, unit))

	model := world.NewWorldModel(8, 8)
	model.UnitNames = map[int16]string{1: "dagger"}
	if err := decodeEntitiesChunk(raw, model); err != nil {
		t.Fatalf("decode entities chunk: %v", err)
	}
	if len(model.Entities) != 1 {
		t.Fatalf("expected 1 decoded entity, got %d", len(model.Entities))
	}
	got := model.Entities[0]
	if got.ID != 77 || got.TypeID != 1 || got.Team != 2 {
		t.Fatalf("unexpected entity identity: %+v", got)
	}
	if got.Health != 42 || got.PlayerID != 9 || got.MineTilePos != 11 {
		t.Fatalf("unexpected entity runtime state: %+v", got)
	}
	if got.Stack.Item != 4 || got.Stack.Amount != 6 {
		t.Fatalf("unexpected stack: %+v", got.Stack)
	}
	if got.X != 40 || got.Y != 56 || got.VelX != 1.5 || got.VelY != -2.5 {
		t.Fatalf("unexpected transform: %+v", got)
	}
	if model.NextEntityID != 78 {
		t.Fatalf("expected next entity id 78, got %d", model.NextEntityID)
	}
	for i, tile := range model.Tiles {
		if tile.Build != nil {
			t.Fatalf("decodeEntitiesChunk must not overwrite map buildings, tile=%d build=%+v", i, tile.Build)
		}
	}
}

func TestWriteEntitiesChunkFromModelPreservesUnknownChunksAndRoundTripsUnits(t *testing.T) {
	unit := &protocol.UnitEntitySync{
		IDValue:    10,
		Controller: &protocol.ControllerState{Type: protocol.ControllerGenericAI},
		Health:     25,
		Shield:     3,
		Ammo:       4,
		MineTile:   protocol.TileBox{PosValue: 12},
		Stack:      protocol.ItemStack{Item: protocol.ItemRef{ItmID: 5}, Amount: 2},
		TeamID:     2,
		TypeID:     1,
		Vel:        protocol.Vec2{X: 0.5, Y: 1.25},
		X:          24,
		Y:          32,
	}
	unit.ApplyLayoutByName("dagger")
	unknown := []byte{99, 0, 0, 1, 44, 7, 8, 9}
	model := world.NewWorldModel(8, 8)
	model.UnitNames = map[int16]string{1: "dagger"}
	model.RawEntities = buildMSAVEntitiesChunkForTest(t, nil, buildUnitSaveEntityEntry(t, unit), unknown)
	model.Entities = []world.RawEntity{world.RawEntityFromUnitEntitySave(unit)}

	rebuilt, err := writeEntitiesChunkFromModel(model)
	if err != nil {
		t.Fatalf("write entities chunk: %v", err)
	}
	_, _, chunks, err := splitMSAVEntitiesChunk(rebuilt)
	if err != nil {
		t.Fatalf("split rebuilt entities chunk: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected rebuilt chunk to keep 2 world entities, got %d", len(chunks))
	}
	var keptUnknown bool
	for _, chunk := range chunks {
		if chunk.ClassID == 99 && bytes.Equal(chunk.Raw, unknown) {
			keptUnknown = true
		}
	}
	if !keptUnknown {
		t.Fatal("expected unsupported world-entity chunks to be preserved verbatim")
	}

	roundtrip := world.NewWorldModel(8, 8)
	roundtrip.UnitNames = map[int16]string{1: "dagger"}
	if err := decodeEntitiesChunk(rebuilt, roundtrip); err != nil {
		t.Fatalf("decode rebuilt entities chunk: %v", err)
	}
	if len(roundtrip.Entities) != 1 {
		t.Fatalf("expected one decoded unit after roundtrip, got %d", len(roundtrip.Entities))
	}
	got := roundtrip.Entities[0]
	if got.ID != 10 || got.TypeID != 1 || got.Team != 2 {
		t.Fatalf("unexpected roundtrip entity identity: %+v", got)
	}
	if got.Health != 25 || got.Shield != 3 || got.Ammo != 4 {
		t.Fatalf("unexpected roundtrip combat state: %+v", got)
	}
	if got.Stack.Item != 5 || got.Stack.Amount != 2 || got.MineTilePos != 12 {
		t.Fatalf("unexpected roundtrip inventory/mining state: %+v", got)
	}
}

func TestWriteMSAVFromModelPreservesLegacyEntityEncodingForLegacyVersions(t *testing.T) {
	model := world.NewWorldModel(1, 1)
	model.MSAVVersion = 6
	model.Tags = map[string]string{"rules": "{}", "locales": "{}"}
	model.Content = buildMSAVContentChunk(t)
	model.RawMap = buildMSAVMapChunk(t, 6)
	model.RawEntities = buildShortEntitySectionRaw(t, true, true)

	path := filepath.Join(t.TempDir(), "legacy-save6.msav")
	if err := WriteMSAVFromModel(path, model, nil); err != nil {
		t.Fatalf("WriteMSAVFromModel: %v", err)
	}
	defer os.Remove(path)

	data, err := readMSAV(path)
	if err != nil {
		t.Fatalf("readMSAV: %v", err)
	}
	if data.Version != 6 {
		t.Fatalf("expected version 6, got %d", data.Version)
	}
	if !bytes.Equal(data.RawEntities, model.RawEntities) {
		t.Fatal("expected legacy save write to preserve legacy entity bytes")
	}
	if !data.WorldEntitiesShortChunks {
		t.Fatal("expected preserved legacy entity section to remain short-chunk encoded")
	}
}
