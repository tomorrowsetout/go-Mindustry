package worldstream

import (
	"path/filepath"
	"testing"
)

func TestDiagHidden116WorldStreamChoice(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "116.msav")
	model, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	data, err := readMSAV(path)
	if err != nil {
		t.Fatalf("read msav: %v", err)
	}
	modelPayload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("model payload: %v", err)
	}
	rawPayload, err := buildRawWorldStreamFromMSAVData(path, data, model)
	if err != nil {
		t.Fatalf("raw payload: %v", err)
	}
	finalPayload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("final payload: %v", err)
	}

	modelOK, modelErr := worldStreamPayloadMatchesModel(modelPayload, model)
	rawOK, rawErr := worldStreamPayloadMatchesModel(rawPayload, model)
	finalOK, finalErr := worldStreamPayloadMatchesModel(finalPayload, model)
	t.Logf("model len=%d ok=%v err=%v", len(modelPayload), modelOK, modelErr)
	t.Logf("raw len=%d ok=%v err=%v", len(rawPayload), rawOK, rawErr)
	t.Logf("final len=%d ok=%v err=%v", len(finalPayload), finalOK, finalErr)
}
