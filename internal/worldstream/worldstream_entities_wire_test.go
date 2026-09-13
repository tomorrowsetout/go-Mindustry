package worldstream

import (
	"bytes"
	"path/filepath"
	"testing"
)

// Java NetworkIO.loadWorld -> SaveVersion.readEntities expects
// entityMapping (int16) + teamBlocks + worldEntities (int32) after the map.
// Omitting mapping/counts makes the client misread teamBlocks and drop.
func TestWorldStreamWritesEntityMappingAndWorldEntityCount(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	payload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("build stream: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	_, _, mapEnd, entitiesLen, _, _, _, err := inspectWorldSections(raw, contentStart)
	if err != nil {
		t.Fatalf("inspect world sections: %v", err)
	}
	if mapEnd < 0 || entitiesLen <= 0 || mapEnd+entitiesLen > len(raw) {
		t.Fatalf("bad section bounds mapEnd=%d entitiesLen=%d raw=%d", mapEnd, entitiesLen, len(raw))
	}
	entities := raw[mapEnd : mapEnd+entitiesLen]
	r := newJavaReader(entities)
	mapCount, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read entity mapping count: %v", err)
	}
	if mapCount != 0 {
		t.Fatalf("expected empty entity mapping, got %d", mapCount)
	}
	if _, _, err := readModernTeamBlocksRaw(r, entities); err != nil {
		t.Fatalf("read team blocks: %v", err)
	}
	entityCount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read world entity count: %v", err)
	}
	if entityCount != 0 {
		t.Fatalf("expected 0 world entities, got %d", entityCount)
	}
	if r.Offset() != len(entities) {
		t.Fatalf("entity section trailing bytes: offset=%d len=%d", r.Offset(), len(entities))
	}
	if !bytes.Equal(entities[:2], []byte{0, 0}) {
		t.Fatalf("expected leading empty mapping short, got %v", entities[:2])
	}
}
