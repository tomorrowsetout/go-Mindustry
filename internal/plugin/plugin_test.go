package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writePlugin creates a minimal plugin directory layout for tests.
func writePlugin(t *testing.T, root, name, hjson, script string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.hjson"), []byte(hjson), 0o644); err != nil {
		t.Fatal(err)
	}
	if script != "" {
		if err := os.WriteFile(filepath.Join(dir, "scripts", "main.js"), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadModuleParsesYFFormat(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "demo", `{
  id: "demo"
  name: "演示插件"
  author: "tester"
  description: "unit test plugin"
  version: "1.2.3"
  main: "scripts/main.js"
  runtime: "js"
  enabled: true
  category: "Test"
  tags: ["a", "b"]
  depends: ["tester/base"]
  softDepends: ["tester/optional"]
  loadType: "plugin"
}`, `// noop`)

	def, err := LoadModule(filepath.Join(root, "demo"))
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if def == nil {
		t.Fatal("expected a definition")
	}
	if def.Meta.ID != "demo" || def.Meta.Name != "演示插件" || def.Meta.Author != "tester" {
		t.Errorf("meta mismatch: %+v", def.Meta)
	}
	if def.Meta.Version != "1.2.3" || def.Meta.Runtime != "js" || !def.Meta.Enabled {
		t.Errorf("meta mismatch: %+v", def.Meta)
	}
	if len(def.Meta.Tags) != 2 || def.Meta.Tags[0] != "a" {
		t.Errorf("tags mismatch: %v", def.Meta.Tags)
	}
	if len(def.Meta.Depends) != 1 || def.Meta.Depends[0] != "tester/base" {
		t.Errorf("depends mismatch: %v", def.Meta.Depends)
	}
	if len(def.Meta.SoftDepends) != 1 {
		t.Errorf("softDepends mismatch: %v", def.Meta.SoftDepends)
	}
	if def.Meta.LoadType != "plugin" {
		t.Errorf("loadType mismatch: %q", def.Meta.LoadType)
	}
	if def.FullID() != "tester/demo" {
		t.Errorf("fullId mismatch: %q", def.FullID())
	}
	if !def.HasMain() {
		t.Error("main script should exist")
	}
}

func TestLoadModuleDefaults(t *testing.T) {
	root := t.TempDir()
	// missing id/author/main/runtime → defaults from directory layout
	writePlugin(t, root, "bare", `{ name: "bare plugin" }`, `// noop`)
	def, err := LoadModule(filepath.Join(root, "bare"))
	if err != nil {
		t.Fatal(err)
	}
	if def.Meta.ID != "bare" {
		t.Errorf("id default: %q", def.Meta.ID)
	}
	if def.Meta.Main != "scripts/main.js" {
		t.Errorf("main default: %q", def.Meta.Main)
	}
	if def.Meta.Runtime != "js" {
		t.Errorf("runtime default: %q", def.Meta.Runtime)
	}
	if !def.Meta.Enabled {
		t.Error("enabled should default to true")
	}
}

func TestResolveTopologicalOrder(t *testing.T) {
	mk := func(author, id string, depends ...string) *Definition {
		return &Definition{Meta: Meta{ID: id, Author: author, Depends: depends, Enabled: true}}
	}
	c := mk("t", "c", "t/b")
	b := mk("t", "b", "t/a")
	a := mk("t", "a")

	res := Resolve([]*Definition{c, b, a})
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	var ids []string
	for _, def := range res.Ordered {
		ids = append(ids, def.FullID())
	}
	joined := strings.Join(ids, ",")
	if index := strings.Index(joined, "t/a"); index > strings.Index(joined, "t/b") {
		t.Errorf("a must load before b: %s", joined)
	}
	if index := strings.Index(joined, "t/b"); index > strings.Index(joined, "t/c") {
		t.Errorf("b must load before c: %s", joined)
	}
}

func TestResolveMissingHardDependency(t *testing.T) {
	def := &Definition{Meta: Meta{ID: "x", Author: "t", Depends: []string{"t/missing"}, Enabled: true}}
	res := Resolve([]*Definition{def})
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "缺少硬依赖") {
		t.Fatalf("expected missing hard dependency error, got: %v", res.Errors)
	}
}

func TestResolveMissingSoftDependencyIsWarning(t *testing.T) {
	def := &Definition{Meta: Meta{ID: "x", Author: "t", SoftDepends: []string{"t/optional"}, Enabled: true}}
	res := Resolve([]*Definition{def})
	if len(res.Errors) != 0 {
		t.Fatalf("soft dependency must not error: %v", res.Errors)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "缺少软依赖") {
		t.Fatalf("expected soft dependency warning, got: %v", res.Warnings)
	}
}

func TestResolveCycle(t *testing.T) {
	a := &Definition{Meta: Meta{ID: "a", Author: "t", Depends: []string{"t/b"}, Enabled: true}}
	b := &Definition{Meta: Meta{ID: "b", Author: "t", Depends: []string{"t/a"}, Enabled: true}}
	res := Resolve([]*Definition{a, b})
	foundCycle := false
	for _, errMsg := range res.Errors {
		if strings.Contains(errMsg, "循环依赖") {
			foundCycle = true
		}
	}
	if !foundCycle {
		t.Fatalf("expected cycle error, got: %v", res.Errors)
	}
}

func TestBrokenSetSkipsDependentAndCycle(t *testing.T) {
	r := NewRegistry(t.TempDir(), nil)
	dep := &Definition{Meta: Meta{ID: "dep", Author: "t", Depends: []string{"t/missing"}, Enabled: true}}
	cycleA := &Definition{Meta: Meta{ID: "ca", Author: "t", Depends: []string{"t/cb"}, Enabled: true}}
	cycleB := &Definition{Meta: Meta{ID: "cb", Author: "t", Depends: []string{"t/ca"}, Enabled: true}}
	res := Resolve([]*Definition{dep, cycleA, cycleB})
	skipped := r.brokenSet(res)
	for _, id := range []string{"t/dep", "t/ca", "t/cb"} {
		if !skipped[id] {
			t.Errorf("expected %s to be skipped", id)
		}
	}
}

// TestRegistryLoadsJSPlugin exercises the full path: scan → resolve →
// start JS runtime → events → commands → config → reload.
func TestRegistryLoadsJSPlugin(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	script := `
var counter = 0;
var sawJoin = null;
yzf.commands.register("ping", "测试命令", function(a) {
	counter++;
	return { ok: true, counter: counter, arg: a === undefined ? null : a };
});
yzf.playerCommand("hello", "/hello", "打招呼", function(player, args) {
	yzf.net.broadcast("你好 " + player.name);
});
yzf.on("PlayerJoin", function(payload) {
	sawJoin = payload;
	return true;
});
yzf.on("PlayerChatEvent", function(payload) {
	if (payload && payload.message === "cancel-me") return false;
	return true;
});
yzf.config.set("marker", "loaded");
`
	writePlugin(t, plugins, "js-demo", `{
  id: "js-demo"
  author: "tester"
  name: "JS 演示"
  runtime: "js"
  enabled: true
  loadType: "plugin"
}`, script)

	registry := NewRegistry(yzfRoot, func(format string, args ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registry.loadedCount() != 1 {
		t.Fatalf("expected 1 loaded module, got %d", registry.loadedCount())
	}

	summaries := registry.LoadedSummaries()
	if len(summaries) != 1 || summaries[0].FullID != "tester/js-demo" {
		t.Fatalf("summary mismatch: %+v", summaries)
	}

	// callable command registered by the script
	found, result := registry.Commands().CallCallable("ping", []any{"x"})
	if !found {
		t.Fatal("ping command not registered")
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["ok"] != true {
		t.Fatalf("ping result mismatch: %v", result)
	}

	// player command registered by the script
	playerFound, err := registry.Commands().RunPlayerCommand("hello", PlayerInfo{ID: 1, Name: "测试"}, nil)
	if err != nil || !playerFound {
		t.Fatalf("hello command failed: found=%v err=%v", playerFound, err)
	}

	// event delivery through registry.FireEvent
	if !registry.FireEvent(context.Background(), "PlayerJoin", EventPayload{"name": "测试"}) {
		t.Fatal("PlayerJoin should not be consumed")
	}
	// cancellable event
	if registry.FireEvent(context.Background(), "PlayerChatEvent", EventPayload{"message": "cancel-me"}) {
		t.Fatal("cancel-me chat should be consumed by the plugin")
	}
	if !registry.FireEvent(context.Background(), "PlayerChatEvent", EventPayload{"message": "normal"}) {
		t.Fatal("normal chat must pass through")
	}

	// config persisted to data/config/config.hjson
	store := registry.bridge.config("tester/js-demo")
	if store == nil {
		t.Fatal("config store missing")
	}
	if got := store.GetString("marker", ""); got != "loaded" {
		t.Errorf("config marker = %q, want loaded", got)
	}

	// reload round-trip: counter resets after restart
	registry.performReload(context.Background(), "tester/js-demo")
	found2, _ := registry.Commands().CallCallable("ping", nil)
	if !found2 {
		t.Fatal("ping command missing after reload")
	}
	if registry.loadedCount() != 1 {
		t.Fatalf("expected 1 module after reload, got %d", registry.loadedCount())
	}

	registry.StopAll(context.Background())
	if registry.loadedCount() != 0 {
		t.Fatal("all modules should stop")
	}
	if foundAfter, _ := registry.Commands().CallCallable("ping", nil); foundAfter {
		t.Fatal("commands must detach on unload")
	}
}

// TestRegistrySkipsDisabledAndBroken verifies load filtering.
func TestRegistrySkipsDisabledAndBroken(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	writePlugin(t, plugins, "off", `{
  id: "off"
  author: "tester"
  enabled: false
}`, `yzf.log("should not run");`)
	writePlugin(t, plugins, "broken", `{
  id: "broken"
  author: "tester"
  depends: ["tester/does-not-exist"]
}`, `yzf.log("should not run");`)
	writePlugin(t, plugins, "good", `{
  id: "good"
  author: "tester"
  enabled: true
}`, `yzf.log("ok");`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registry.loadedCount() != 1 {
		t.Fatalf("expected only the good plugin to load, got %d", registry.loadedCount())
	}
	summaries := registry.LoadedSummaries()
	if len(summaries) != 1 || summaries[0].FullID != "tester/good" {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
	registry.StopAll(context.Background())
}

// TestRegistryLoadOrder verifies dependency ordering across two plugins.
func TestRegistryLoadOrder(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	// dependent declares loadOrder via a global the base sets
	writePlugin(t, plugins, "base", `{
  id: "base"
  author: "tester"
}`, `yzf.commands.register("base-ready", "", function(){ return true; });`)
	writePlugin(t, plugins, "child", `{
  id: "child"
  author: "tester"
  depends: ["tester/base"]
}`, `
var ready = yzf.commands.has("base-ready");
yzf.commands.register("child-sees-base", "", function(){ return ready; });
`)

	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	found, result := registry.Commands().CallCallable("child-sees-base", nil)
	if !found {
		t.Fatal("child command missing")
	}
	if result != true {
		t.Fatalf("child loaded before base: %v", result)
	}
	registry.StopAll(context.Background())
}

// TestJSTimers checks yzf.after delivery on the JS goroutine.
func TestJSTimers(t *testing.T) {
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")
	writePlugin(t, plugins, "timers", `{
  id: "timers"
  author: "tester"
}`, `
yzf.commands.register("fired", "", function(){ return globalThis.__fired === true; });
yzf.after(0.05, function(){ globalThis.__fired = true; });
`)
	registry := NewRegistry(yzfRoot, func(string, ...any) {})
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if found, result := registry.Commands().CallCallable("fired", nil); found && result == true {
			registry.StopAll(context.Background())
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("yzf.after callback never fired")
}

func TestEventBusModuleScope(t *testing.T) {
	bus := NewEventBus()
	hits := 0
	bus.Subscribe("PlayerJoin", "tester/a", 0, func(EventPayload) bool {
		hits++
		return true
	})
	bus.Fire("PlayerJoin", nil)
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
	bus.UnsubscribeModule("tester/a")
	bus.Fire("PlayerJoin", nil)
	if hits != 1 {
		t.Fatal("handler should be removed with its module")
	}
}

func TestConfigStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "cfg", `{ id: "cfg", author: "tester" }`, `// noop`)
	def, err := LoadModule(filepath.Join(root, "cfg"))
	if err != nil || def == nil {
		t.Fatal(err)
	}
	store := NewConfigStore(def)
	if err := store.SetString("name", "值"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetInt("count", 42); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBool("flag", true); err != nil {
		t.Fatal(err)
	}
	// reopen from disk
	store2 := NewConfigStore(def)
	if got := store2.GetString("name", ""); got != "值" {
		t.Errorf("name = %q", got)
	}
	if got := store2.GetInt("count", 0); got != 42 {
		t.Errorf("count = %d", got)
	}
	if got := store2.GetBool("flag", false); !got {
		t.Error("flag should be true")
	}
	// root index file must exist and point at the runtime store
	indexRaw, err := os.ReadFile(filepath.Join(def.Root, "config.hjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(indexRaw), "data/config/config.hjson") {
		t.Errorf("index file does not point at runtime store: %s", indexRaw)
	}
}
