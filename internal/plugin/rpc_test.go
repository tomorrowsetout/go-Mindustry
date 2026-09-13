package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestGoRPCPlugin boots a real Go plugin subprocess (via go run) and
// verifies the init handshake, command registration, event delivery
// and clean shutdown — the Go-runtime half of the "two test plugins"
// acceptance criterion (the JS half lives in plugin_test.go).
func TestGoRPCPlugin(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	yzfRoot := t.TempDir()
	plugins := filepath.Join(yzfRoot, "plugins")

	// Minimal JSON-RPC plugin: answers init/shutdown/event, registers a
	// callable command right after the handshake.
	const pluginSource = `package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type msg struct {
	ID     int64           ` + "`json:\"id,omitempty\"`" + `
	Method string          ` + "`json:\"method,omitempty\"`" + `
	Params json.RawMessage ` + "`json:\"params,omitempty\"`" + `
	Result json.RawMessage ` + "`json:\"result,omitempty\"`" + `
	Error  string          ` + "`json:\"error,omitempty\"`" + `
}

func main() {
	out := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var m msg
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			continue
		}
		reply := msg{ID: m.ID}
		switch m.Method {
		case "init":
			reply.Result = json.RawMessage("{\"ok\":true}")
		case "shutdown":
			reply.Result = json.RawMessage("{\"ok\":true}")
			out.Encode(reply)
			return
		case "event":
			reply.Result = json.RawMessage("{\"consumed\":false}")
		case "callableCommand":
			reply.Result = json.RawMessage("{\"ok\":true,\"from\":\"go-plugin\"}")
		default:
			if m.ID != 0 {
				reply.Result = json.RawMessage("null")
			}
		}
		if m.ID != 0 {
			out.Encode(reply)
		}
		if m.Method == "init" {
			reg := msg{ID: 100, Method: "registerCallable",
				Params: json.RawMessage("{\"Name\":\"go-ping\",\"Desc\":\"rpc test\"}")}
			out.Encode(reg)
		}
	}
}
`
	dir := filepath.Join(plugins, "go-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hjson := `{
  id: "go-demo"
  author: "tester"
  name: "Go 演示"
  runtime: "go"
  main: "main.go"
  enabled: true
  loadType: "plugin"
}`
	if err := os.WriteFile(filepath.Join(dir, "module.hjson"), []byte(hjson), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(pluginSource), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(yzfRoot, func(format string, args ...any) { t.Logf("[verb] "+format, args...) })
	if err := registry.ScanAndLoad(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registry.loadedCount() != 1 {
		t.Fatalf("expected 1 loaded module, got %d", registry.loadedCount())
	}

	// The plugin registers go-ping right after init (async); wait for it.
	deadline := time.Now().Add(30 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		if registry.Commands().HasCallable("go-ping") {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatal("go-ping command never registered by the plugin process")
	}

	// Calling the command round-trips through the subprocess.
	callFound, result := registry.Commands().CallCallable("go-ping", nil)
	if !callFound {
		t.Fatal("go-ping not callable")
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["from"] != "go-plugin" {
		t.Fatalf("unexpected result: %v", result)
	}

	// Events reach the subprocess and are not consumed.
	if !registry.FireEvent(context.Background(), "PlayerJoin", EventPayload{"name": "测试"}) {
		t.Fatal("go plugin must not consume PlayerJoin")
	}

	registry.StopAll(context.Background())
	if registry.loadedCount() != 0 {
		t.Fatal("module should stop")
	}
	if registry.Commands().HasCallable("go-ping") {
		t.Fatal("commands must detach on unload")
	}
}
