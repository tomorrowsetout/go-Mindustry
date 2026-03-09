package net

import "testing"

func TestSetTileConfigNilClearsEntry(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	pos := int32((10 << 16) | 20)

	s.SetTileConfig(pos, int32(123))
	snap := s.SnapshotTileConfigs()
	if v, ok := snap[pos]; !ok || v.(int32) != 123 {
		t.Fatalf("expected tile config set, got ok=%v value=%v", ok, v)
	}

	s.SetTileConfig(pos, nil)
	snap = s.SnapshotTileConfigs()
	if _, ok := snap[pos]; ok {
		t.Fatalf("expected tile config cleared by nil value")
	}
}

