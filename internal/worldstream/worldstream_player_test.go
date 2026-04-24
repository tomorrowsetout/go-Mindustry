package worldstream

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func contentStartFromWorldStreamRaw(raw []byte) (int, int, error) {
	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return 0, 0, err
	}
	entries, err := r.ReadInt16()
	if err != nil {
		return 0, 0, err
	}
	for i := 0; i < int(entries); i++ {
		if err := r.SkipUTF(); err != nil {
			return 0, 0, err
		}
		if err := r.SkipUTF(); err != nil {
			return 0, 0, err
		}
	}
	if _, err := r.ReadInt32(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadFloat32(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadFloat64(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return 0, 0, err
	}
	if _, err := r.ReadInt32(); err != nil {
		return 0, 0, err
	}

	playerStart := r.Offset()
	playerRev, err := r.ReadInt16()
	if err != nil {
		return 0, 0, err
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return 0, 0, err
	}
	return playerStart, r.Offset(), nil
}

func readWorldStreamRuntimeState(raw []byte) (int32, float32, float64, int32, int, int, error) {
	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	entries, err := r.ReadInt16()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	for i := 0; i < int(entries); i++ {
		if err := r.SkipUTF(); err != nil {
			return 0, 0, 0, 0, 0, 0, err
		}
		if err := r.SkipUTF(); err != nil {
			return 0, 0, 0, 0, 0, 0, err
		}
	}

	wave, err := r.ReadInt32()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	wavetime, err := r.ReadFloat32()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	tick, err := r.ReadFloat64()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	playerID, err := r.ReadInt32()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}

	playerStart := r.Offset()
	playerRev, err := r.ReadInt16()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	return wave, wavetime, tick, playerID, playerStart, r.Offset(), nil
}

func decompressWorldStream(t *testing.T, payload []byte) []byte {
	t.Helper()
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("zlib reader: %v", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read zlib payload: %v", err)
	}
	return raw
}

func readBootstrapWorldRawForTest(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "assets", "bootstrap-world.bin")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("bootstrap-world.bin not present in workspace")
		}
		t.Fatalf("stat bootstrap world: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap world: %v", err)
	}
	return decompressWorldStream(t, data)
}

func directPlayerPayloadForTest(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := writeDirectPlayerPayload(w); err != nil {
		t.Fatalf("write direct player payload: %v", err)
	}
	return out.Bytes()
}

func assertContentStartsCleanly(t *testing.T, raw []byte, start int) {
	t.Helper()
	if _, _, _, _, _, _, _, err := inspectWorldSections(raw, start); err != nil {
		t.Fatalf("world section parse failed at %d: %v", start, err)
	}
}

func readWorldStreamCoreSections(t *testing.T, payload []byte, teamBlocksLen, markersLen, customLen int) ([]byte, []byte, []byte, []byte, []byte, []byte) {
	t.Helper()

	raw := decompressWorldStream(t, payload)
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}

	contentEnd, patchesEnd, mapEnd, builtTeamBlocksLen, builtMarkersLen, builtCustomLen, chunked, err := inspectWorldSections(raw, contentStart)
	if err != nil {
		t.Fatalf("inspect world sections: %v", err)
	}

	var content, patches, mapChunk, builtTeamBlocks, markers, custom []byte
	if chunked {
		r := newJavaReader(raw[contentStart:])
		readChunk := func(name string) []byte {
			chunk, err := r.ReadChunk()
			if err != nil {
				t.Fatalf("read %s chunk: %v", name, err)
			}
			return chunk
		}
		content = readChunk("content")
		patches = readChunk("patches")
		mapChunk = readChunk("map")
		builtTeamBlocks = readChunk("teamBlocks")
		markers = readChunk("markers")
		custom = readChunk("custom")
	} else {
		content = raw[contentStart:contentEnd]
		patches = raw[contentEnd:patchesEnd]
		mapChunk = raw[patchesEnd:mapEnd]
		tail := raw[mapEnd:]
		if builtTeamBlocksLen > len(tail) {
			t.Fatalf("legacy teamBlocks exceed tail: %d > %d", builtTeamBlocksLen, len(tail))
		}
		builtTeamBlocks = tail[:builtTeamBlocksLen]
		tail = tail[builtTeamBlocksLen:]
		if markersLen >= 0 {
			builtMarkersLen = markersLen
		}
		if customLen >= 0 {
			builtCustomLen = customLen
		}
		if builtMarkersLen > len(tail) {
			builtMarkersLen = len(tail)
		}
		markers = tail[:builtMarkersLen]
		tail = tail[builtMarkersLen:]
		if builtCustomLen > len(tail) {
			builtCustomLen = len(tail)
		}
		custom = tail[:builtCustomLen]
	}

	if teamBlocksLen >= 0 && len(builtTeamBlocks) != teamBlocksLen {
		t.Fatalf("unexpected teamBlocks length: got=%d want=%d", len(builtTeamBlocks), teamBlocksLen)
	}
	if markersLen >= 0 && len(markers) != markersLen {
		t.Fatalf("unexpected markers length: got=%d want=%d", len(markers), markersLen)
	}
	if customLen >= 0 && len(custom) != customLen {
		t.Fatalf("unexpected custom length: got=%d want=%d", len(custom), customLen)
	}

	return content, patches, mapChunk, builtTeamBlocks, markers, custom
}

func TestExtractPlayerPayloadFromBootstrapAlignsContentHeader(t *testing.T) {
	raw := readBootstrapWorldRawForTest(t)

	playerStart, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	assertContentStartsCleanly(t, raw, contentStart)

	payload, err := extractPlayerPayloadFromWorldStream(raw)
	if err != nil {
		t.Fatalf("extract player payload: %v", err)
	}
	if !bytes.Equal(payload, raw[playerStart:contentStart]) {
		t.Fatalf("unexpected extracted payload length: got=%d want=%d", len(payload), contentStart-playerStart)
	}
}

func TestBuildWorldStreamFromMSAVAlignsContentHeader(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build world stream: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	assertContentStartsCleanly(t, raw, contentStart)
}

func TestBuildWorldStreamFromLegacyMSAVNormalizesMapChunk(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "erekir", "ravine.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("ravine.msav not present in workspace")
		}
		t.Fatalf("stat ravine msav: %v", err)
	}

	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build world stream: %v", err)
	}
	original, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load original model: %v", err)
	}

	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	if err := skipMapData(newJavaReader(mapChunk)); err != nil {
		t.Fatalf("expected normalized legacy map chunk to parse as current worldstream map, got %v", err)
	}
	decoded, err := decodeMapChunk(mapChunk)
	if err != nil {
		t.Fatalf("decode normalized legacy map chunk: %v", err)
	}
	if decoded.Width != original.Width || decoded.Height != original.Height {
		t.Fatalf("expected normalized legacy worldstream size %dx%d, got %dx%d", original.Width, original.Height, decoded.Width, decoded.Height)
	}
	if len(decoded.Tiles) != len(original.Tiles) {
		t.Fatalf("expected normalized legacy worldstream tile count %d, got %d", len(original.Tiles), len(decoded.Tiles))
	}
	for i := range original.Tiles {
		want := original.Tiles[i]
		got := decoded.Tiles[i]
		if want.Floor != got.Floor || want.Overlay != got.Overlay || want.Block != got.Block || want.Team != got.Team || want.Rotation != got.Rotation {
			t.Fatalf("expected normalized legacy worldstream tile %d to match original model", i)
		}
		if (want.Build != nil) != (got.Build != nil) {
			t.Fatalf("expected normalized legacy worldstream tile %d build presence to match original model", i)
		}
	}
}

func TestBuildWorldStreamFromMSAVPreservesWorldSections(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	data, err := readMSAV(path)
	if err != nil {
		t.Fatalf("read msav: %v", err)
	}

	var expectedTeamBlocks bytes.Buffer
	if err := writeMinimalTeamBlocks(&javaWriter{buf: &expectedTeamBlocks}); err != nil {
		t.Fatalf("write minimal team blocks: %v", err)
	}

	expectedPatches := data.Patches
	if len(expectedPatches) == 0 {
		expectedPatches = []byte{0}
	}
	expectedMarkers := data.Markers
	if len(expectedMarkers) == 0 {
		expectedMarkers = []byte{0x7B, 0x7D}
	}
	expectedCustom := data.Custom
	if len(expectedCustom) == 0 {
		expectedCustom = []byte{0, 0, 0, 0}
	}

	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build world stream: %v", err)
	}

	content, patches, mapChunk, builtTeamBlocks, markers, custom := readWorldStreamCoreSections(
		t,
		payload,
		expectedTeamBlocks.Len(),
		len(expectedMarkers),
		len(expectedCustom),
	)

	if !bytes.Equal(content, data.Content) {
		t.Fatal("expected world stream content header bytes to match msav content chunk")
	}
	if !bytes.Equal(patches, expectedPatches) {
		t.Fatal("expected world stream content patches bytes to match msav patches chunk")
	}
	original, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load original model: %v", err)
	}
	decoded, err := decodeMapChunkForVersion(mapChunk, original.MSAVVersion, original.BlockNames)
	if err != nil {
		t.Fatalf("decode built world stream map chunk: %v", err)
	}
	if decoded.Width != original.Width || decoded.Height != original.Height {
		t.Fatalf("expected normalized world stream map size %dx%d, got %dx%d", original.Width, original.Height, decoded.Width, decoded.Height)
	}
	if len(decoded.Tiles) != len(original.Tiles) {
		t.Fatalf("expected normalized world stream tile count %d, got %d", len(original.Tiles), len(decoded.Tiles))
	}
	for i := range original.Tiles {
		want := original.Tiles[i]
		got := decoded.Tiles[i]
		if want.Floor != got.Floor || want.Overlay != got.Overlay || want.Block != got.Block || want.Team != got.Team || want.Rotation != got.Rotation {
			t.Fatalf("expected normalized world stream tile %d to match original model", i)
		}
		if (want.Build != nil) != (got.Build != nil) {
			t.Fatalf("expected normalized world stream tile %d build presence to match original model", i)
		}
	}
	if !bytes.Equal(builtTeamBlocks, expectedTeamBlocks.Bytes()) {
		t.Fatal("expected world stream team blocks bytes to use minimal legal runtime team block payload")
	}
	if !bytes.Equal(markers, expectedMarkers) {
		t.Fatal("expected world stream markers bytes to match msav markers chunk")
	}
	if !bytes.Equal(custom, expectedCustom) {
		t.Fatal("expected world stream custom bytes to match msav custom chunk")
	}
}

func TestBuildWorldStreamFromMSAVUsesDirectPlayerPayload(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build world stream: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	playerPayload, err := extractPlayerPayloadFromWorldStream(raw)
	if err != nil {
		t.Fatalf("extract built player payload: %v", err)
	}

	expected := directPlayerPayloadForTest(t)
	if !bytes.Equal(playerPayload, expected) {
		t.Fatalf("expected BuildWorldStreamFromMSAV to use direct player payload, got len=%d want=%d", len(playerPayload), len(expected))
	}
}

func TestRewriteRulesInWorldStreamReplacesLeadingRulesBlob(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build world stream: %v", err)
	}

	wantRules := `{"planet":"erekir","waves":true}`
	patched, err := RewriteRulesInWorldStream(payload, wantRules)
	if err != nil {
		t.Fatalf("rewrite rules: %v", err)
	}

	raw := decompressWorldStream(t, patched)
	r := newJavaReader(raw)
	gotRules, err := r.ReadUTF()
	if err != nil {
		t.Fatalf("read rewritten rules: %v", err)
	}
	if gotRules != wantRules {
		t.Fatalf("expected rewritten rules %q, got %q", wantRules, gotRules)
	}
	if _, err := r.ReadUTF(); err != nil {
		t.Fatalf("read locales after rewritten rules: %v", err)
	}
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	assertContentStartsCleanly(t, raw, contentStart)
}

func TestRewritePlayerIDInWorldStreamPreservesContentHeaderBoundary(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "bootstrap-world.bin")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("bootstrap-world.bin not present in workspace")
		}
		t.Fatalf("stat bootstrap world payload: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap world payload: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, _, _, _, playerStart, contentStart, err := readWorldStreamRuntimeState(raw)
	if err != nil {
		t.Fatalf("read original runtime state: %v", err)
	}

	patched, err := RewritePlayerIDInWorldStream(payload, 77)
	if err != nil {
		t.Fatalf("rewrite player id: %v", err)
	}

	patchedRaw := decompressWorldStream(t, patched)
	_, _, _, playerID, patchedPlayerStart, patchedContentStart, err := readWorldStreamRuntimeState(patchedRaw)
	if err != nil {
		t.Fatalf("read patched runtime state: %v", err)
	}

	if playerID != 77 {
		t.Fatalf("expected patched player id 77, got %d", playerID)
	}
	if playerStart != patchedPlayerStart {
		t.Fatalf("expected player payload offset to stay stable, got=%d want=%d", patchedPlayerStart, playerStart)
	}
	if contentStart != patchedContentStart {
		t.Fatalf("expected content header offset to stay stable, got=%d want=%d", patchedContentStart, contentStart)
	}
	if !bytes.Equal(raw[contentStart:], patchedRaw[patchedContentStart:]) {
		t.Fatal("expected content header and following world bytes to remain unchanged after player id rewrite")
	}
	assertContentStartsCleanly(t, patchedRaw, patchedContentStart)
}

func TestRewriteRuntimeStateInWorldStreamPreservesContentHeaderBoundary(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "bootstrap-world.bin")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("bootstrap-world.bin not present in workspace")
		}
		t.Fatalf("stat bootstrap world payload: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap world payload: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, _, _, _, playerStart, contentStart, err := readWorldStreamRuntimeState(raw)
	if err != nil {
		t.Fatalf("read original runtime state: %v", err)
	}

	patched, err := RewriteRuntimeStateInWorldStream(payload, 9, 321.5, 654.25, 123)
	if err != nil {
		t.Fatalf("rewrite runtime state: %v", err)
	}

	patchedRaw := decompressWorldStream(t, patched)
	wave, wavetime, tick, playerID, patchedPlayerStart, patchedContentStart, err := readWorldStreamRuntimeState(patchedRaw)
	if err != nil {
		t.Fatalf("read patched runtime state: %v", err)
	}

	if wave != 9 {
		t.Fatalf("expected patched wave 9, got %d", wave)
	}
	if wavetime != 321.5 {
		t.Fatalf("expected patched wavetime 321.5, got %f", wavetime)
	}
	if tick != 654.25 {
		t.Fatalf("expected patched tick 654.25, got %f", tick)
	}
	if playerID != 123 {
		t.Fatalf("expected patched player id 123, got %d", playerID)
	}
	if playerStart != patchedPlayerStart {
		t.Fatalf("expected player payload offset to stay stable, got=%d want=%d", patchedPlayerStart, playerStart)
	}
	if contentStart != patchedContentStart {
		t.Fatalf("expected content header offset to stay stable, got=%d want=%d", patchedContentStart, contentStart)
	}
	if !bytes.Equal(raw[contentStart:], patchedRaw[patchedContentStart:]) {
		t.Fatal("expected content header and following world bytes to remain unchanged after runtime rewrite")
	}
	assertContentStartsCleanly(t, patchedRaw, patchedContentStart)
}

func TestBuildWorldStreamFromModelSnapshotPreservesInlineBuildingChunk(t *testing.T) {
	payloadWriter := protocol.NewWriter()
	if err := payloadWriter.WriteFloat32(100); err != nil {
		t.Fatalf("write health: %v", err)
	}
	if err := payloadWriter.WriteByte(0x80); err != nil {
		t.Fatalf("write rotation/version flag: %v", err)
	}
	if err := payloadWriter.WriteByte(1); err != nil {
		t.Fatalf("write team: %v", err)
	}
	if err := payloadWriter.WriteByte(3); err != nil {
		t.Fatalf("write building version: %v", err)
	}
	if err := payloadWriter.WriteByte(1); err != nil {
		t.Fatalf("write enabled: %v", err)
	}
	if err := payloadWriter.WriteByte(1 << 3); err != nil {
		t.Fatalf("write module bits: %v", err)
	}
	if err := payloadWriter.WriteByte(255); err != nil {
		t.Fatalf("write efficiency: %v", err)
	}
	if err := payloadWriter.WriteByte(255); err != nil {
		t.Fatalf("write optional efficiency: %v", err)
	}
	if err := payloadWriter.WriteInt32(0); err != nil {
		t.Fatalf("write conveyor item len: %v", err)
	}

	model := world.NewWorldModel(24, 24)
	model.Content = []byte{0}
	model.BlockNames = map[int16]string{
		257: "conveyor",
	}
	tile, err := model.TileAt(6, 6)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Block = 257
	tile.Team = 1
	tile.Rotation = 0
	tile.Build = &world.Building{
		Block:           257,
		Team:            1,
		Rotation:        0,
		X:               6,
		Y:               6,
		Health:          100,
		MaxHealth:       100,
		MapSyncRevision: 1,
		MapSyncData:     append([]byte(nil), payloadWriter.Bytes()...),
	}

	payload, err := BuildWorldStreamFromModelSnapshot(model, 7, world.Snapshot{Wave: 3, WaveTime: 12, Tick: 44})
	if err != nil {
		t.Fatalf("build world stream from model snapshot: %v", err)
	}

	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	decoded, err := decodeMapChunk(mapChunk)
	if err != nil {
		t.Fatalf("decode map chunk: %v", err)
	}
	decodedTile, err := decoded.TileAt(6, 6)
	if err != nil || decodedTile == nil || decodedTile.Build == nil {
		t.Fatalf("decoded conveyor tile missing build: %v", err)
	}
	if decodedTile.Build.MapSyncRevision != 1 {
		t.Fatalf("expected conveyor revision 1, got %d", decodedTile.Build.MapSyncRevision)
	}
	if !bytes.Equal(decodedTile.Build.MapSyncData, payloadWriter.Bytes()) {
		t.Fatalf("expected inline building chunk to survive world stream round-trip, got %v", decodedTile.Build.MapSyncData)
	}
}

func TestBuildWorldStreamFromModelSnapshotPreservesPatchesAndCustom(t *testing.T) {
	model := world.NewWorldModel(4, 4)
	model.Content = []byte{0}
	model.Patches = []byte{1, 0, 0, 0, 1, 0x42}
	model.Markers = []byte{0x7B, 0x7D}
	model.Custom = []byte{0, 0, 0, 1, 0, 0, 0, 0}

	payload, err := BuildWorldStreamFromModelSnapshot(model, 9, world.Snapshot{Wave: 2, WaveTime: 3, Tick: 4})
	if err != nil {
		t.Fatalf("build world stream from model snapshot: %v", err)
	}

	_, patches, _, _, _, custom := readWorldStreamCoreSections(t, payload, -1, len(model.Markers), len(model.Custom))
	if !bytes.Equal(patches, model.Patches) {
		t.Fatalf("expected patches to be preserved, got %v want %v", patches, model.Patches)
	}
	if !bytes.Equal(custom, model.Custom) {
		t.Fatalf("expected custom chunks to be preserved, got %v want %v", custom, model.Custom)
	}
}
