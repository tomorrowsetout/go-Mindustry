package world

import "testing"

func TestSnapshotWaveTimeTicksUsesVanillaSixtyHz(t *testing.T) {
	snap := Snapshot{
		WaveTime: 12.5,
		Tps:      120,
	}

	if got, want := snap.WaveTimeTicks(), float32(750); got != want {
		t.Fatalf("expected wave time ticks %v, got %v", want, got)
	}
}

func TestSnapshotWaveTimeTicksDefaultsToSixty(t *testing.T) {
	snap := Snapshot{
		WaveTime: 12.5,
		Tps:      0,
	}

	if got, want := snap.WaveTimeTicks(), float32(750); got != want {
		t.Fatalf("expected default wave time ticks %v, got %v", want, got)
	}
}
