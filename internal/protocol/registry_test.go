package protocol

import "testing"

func TestGetRegistryStatus(t *testing.T) {
	s := GetRegistryStatus(155)
	if s.BasePackets != 4 {
		t.Fatalf("base packet count mismatch: got %d want 4", s.BasePackets)
	}
	if s.TotalPackets != s.BasePackets+s.RemotePackets {
		t.Fatalf("inconsistent counts: total=%d base=%d remote=%d", s.TotalPackets, s.BasePackets, s.RemotePackets)
	}
	if s.RemotePackets < 100 {
		t.Fatalf("remote packet count unexpectedly low: %d", s.RemotePackets)
	}
}
