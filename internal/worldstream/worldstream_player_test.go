package worldstream

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"path/filepath"
	"testing"
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap world: %v", err)
	}
	return decompressWorldStream(t, data)
}

func assertContentStartsCleanly(t *testing.T, raw []byte, start int) {
	t.Helper()
	r := newJavaReader(raw[start:])
	if err := skipContentHeader(r); err != nil {
		t.Fatalf("content header parse failed at %d: %v", start, err)
	}
	if err := skipContentPatches(r); err != nil {
		t.Fatalf("content patches parse failed at %d: %v", start, err)
	}
	if err := skipMapData(r); err != nil {
		t.Fatalf("map parse failed at %d: %v", start, err)
	}
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
