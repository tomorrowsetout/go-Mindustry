package worldstream

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mdt-server/internal/protocol"
)

func writeMSAVChunk(t *testing.T, raw *bytes.Buffer, chunk []byte) {
	t.Helper()
	if err := binary.Write(raw, binary.BigEndian, int32(len(chunk))); err != nil {
		t.Fatalf("write chunk length: %v", err)
	}
	if _, err := raw.Write(chunk); err != nil {
		t.Fatalf("write chunk data: %v", err)
	}
}

func buildMSAVMetaChunk(t *testing.T, tags map[string]string) []byte {
	t.Helper()
	var meta bytes.Buffer
	w := &javaWriter{buf: &meta}
	if err := w.WriteStringMap(tags); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	return meta.Bytes()
}

func buildMSAVContentChunk(t *testing.T) []byte {
	t.Helper()
	var content bytes.Buffer
	w := &javaWriter{buf: &content}
	if err := w.WriteByte(2); err != nil { // mapped content types: block + unit
		t.Fatalf("write mapped count: %v", err)
	}
	if err := w.WriteByte(1); err != nil { // block
		t.Fatalf("write block content type: %v", err)
	}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write block total: %v", err)
	}
	if err := w.WriteUTF("air"); err != nil {
		t.Fatalf("write block name: %v", err)
	}
	if err := w.WriteByte(6); err != nil { // unit
		t.Fatalf("write unit content type: %v", err)
	}
	if err := w.WriteInt16(36); err != nil {
		t.Fatalf("write unit total: %v", err)
	}
	for i := 0; i < 35; i++ {
		if err := w.WriteUTF(fmt.Sprintf("unit-%d", i)); err != nil {
			t.Fatalf("write filler unit name: %v", err)
		}
	}
	if err := w.WriteUTF("alpha"); err != nil {
		t.Fatalf("write unit name alpha: %v", err)
	}
	return content.Bytes()
}

func buildMSAVContentChunkWithBlocks(t *testing.T, blocks []string) []byte {
	t.Helper()
	var content bytes.Buffer
	w := &javaWriter{buf: &content}
	if err := w.WriteByte(1); err != nil {
		t.Fatalf("write mapped count: %v", err)
	}
	if err := w.WriteByte(1); err != nil {
		t.Fatalf("write block content type: %v", err)
	}
	if err := w.WriteInt16(int16(len(blocks))); err != nil {
		t.Fatalf("write block total: %v", err)
	}
	for _, name := range blocks {
		if err := w.WriteUTF(name); err != nil {
			t.Fatalf("write block name %q: %v", name, err)
		}
	}
	return content.Bytes()
}

func buildMSAVMapChunk(t *testing.T, version int32) []byte {
	t.Helper()
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write width: %v", err)
	}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write height: %v", err)
	}
	if err := w.WriteInt16(0); err != nil {
		t.Fatalf("write floor: %v", err)
	}
	if err := w.WriteInt16(0); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write floor run: %v", err)
	}
	if err := w.WriteInt16(0); err != nil {
		t.Fatalf("write block id: %v", err)
	}
	if version <= 5 {
		if err := w.WriteByte(0); err != nil {
			t.Fatalf("write legacy block run: %v", err)
		}
		return out.Bytes()
	}
	if err := w.WriteByte(0); err != nil { // packed flags
		t.Fatalf("write packed flags: %v", err)
	}
	if err := w.WriteByte(0); err != nil { // block run
		t.Fatalf("write block run: %v", err)
	}
	return out.Bytes()
}

func buildSingleTileMSAVMapChunk(t *testing.T, floor, overlay, block int16) []byte {
	t.Helper()
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write width: %v", err)
	}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write height: %v", err)
	}
	if err := w.WriteInt16(floor); err != nil {
		t.Fatalf("write floor: %v", err)
	}
	if err := w.WriteInt16(overlay); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write floor run: %v", err)
	}
	if err := w.WriteInt16(block); err != nil {
		t.Fatalf("write block id: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write packed flags: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write block run: %v", err)
	}
	return out.Bytes()
}

func buildMSAVWithContentAndMap(t *testing.T, contentChunk, mapChunk []byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	if _, err := raw.WriteString("MSAV"); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	if err := binary.Write(&raw, binary.BigEndian, int32(11)); err != nil {
		t.Fatalf("write version: %v", err)
	}
	writeMSAVChunk(t, &raw, buildMSAVMetaChunk(t, map[string]string{
		"name":     "content-mapping",
		"rules":    "{}",
		"locales":  "{}",
		"wave":     "1",
		"wavetime": "0",
		"tick":     "0",
	}))
	writeMSAVChunk(t, &raw, contentChunk)
	writeMSAVChunk(t, &raw, buildMSAVPatchesChunk(t))
	writeMSAVChunk(t, &raw, mapChunk)
	writeMSAVChunk(t, &raw, buildEntitiesChunkForVersion(t, 11))
	writeMSAVChunk(t, &raw, buildMSAVMarkersChunk(t))
	writeMSAVChunk(t, &raw, buildMSAVCustomChunk(t))

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatalf("compress msav: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}
	return compressed.Bytes()
}

func buildEntityPayload(t *testing.T) []byte {
	t.Helper()
	unit := &protocol.UnitEntitySync{
		IDValue:        777,
		ClassIDValue:   0,
		ClassIDSet:     true,
		Ammo:           1,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerFormation},
		Health:         10,
		MineTile:       nil,
		Mounts:         []protocol.WeaponMount{},
		Payloads:       []protocol.Payload{},
		Plans:          []*protocol.BuildPlan{},
		Statuses:       []protocol.StatusEntry{},
		TeamID:         1,
		TypeID:         35,
		UpdateBuilding: false,
		Vel:            protocol.Vec2{},
		X:              8,
		Y:              8,
	}
	unit.ApplyLayoutByName("alpha")
	writer := protocol.NewWriter()
	if err := unit.WriteEntity(writer); err != nil {
		t.Fatalf("write entity payload: %v", err)
	}
	return writer.Bytes()
}

func buildLegacyEntityGroupsChunk(t *testing.T) []byte {
	t.Helper()
	return []byte{0}
}

func buildMSAVPatchesChunk(t *testing.T) []byte {
	t.Helper()
	return []byte{1, 0, 0, 0, 1, 0x42}
}

func buildMSAVMarkersChunk(t *testing.T) []byte {
	t.Helper()
	return []byte{0x7B, 0x53, 0x7D}
}

func buildMSAVCustomChunk(t *testing.T) []byte {
	t.Helper()
	return []byte{0, 0, 0, 1, 0, 0, 0, 0}
}

func buildSave3TeamBlocksRaw(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := binary.Write(&out, binary.BigEndian, int32(1)); err != nil {
		t.Fatalf("write team count: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, int32(1)); err != nil {
		t.Fatalf("write team id: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, int32(1)); err != nil {
		t.Fatalf("write block count: %v", err)
	}
	for _, v := range []int16{3, 4, 1, 0} {
		if err := binary.Write(&out, binary.BigEndian, v); err != nil {
			t.Fatalf("write save3 team plan: %v", err)
		}
	}
	if err := binary.Write(&out, binary.BigEndian, int32(123)); err != nil {
		t.Fatalf("write save3 config: %v", err)
	}
	return out.Bytes()
}

func buildModernTeamBlocksRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := encodeTeamBlocks([]msavTeamBlockPlan{{
		teamID: 1,
		x:      3,
		y:      4,
		rot:    1,
		block:  0,
		config: int32(123),
	}})
	if err != nil {
		t.Fatalf("encode team blocks: %v", err)
	}
	return raw
}

func buildEntityMappingRaw(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write mapping count: %v", err)
	}
	if err := w.WriteInt16(0); err != nil {
		t.Fatalf("write mapping id: %v", err)
	}
	if err := w.WriteUTF("alpha"); err != nil {
		t.Fatalf("write mapping name: %v", err)
	}
	return out.Bytes()
}

func buildShortEntitySectionRaw(t *testing.T, withMapping, withIDs bool) []byte {
	t.Helper()
	payload := buildEntityPayload(t)
	var out bytes.Buffer
	if withMapping {
		if _, err := out.Write(buildEntityMappingRaw(t)); err != nil {
			t.Fatalf("write mapping raw: %v", err)
		}
	}
	if _, err := out.Write(buildModernTeamBlocksRaw(t)); err != nil {
		t.Fatalf("write team blocks raw: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, int32(1)); err != nil {
		t.Fatalf("write entity count: %v", err)
	}
	var chunk bytes.Buffer
	if err := chunk.WriteByte(0); err != nil {
		t.Fatalf("write class id: %v", err)
	}
	if withIDs {
		if err := binary.Write(&chunk, binary.BigEndian, int32(777)); err != nil {
			t.Fatalf("write entity id: %v", err)
		}
	}
	if _, err := chunk.Write(payload); err != nil {
		t.Fatalf("write entity body: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, uint16(chunk.Len())); err != nil {
		t.Fatalf("write short chunk length: %v", err)
	}
	if _, err := out.Write(chunk.Bytes()); err != nil {
		t.Fatalf("write short chunk payload: %v", err)
	}
	return out.Bytes()
}

func buildModernEntitySectionRaw(t *testing.T) []byte {
	t.Helper()
	payload := buildEntityPayload(t)
	var out bytes.Buffer
	if _, err := out.Write(buildEntityMappingRaw(t)); err != nil {
		t.Fatalf("write mapping raw: %v", err)
	}
	if _, err := out.Write(buildModernTeamBlocksRaw(t)); err != nil {
		t.Fatalf("write team blocks raw: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, int32(1)); err != nil {
		t.Fatalf("write entity count: %v", err)
	}
	var chunk bytes.Buffer
	if err := chunk.WriteByte(0); err != nil {
		t.Fatalf("write class id: %v", err)
	}
	if err := binary.Write(&chunk, binary.BigEndian, int32(777)); err != nil {
		t.Fatalf("write entity id: %v", err)
	}
	if _, err := chunk.Write(payload); err != nil {
		t.Fatalf("write entity body: %v", err)
	}
	if err := binary.Write(&out, binary.BigEndian, int32(chunk.Len())); err != nil {
		t.Fatalf("write int chunk length: %v", err)
	}
	if _, err := out.Write(chunk.Bytes()); err != nil {
		t.Fatalf("write int chunk payload: %v", err)
	}
	return out.Bytes()
}

func buildEntitiesChunkForVersion(t *testing.T, version int32) []byte {
	t.Helper()
	switch version {
	case 1, 2:
		return buildLegacyEntityGroupsChunk(t)
	case 3:
		var out bytes.Buffer
		if _, err := out.Write(buildSave3TeamBlocksRaw(t)); err != nil {
			t.Fatalf("write save3 team blocks: %v", err)
		}
		if _, err := out.Write(buildLegacyEntityGroupsChunk(t)); err != nil {
			t.Fatalf("write save3 groups: %v", err)
		}
		return out.Bytes()
	case 4:
		return buildShortEntitySectionRaw(t, false, false)
	case 5:
		return buildShortEntitySectionRaw(t, true, false)
	case 6, 7, 8, 9:
		return buildShortEntitySectionRaw(t, true, true)
	default:
		return buildModernEntitySectionRaw(t)
	}
}

func buildLegacyLoadableMSAV(t *testing.T, version int32) []byte {
	t.Helper()

	var raw bytes.Buffer
	if _, err := raw.WriteString("MSAV"); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	if err := binary.Write(&raw, binary.BigEndian, version); err != nil {
		t.Fatalf("write version: %v", err)
	}
	writeMSAVChunk(t, &raw, buildMSAVMetaChunk(t, map[string]string{
		"name":     "legacy",
		"rules":    "{}",
		"locales":  "{}",
		"wave":     "1",
		"wavetime": "0",
		"tick":     "0",
	}))
	writeMSAVChunk(t, &raw, buildMSAVContentChunk(t))
	if version >= 11 {
		writeMSAVChunk(t, &raw, buildMSAVPatchesChunk(t))
	}
	writeMSAVChunk(t, &raw, buildMSAVMapChunk(t, version))
	writeMSAVChunk(t, &raw, buildEntitiesChunkForVersion(t, version))
	if version >= 8 {
		writeMSAVChunk(t, &raw, buildMSAVMarkersChunk(t))
	}
	if version >= 7 {
		writeMSAVChunk(t, &raw, buildMSAVCustomChunk(t))
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatalf("compress msav: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}
	return compressed.Bytes()
}

func TestLegacyMSAVVersionsBuildWorldStreamAndModel(t *testing.T) {
	for version := int32(1); version <= 11; version++ {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.msav")
			if err := os.WriteFile(path, buildLegacyLoadableMSAV(t, version), 0o600); err != nil {
				t.Fatalf("write test msav: %v", err)
			}

			gotVersion, err := ReadMSAVVersion(path)
			if err != nil {
				t.Fatalf("ReadMSAVVersion: %v", err)
			}
			if gotVersion != version {
				t.Fatalf("expected version %d, got %d", version, gotVersion)
			}

			data, err := readMSAV(path)
			if err != nil {
				t.Fatalf("readMSAV: %v", err)
			}
			if data.Version != version {
				t.Fatalf("expected data version %d, got %d", version, data.Version)
			}
			switch {
			case version >= 11:
				if !bytes.Equal(data.Patches, buildMSAVPatchesChunk(t)) {
					t.Fatalf("expected version %d to read patches chunk", version)
				}
			case len(data.Patches) != 0:
				t.Fatalf("expected version %d to leave patches empty", version)
			}
			switch {
			case version >= 8:
				if !bytes.Equal(data.Markers, buildMSAVMarkersChunk(t)) {
					t.Fatalf("expected version %d to read markers chunk", version)
				}
			case len(data.Markers) != 0:
				t.Fatalf("expected version %d to leave markers empty", version)
			}
			switch {
			case version >= 7:
				if !bytes.Equal(data.Custom, buildMSAVCustomChunk(t)) {
					t.Fatalf("expected version %d to read custom chunk", version)
				}
			case len(data.Custom) != 0:
				t.Fatalf("expected version %d to leave custom empty", version)
			}
			switch version {
			case 1, 2:
				if len(data.TeamBlocks) != 0 {
					t.Fatalf("expected no team blocks for version %d", version)
				}
				if len(data.WorldEntityChunks) != 0 {
					t.Fatalf("expected no parsed world entities for version %d", version)
				}
			case 3:
				if len(data.TeamBlocks) == 0 {
					t.Fatalf("expected converted team blocks for version 3")
				}
				if len(data.WorldEntityChunks) != 0 {
					t.Fatalf("expected no parsed world entities for version 3")
				}
			case 4:
				if len(data.TeamBlocks) == 0 || data.WorldEntitiesHaveIDs || !data.WorldEntitiesShortChunks {
					t.Fatalf("unexpected entity metadata for version 4: team=%d ids=%v short=%v", len(data.TeamBlocks), data.WorldEntitiesHaveIDs, data.WorldEntitiesShortChunks)
				}
				if len(data.WorldEntityChunks) != 1 {
					t.Fatalf("expected one world entity chunk for version 4, got %d", len(data.WorldEntityChunks))
				}
			case 5:
				if len(data.EntityMapping) == 0 || len(data.TeamBlocks) == 0 || data.WorldEntitiesHaveIDs || !data.WorldEntitiesShortChunks {
					t.Fatalf("unexpected entity metadata for version 5")
				}
			case 6, 7, 8, 9:
				if len(data.EntityMapping) == 0 || len(data.TeamBlocks) == 0 || !data.WorldEntitiesHaveIDs || !data.WorldEntitiesShortChunks {
					t.Fatalf("unexpected entity metadata for version %d", version)
				}
			case 10, 11:
				if len(data.EntityMapping) == 0 || len(data.TeamBlocks) == 0 || !data.WorldEntitiesHaveIDs || data.WorldEntitiesShortChunks {
					t.Fatalf("unexpected entity metadata for version %d", version)
				}
			}

			model, err := LoadWorldModelFromMSAV(path, nil)
			if err != nil {
				t.Fatalf("LoadWorldModelFromMSAV: %v", err)
			}
			if model == nil || model.Width != 1 || model.Height != 1 {
				t.Fatalf("expected 1x1 model for version %d, got %+v", version, model)
			}
			switch {
			case version >= 3:
				if len(model.TeamBlocks) == 0 {
					t.Fatalf("expected model to preserve team blocks for version %d", version)
				}
			case len(model.TeamBlocks) != 0:
				t.Fatalf("expected version %d model to have no team blocks", version)
			}
			if version >= 4 && len(model.Entities) != 1 {
				t.Fatalf("expected one decoded entity for version %d, got %d", version, len(model.Entities))
			}

			payload, err := BuildWorldStreamFromMSAV(path)
			if err != nil {
				t.Fatalf("BuildWorldStreamFromMSAV: %v", err)
			}
			if len(payload) == 0 {
				t.Fatalf("expected world stream payload for version %d", version)
			}
		})
	}
}

type testContent struct {
	typ  protocol.ContentType
	id   int16
	name string
}

func (c testContent) ContentType() protocol.ContentType { return c.typ }
func (c testContent) ID() int16                         { return c.id }
func (c testContent) Name() string                      { return c.name }

func TestLoadWorldModelFromMSAVPreservesMapContentHeaderOverRegistry(t *testing.T) {
	contentChunk := buildMSAVContentChunkWithBlocks(t, []string{
		"air",
		"sand-floor",
		"copper-wall",
	})
	mapChunk := buildSingleTileMSAVMapChunk(t, 1, 0, 2)
	path := filepath.Join(t.TempDir(), "content-mapping.msav")
	if err := os.WriteFile(path, buildMSAVWithContentAndMap(t, contentChunk, mapChunk), 0o600); err != nil {
		t.Fatalf("write test msav: %v", err)
	}

	reg := protocol.NewContentRegistry()
	reg.RegisterBlock(testContent{typ: protocol.ContentBlock, id: 0, name: "air"})
	reg.RegisterBlock(testContent{typ: protocol.ContentBlock, id: 1, name: "duo"})
	reg.RegisterBlock(testContent{typ: protocol.ContentBlock, id: 2, name: "copper-wall"})

	model, err := LoadWorldModelFromMSAV(path, reg)
	if err != nil {
		t.Fatalf("LoadWorldModelFromMSAV: %v", err)
	}
	if got := model.BlockNames[1]; got != "sand-floor" {
		t.Fatalf("expected map content header to keep block id 1 as sand-floor, got %q", got)
	}
	if !bytes.Equal(model.Content, contentChunk) {
		t.Fatal("expected model content header to remain the map-local header")
	}

	payload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromModel: %v", err)
	}
	streamContent, _, streamMap, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	if !bytes.Equal(streamContent, contentChunk) {
		t.Fatal("expected world stream to preserve map-local content header")
	}
	decoded, err := decodeMapChunk(streamMap)
	if err != nil {
		t.Fatalf("decode stream map chunk: %v", err)
	}
	tile, err := decoded.TileAt(0, 0)
	if err != nil || tile == nil {
		t.Fatalf("lookup decoded tile: %v", err)
	}
	if tile.Floor != 1 {
		t.Fatalf("expected decoded floor id 1, got %d", tile.Floor)
	}
}

func TestLoadLegacyMSAVModelBuildWorldStreamUsesMinimalTeamBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save3.msav")
	if err := os.WriteFile(path, buildLegacyLoadableMSAV(t, 3), 0o600); err != nil {
		t.Fatalf("write test msav: %v", err)
	}

	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("LoadWorldModelFromMSAV: %v", err)
	}
	if len(model.TeamBlocks) == 0 {
		t.Fatal("expected converted team blocks on loaded model")
	}

	payload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromModel: %v", err)
	}

	var expected bytes.Buffer
	if err := writeMinimalTeamBlocks(&javaWriter{buf: &expected}); err != nil {
		t.Fatalf("write minimal team blocks: %v", err)
	}
	_, _, _, builtTeamBlocks, _, _ := readWorldStreamCoreSections(t, payload, expected.Len(), 2, 4)
	if !bytes.Equal(builtTeamBlocks, expected.Bytes()) {
		t.Fatal("expected model-built world stream to use minimal legal runtime team blocks")
	}
}
