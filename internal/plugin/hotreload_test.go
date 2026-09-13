package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor polls a condition until it holds or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", msg)
}

// TestTransactionalReloadRollback verifies that a module whose new script is
// broken is rolled back to its previous working version.
func TestTransactionalReloadRollback(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	workingScript := `
yzf.commands.register("status", "", function() { return "working-v1"; });
`
	writePlugin(t, plugins, "rollback-test", `{
  id: "rollback-test"
  author: "tester"
  name: "回滚测试"
  runtime: "js"
  enabled: true
}`, workingScript)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer registry.StopAll(context.Background())

	// v1 loaded and callable.
	found, result := registry.Commands().CallCallable("status", nil)
	if !found || result != "working-v1" {
		t.Fatalf("v1 status = %v, want working-v1", result)
	}

	// Break the script with a syntax error and reload.
	brokenPath := filepath.Join(plugins, "rollback-test", "scripts", "main.js")
	if err := os.WriteFile(brokenPath, []byte("this is ((( broken js"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry.performReload(context.Background(), "tester/rollback-test")

	// The broken revision failed to load; the rollback restored v1.
	found, result = registry.Commands().CallCallable("status", nil)
	if !found {
		t.Fatal("status command missing after rollback")
	}
	if result != "working-v1" {
		t.Fatalf("after rollback status = %v, want working-v1 (rollback did not restore old version)", result)
	}
}

// TestReloadRestoresSuccessfulUpdate verifies the happy path: a working edit
// is picked up and the old version is replaced.
func TestReloadRestoresSuccessfulUpdate(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	writePlugin(t, plugins, "update-test", `{
  id: "update-test"
  author: "tester"
  name: "更新测试"
  runtime: "js"
  enabled: true
}`, `yzf.commands.register("ver", "", function() { return "v1"; });`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer registry.StopAll(context.Background())

	if _, v := registry.Commands().CallCallable("ver", nil); v != "v1" {
		t.Fatalf("initial ver = %v, want v1", v)
	}

	scriptPath := filepath.Join(plugins, "update-test", "scripts", "main.js")
	if err := os.WriteFile(scriptPath, []byte(`yzf.commands.register("ver", "", function() { return "v2"; });`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry.performReload(context.Background(), "tester/update-test")

	if _, v := registry.Commands().CallCallable("ver", nil); v != "v2" {
		t.Fatalf("after reload ver = %v, want v2", v)
	}
}

// TestReloadScopeIncludesDependents verifies that reloading a module also
// restarts modules that depend on it.
func TestReloadScopeIncludesDependents(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	writePlugin(t, plugins, "dep-base", `{
  id: "dep-base"
  author: "tester"
  runtime: "js"
  enabled: true
}`, `yzf.commands.register("base-ver", "", function() { return "base-v2"; });`)

	writePlugin(t, plugins, "dep-child", `{
  id: "dep-child"
  author: "tester"
  runtime: "js"
  enabled: true
  depends: ["tester/dep-base"]
}`, `yzf.commands.register("child-ver", "", function() { return "child-v1"; });`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer registry.StopAll(context.Background())

	// Update the base module's script; the child depends on it and must be
	// restarted too (its registration re-created in the same transaction).
	baseScript := filepath.Join(plugins, "dep-base", "scripts", "main.js")
	if err := os.WriteFile(baseScript, []byte(`yzf.commands.register("base-ver", "", function() { return "base-v2-new"; });`), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := registry.resolveReloadPlan("tester/dep-base")
	planIDs := make(map[string]bool)
	for _, def := range plan {
		planIDs[def.FullID()] = true
	}
	if !planIDs["tester/dep-base"] {
		t.Fatal("reload plan must include the target")
	}
	if !planIDs["tester/dep-child"] {
		t.Fatal("reload plan must include dependents")
	}

	registry.performReload(context.Background(), "tester/dep-base")
	if _, v := registry.Commands().CallCallable("base-ver", nil); v != "base-v2-new" {
		t.Fatalf("base-ver = %v, want base-v2-new", v)
	}
	if _, v := registry.Commands().CallCallable("child-ver", nil); v != "child-v1" {
		t.Fatalf("child-ver = %v, want child-v1 (dependent must be re-registered)", v)
	}
}

// TestReloadMissingModuleStopsOldInstance verifies that deleting a plugin
// directory stops the running instance on the next reload.
func TestReloadMissingModuleStopsOldInstance(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	writePlugin(t, plugins, "remove-test", `{
  id: "remove-test"
  author: "tester"
  runtime: "js"
  enabled: true
}`, `yzf.commands.register("gone", "", function() { return "here"; });`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer registry.StopAll(context.Background())

	if found, _ := registry.Commands().CallCallable("gone", nil); !found {
		t.Fatal("gone command should be registered initially")
	}

	if err := os.RemoveAll(filepath.Join(plugins, "remove-test")); err != nil {
		t.Fatal(err)
	}
	registry.performReload(context.Background(), "tester/remove-test")

	if found, _ := registry.Commands().CallCallable("gone", nil); found {
		t.Fatal("gone command must be detached after the module directory is removed")
	}
	if registry.loadedCount() != 0 {
		t.Fatalf("loaded count = %d, want 0 after removal", registry.loadedCount())
	}
}

// TestFileWatcherTriggersReload verifies that editing a plugin script on disk
// causes the watcher to queue and execute a reload.
func TestFileWatcherTriggersReload(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	writePlugin(t, plugins, "watch-test", `{
  id: "watch-test"
  author: "tester"
  runtime: "js"
  enabled: true
}`, `yzf.commands.register("watch-ver", "", function() { return "v1"; });`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := registry.ScanAndLoad(ctx); err != nil {
		t.Fatal(err)
	}
	go registry.ReloadLoop(ctx)
	defer registry.StopAll(ctx)

	watcher := NewFileWatcher(registry)
	if !watcher.Start() {
		t.Fatal("watcher failed to start")
	}
	defer watcher.Stop()

	// Wait for the watcher goroutine to settle before editing.
	time.Sleep(200 * time.Millisecond)

	scriptPath := filepath.Join(plugins, "watch-test", "scripts", "main.js")
	if err := os.WriteFile(scriptPath, []byte(`yzf.commands.register("watch-ver", "", function() { return "v2"; });`), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 5*time.Second, func() bool {
		_, v := registry.Commands().CallCallable("watch-ver", nil)
		return v == "v2"
	}, "file watcher should reload the module to v2")
}

// TestRelevantChangeFilters verifies the watcher's change filtering rules.
func TestRelevantChangeFilters(t *testing.T) {
	yzfRoot := t.TempDir()
	registry := NewRegistry(yzfRoot, nil)
	watcher := NewFileWatcher(registry)

	base := filepath.Join(yzfRoot, "plugins", "demo")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(base, "module.hjson"), true},
		{filepath.Join(base, "module.json"), true},
		{filepath.Join(base, "scripts", "main.js"), true},
		{filepath.Join(base, "scripts", "helper.mjs"), true},
		{filepath.Join(base, "data", "config", "config.hjson"), true},
		{filepath.Join(base, "scripts", "notes.txt"), false},
		{filepath.Join(base, "scripts", "main.js.tmp"), false},
		{filepath.Join(base, "scripts", "main.js.bak"), false},
		{filepath.Join(base, "cache", "out.js"), false},
		{filepath.Join(base, "node_modules", "dep", "index.js"), false},
		{filepath.Join(base, "logs", "run.log"), false},
		{filepath.Join(base, "stable-api-debug.js"), false},
	}
	for _, tc := range cases {
		// Create a real file so the directory check in relevantChange sees a file.
		if err := os.MkdirAll(filepath.Dir(tc.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tc.path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := watcher.relevantChange(tc.path); got != tc.want {
			t.Errorf("relevantChange(%s) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestCoalesceReloadTargets verifies the burst coalescing logic.
func TestCoalesceReloadTargets(t *testing.T) {
	out := coalesceReloadTargets([]string{"a/x", "a/x", "b/y"})
	if len(out) != 2 {
		t.Fatalf("coalesce = %v, want 2 unique targets", out)
	}
	out = coalesceReloadTargets([]string{"a/x", "*", "b/y"})
	if len(out) != 1 || out[0] != "*" {
		t.Fatalf("coalesce with * = %v, want [*]", out)
	}
	out = coalesceReloadTargets(nil)
	if len(out) != 0 {
		t.Fatalf("coalesce(nil) = %v, want empty", out)
	}
}
