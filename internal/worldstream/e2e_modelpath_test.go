package worldstream

import (
	"path/filepath"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
)

// TestModelPathWorldStreamLayoutMatchesClient verifies the ACTUAL production
// path (LoadWorldModelFromMSAV with vanilla content remap -> BuildWorldStreamFromModel)
// produces a stream whose content header is immediately followed by the map
// chunk (zero gap), matching what the Java client's loadWorld expects.
func TestModelPathWorldStreamLayoutMatchesClient(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")

	ids, err := vanilla.LoadContentIDs(filepath.Join("..", "..", "data", "vanilla", "content_ids.json"))
	if err != nil {
		t.Fatalf("load content ids: %v", err)
	}
	reg := protocol.NewContentRegistry()
	vanilla.ApplyContentIDs(reg, ids)

	model, err := LoadWorldModelFromMSAV(path, reg)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}

	payload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("build world stream from model: %v", err)
	}

	raw := decompressWorldStream(t, payload)
	_, contentStart, err := contentStartFromWorldStreamRaw(raw)
	if err != nil {
		t.Fatalf("locate content start: %v", err)
	}
	contentEnd, patchesEnd, mapEnd, _, _, _, chunked, err := inspectWorldSections(raw, contentStart)
	if err != nil {
		t.Fatalf("inspect world sections: %v", err)
	}
	if chunked {
		t.Fatal("expected raw network layout")
	}
	if patchesEnd != contentEnd {
		t.Fatalf("model path: patchesEnd %d must equal contentEnd %d (no gap)", patchesEnd, contentEnd)
	}
	if mapEnd <= contentEnd {
		t.Fatalf("model path: empty map chunk")
	}
}
