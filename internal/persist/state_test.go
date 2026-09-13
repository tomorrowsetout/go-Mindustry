package persist

import (
	"os"
	"path/filepath"
	"testing"

	"mdt-server/internal/config"
)

func TestColdSnapshotSaveLoadWritesLatestAndArchive(t *testing.T) {
	dir := t.TempDir()
	cfg := config.PersistConfig{
		Enabled:       true,
		Directory:     dir,
		File:          "latest.json",
		RetentionDays: 7,
	}
	state := State{
		MapPath:  "assets/worlds/test.msav",
		WaveTime: 12.5,
		Wave:     3,
		Tick:     99,
		TimeData: 45,
		Rand0:    1,
		Rand1:    2,
	}
	if err := Save(cfg, state); err != nil {
		t.Fatalf("save cold snapshot: %v", err)
	}
	loaded, ok, err := Load(cfg)
	if err != nil {
		t.Fatalf("load cold snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected cold snapshot to load")
	}
	if loaded.Kind != coldSnapshotKind || loaded.Version != SnapshotVersion {
		t.Fatalf("unexpected snapshot metadata: %+v", loaded)
	}
	if loaded.MapPath != state.MapPath || loaded.Wave != state.Wave || loaded.Tick != state.Tick {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read snapshot dir: %v", err)
	}
	archives := 0
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if filepath.Ext(name) == ".json" && name != "latest.json" {
			archives++
		}
	}
	if archives != 1 {
		t.Fatalf("expected one archived cold snapshot, got %d", archives)
	}
}

func TestHotSnapshotStore(t *testing.T) {
	store := NewHotSnapshotStore()
	if _, ok := store.Get(); ok {
		t.Fatal("empty hot snapshot store should not return a snapshot")
	}
	got := store.Update(State{MapPath: "assets/worlds/test.msav", Wave: 5})
	if got.Kind != hotSnapshotKind || got.Version != SnapshotVersion || got.CapturedAt == "" {
		t.Fatalf("unexpected hot snapshot metadata: %+v", got)
	}
	loaded, ok := store.Get()
	if !ok {
		t.Fatal("expected hot snapshot")
	}
	if loaded.MapPath != got.MapPath || loaded.Wave != got.Wave {
		t.Fatalf("unexpected hot snapshot: %+v", loaded)
	}
	store.Reset()
	if _, ok := store.Get(); ok {
		t.Fatal("reset hot snapshot store should be empty")
	}
}
