package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	payload := `
[runtime]
cores = 2
scheduler_enabled = true

[server]
name = "test-server"
desc = "hello"
virtual_players = 9

[sync]
entity_ms = 120
state_ms = 260

[data]
mode = "file"
directory = "data/events"
database_enabled = false

[mods]
enabled = true
directory = "mods"

[persist]
enabled = true
directory = "data/state"
file = "server-state.json"
interval_sec = 15
save_msav = true
msav_dir = "data/snapshots"

[script]
file = "data/state/scripts.json"
daily_gc_time = "04:30"

[api]
enabled = true
bind = "127.0.0.1:9000"
key = "mdt-server-go-aaaaaaaaaaaaaaa-bbbbbbbbbbbbb-ccccccccccccccc-ddddddddddddddddddd-eeeeeeeeeeee-yzf-ffffffffff"
keys = ["mdt-server-go-111111111111111-2222222222222-333333333333333-4444444444444444444-555555555555-yzf-6666666666", "mdt-server-go-777777777777777-8888888888888-999999999999999-0000000000000000000-aaaaaaaaaaaa-yzf-bbbbbbbbbb"]
config_file = "api.toml"

[authority_sync]
strategy = "static"
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	// core.toml sidecar
	corePath := filepath.Join(dir, "core.toml")
	corePayload := `
[core]
dual_core_enabled = false
tps = 120

[memory]
limit_mb = 0
startup_max_mb = 0
gc_trigger_mb = 0
check_interval_sec = 5
free_os_memory = false
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
	if cfg.Core.DualCoreEnabled || cfg.Core.TPS != 120 {
		t.Fatalf("core sidecar not loaded: %+v", cfg.Core)
	}
	if cfg.Sync.Strategy != AuthoritySyncStatic {
		t.Fatalf("authority sync strategy not loaded: %+v", cfg.Sync)
	}
}

func TestSaveCoreSidecarIncludesTPS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Default()
	cfg.Source = path
	cfg.Core.DualCoreEnabled = false
	cfg.Core.TPS = 120

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	coreRaw, err := os.ReadFile(filepath.Join(dir, "core.toml"))
	if err != nil {
		t.Fatalf("read core toml: %v", err)
	}
	coreText := string(coreRaw)
	if !strings.Contains(coreText, "dual_core_enabled = false") {
		t.Fatalf("expected dual_core_enabled in core toml, got:\n%s", coreText)
	}
	if !strings.Contains(coreText, "tps = 120") {
		t.Fatalf("expected tps in core toml, got:\n%s", coreText)
	}
}

func TestLoadCoreSidecarNormalizesTPSBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	payload := `
[core]
tps = 999
`
	if err := os.WriteFile(filepath.Join(dir, "core.toml"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write core toml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Core.TPS != 120 {
		t.Fatalf("expected core tps to clamp at 120, got %d", cfg.Core.TPS)
	}
}

func TestLoadConfigTOML_InvalidAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	payload := `
[api]
enabled = true
bind = "127.0.0.1:9000"
key = "abc"
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
	path := filepath.Join(dir, "config.toml")
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

	devRaw, err := os.ReadFile(filepath.Join(dir, "development.toml"))
	if err != nil {
		t.Fatalf("read development toml: %v", err)
	}
	devText := string(devRaw)
	if !strings.Contains(devText, "terminal_player_logs_enabled = false") {
		t.Fatalf("expected terminal player log toggle in development toml, got:\n%s", devText)
	}
	if !strings.Contains(devText, "build_finish_logs_enabled = false") {
		t.Fatalf("expected build finish terminal toggle in development toml, got:\n%s", devText)
	}
	if strings.Contains(devText, "net_event_logs_enabled") {
		t.Fatalf("development toml should not contain file log toggles, got:\n%s", devText)
	}

	sundriesRaw, err := os.ReadFile(filepath.Join(dir, "sundries.toml"))
	if err != nil {
		t.Fatalf("read sundries toml: %v", err)
	}
	sundriesText := string(sundriesRaw)
	if !strings.Contains(sundriesText, "net_event_logs_enabled = false") {
		t.Fatalf("expected net event log toggle in sundries toml, got:\n%s", sundriesText)
	}
	if !strings.Contains(sundriesText, "chat_logs_enabled = false") {
		t.Fatalf("expected chat log toggle in sundries toml, got:\n%s", sundriesText)
	}
	if !strings.Contains(sundriesText, "build_finish_logs_enabled = false") {
		t.Fatalf("expected build finish file toggle in sundries toml, got:\n%s", sundriesText)
	}
	if strings.Contains(sundriesText, "packet_recv_events_enabled") {
		t.Fatalf("sundries toml should not contain development packet toggles, got:\n%s", sundriesText)
	}
}

func TestSaveSidecarsRelativizesWorkspaceAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configs")
	path := filepath.Join(configDir, "config.toml")
	cfg := Default()
	cfg.Source = path
	cfg.Runtime.AssetsDir = filepath.Join(root, "assets")
	cfg.Runtime.WorldsDir = filepath.Join(root, "assets", "worlds")
	cfg.Runtime.LogsDir = filepath.Join(root, "logs")
	cfg.Runtime.VanillaProfiles = filepath.Join(root, "data", "vanilla", "profiles.json")
	cfg.Storage.Directory = filepath.Join(root, "data", "events")
	cfg.Mods.Directory = filepath.Join(root, "mods")
	cfg.Mods.JSDir = filepath.Join(root, "mods", "js")
	cfg.Mods.GoDir = filepath.Join(root, "mods", "go")
	cfg.Mods.NodeDir = filepath.Join(root, "mods", "node")
	cfg.Persist.Directory = filepath.Join(root, "data", "state")
	cfg.Persist.MSAVDir = filepath.Join(root, "data", "snapshots")
	cfg.Script.File = filepath.Join(root, "data", "state", "scripts.json")
	cfg.Admin.OpsFile = filepath.Join(configDir, "json", "ops.json")
	cfg.Admin.WhitelistFile = filepath.Join(root, "data", "state", "whitelist.json")
	cfg.Control.PublicConnUUIDFile = filepath.Join(configDir, "json", "conn_uuid.json")
	cfg.Personalization.PlayerIdentityFile = filepath.Join(configDir, "json", "player_identity.json")
	cfg.Tracepoints.File = filepath.Join(root, "logs", "tracepoints.jsonl")

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	miscRaw, err := os.ReadFile(filepath.Join(configDir, "misc.toml"))
	if err != nil {
		t.Fatalf("read misc toml: %v", err)
	}
	miscText := string(miscRaw)
	for _, want := range []string{
		`assets_dir = "assets"`,
		`worlds_dir = "assets\worlds"`,
		`logs_dir = "logs"`,
		`directory = "data\events"`,
		`ops_file = "configs\json\ops.json"`,
		`whitelist_file = "data\state\whitelist.json"`,
	} {
		if !strings.Contains(miscText, want) {
			t.Fatalf("expected %q in misc.toml, got:\n%s", want, miscText)
		}
	}
	if strings.Contains(miscText, root) {
		t.Fatalf("expected misc.toml to avoid absolute workspace paths, got:\n%s", miscText)
	}

	mainRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read main config: %v", err)
	}
	mainText := string(mainRaw)
	if !strings.Contains(mainText, `public_conn_uuid_file = "json\conn_uuid.json"`) {
		t.Fatalf("expected relative public conn uuid file, got:\n%s", mainText)
	}
}

func TestLoadConfigTOMLSidecarsWithoutMainConfig(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(filepath.Join(dir, "development.toml"), []byte(`
[development]
terminal_player_logs_enabled = false
`), 0o644); err != nil {
		t.Fatalf("write development toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sundries.toml"), []byte(`
[sundries]
chat_logs_enabled = false
build_finish_logs_enabled = false
`), 0o644); err != nil {
		t.Fatalf("write sundries toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sync.toml"), []byte(`
[sync]
block_sync_logs_enabled = true
`), 0o644); err != nil {
		t.Fatalf("write sync toml: %v", err)
	}

	cfg, err := Load(mainPath)
	if err != nil {
		t.Fatalf("load config from toml sidecars only: %v", err)
	}
	if cfg.Development.TerminalPlayerLogsEnabled {
		t.Fatalf("expected development sidecar toml to load without main config")
	}
	if cfg.Sundries.ChatLogsEnabled {
		t.Fatalf("expected sundries chat log toggle from toml")
	}
	if cfg.Sundries.BuildFinishLogsEnabled {
		t.Fatalf("expected sundries build finish toggle from toml")
	}
	if !cfg.Sync.BlockSyncLogsEnabled {
		t.Fatalf("expected sync.toml to load")
	}
}

func TestLoadConfigTOML_PreservesMindustryColorTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	payload := `
[server]
name = "[#F285D1]镜[#F285E3]像[#E59AFF]物[#CC99FF]语"
desc = "[accent]欢迎[] [#87ceeb]测试[]"
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
	path := filepath.Join(dir, "config.toml")

	payload := `
[join_popup]
enabled = true
delay_ms = 150
title = "[accent]入服菜单[]"
message = """
第一行
[accent]
第二行[]
"""
announcement_text = """
[accent]公告[]

[#87ceeb]第三行[]
"""
link_url = "https://example.com/rules"
help_text = """
[white]/help[] 查看帮助
[white]/sync[] 请求同步
"""
`
	if err := os.WriteFile(filepath.Join(dir, "join_popup.toml"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write join popup toml: %v", err)
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
	path := filepath.Join(dir, "config.toml")

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

	popupRaw, err := os.ReadFile(filepath.Join(dir, "join_popup.toml"))
	if err != nil {
		t.Fatalf("read join popup toml: %v", err)
	}
	popupText := string(popupRaw)
	for _, want := range []string{
		"[join_popup]",
		"enabled = true",
		"delay_ms = 480",
		"title = \"[accent]测试公告[]\"",
		"message = \"\"\"\n第一行\n第二行\n\"\"\"",
		"announcement_text = \"\"\"\n[accent]公告[]\n\n[#87ceeb]内容[]\n\"\"\"",
		"link_url = \"https://example.com/join\"",
		"help_text = \"\"\"\n[white]/help[]\n[white]/sync[]\n\"\"\"",
	} {
		if !strings.Contains(popupText, want) {
			t.Fatalf("expected join popup toml to contain %q, got:\n%s", want, popupText)
		}
	}

	personalizationRaw, err := os.ReadFile(filepath.Join(dir, "personalization.toml"))
	if err != nil {
		t.Fatalf("read personalization toml: %v", err)
	}
	personalizationText := string(personalizationRaw)
	if strings.Contains(personalizationText, "join_popup_") {
		t.Fatalf("personalization toml should not contain join popup fields, got:\n%s", personalizationText)
	}
}

func TestLoadMapVoteSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	payload := `
[map_vote]
duration_sec = 21
status_refresh_ms = 1200
popup_duration_ms = 1600
home_link_url = "https://example.com/maps"
align = "left"
top = 32
left = 14
bottom = 7
right = 3
`
	if err := os.WriteFile(filepath.Join(dir, "map_vote.toml"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write vote map toml: %v", err)
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
	path := filepath.Join(dir, "config.toml")

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

	raw, err := os.ReadFile(filepath.Join(dir, "map_vote.toml"))
	if err != nil {
		t.Fatalf("read vote map toml: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"[map_vote]",
		"duration_sec = 25",
		"status_refresh_ms = 900",
		"popup_duration_ms = 1300",
		"home_link_url = \"https://example.com/votemap\"",
		"align = \"left\"",
		"top = 48",
		"left = 12",
		"bottom = 5",
		"right = 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected vote map toml to contain %q, got:\n%s", want, text)
		}
	}
}
