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

func TestLoadConfigINI_PreservesMindustryColorTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	payload := `
[server]
name = [#F285D1]镜[#F285E3]像[#E59AFF]物[#CC99FF]语
desc = [accent]欢迎[] [#87ceeb]测试[]
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.ServerName != "[#F285D1]镜[#F285E3]像[#E59AFF]物[#CC99FF]语" {
		t.Fatalf("server name color tags lost: %q", cfg.Runtime.ServerName)
	}
	if cfg.Runtime.ServerDesc != "[accent]欢迎[] [#87ceeb]测试[]" {
		t.Fatalf("server desc color tags lost: %q", cfg.Runtime.ServerDesc)
	}
}

func TestLoadJoinPopupSidecarPreservesMultilineAndColorTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	payload := `
[join_popup]
enabled = 1
delay_ms = 150

[title]
[accent]入服菜单[]

[message]
第一行
[accent]
第二行[]

[announcement]
[accent]公告[]

[#87ceeb]第三行[]

[link_url]
https://example.com/rules

[help]
[white]/help[] 查看帮助
[white]/sync[] 请求同步
`
	if err := os.WriteFile(filepath.Join(dir, "Join popup.ini"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write join popup ini: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.JoinPopup.Enabled || cfg.JoinPopup.DelayMs != 150 {
		t.Fatalf("join popup meta not loaded: %+v", cfg.JoinPopup)
	}
	if cfg.JoinPopup.Title != "[accent]入服菜单[]" {
		t.Fatalf("join popup title lost: %q", cfg.JoinPopup.Title)
	}
	if cfg.JoinPopup.Message != "第一行\n[accent]\n第二行[]" {
		t.Fatalf("join popup message lost multiline/color tags: %q", cfg.JoinPopup.Message)
	}
	if cfg.JoinPopup.AnnouncementText != "[accent]公告[]\n\n[#87ceeb]第三行[]" {
		t.Fatalf("join popup announcement lost multiline/color tags: %q", cfg.JoinPopup.AnnouncementText)
	}
	if cfg.JoinPopup.LinkURL != "https://example.com/rules" {
		t.Fatalf("join popup link not loaded: %q", cfg.JoinPopup.LinkURL)
	}
	if cfg.JoinPopup.HelpText != "[white]/help[] 查看帮助\n[white]/sync[] 请求同步" {
		t.Fatalf("join popup help not loaded: %q", cfg.JoinPopup.HelpText)
	}
}

func TestSaveJoinPopupSidecarSeparatesPopupContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	cfg := Default()
	cfg.JoinPopup.Enabled = true
	cfg.JoinPopup.DelayMs = 480
	cfg.JoinPopup.Title = "[accent]测试公告[]"
	cfg.JoinPopup.Message = "第一行\n第二行"
	cfg.JoinPopup.AnnouncementText = "[accent]公告[]\n\n[#87ceeb]内容[]"
	cfg.JoinPopup.LinkURL = "https://example.com/join"
	cfg.JoinPopup.HelpText = "[white]/help[]\n[white]/sync[]"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	popupRaw, err := os.ReadFile(filepath.Join(dir, "Join popup.ini"))
	if err != nil {
		t.Fatalf("read join popup ini: %v", err)
	}
	popupText := string(popupRaw)
	for _, want := range []string{
		"[join_popup]",
		"enabled = 1",
		"delay_ms = 480",
		"[title]\n[accent]测试公告[]",
		"[message]\n第一行\n第二行",
		"[announcement]\n[accent]公告[]\n\n[#87ceeb]内容[]",
		"[link_url]\nhttps://example.com/join",
		"[help]\n[white]/help[]\n[white]/sync[]",
	} {
		if !strings.Contains(popupText, want) {
			t.Fatalf("expected join popup ini to contain %q, got:\n%s", want, popupText)
		}
	}

	personalizationRaw, err := os.ReadFile(filepath.Join(dir, "Personalization.ini"))
	if err != nil {
		t.Fatalf("read personalization ini: %v", err)
	}
	personalizationText := string(personalizationRaw)
	if strings.Contains(personalizationText, "join_popup_") {
		t.Fatalf("personalization ini should not contain join popup fields, got:\n%s", personalizationText)
	}
}

func TestLoadMapVoteSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	payload := `
[map_vote]
duration_sec = 21
status_refresh_ms = 1200
popup_duration_ms = 1600
home_link_url = https://example.com/maps
align = left
top = 32
left = 14
bottom = 7
right = 3
`
	if err := os.WriteFile(filepath.Join(dir, "Vote map.ini"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write vote map ini: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.MapVote.DurationSec != 21 || cfg.MapVote.StatusRefreshMs != 1200 || cfg.MapVote.PopupDurationMs != 1600 {
		t.Fatalf("map vote timing not loaded: %+v", cfg.MapVote)
	}
	if cfg.MapVote.HomeLinkURL != "https://example.com/maps" {
		t.Fatalf("map vote link not loaded: %+v", cfg.MapVote)
	}
	if cfg.MapVote.Align != "left" || cfg.MapVote.Top != 32 || cfg.MapVote.Left != 14 || cfg.MapVote.Bottom != 7 || cfg.MapVote.Right != 3 {
		t.Fatalf("map vote popup position not loaded: %+v", cfg.MapVote)
	}
}

func TestSaveMapVoteSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	cfg := Default()
	cfg.MapVote.DurationSec = 25
	cfg.MapVote.StatusRefreshMs = 900
	cfg.MapVote.PopupDurationMs = 1300
	cfg.MapVote.HomeLinkURL = "https://example.com/votemap"
	cfg.MapVote.Align = "left"
	cfg.MapVote.Top = 48
	cfg.MapVote.Left = 12
	cfg.MapVote.Bottom = 5
	cfg.MapVote.Right = 1

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "Vote map.ini"))
	if err != nil {
		t.Fatalf("read vote map ini: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"[map_vote]",
		"duration_sec = 25",
		"status_refresh_ms = 900",
		"popup_duration_ms = 1300",
		"home_link_url = https://example.com/votemap",
		"align = left",
		"top = 48",
		"left = 12",
		"bottom = 5",
		"right = 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected vote map ini to contain %q, got:\n%s", want, text)
		}
	}
}
