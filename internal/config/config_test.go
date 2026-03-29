package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	payload := `
[runtime]
cores = 2
scheduler_enabled = 1

[server]
name = test-server
desc = hello
virtual_players = 9

[sync]
entity_ms = 120
state_ms = 260

[data]
mode = file
directory = data/events
database_enabled = 0

[mods]
enabled = 1
directory = mods

[persist]
enabled = 1
directory = data/state
file = server-state.json
interval_sec = 15
save_msav = 1
msav_dir = data/snapshots

[script]
file = data/state/scripts.json
daily_gc_time = 04:30

[api]
enabled = 1
bind = 127.0.0.1:9000
key = mdt-server-go-aaaaaaaaaaaaaaa-bbbbbbbbbbbbb-ccccccccccccccc-ddddddddddddddddddd-eeeeeeeeeeee-yzf-ffffffffff
keys = mdt-server-go-111111111111111-2222222222222-333333333333333-4444444444444444444-555555555555-yzf-6666666666,mdt-server-go-777777777777777-8888888888888-999999999999999-0000000000000000000-aaaaaaaaaaaa-yzf-bbbbbbbbbb
config_file = api.ini
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	// core.ini sidecar
	corePath := filepath.Join(dir, "core.ini")
	corePayload := `
[core]
dual_core_enabled = 0

[memory]
limit_mb = 0
startup_max_mb = 0
gc_trigger_mb = 0
check_interval_sec = 5
free_os_memory = 0
`
	if err := os.WriteFile(corePath, []byte(corePayload), 0o644); err != nil {
		t.Fatalf("write temp core config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.API.Enabled || cfg.API.Key == "" || cfg.API.Bind != "127.0.0.1:9000" {
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
	if cfg.Script.File != "data/state/scripts.json" || cfg.Script.DailyGCTime != "04:30" {
		t.Fatalf("script config not loaded: %+v", cfg.Script)
	}
	if cfg.Core.DualCoreEnabled {
		t.Fatalf("core sidecar not loaded: %+v", cfg.Core)
	}
}

func TestLoadConfigINI_InvalidAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	payload := `
[api]
enabled = 1
bind = 127.0.0.1:9000
key = abc
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected invalid api key error")
	}
}

func TestSaveSidecarsSeparatesDevelopmentAndSundriesLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	cfg := Default()
	cfg.Source = path
	cfg.Development.TerminalPlayerLogsEnabled = false
	cfg.Development.BuildFinishLogsEnabled = false
	cfg.Sundries.NetEventLogsEnabled = false
	cfg.Sundries.ChatLogsEnabled = false
	cfg.Sundries.BuildFinishLogsEnabled = false

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	devRaw, err := os.ReadFile(filepath.Join(dir, "Development mode.ini"))
	if err != nil {
		t.Fatalf("read development ini: %v", err)
	}
	devText := string(devRaw)
	if !strings.Contains(devText, "terminal_player_logs_enabled = 0") {
		t.Fatalf("expected terminal player log toggle in development ini, got:\n%s", devText)
	}
	if !strings.Contains(devText, "build_finish_logs_enabled = 0") {
		t.Fatalf("expected build finish terminal toggle in development ini, got:\n%s", devText)
	}
	if strings.Contains(devText, "net_event_logs_enabled") {
		t.Fatalf("development ini should not contain file log toggles, got:\n%s", devText)
	}

	sundriesRaw, err := os.ReadFile(filepath.Join(dir, "Sundries.ini"))
	if err != nil {
		t.Fatalf("read sundries ini: %v", err)
	}
	sundriesText := string(sundriesRaw)
	if !strings.Contains(sundriesText, "net_event_logs_enabled = 0") {
		t.Fatalf("expected net event log toggle in sundries ini, got:\n%s", sundriesText)
	}
	if !strings.Contains(sundriesText, "chat_logs_enabled = 0") {
		t.Fatalf("expected chat log toggle in sundries ini, got:\n%s", sundriesText)
	}
	if !strings.Contains(sundriesText, "build_finish_logs_enabled = 0") {
		t.Fatalf("expected build finish file toggle in sundries ini, got:\n%s", sundriesText)
	}
	if strings.Contains(sundriesText, "packet_recv_events_enabled") {
		t.Fatalf("sundries ini should not contain development packet toggles, got:\n%s", sundriesText)
	}
}

func TestLoadConfigSidecarsWithoutMainConfig(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.ini")

	if err := os.WriteFile(filepath.Join(dir, "Development mode.ini"), []byte(`
[development]
terminal_player_logs_enabled = 0
`), 0o644); err != nil {
		t.Fatalf("write development ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Sundries.ini"), []byte(`
[sundries]
chat_logs_enabled = 0
build_finish_logs_enabled = 0
`), 0o644); err != nil {
		t.Fatalf("write sundries ini: %v", err)
	}

	cfg, err := Load(mainPath)
	if err != nil {
		t.Fatalf("load config from sidecars only: %v", err)
	}
	if cfg.Development.TerminalPlayerLogsEnabled {
		t.Fatalf("expected development sidecar to load without main config")
	}
	if cfg.Sundries.ChatLogsEnabled {
		t.Fatalf("expected sundries chat log toggle to load without main config")
	}
	if cfg.Sundries.BuildFinishLogsEnabled {
		t.Fatalf("expected sundries build finish toggle to load without main config")
	}
}
