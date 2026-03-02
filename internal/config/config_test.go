package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "api": { "enabled": true, "key": "abc", "keys": ["k1","k2"], "bind": "127.0.0.1:9000" },
  "runtime": { "cores": 2, "scheduler_enabled": true },
  "storage": { "mode": "file", "directory": "data/events", "database_enabled": false, "dsn": "" },
  "mods": { "enabled": true, "directory": "mods", "java_home": "" },
  "persist": { "enabled": true, "directory": "data/state", "file": "server-state.json", "interval_sec": 15, "save_msav": true, "msav_dir": "data/snapshots", "msav_file": "" },
  "script": { "file":"data/state/scripts.json", "startup_tasks": [{"delay_sec":2,"runtime":"node","target":"boot.js","args":["a"]}], "daily_gc_time":"04:30" }
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.API.Enabled || cfg.API.Key != "abc" || cfg.API.Bind != "127.0.0.1:9000" {
		t.Fatalf("api config not loaded: %+v", cfg.API)
	}
	if len(cfg.API.Keys) != 2 {
		t.Fatalf("api keys not loaded: %+v", cfg.API.Keys)
	}
	if !cfg.Runtime.SchedulerEnabled || cfg.Runtime.Cores != 2 {
		t.Fatalf("runtime config not loaded: %+v", cfg.Runtime)
	}
	if !cfg.Mods.Enabled || cfg.Mods.Directory != "mods" {
		t.Fatalf("mods config not loaded: %+v", cfg.Mods)
	}
	if !cfg.Persist.Enabled || cfg.Persist.IntervalSec != 15 {
		t.Fatalf("persist config not loaded: %+v", cfg.Persist)
	}
	if !cfg.Persist.SaveMSAV || cfg.Persist.MSAVDir != "data/snapshots" {
		t.Fatalf("persist msav config not loaded: %+v", cfg.Persist)
	}
	if cfg.Script.File != "data/state/scripts.json" || cfg.Script.DailyGCTime != "04:30" || len(cfg.Script.StartupTasks) != 1 {
		t.Fatalf("script config not loaded: %+v", cfg.Script)
	}
}
