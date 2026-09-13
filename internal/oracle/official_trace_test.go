package oracle

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCollectOfficialTraceSmoke(t *testing.T) {
	if os.Getenv("RUN_MDT_ORACLE_INTEGRATION") != "1" {
		t.Skip("set RUN_MDT_ORACLE_INTEGRATION=1 to run the official Java oracle smoke test")
	}
	if _, err := ResolveRoot(); err != nil {
		t.Skipf("official oracle root not present: %v", err)
	}

	repoRoot := oracleRepoRoot(t)
	trace, err := CollectOfficialTrace(Scenario{
		Name:           "official-smoke",
		MapPath:        "assets/worlds/file.msav",
		Ticks:          0,
		CaptureInitial: true,
	}, OfficialTraceOptions{
		WorkspaceRoot: repoRoot,
		WorkspaceDir:  filepath.Join(t.TempDir(), "official-smoke"),
	})
	if err != nil {
		t.Fatalf("CollectOfficialTrace: %v", err)
	}
	if len(trace.Ticks) != 1 {
		t.Fatalf("expected exactly one initial tick, got %d", len(trace.Ticks))
	}
	if len(trace.Ticks[0].Tiles) == 0 {
		t.Fatal("expected official trace to contain tile snapshots")
	}
}

func TestCompareOfficialAndGoTraceSmoke(t *testing.T) {
	if os.Getenv("RUN_MDT_ORACLE_INTEGRATION") != "1" {
		t.Skip("set RUN_MDT_ORACLE_INTEGRATION=1 to run the official Java oracle comparison")
	}
	if _, err := ResolveRoot(); err != nil {
		t.Skipf("official oracle root not present: %v", err)
	}

	repoRoot := oracleRepoRoot(t)
	scenario := Scenario{
		Name:                "official-go-smoke",
		MapPath:             "assets/worlds/file.msav",
		VanillaProfilesPath: "data/vanilla/profiles.json",
		Ticks:               0,
		CaptureInitial:      true,
	}
	official, err := CollectOfficialTrace(scenario, OfficialTraceOptions{
		WorkspaceRoot: repoRoot,
		WorkspaceDir:  filepath.Join(t.TempDir(), "official-go-smoke"),
	})
	if err != nil {
		t.Fatalf("CollectOfficialTrace: %v", err)
	}
	goTrace, err := CollectGoTrace(scenario, GoTraceOptions{WorkspaceRoot: repoRoot})
	if err != nil {
		t.Fatalf("CollectGoTrace: %v", err)
	}
	if diffs := CompareTraces(official, goTrace); len(diffs) != 0 {
		t.Fatalf("official/go trace mismatch:\n%s", formatDiffs(diffs, 20))
	}
}

func TestOfficialTraceReportsRuntimeContentIDs(t *testing.T) {
	if os.Getenv("RUN_MDT_ORACLE_INTEGRATION") != "1" {
		t.Skip("set RUN_MDT_ORACLE_INTEGRATION=1 to run the official Java oracle content-id smoke test")
	}
	if _, err := ResolveRoot(); err != nil {
		t.Skipf("official oracle root not present: %v", err)
	}

	repoRoot := oracleRepoRoot(t)
	workspaceDir := filepath.Join(t.TempDir(), "official-content-ids")
	if keep := strings.TrimSpace(os.Getenv("MDT_ORACLE_KEEP_WORKSPACE")); keep != "" {
		workspaceDir = keep
	}
	trace, err := CollectOfficialTrace(Scenario{
		Name:           "official-content-ids",
		MapPath:        "assets/worlds/file.msav",
		Ticks:          0,
		CaptureInitial: true,
	}, OfficialTraceOptions{
		WorkspaceRoot: repoRoot,
		WorkspaceDir:  workspaceDir,
	})
	if err != nil {
		t.Fatalf("CollectOfficialTrace: %v", err)
	}
	for _, key := range []string{
		"content.blocks.count",
		"content.block.rotary-pump",
		"content.block.water-extractor",
		"content.block.world-processor",
		"content.block.world-cell",
		"content.block.world-message",
		"content.block.world-switch",
	} {
		value := trace.Metadata[key]
		if value == "" {
			t.Fatalf("expected official trace metadata %q", key)
		}
		t.Logf("%s=%s", key, value)
	}
}

func formatDiffs(diffs []Difference, limit int) string {
	if limit <= 0 || limit > len(diffs) {
		limit = len(diffs)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(diffs[i].Path)
		b.WriteString(": ")
		b.WriteString(diffs[i].Message)
	}
	if len(diffs) > limit {
		b.WriteString("\n... ")
		b.WriteString(strconv.Itoa(len(diffs) - limit))
		b.WriteString(" more")
	}
	return b.String()
}
