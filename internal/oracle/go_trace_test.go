package oracle

import (
	"path/filepath"
	"runtime"
	"testing"

	"mdt-server/internal/world"
)

func TestCollectGoTraceFromBundledMap(t *testing.T) {
	repoRoot := oracleRepoRoot(t)
	trace, err := CollectGoTrace(Scenario{
		Name:                "bundled-map-smoke",
		MapPath:             "assets/worlds/file.msav",
		VanillaProfilesPath: "data/vanilla/profiles.json",
		Ticks:               2,
		CaptureInitial:      true,
	}, GoTraceOptions{WorkspaceRoot: repoRoot})
	if err != nil {
		t.Fatalf("CollectGoTrace: %v", err)
	}
	if got := len(trace.Ticks); got != 3 {
		t.Fatalf("expected initial snapshot + 2 ticks, got %d", got)
	}
	if len(trace.Ticks[0].Tiles) == 0 {
		t.Fatal("expected traced map to contain tile snapshots")
	}
	if trace.Metadata["mapPathResolved"] == "" {
		t.Fatal("expected resolved map path metadata")
	}
}

func TestCollectGoTraceMarksHotReloadTick(t *testing.T) {
	repoRoot := oracleRepoRoot(t)
	trace, err := CollectGoTrace(Scenario{
		Name:                "hot-reload-smoke",
		MapPath:             "assets/worlds/file.msav",
		HotReloadMapPath:    "assets/worlds/file.msav",
		HotReloadTick:       1,
		VanillaProfilesPath: "data/vanilla/profiles.json",
		Ticks:               1,
	}, GoTraceOptions{WorkspaceRoot: repoRoot})
	if err != nil {
		t.Fatalf("CollectGoTrace: %v", err)
	}
	if len(trace.Ticks) != 1 {
		t.Fatalf("expected one tick trace, got %d", len(trace.Ticks))
	}
	if len(trace.Ticks[0].Events) != 1 || trace.Ticks[0].Events[0].Kind != "map_hot_reload" {
		t.Fatalf("expected map_hot_reload event, got %#v", trace.Ticks[0].Events)
	}
}

func TestCompareTracesHonorsTolerances(t *testing.T) {
	expected := NewTrace("expected", Scenario{Name: "cmp", Ticks: 1})
	actual := NewTrace("actual", Scenario{Name: "cmp", Ticks: 1})
	expected.Ticks = []TickTrace{{
		Index: 1,
		Units: []UnitState{{
			ID:       7,
			TypeID:   35,
			TeamID:   1,
			X:        10,
			Y:        20,
			Rotation: 90,
			Health:   100,
		}},
	}}
	actual.Ticks = []TickTrace{{
		Index: 1,
		Units: []UnitState{{
			ID:       7,
			TypeID:   35,
			TeamID:   1,
			X:        10.005,
			Y:        20.005,
			Rotation: 90.05,
			Health:   100.00001,
		}},
	}}
	if diffs := CompareTraces(expected, actual); len(diffs) != 0 {
		t.Fatalf("expected no diffs within tolerance, got %#v", diffs)
	}
	actual.Ticks[0].Units[0].Health = 101
	diffs := CompareTraces(expected, actual)
	if len(diffs) == 0 {
		t.Fatal("expected health mismatch to be detected")
	}
}

func TestControllerTypeInfersDefaultCommandAIForCommandableUnits(t *testing.T) {
	ent := world.RawEntity{TypeID: 24, Team: 3}
	if got := controllerType(ent, "oct"); got != "command" {
		t.Fatalf("expected commandable team unit to report command controller, got %q", got)
	}
	if got := controllerType(ent, "scathe-missile"); got != "none" {
		t.Fatalf("expected missile unit to keep generic controller, got %q", got)
	}
	if got := controllerType(ent, "assembly-drone"); got != "none" {
		t.Fatalf("expected assembly drone to keep generic controller, got %q", got)
	}
}

func TestControllerTypeUsesRuntimeControllerOverride(t *testing.T) {
	ent := world.RawEntity{TypeID: 24, Team: 2, ControllerType: "DefenderAI"}
	if got := controllerType(ent, "oct"); got != "DefenderAI" {
		t.Fatalf("expected runtime controller override, got %q", got)
	}
}

func oracleRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
