package worldstream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildWorldStreamFromHidden111MSAVIsInspectable(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "111.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("111.msav not present in workspace")
		}
		t.Fatalf("stat hidden 111 map: %v", err)
	}

	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromMSAV failed: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty world stream payload")
	}
	if _, err := InspectWorldStreamPayload(payload); err != nil {
		t.Fatalf("InspectWorldStreamPayload failed: %v", err)
	}
}
