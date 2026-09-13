package worldstream

import (
	"bytes"
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

func TestBuildWorldStreamFromHidden55MSAVPreservesRawMapChunk(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "55.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("55.msav not present in workspace")
		}
		t.Fatalf("stat hidden 55 map: %v", err)
	}

	data, err := readMSAV(path)
	if err != nil {
		t.Fatalf("read hidden 55 msav: %v", err)
	}
	if err := skipMapData(newJavaReader(data.Map)); err != nil {
		t.Fatalf("expected hidden 55 raw map chunk to be reusable, got %v", err)
	}

	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromMSAV failed: %v", err)
	}

	_, _, mapChunk, _, _, _ := readWorldStreamCoreSections(t, payload, -1, -1, -1)
	if !bytes.Equal(mapChunk, data.Map) {
		t.Fatalf("expected hidden 55 join-world map chunk to preserve raw msav map bytes, got len=%d want=%d", len(mapChunk), len(data.Map))
	}
}

func TestHidden55Tile144x98MatchesBetweenMSAVAndWorldStreamModel(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "maps", "serpulo", "hidden", "55.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("55.msav not present in workspace")
		}
		t.Fatalf("stat hidden 55 map: %v", err)
	}

	msavModel, err := LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load hidden 55 msav model: %v", err)
	}
	payload, err := BuildWorldStreamFromMSAV(path)
	if err != nil {
		t.Fatalf("build hidden 55 world stream: %v", err)
	}
	streamModel, err := LoadWorldModelFromWorldStreamPayload(payload, nil)
	if err != nil {
		t.Fatalf("load hidden 55 world stream model: %v", err)
	}

	const (
		x = 144
		y = 98
	)
	msavTile, err := msavModel.TileAt(x, y)
	if err != nil || msavTile == nil {
		t.Fatalf("hidden 55 msav tile lookup (%d,%d): %v", x, y, err)
	}
	streamTile, err := streamModel.TileAt(x, y)
	if err != nil || streamTile == nil {
		t.Fatalf("hidden 55 world stream tile lookup (%d,%d): %v", x, y, err)
	}

	msavBuild := msavTile.Build != nil
	streamBuild := streamTile.Build != nil
	if msavTile.Block != streamTile.Block ||
		msavTile.Team != streamTile.Team ||
		msavTile.Rotation != streamTile.Rotation ||
		msavBuild != streamBuild {
		t.Fatalf("hidden 55 tile (%d,%d) mismatch msav(block=%d team=%d rot=%d build=%v) stream(block=%d team=%d rot=%d build=%v)",
			x, y,
			msavTile.Block, msavTile.Team, msavTile.Rotation, msavBuild,
			streamTile.Block, streamTile.Team, streamTile.Rotation, streamBuild,
		)
	}
}
