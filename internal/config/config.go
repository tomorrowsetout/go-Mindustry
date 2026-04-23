package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type RuntimeConfig struct {
	Cores            int
	SchedulerEnabled bool
	VanillaProfiles  string
	AssetsDir        string
	WorldsDir        string
	LogsDir          string
	ServerName       string
	ServerDesc       string
	VirtualPlayers   int
	DevLogEnabled    bool
}

type CoreConfig struct {
	DualCoreEnabled        bool
	TPS                    int
	MemoryLimitMB          int
	MemoryStartupMaxMB     int
	MemoryGCTriggerMB      int
	MemoryCheckIntervalSec int
	MemoryFreeOSMemory     bool
}

type APIConfig struct {
	Enabled    bool
	Key        string
	Keys       []string
	Bind       string
	ConfigFile string
}

type StorageConfig struct {
	Mode            string
	Directory       string
	DatabaseEnabled bool
	DSN             string
}

type NetConfig struct {
	UdpRetryCount   int
	UdpRetryDelayMs int
	UdpFallbackTCP  bool
	SyncEntityMs    int
	SyncStateMs     int
}

type AuthoritySyncStrategy string

const (
	AuthoritySyncOfficial AuthoritySyncStrategy = "official"
	AuthoritySyncStatic   AuthoritySyncStrategy = "static"
	AuthoritySyncDynamic  AuthoritySyncStrategy = "dynamic"
)

func normalizeAuthoritySyncStrategy(v string) AuthoritySyncStrategy {
	if parsed, ok := ParseAuthoritySyncStrategy(v); ok {
		return parsed
	}
	return AuthoritySyncDynamic
}

func ParseAuthoritySyncStrategy(v string) (AuthoritySyncStrategy, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(AuthoritySyncOfficial):
		return AuthoritySyncOfficial, true
	case string(AuthoritySyncStatic):
		return AuthoritySyncStatic, true
	case string(AuthoritySyncDynamic):
		return AuthoritySyncDynamic, true
	default:
		return "", false
	}
}

type SyncConfig struct {
	Strategy               AuthoritySyncStrategy
	UseMapSyncDataFallback bool
	BlockSyncLogsEnabled   bool
}

type PersistConfig struct {
	Enabled     bool
	Directory   string
	File        string
	IntervalSec int
	SaveMSAV    bool
	MSAVDir     string
	MSAVFile    string
}

type ModsConfig struct {
	Enabled            bool
	Directory          string
	JavaHome           string
	JSDir              string
	GoDir              string
	NodeDir            string
	ExpectedClientMods []string
}

type ScriptTask struct {
	DelaySec int
	Runtime  string
	Target   string
	Args     []string
}

type ScriptConfig struct {
	File         string
	StartupTasks []ScriptTask
	DailyGCTime  string
}

type AdminConfig struct {
	OpsFile            string
	PlayerLimit        int
	StrictIdentity     bool
	AllowCustomClients bool
	WhitelistEnabled   bool
	WhitelistFile      string
	BannedNames        []string
	BannedSubnets      []string
	RecentKickSeconds  int
}

type SundriesConfig struct {
	DetailedLogMaxMB           int
	DetailedLogMaxFiles        int
	NetEventLogsEnabled        bool
	ChatLogsEnabled            bool
	RespawnCoreLogsEnabled     bool
	RespawnUnitLogsEnabled     bool
	BuildPlaceLogsEnabled      bool
	BuildFinishLogsEnabled     bool
	BuildBreakStartLogsEnabled bool
	BuildBreakDoneLogsEnabled  bool
}

type ControlConfig struct {
	ReloadIntervalSec               int
	ReloadLogEnabled                bool
	TranslatedConnLogEnabled        bool
	PublicConnUUIDEnabled           bool
	PublicConnUUIDFile              string
	ConnUUIDAutoCreateEnabled       bool
	PlayerIdentityAutoCreateEnabled bool
}

type DevelopmentConfig struct {
	PacketEventsEnabled        bool
	PacketRecvEventsEnabled    bool
	PacketSendEventsEnabled    bool
	TerminalPlayerLogsEnabled  bool
	TerminalPlayerUUIDEnabled  bool
	RespawnCoreLogsEnabled     bool
	RespawnUnitLogsEnabled     bool
	RespawnPacketLogsEnabled   bool
	BuildSnapshotLogsEnabled   bool
	BuildPlaceLogsEnabled      bool
	BuildFinishLogsEnabled     bool
	BuildBreakStartLogsEnabled bool
	BuildBreakDoneLogsEnabled  bool
}

type PersonalizationConfig struct {
	StartupReportEnabled          bool
	MapLoadDetailsEnabled         bool
	UnitIDListEnabled             bool
	StartupCurrentMapLineEnabled  bool
	ConsoleIntroEnabled           bool
	ConsoleIntroServerNameEnabled bool
	ConsoleIntroCurrentMapEnabled bool
	ConsoleIntroListenAddrEnabled bool
	ConsoleIntroLocalIPEnabled    bool
	ConsoleIntroAPIEnabled        bool
	ConsoleIntroHelpHintEnabled   bool
	StartupHelpEnabled            bool
	JoinLeaveChatEnabled          bool
	PlayerNameColorEnabled        bool
	PlayerNamePrefix              string
	PlayerNameSuffix              string
	PlayerBindPrefixEnabled       bool
	PlayerBoundPrefix             string
	PlayerUnboundPrefix           string
	PlayerTitleEnabled            bool
	PlayerIdentityFile            string
	PlayerBindSource              string
	PlayerBindAPIURL              string
	PlayerBindAPITimeoutMs        int
	PlayerBindAPICacheSec         int
	PlayerConnIDSuffixEnabled     bool
	PlayerConnIDSuffixFormat      string
	MainConsoleTitle              string
	Core2ConsoleTitle             string
	Core3ConsoleTitle             string
	Core4ConsoleTitle             string
	Core2ProcessName              string
	Core3ProcessName              string
	Core4ProcessName              string
}

type JoinPopupConfig struct {
	Enabled          bool
	DelayMs          int
	Title            string
	Message          string
	AnnouncementText string
	LinkURL          string
	HelpText         string
}

type StatusBarConfig struct {
	Enabled              bool
	RefreshIntervalSec   int
	PopupDurationMs      int
	Align                string
	Top                  int
	Left                 int
	Bottom               int
	Right                int
	PopupID              string
	HeaderEnabled        bool
	HeaderText           string
	ServerNameEnabled    bool
	ServerNameFormat     string
	PerformanceEnabled   bool
	PerformanceFormat    string
	CurrentMapEnabled    bool
	CurrentMapFormat     string
	GameTimeEnabled      bool
	GameTimeFormat       string
	PlayerCountEnabled   bool
	PlayerCountFormat    string
	WelcomeEnabled       bool
	WelcomeFormat        string
	QQGroupEnabled       bool
	QQGroupText          string
	QQGroupFormat        string
	CustomMessageEnabled bool
	CustomMessageText    string
	CustomMessageFormat  string
}

type MapVoteConfig struct {
	DurationSec     int
	StatusRefreshMs int
	PopupDurationMs int
	HomeLinkURL     string
	Align           string
	Top             int
	Left            int
	Bottom          int
	Right           int
}

type BuildingLogConfig struct {
	Enabled    bool
	Translated bool
}

type TracepointsConfig struct {
	Enabled               bool
	File                  string
	ClientRequestsEnabled bool
	ServerSendsEnabled    bool
	WorldRuntimeEnabled   bool
	StateBuildEnabled     bool
	WorldStreamEnabled    bool
}

type Config struct {
	Source          string
	Control         ControlConfig
	Development     DevelopmentConfig
	Personalization PersonalizationConfig
	JoinPopup       JoinPopupConfig
	StatusBar       StatusBarConfig
	MapVote         MapVoteConfig
	Building        BuildingLogConfig
	Tracepoints     TracepointsConfig
	Sundries        SundriesConfig
	Runtime         RuntimeConfig
	Core            CoreConfig
	API             APIConfig
	Storage         StorageConfig
	Net             NetConfig
	Sync            SyncConfig
	Persist         PersistConfig
	Mods            ModsConfig
	Script          ScriptConfig
	Admin           AdminConfig
}

var apiKeyPattern = regexp.MustCompile(`^mdt-server-go-[a-z0-9]{15}-[a-z0-9]{13}-[a-z0-9]{15}-[a-z0-9]{19}-[a-z0-9]{12}-yzf-[a-z0-9]{10}$`)

func IsValidAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	return apiKeyPattern.MatchString(key)
}

func ValidateAPIKeys(keys []string) error {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !IsValidAPIKey(key) {
			return fmt.Errorf("API密钥不合格: %s", key)
		}
	}
	return nil
}

type iniData map[string]map[string]string

func newIniData() iniData {
	return map[string]map[string]string{}
}

func (d iniData) set(section, key, value string) {
	section = strings.ToLower(strings.TrimSpace(section))
	key = strings.ToLower(strings.TrimSpace(key))
	if section == "" || key == "" {
		return
	}
	if _, ok := d[section]; !ok {
		d[section] = map[string]string{}
	}
	d[section][key] = strings.TrimSpace(value)
}

func (d iniData) get(section, key string) (string, bool) {
	section = strings.ToLower(strings.TrimSpace(section))
	key = strings.ToLower(strings.TrimSpace(key))
	m, ok := d[section]
	if !ok {
		return "", false
	}
	v, ok := m[key]
	return v, ok
}

func parseINI(path string) (iniData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := newIniData()
	section := ""
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "[") && strings.Contains(s, "]") {
			i := strings.Index(s, "]")
			section = strings.ToLower(strings.TrimSpace(s[1:i]))
			continue
		}
		s = stripINIComment(s)
		if s == "" {
			continue
		}
		eq := strings.Index(s, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(s[:eq])
		val := strings.TrimSpace(s[eq+1:])
		out.set(section, key, val)
	}
	return out, nil
}

func stripINIComment(s string) string {
	inBracket := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			inBracket = true
		case ']':
			inBracket = false
		case ';', '#':
			if inBracket {
				continue
			}
			if i == 0 {
				return ""
			}
			prev := s[i-1]
			if prev == ' ' || prev == '\t' {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func decodeINIInlineText(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, `\r\n`, "\n")
	v = strings.ReplaceAll(v, `\n`, "\n")
	return v
}

func boolToIni(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func asBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func asInt(v string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

func asCSV(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseConfigData(path string) (iniData, error) {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".toml":
		return parseTOML(path)
	default:
		return nil, fmt.Errorf("仅支持 TOML 配置文件: %s", path)
	}
}

func parseTOML(path string) (iniData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), "\n")
	out := newIniData()
	section := ""
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.HasPrefix(line, "[[") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		valueText := strings.TrimSpace(line[eq+1:])
		if strings.HasPrefix(valueText, `"""`) {
			value, next, err := readTOMLMultilineValue(lines, i, valueText)
			if err != nil {
				return nil, fmt.Errorf("parse toml %s line %d: %w", path, i+1, err)
			}
			out.set(section, key, value)
			i = next
			continue
		}
		valueText = stripTOMLComment(valueText)
		if valueText == "" {
			continue
		}
		value, err := decodeTOMLValue(valueText)
		if err != nil {
			return nil, fmt.Errorf("parse toml %s line %d: %w", path, i+1, err)
		}
		out.set(section, key, value)
	}
	return out, nil
}

func readTOMLMultilineValue(lines []string, start int, first string) (string, int, error) {
	if !strings.HasPrefix(first, `"""`) {
		return "", start, fmt.Errorf("missing multiline delimiter")
	}
	first = first[3:]
	if end := strings.Index(first, `"""`); end >= 0 {
		return first[:end], start, nil
	}
	parts := make([]string, 0, 8)
	if first != "" {
		parts = append(parts, first)
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if end := strings.Index(line, `"""`); end >= 0 {
			if prefix := line[:end]; prefix != "" || len(parts) == 0 {
				parts = append(parts, prefix)
			}
			return strings.Join(parts, "\n"), i, nil
		}
		parts = append(parts, line)
	}
	return "", start, fmt.Errorf("unterminated multiline string")
}

func stripTOMLComment(s string) string {
	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			if inDouble {
				escaped = true
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '#':
			if !inDouble && !inSingle {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func decodeTOMLValue(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", nil
	case strings.HasPrefix(v, `"""`) && strings.HasSuffix(v, `"""`) && len(v) >= 6:
		return v[3 : len(v)-3], nil
	case strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) && len(v) >= 2:
		return decodeTOMLBasicString(v[1 : len(v)-1]), nil
	case strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`) && len(v) >= 2:
		return v[1 : len(v)-1], nil
	case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
		items, err := parseTOMLArray(v[1 : len(v)-1])
		if err != nil {
			return "", err
		}
		return strings.Join(items, ","), nil
	default:
		return v, nil
	}
}

func decodeTOMLBasicString(v string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
		`\"`, `"`,
	)
	return replacer.Replace(v)
}

func parseTOMLArray(v string) ([]string, error) {
	out := make([]string, 0, 4)
	parts := splitTOMLArray(v)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := decodeTOMLValue(part)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func splitTOMLArray(v string) []string {
	out := make([]string, 0, 4)
	var buf strings.Builder
	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			if inDouble {
				escaped = true
			}
			buf.WriteByte(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			buf.WriteByte(ch)
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			buf.WriteByte(ch)
		case ',':
			if !inDouble && !inSingle {
				out = append(out, buf.String())
				buf.Reset()
				continue
			}
			buf.WriteByte(ch)
		default:
			buf.WriteByte(ch)
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		out = append(out, buf.String())
	}
	return out
}

func applyINI(cfg *Config, d iniData) {
	if cfg == nil || d == nil {
		return
	}
	if v, ok := d.get("config", "reload_interval_sec"); ok {
		cfg.Control.ReloadIntervalSec = asInt(v, cfg.Control.ReloadIntervalSec)
	}
	if v, ok := d.get("config", "reload_log_enabled"); ok {
		cfg.Control.ReloadLogEnabled = asBool(v, cfg.Control.ReloadLogEnabled)
	}
	if v, ok := d.get("config", "translated_conn_log_enabled"); ok {
		cfg.Control.TranslatedConnLogEnabled = asBool(v, cfg.Control.TranslatedConnLogEnabled)
	}
	if v, ok := d.get("config", "public_conn_uuid_enabled"); ok {
		cfg.Control.PublicConnUUIDEnabled = asBool(v, cfg.Control.PublicConnUUIDEnabled)
	}
	if v, ok := d.get("config", "public_conn_uuid_file"); ok && strings.TrimSpace(v) != "" {
		cfg.Control.PublicConnUUIDFile = strings.TrimSpace(v)
	}
	if v, ok := d.get("config", "conn_uuid_auto_create"); ok {
		cfg.Control.ConnUUIDAutoCreateEnabled = asBool(v, cfg.Control.ConnUUIDAutoCreateEnabled)
	}
	if v, ok := d.get("config", "player_identity_auto_create"); ok {
		cfg.Control.PlayerIdentityAutoCreateEnabled = asBool(v, cfg.Control.PlayerIdentityAutoCreateEnabled)
	}
	if v, ok := d.get("authority_sync", "strategy"); ok {
		cfg.Sync.Strategy = normalizeAuthoritySyncStrategy(v)
	}
	if v, ok := d.get("sync", "strategy"); ok {
		cfg.Sync.Strategy = normalizeAuthoritySyncStrategy(v)
	}
	if v, ok := d.get("development", "packet_events_enabled"); ok {
		enabled := asBool(v, cfg.Development.PacketEventsEnabled)
		cfg.Development.PacketEventsEnabled = enabled
		cfg.Development.PacketRecvEventsEnabled = enabled
		cfg.Development.PacketSendEventsEnabled = enabled
	}
	if v, ok := d.get("development", "packet_recv_events_enabled"); ok {
		cfg.Development.PacketRecvEventsEnabled = asBool(v, cfg.Development.PacketRecvEventsEnabled)
	}
	if v, ok := d.get("development", "packet_send_events_enabled"); ok {
		cfg.Development.PacketSendEventsEnabled = asBool(v, cfg.Development.PacketSendEventsEnabled)
	}
	if v, ok := d.get("development", "terminal_player_logs_enabled"); ok {
		cfg.Development.TerminalPlayerLogsEnabled = asBool(v, cfg.Development.TerminalPlayerLogsEnabled)
	}
	if v, ok := d.get("development", "terminal_player_uuid_enabled"); ok {
		cfg.Development.TerminalPlayerUUIDEnabled = asBool(v, cfg.Development.TerminalPlayerUUIDEnabled)
	}
	if v, ok := d.get("development", "respawn_core_logs_enabled"); ok {
		cfg.Development.RespawnCoreLogsEnabled = asBool(v, cfg.Development.RespawnCoreLogsEnabled)
	}
	if v, ok := d.get("development", "respawn_unit_logs_enabled"); ok {
		cfg.Development.RespawnUnitLogsEnabled = asBool(v, cfg.Development.RespawnUnitLogsEnabled)
	}
	if v, ok := d.get("development", "respawn_packet_logs_enabled"); ok {
		cfg.Development.RespawnPacketLogsEnabled = asBool(v, cfg.Development.RespawnPacketLogsEnabled)
	}
	if v, ok := d.get("development", "build_snapshot_logs_enabled"); ok {
		cfg.Development.BuildSnapshotLogsEnabled = asBool(v, cfg.Development.BuildSnapshotLogsEnabled)
	}
	if v, ok := d.get("development", "build_place_logs_enabled"); ok {
		cfg.Development.BuildPlaceLogsEnabled = asBool(v, cfg.Development.BuildPlaceLogsEnabled)
	}
	if v, ok := d.get("development", "build_finish_logs_enabled"); ok {
		cfg.Development.BuildFinishLogsEnabled = asBool(v, cfg.Development.BuildFinishLogsEnabled)
	}
	if v, ok := d.get("development", "build_break_start_logs_enabled"); ok {
		cfg.Development.BuildBreakStartLogsEnabled = asBool(v, cfg.Development.BuildBreakStartLogsEnabled)
	}
	if v, ok := d.get("development", "build_break_done_logs_enabled"); ok {
		cfg.Development.BuildBreakDoneLogsEnabled = asBool(v, cfg.Development.BuildBreakDoneLogsEnabled)
	}
	if v, ok := d.get("tracepoints", "enabled"); ok {
		cfg.Tracepoints.Enabled = asBool(v, cfg.Tracepoints.Enabled)
	}
	if v, ok := d.get("tracepoints", "file"); ok && strings.TrimSpace(v) != "" {
		cfg.Tracepoints.File = strings.TrimSpace(v)
	}
	if v, ok := d.get("tracepoints", "client_requests_enabled"); ok {
		cfg.Tracepoints.ClientRequestsEnabled = asBool(v, cfg.Tracepoints.ClientRequestsEnabled)
	}
	if v, ok := d.get("tracepoints", "server_sends_enabled"); ok {
		cfg.Tracepoints.ServerSendsEnabled = asBool(v, cfg.Tracepoints.ServerSendsEnabled)
	}
	if v, ok := d.get("tracepoints", "world_runtime_enabled"); ok {
		cfg.Tracepoints.WorldRuntimeEnabled = asBool(v, cfg.Tracepoints.WorldRuntimeEnabled)
	}
	if v, ok := d.get("tracepoints", "state_build_enabled"); ok {
		cfg.Tracepoints.StateBuildEnabled = asBool(v, cfg.Tracepoints.StateBuildEnabled)
	}
	if v, ok := d.get("tracepoints", "world_stream_enabled"); ok {
		cfg.Tracepoints.WorldStreamEnabled = asBool(v, cfg.Tracepoints.WorldStreamEnabled)
	}
	if v, ok := d.get("personalization", "startup_report_enabled"); ok {
		cfg.Personalization.StartupReportEnabled = asBool(v, cfg.Personalization.StartupReportEnabled)
	}
	if v, ok := d.get("personalization", "map_load_details_enabled"); ok {
		cfg.Personalization.MapLoadDetailsEnabled = asBool(v, cfg.Personalization.MapLoadDetailsEnabled)
	}
	if v, ok := d.get("personalization", "unit_id_list_enabled"); ok {
		cfg.Personalization.UnitIDListEnabled = asBool(v, cfg.Personalization.UnitIDListEnabled)
	}
	if v, ok := d.get("personalization", "startup_current_map_line_enabled"); ok {
		cfg.Personalization.StartupCurrentMapLineEnabled = asBool(v, cfg.Personalization.StartupCurrentMapLineEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_enabled"); ok {
		cfg.Personalization.ConsoleIntroEnabled = asBool(v, cfg.Personalization.ConsoleIntroEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_server_name_enabled"); ok {
		cfg.Personalization.ConsoleIntroServerNameEnabled = asBool(v, cfg.Personalization.ConsoleIntroServerNameEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_current_map_enabled"); ok {
		cfg.Personalization.ConsoleIntroCurrentMapEnabled = asBool(v, cfg.Personalization.ConsoleIntroCurrentMapEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_listen_addr_enabled"); ok {
		cfg.Personalization.ConsoleIntroListenAddrEnabled = asBool(v, cfg.Personalization.ConsoleIntroListenAddrEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_local_ip_enabled"); ok {
		cfg.Personalization.ConsoleIntroLocalIPEnabled = asBool(v, cfg.Personalization.ConsoleIntroLocalIPEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_api_enabled"); ok {
		cfg.Personalization.ConsoleIntroAPIEnabled = asBool(v, cfg.Personalization.ConsoleIntroAPIEnabled)
	}
	if v, ok := d.get("personalization", "console_intro_help_hint_enabled"); ok {
		cfg.Personalization.ConsoleIntroHelpHintEnabled = asBool(v, cfg.Personalization.ConsoleIntroHelpHintEnabled)
	}
	if v, ok := d.get("personalization", "startup_help_enabled"); ok {
		cfg.Personalization.StartupHelpEnabled = asBool(v, cfg.Personalization.StartupHelpEnabled)
	}
	if v, ok := d.get("personalization", "join_leave_chat_enabled"); ok {
		cfg.Personalization.JoinLeaveChatEnabled = asBool(v, cfg.Personalization.JoinLeaveChatEnabled)
	}
	if v, ok := d.get("personalization", "player_name_color_enabled"); ok {
		cfg.Personalization.PlayerNameColorEnabled = asBool(v, cfg.Personalization.PlayerNameColorEnabled)
	}
	if v, ok := d.get("personalization", "player_name_prefix"); ok {
		cfg.Personalization.PlayerNamePrefix = v
	}
	if v, ok := d.get("personalization", "player_name_suffix"); ok {
		cfg.Personalization.PlayerNameSuffix = v
	}
	if v, ok := d.get("personalization", "player_bind_prefix_enabled"); ok {
		cfg.Personalization.PlayerBindPrefixEnabled = asBool(v, cfg.Personalization.PlayerBindPrefixEnabled)
	}
	if v, ok := d.get("personalization", "player_bound_prefix"); ok {
		cfg.Personalization.PlayerBoundPrefix = v
	}
	if v, ok := d.get("personalization", "player_unbound_prefix"); ok {
		cfg.Personalization.PlayerUnboundPrefix = v
	}
	if v, ok := d.get("personalization", "player_title_enabled"); ok {
		cfg.Personalization.PlayerTitleEnabled = asBool(v, cfg.Personalization.PlayerTitleEnabled)
	}
	if v, ok := d.get("personalization", "player_identity_file"); ok && strings.TrimSpace(v) != "" {
		cfg.Personalization.PlayerIdentityFile = strings.TrimSpace(v)
	}
	if v, ok := d.get("personalization", "player_bind_source"); ok && strings.TrimSpace(v) != "" {
		cfg.Personalization.PlayerBindSource = strings.TrimSpace(v)
	}
	if v, ok := d.get("personalization", "player_bind_api_url"); ok {
		cfg.Personalization.PlayerBindAPIURL = v
	}
	if v, ok := d.get("personalization", "player_bind_api_timeout_ms"); ok {
		cfg.Personalization.PlayerBindAPITimeoutMs = asInt(v, cfg.Personalization.PlayerBindAPITimeoutMs)
	}
	if v, ok := d.get("personalization", "player_bind_api_cache_sec"); ok {
		cfg.Personalization.PlayerBindAPICacheSec = asInt(v, cfg.Personalization.PlayerBindAPICacheSec)
	}
	if v, ok := d.get("personalization", "player_conn_id_suffix_enabled"); ok {
		cfg.Personalization.PlayerConnIDSuffixEnabled = asBool(v, cfg.Personalization.PlayerConnIDSuffixEnabled)
	}
	if v, ok := d.get("personalization", "player_conn_id_suffix_format"); ok {
		cfg.Personalization.PlayerConnIDSuffixFormat = v
	}
	if v, ok := d.get("personalization", "main_console_title"); ok {
		cfg.Personalization.MainConsoleTitle = v
	}
	if v, ok := d.get("personalization", "core2_console_title"); ok {
		cfg.Personalization.Core2ConsoleTitle = v
	}
	if v, ok := d.get("personalization", "core3_console_title"); ok {
		cfg.Personalization.Core3ConsoleTitle = v
	}
	if v, ok := d.get("personalization", "core4_console_title"); ok {
		cfg.Personalization.Core4ConsoleTitle = v
	}
	if v, ok := d.get("personalization", "join_popup_enabled"); ok {
		cfg.JoinPopup.Enabled = asBool(v, cfg.JoinPopup.Enabled)
	}
	if v, ok := d.get("personalization", "join_popup_delay_ms"); ok {
		cfg.JoinPopup.DelayMs = asInt(v, cfg.JoinPopup.DelayMs)
	}
	if v, ok := d.get("personalization", "join_popup_title"); ok {
		cfg.JoinPopup.Title = decodeINIInlineText(v)
	}
	if v, ok := d.get("personalization", "join_popup_message"); ok {
		cfg.JoinPopup.Message = decodeINIInlineText(v)
	}
	if v, ok := d.get("personalization", "join_popup_announcement_text"); ok {
		cfg.JoinPopup.AnnouncementText = decodeINIInlineText(v)
	}
	if v, ok := d.get("personalization", "join_popup_link_url"); ok {
		cfg.JoinPopup.LinkURL = decodeINIInlineText(v)
	}
	if v, ok := d.get("personalization", "join_popup_help_text"); ok {
		cfg.JoinPopup.HelpText = decodeINIInlineText(v)
	}
	if v, ok := d.get("join_popup", "enabled"); ok {
		cfg.JoinPopup.Enabled = asBool(v, cfg.JoinPopup.Enabled)
	}
	if v, ok := d.get("join_popup", "delay_ms"); ok {
		cfg.JoinPopup.DelayMs = asInt(v, cfg.JoinPopup.DelayMs)
	}
	if v, ok := d.get("join_popup", "title"); ok {
		cfg.JoinPopup.Title = decodeINIInlineText(v)
	}
	if v, ok := d.get("join_popup", "message"); ok {
		cfg.JoinPopup.Message = decodeINIInlineText(v)
	}
	if v, ok := d.get("join_popup", "announcement_text"); ok {
		cfg.JoinPopup.AnnouncementText = decodeINIInlineText(v)
	}
	if v, ok := d.get("join_popup", "link_url"); ok {
		cfg.JoinPopup.LinkURL = decodeINIInlineText(v)
	}
	if v, ok := d.get("join_popup", "help_text"); ok {
		cfg.JoinPopup.HelpText = decodeINIInlineText(v)
	}
	if v, ok := d.get("status_bar", "enabled"); ok {
		cfg.StatusBar.Enabled = asBool(v, cfg.StatusBar.Enabled)
	}
	if v, ok := d.get("status_bar", "refresh_interval_sec"); ok {
		cfg.StatusBar.RefreshIntervalSec = asInt(v, cfg.StatusBar.RefreshIntervalSec)
	}
	if v, ok := d.get("status_bar", "popup_duration_ms"); ok {
		cfg.StatusBar.PopupDurationMs = asInt(v, cfg.StatusBar.PopupDurationMs)
	}
	if v, ok := d.get("status_bar", "align"); ok && strings.TrimSpace(v) != "" {
		cfg.StatusBar.Align = strings.TrimSpace(v)
	}
	if v, ok := d.get("status_bar", "top"); ok {
		cfg.StatusBar.Top = asInt(v, cfg.StatusBar.Top)
	}
	if v, ok := d.get("status_bar", "left"); ok {
		cfg.StatusBar.Left = asInt(v, cfg.StatusBar.Left)
	}
	if v, ok := d.get("status_bar", "bottom"); ok {
		cfg.StatusBar.Bottom = asInt(v, cfg.StatusBar.Bottom)
	}
	if v, ok := d.get("status_bar", "right"); ok {
		cfg.StatusBar.Right = asInt(v, cfg.StatusBar.Right)
	}
	if v, ok := d.get("status_bar", "popup_id"); ok {
		cfg.StatusBar.PopupID = v
	}
	if v, ok := d.get("status_bar", "header_enabled"); ok {
		cfg.StatusBar.HeaderEnabled = asBool(v, cfg.StatusBar.HeaderEnabled)
	}
	if v, ok := d.get("status_bar", "header_text"); ok {
		cfg.StatusBar.HeaderText = v
	}
	if v, ok := d.get("status_bar", "server_name_enabled"); ok {
		cfg.StatusBar.ServerNameEnabled = asBool(v, cfg.StatusBar.ServerNameEnabled)
	}
	if v, ok := d.get("status_bar", "server_name_format"); ok {
		cfg.StatusBar.ServerNameFormat = v
	}
	if v, ok := d.get("status_bar", "performance_enabled"); ok {
		cfg.StatusBar.PerformanceEnabled = asBool(v, cfg.StatusBar.PerformanceEnabled)
	}
	if v, ok := d.get("status_bar", "performance_format"); ok {
		cfg.StatusBar.PerformanceFormat = v
	}
	if v, ok := d.get("status_bar", "current_map_enabled"); ok {
		cfg.StatusBar.CurrentMapEnabled = asBool(v, cfg.StatusBar.CurrentMapEnabled)
	}
	if v, ok := d.get("status_bar", "current_map_format"); ok {
		cfg.StatusBar.CurrentMapFormat = v
	}
	if v, ok := d.get("status_bar", "game_time_enabled"); ok {
		cfg.StatusBar.GameTimeEnabled = asBool(v, cfg.StatusBar.GameTimeEnabled)
	}
	if v, ok := d.get("status_bar", "game_time_format"); ok {
		cfg.StatusBar.GameTimeFormat = v
	}
	if v, ok := d.get("status_bar", "player_count_enabled"); ok {
		cfg.StatusBar.PlayerCountEnabled = asBool(v, cfg.StatusBar.PlayerCountEnabled)
	}
	if v, ok := d.get("status_bar", "player_count_format"); ok {
		cfg.StatusBar.PlayerCountFormat = v
	}
	if v, ok := d.get("status_bar", "welcome_enabled"); ok {
		cfg.StatusBar.WelcomeEnabled = asBool(v, cfg.StatusBar.WelcomeEnabled)
	}
	if v, ok := d.get("status_bar", "welcome_format"); ok {
		cfg.StatusBar.WelcomeFormat = v
	}
	if v, ok := d.get("status_bar", "qq_group_enabled"); ok {
		cfg.StatusBar.QQGroupEnabled = asBool(v, cfg.StatusBar.QQGroupEnabled)
	}
	if v, ok := d.get("status_bar", "qq_group_text"); ok {
		cfg.StatusBar.QQGroupText = v
	}
	if v, ok := d.get("status_bar", "qq_group_format"); ok {
		cfg.StatusBar.QQGroupFormat = v
	}
	if v, ok := d.get("status_bar", "custom_message_enabled"); ok {
		cfg.StatusBar.CustomMessageEnabled = asBool(v, cfg.StatusBar.CustomMessageEnabled)
	}
	if v, ok := d.get("status_bar", "custom_message_text"); ok {
		cfg.StatusBar.CustomMessageText = v
	}
	if v, ok := d.get("status_bar", "custom_message_format"); ok {
		cfg.StatusBar.CustomMessageFormat = v
	}
	if v, ok := d.get("map_vote", "duration_sec"); ok {
		cfg.MapVote.DurationSec = asInt(v, cfg.MapVote.DurationSec)
	}
	if v, ok := d.get("map_vote", "status_refresh_ms"); ok {
		cfg.MapVote.StatusRefreshMs = asInt(v, cfg.MapVote.StatusRefreshMs)
	}
	if v, ok := d.get("map_vote", "popup_duration_ms"); ok {
		cfg.MapVote.PopupDurationMs = asInt(v, cfg.MapVote.PopupDurationMs)
	}
	if v, ok := d.get("map_vote", "home_link_url"); ok {
		cfg.MapVote.HomeLinkURL = decodeINIInlineText(v)
	}
	if v, ok := d.get("map_vote", "align"); ok && strings.TrimSpace(v) != "" {
		cfg.MapVote.Align = strings.TrimSpace(v)
	}
	if v, ok := d.get("map_vote", "top"); ok {
		cfg.MapVote.Top = asInt(v, cfg.MapVote.Top)
	}
	if v, ok := d.get("map_vote", "left"); ok {
		cfg.MapVote.Left = asInt(v, cfg.MapVote.Left)
	}
	if v, ok := d.get("map_vote", "bottom"); ok {
		cfg.MapVote.Bottom = asInt(v, cfg.MapVote.Bottom)
	}
	if v, ok := d.get("map_vote", "right"); ok {
		cfg.MapVote.Right = asInt(v, cfg.MapVote.Right)
	}
	if v, ok := d.get("config", "api_file"); ok && strings.TrimSpace(v) != "" {
		cfg.API.ConfigFile = strings.TrimSpace(v)
	}
	if v, ok := d.get("building", "log_enabled"); ok {
		cfg.Building.Enabled = asBool(v, cfg.Building.Enabled)
	}
	if v, ok := d.get("building", "translated_enabled"); ok {
		cfg.Building.Translated = asBool(v, cfg.Building.Translated)
	}
	if v, ok := d.get("sundries", "detailed_log_max_mb"); ok {
		cfg.Sundries.DetailedLogMaxMB = asInt(v, cfg.Sundries.DetailedLogMaxMB)
	}
	if v, ok := d.get("sundries", "detailed_log_max_files"); ok {
		cfg.Sundries.DetailedLogMaxFiles = asInt(v, cfg.Sundries.DetailedLogMaxFiles)
	}
	if v, ok := d.get("sundries", "net_event_logs_enabled"); ok {
		cfg.Sundries.NetEventLogsEnabled = asBool(v, cfg.Sundries.NetEventLogsEnabled)
	}
	if v, ok := d.get("sundries", "chat_logs_enabled"); ok {
		cfg.Sundries.ChatLogsEnabled = asBool(v, cfg.Sundries.ChatLogsEnabled)
	}
	if v, ok := d.get("sundries", "respawn_core_logs_enabled"); ok {
		cfg.Sundries.RespawnCoreLogsEnabled = asBool(v, cfg.Sundries.RespawnCoreLogsEnabled)
	}
	if v, ok := d.get("sundries", "respawn_unit_logs_enabled"); ok {
		cfg.Sundries.RespawnUnitLogsEnabled = asBool(v, cfg.Sundries.RespawnUnitLogsEnabled)
	}
	if v, ok := d.get("sundries", "build_place_logs_enabled"); ok {
		cfg.Sundries.BuildPlaceLogsEnabled = asBool(v, cfg.Sundries.BuildPlaceLogsEnabled)
	}
	if v, ok := d.get("sundries", "build_finish_logs_enabled"); ok {
		cfg.Sundries.BuildFinishLogsEnabled = asBool(v, cfg.Sundries.BuildFinishLogsEnabled)
	}
	if v, ok := d.get("sundries", "build_break_start_logs_enabled"); ok {
		cfg.Sundries.BuildBreakStartLogsEnabled = asBool(v, cfg.Sundries.BuildBreakStartLogsEnabled)
	}
	if v, ok := d.get("sundries", "build_break_done_logs_enabled"); ok {
		cfg.Sundries.BuildBreakDoneLogsEnabled = asBool(v, cfg.Sundries.BuildBreakDoneLogsEnabled)
	}
	if v, ok := d.get("runtime", "cores"); ok {
		cfg.Runtime.Cores = asInt(v, cfg.Runtime.Cores)
	}
	if v, ok := d.get("runtime", "scheduler_enabled"); ok {
		cfg.Runtime.SchedulerEnabled = asBool(v, cfg.Runtime.SchedulerEnabled)
	}
	if v, ok := d.get("runtime", "devlog_enabled"); ok {
		cfg.Runtime.DevLogEnabled = asBool(v, cfg.Runtime.DevLogEnabled)
	}
	if v, ok := d.get("runtime", "vanilla_profiles"); ok && strings.TrimSpace(v) != "" {
		cfg.Runtime.VanillaProfiles = strings.TrimSpace(v)
	}
	if v, ok := d.get("core", "dual_core_enabled"); ok {
		cfg.Core.DualCoreEnabled = asBool(v, cfg.Core.DualCoreEnabled)
	}
	if v, ok := d.get("core", "tps"); ok {
		cfg.Core.TPS = asInt(v, cfg.Core.TPS)
	}
	if v, ok := d.get("memory", "limit_mb"); ok {
		cfg.Core.MemoryLimitMB = asInt(v, cfg.Core.MemoryLimitMB)
	}
	if v, ok := d.get("memory", "startup_max_mb"); ok {
		cfg.Core.MemoryStartupMaxMB = asInt(v, cfg.Core.MemoryStartupMaxMB)
	}
	if v, ok := d.get("memory", "gc_trigger_mb"); ok {
		cfg.Core.MemoryGCTriggerMB = asInt(v, cfg.Core.MemoryGCTriggerMB)
	}
	if v, ok := d.get("memory", "check_interval_sec"); ok {
		cfg.Core.MemoryCheckIntervalSec = asInt(v, cfg.Core.MemoryCheckIntervalSec)
	}
	if v, ok := d.get("memory", "free_os_memory"); ok {
		cfg.Core.MemoryFreeOSMemory = asBool(v, cfg.Core.MemoryFreeOSMemory)
	}

	if v, ok := d.get("server", "name"); ok {
		cfg.Runtime.ServerName = strings.TrimSpace(v)
	}
	if v, ok := d.get("server", "desc"); ok {
		cfg.Runtime.ServerDesc = strings.TrimSpace(v)
	}
	if v, ok := d.get("server", "virtual_players"); ok {
		cfg.Runtime.VirtualPlayers = asInt(v, cfg.Runtime.VirtualPlayers)
	}

	if v, ok := d.get("sync", "entity_ms"); ok {
		cfg.Net.SyncEntityMs = asInt(v, cfg.Net.SyncEntityMs)
	}
	if v, ok := d.get("sync", "state_ms"); ok {
		cfg.Net.SyncStateMs = asInt(v, cfg.Net.SyncStateMs)
	}
	if v, ok := d.get("sync", "udp_retry_count"); ok {
		cfg.Net.UdpRetryCount = asInt(v, cfg.Net.UdpRetryCount)
	}
	if v, ok := d.get("sync", "udp_retry_delay_ms"); ok {
		cfg.Net.UdpRetryDelayMs = asInt(v, cfg.Net.UdpRetryDelayMs)
	}
	if v, ok := d.get("sync", "udp_fallback_tcp"); ok {
		cfg.Net.UdpFallbackTCP = asBool(v, cfg.Net.UdpFallbackTCP)
	}
	if v, ok := d.get("sync", "use_map_sync_data_fallback"); ok {
		cfg.Sync.UseMapSyncDataFallback = asBool(v, cfg.Sync.UseMapSyncDataFallback)
	}
	if v, ok := d.get("sync", "block_sync_logs_enabled"); ok {
		cfg.Sync.BlockSyncLogsEnabled = asBool(v, cfg.Sync.BlockSyncLogsEnabled)
	}

	if v, ok := d.get("data", "mode"); ok && strings.TrimSpace(v) != "" {
		cfg.Storage.Mode = strings.TrimSpace(v)
	}
	if v, ok := d.get("data", "directory"); ok && strings.TrimSpace(v) != "" {
		cfg.Storage.Directory = strings.TrimSpace(v)
	}
	if v, ok := d.get("data", "database_enabled"); ok {
		cfg.Storage.DatabaseEnabled = asBool(v, cfg.Storage.DatabaseEnabled)
	}
	if v, ok := d.get("data", "dsn"); ok {
		cfg.Storage.DSN = strings.TrimSpace(v)
	}

	if v, ok := d.get("mods", "enabled"); ok {
		cfg.Mods.Enabled = asBool(v, cfg.Mods.Enabled)
	}
	if v, ok := d.get("mods", "directory"); ok && strings.TrimSpace(v) != "" {
		cfg.Mods.Directory = strings.TrimSpace(v)
	}
	if v, ok := d.get("mods", "java_home"); ok {
		cfg.Mods.JavaHome = strings.TrimSpace(v)
	}
	if v, ok := d.get("mods", "js_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Mods.JSDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("mods", "go_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Mods.GoDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("mods", "node_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Mods.NodeDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("mods", "expected_client_mods"); ok {
		cfg.Mods.ExpectedClientMods = asCSV(v)
	}

	if v, ok := d.get("persist", "enabled"); ok {
		cfg.Persist.Enabled = asBool(v, cfg.Persist.Enabled)
	}
	if v, ok := d.get("persist", "directory"); ok && strings.TrimSpace(v) != "" {
		cfg.Persist.Directory = strings.TrimSpace(v)
	}
	if v, ok := d.get("persist", "file"); ok && strings.TrimSpace(v) != "" {
		cfg.Persist.File = strings.TrimSpace(v)
	}
	if v, ok := d.get("persist", "interval_sec"); ok {
		cfg.Persist.IntervalSec = asInt(v, cfg.Persist.IntervalSec)
	}
	if v, ok := d.get("persist", "save_msav"); ok {
		cfg.Persist.SaveMSAV = asBool(v, cfg.Persist.SaveMSAV)
	}
	if v, ok := d.get("persist", "msav_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Persist.MSAVDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("persist", "msav_file"); ok {
		cfg.Persist.MSAVFile = strings.TrimSpace(v)
	}

	if v, ok := d.get("script", "file"); ok && strings.TrimSpace(v) != "" {
		cfg.Script.File = strings.TrimSpace(v)
	}
	if v, ok := d.get("script", "daily_gc_time"); ok {
		cfg.Script.DailyGCTime = strings.TrimSpace(v)
	}

	if v, ok := d.get("admin", "ops_file"); ok && strings.TrimSpace(v) != "" {
		cfg.Admin.OpsFile = strings.TrimSpace(v)
	}
	if v, ok := d.get("admin", "player_limit"); ok {
		cfg.Admin.PlayerLimit = asInt(v, cfg.Admin.PlayerLimit)
	}
	if v, ok := d.get("admin", "strict_identity"); ok {
		cfg.Admin.StrictIdentity = asBool(v, cfg.Admin.StrictIdentity)
	}
	if v, ok := d.get("admin", "allow_custom_clients"); ok {
		cfg.Admin.AllowCustomClients = asBool(v, cfg.Admin.AllowCustomClients)
	}
	if v, ok := d.get("admin", "whitelist_enabled"); ok {
		cfg.Admin.WhitelistEnabled = asBool(v, cfg.Admin.WhitelistEnabled)
	}
	if v, ok := d.get("admin", "whitelist_file"); ok && strings.TrimSpace(v) != "" {
		cfg.Admin.WhitelistFile = strings.TrimSpace(v)
	}
	if v, ok := d.get("admin", "banned_names"); ok {
		cfg.Admin.BannedNames = asCSV(v)
	}
	if v, ok := d.get("admin", "banned_subnets"); ok {
		cfg.Admin.BannedSubnets = asCSV(v)
	}
	if v, ok := d.get("admin", "recent_kick_seconds"); ok {
		cfg.Admin.RecentKickSeconds = asInt(v, cfg.Admin.RecentKickSeconds)
	}

	if v, ok := d.get("api", "enabled"); ok {
		cfg.API.Enabled = asBool(v, cfg.API.Enabled)
	}
	if v, ok := d.get("api", "bind"); ok && strings.TrimSpace(v) != "" {
		cfg.API.Bind = strings.TrimSpace(v)
	}
	if v, ok := d.get("api", "key"); ok {
		cfg.API.Key = strings.TrimSpace(v)
	}
	if v, ok := d.get("api", "keys"); ok {
		cfg.API.Keys = asCSV(v)
	}
	if v, ok := d.get("api", "config_file"); ok && strings.TrimSpace(v) != "" {
		cfg.API.ConfigFile = strings.TrimSpace(v)
	}

	if v, ok := d.get("paths", "assets_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Runtime.AssetsDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("paths", "worlds_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Runtime.WorldsDir = strings.TrimSpace(v)
	}
	if v, ok := d.get("paths", "logs_dir"); ok && strings.TrimSpace(v) != "" {
		cfg.Runtime.LogsDir = strings.TrimSpace(v)
	}
}

func makeINI(cfg Config) iniData {
	d := newIniData()

	d.set("config", "reload_interval_sec", strconv.Itoa(cfg.Control.ReloadIntervalSec))
	d.set("config", "reload_log_enabled", boolToIni(cfg.Control.ReloadLogEnabled))
	d.set("config", "translated_conn_log_enabled", boolToIni(cfg.Control.TranslatedConnLogEnabled))
	d.set("config", "public_conn_uuid_enabled", boolToIni(cfg.Control.PublicConnUUIDEnabled))
	d.set("config", "public_conn_uuid_file", cfg.Control.PublicConnUUIDFile)
	d.set("config", "conn_uuid_auto_create", boolToIni(cfg.Control.ConnUUIDAutoCreateEnabled))
	d.set("config", "player_identity_auto_create", boolToIni(cfg.Control.PlayerIdentityAutoCreateEnabled))
	d.set("config", "api_file", cfg.API.ConfigFile)
	d.set("authority_sync", "strategy", string(cfg.Sync.Strategy))
	d.set("development", "packet_events_enabled", boolToIni(cfg.Development.PacketEventsEnabled))
	d.set("development", "packet_recv_events_enabled", boolToIni(cfg.Development.PacketRecvEventsEnabled))
	d.set("development", "packet_send_events_enabled", boolToIni(cfg.Development.PacketSendEventsEnabled))
	d.set("development", "terminal_player_logs_enabled", boolToIni(cfg.Development.TerminalPlayerLogsEnabled))
	d.set("development", "terminal_player_uuid_enabled", boolToIni(cfg.Development.TerminalPlayerUUIDEnabled))
	d.set("development", "respawn_core_logs_enabled", boolToIni(cfg.Development.RespawnCoreLogsEnabled))
	d.set("development", "respawn_unit_logs_enabled", boolToIni(cfg.Development.RespawnUnitLogsEnabled))
	d.set("development", "respawn_packet_logs_enabled", boolToIni(cfg.Development.RespawnPacketLogsEnabled))
	d.set("development", "build_snapshot_logs_enabled", boolToIni(cfg.Development.BuildSnapshotLogsEnabled))
	d.set("development", "build_place_logs_enabled", boolToIni(cfg.Development.BuildPlaceLogsEnabled))
	d.set("development", "build_finish_logs_enabled", boolToIni(cfg.Development.BuildFinishLogsEnabled))
	d.set("development", "build_break_start_logs_enabled", boolToIni(cfg.Development.BuildBreakStartLogsEnabled))
	d.set("development", "build_break_done_logs_enabled", boolToIni(cfg.Development.BuildBreakDoneLogsEnabled))
	d.set("tracepoints", "enabled", boolToIni(cfg.Tracepoints.Enabled))
	d.set("tracepoints", "file", cfg.Tracepoints.File)
	d.set("tracepoints", "client_requests_enabled", boolToIni(cfg.Tracepoints.ClientRequestsEnabled))
	d.set("tracepoints", "server_sends_enabled", boolToIni(cfg.Tracepoints.ServerSendsEnabled))
	d.set("tracepoints", "world_runtime_enabled", boolToIni(cfg.Tracepoints.WorldRuntimeEnabled))
	d.set("tracepoints", "state_build_enabled", boolToIni(cfg.Tracepoints.StateBuildEnabled))
	d.set("tracepoints", "world_stream_enabled", boolToIni(cfg.Tracepoints.WorldStreamEnabled))
	d.set("personalization", "startup_report_enabled", boolToIni(cfg.Personalization.StartupReportEnabled))
	d.set("personalization", "map_load_details_enabled", boolToIni(cfg.Personalization.MapLoadDetailsEnabled))
	d.set("personalization", "unit_id_list_enabled", boolToIni(cfg.Personalization.UnitIDListEnabled))
	d.set("personalization", "startup_current_map_line_enabled", boolToIni(cfg.Personalization.StartupCurrentMapLineEnabled))
	d.set("personalization", "console_intro_enabled", boolToIni(cfg.Personalization.ConsoleIntroEnabled))
	d.set("personalization", "console_intro_server_name_enabled", boolToIni(cfg.Personalization.ConsoleIntroServerNameEnabled))
	d.set("personalization", "console_intro_current_map_enabled", boolToIni(cfg.Personalization.ConsoleIntroCurrentMapEnabled))
	d.set("personalization", "console_intro_listen_addr_enabled", boolToIni(cfg.Personalization.ConsoleIntroListenAddrEnabled))
	d.set("personalization", "console_intro_local_ip_enabled", boolToIni(cfg.Personalization.ConsoleIntroLocalIPEnabled))
	d.set("personalization", "console_intro_api_enabled", boolToIni(cfg.Personalization.ConsoleIntroAPIEnabled))
	d.set("personalization", "console_intro_help_hint_enabled", boolToIni(cfg.Personalization.ConsoleIntroHelpHintEnabled))
	d.set("personalization", "startup_help_enabled", boolToIni(cfg.Personalization.StartupHelpEnabled))
	d.set("personalization", "join_leave_chat_enabled", boolToIni(cfg.Personalization.JoinLeaveChatEnabled))
	d.set("personalization", "player_name_color_enabled", boolToIni(cfg.Personalization.PlayerNameColorEnabled))
	d.set("personalization", "player_name_prefix", cfg.Personalization.PlayerNamePrefix)
	d.set("personalization", "player_name_suffix", cfg.Personalization.PlayerNameSuffix)
	d.set("personalization", "player_bind_prefix_enabled", boolToIni(cfg.Personalization.PlayerBindPrefixEnabled))
	d.set("personalization", "player_bound_prefix", cfg.Personalization.PlayerBoundPrefix)
	d.set("personalization", "player_unbound_prefix", cfg.Personalization.PlayerUnboundPrefix)
	d.set("personalization", "player_title_enabled", boolToIni(cfg.Personalization.PlayerTitleEnabled))
	d.set("personalization", "player_identity_file", cfg.Personalization.PlayerIdentityFile)
	d.set("personalization", "player_bind_source", cfg.Personalization.PlayerBindSource)
	d.set("personalization", "player_bind_api_url", cfg.Personalization.PlayerBindAPIURL)
	d.set("personalization", "player_bind_api_timeout_ms", strconv.Itoa(cfg.Personalization.PlayerBindAPITimeoutMs))
	d.set("personalization", "player_bind_api_cache_sec", strconv.Itoa(cfg.Personalization.PlayerBindAPICacheSec))
	d.set("personalization", "player_conn_id_suffix_enabled", boolToIni(cfg.Personalization.PlayerConnIDSuffixEnabled))
	d.set("personalization", "player_conn_id_suffix_format", cfg.Personalization.PlayerConnIDSuffixFormat)
	d.set("personalization", "main_console_title", cfg.Personalization.MainConsoleTitle)
	d.set("personalization", "core2_console_title", cfg.Personalization.Core2ConsoleTitle)
	d.set("personalization", "core3_console_title", cfg.Personalization.Core3ConsoleTitle)
	d.set("personalization", "core4_console_title", cfg.Personalization.Core4ConsoleTitle)
	d.set("join_popup", "enabled", boolToIni(cfg.JoinPopup.Enabled))
	d.set("join_popup", "delay_ms", strconv.Itoa(cfg.JoinPopup.DelayMs))
	d.set("join_popup", "title", cfg.JoinPopup.Title)
	d.set("join_popup", "message", cfg.JoinPopup.Message)
	d.set("join_popup", "announcement_text", cfg.JoinPopup.AnnouncementText)
	d.set("join_popup", "link_url", cfg.JoinPopup.LinkURL)
	d.set("join_popup", "help_text", cfg.JoinPopup.HelpText)
	d.set("status_bar", "enabled", boolToIni(cfg.StatusBar.Enabled))
	d.set("status_bar", "refresh_interval_sec", strconv.Itoa(cfg.StatusBar.RefreshIntervalSec))
	d.set("status_bar", "popup_duration_ms", strconv.Itoa(cfg.StatusBar.PopupDurationMs))
	d.set("status_bar", "align", cfg.StatusBar.Align)
	d.set("status_bar", "top", strconv.Itoa(cfg.StatusBar.Top))
	d.set("status_bar", "left", strconv.Itoa(cfg.StatusBar.Left))
	d.set("status_bar", "bottom", strconv.Itoa(cfg.StatusBar.Bottom))
	d.set("status_bar", "right", strconv.Itoa(cfg.StatusBar.Right))
	d.set("status_bar", "popup_id", cfg.StatusBar.PopupID)
	d.set("status_bar", "header_enabled", boolToIni(cfg.StatusBar.HeaderEnabled))
	d.set("status_bar", "header_text", cfg.StatusBar.HeaderText)
	d.set("status_bar", "server_name_enabled", boolToIni(cfg.StatusBar.ServerNameEnabled))
	d.set("status_bar", "server_name_format", cfg.StatusBar.ServerNameFormat)
	d.set("status_bar", "performance_enabled", boolToIni(cfg.StatusBar.PerformanceEnabled))
	d.set("status_bar", "performance_format", cfg.StatusBar.PerformanceFormat)
	d.set("status_bar", "current_map_enabled", boolToIni(cfg.StatusBar.CurrentMapEnabled))
	d.set("status_bar", "current_map_format", cfg.StatusBar.CurrentMapFormat)
	d.set("status_bar", "game_time_enabled", boolToIni(cfg.StatusBar.GameTimeEnabled))
	d.set("status_bar", "game_time_format", cfg.StatusBar.GameTimeFormat)
	d.set("status_bar", "player_count_enabled", boolToIni(cfg.StatusBar.PlayerCountEnabled))
	d.set("status_bar", "player_count_format", cfg.StatusBar.PlayerCountFormat)
	d.set("status_bar", "welcome_enabled", boolToIni(cfg.StatusBar.WelcomeEnabled))
	d.set("status_bar", "welcome_format", cfg.StatusBar.WelcomeFormat)
	d.set("status_bar", "qq_group_enabled", boolToIni(cfg.StatusBar.QQGroupEnabled))
	d.set("status_bar", "qq_group_text", cfg.StatusBar.QQGroupText)
	d.set("status_bar", "qq_group_format", cfg.StatusBar.QQGroupFormat)
	d.set("status_bar", "custom_message_enabled", boolToIni(cfg.StatusBar.CustomMessageEnabled))
	d.set("status_bar", "custom_message_text", cfg.StatusBar.CustomMessageText)
	d.set("status_bar", "custom_message_format", cfg.StatusBar.CustomMessageFormat)
	d.set("map_vote", "duration_sec", strconv.Itoa(cfg.MapVote.DurationSec))
	d.set("map_vote", "status_refresh_ms", strconv.Itoa(cfg.MapVote.StatusRefreshMs))
	d.set("map_vote", "popup_duration_ms", strconv.Itoa(cfg.MapVote.PopupDurationMs))
	d.set("map_vote", "home_link_url", cfg.MapVote.HomeLinkURL)
	d.set("map_vote", "align", cfg.MapVote.Align)
	d.set("map_vote", "top", strconv.Itoa(cfg.MapVote.Top))
	d.set("map_vote", "left", strconv.Itoa(cfg.MapVote.Left))
	d.set("map_vote", "bottom", strconv.Itoa(cfg.MapVote.Bottom))
	d.set("map_vote", "right", strconv.Itoa(cfg.MapVote.Right))
	d.set("building", "log_enabled", boolToIni(cfg.Building.Enabled))
	d.set("building", "translated_enabled", boolToIni(cfg.Building.Translated))
	d.set("sundries", "detailed_log_max_mb", strconv.Itoa(cfg.Sundries.DetailedLogMaxMB))
	d.set("sundries", "detailed_log_max_files", strconv.Itoa(cfg.Sundries.DetailedLogMaxFiles))
	d.set("sundries", "net_event_logs_enabled", boolToIni(cfg.Sundries.NetEventLogsEnabled))
	d.set("sundries", "chat_logs_enabled", boolToIni(cfg.Sundries.ChatLogsEnabled))
	d.set("sundries", "respawn_core_logs_enabled", boolToIni(cfg.Sundries.RespawnCoreLogsEnabled))
	d.set("sundries", "respawn_unit_logs_enabled", boolToIni(cfg.Sundries.RespawnUnitLogsEnabled))
	d.set("sundries", "build_place_logs_enabled", boolToIni(cfg.Sundries.BuildPlaceLogsEnabled))
	d.set("sundries", "build_finish_logs_enabled", boolToIni(cfg.Sundries.BuildFinishLogsEnabled))
	d.set("sundries", "build_break_start_logs_enabled", boolToIni(cfg.Sundries.BuildBreakStartLogsEnabled))
	d.set("sundries", "build_break_done_logs_enabled", boolToIni(cfg.Sundries.BuildBreakDoneLogsEnabled))

	d.set("runtime", "cores", strconv.Itoa(cfg.Runtime.Cores))
	d.set("runtime", "scheduler_enabled", boolToIni(cfg.Runtime.SchedulerEnabled))
	d.set("runtime", "devlog_enabled", boolToIni(cfg.Runtime.DevLogEnabled))
	d.set("runtime", "vanilla_profiles", cfg.Runtime.VanillaProfiles)

	d.set("core", "dual_core_enabled", boolToIni(cfg.Core.DualCoreEnabled))
	d.set("core", "tps", strconv.Itoa(cfg.Core.TPS))

	d.set("memory", "limit_mb", strconv.Itoa(cfg.Core.MemoryLimitMB))
	d.set("memory", "startup_max_mb", strconv.Itoa(cfg.Core.MemoryStartupMaxMB))
	d.set("memory", "gc_trigger_mb", strconv.Itoa(cfg.Core.MemoryGCTriggerMB))
	d.set("memory", "check_interval_sec", strconv.Itoa(cfg.Core.MemoryCheckIntervalSec))
	d.set("memory", "free_os_memory", boolToIni(cfg.Core.MemoryFreeOSMemory))

	d.set("server", "name", cfg.Runtime.ServerName)
	d.set("server", "desc", cfg.Runtime.ServerDesc)
	d.set("server", "virtual_players", strconv.Itoa(cfg.Runtime.VirtualPlayers))

	d.set("sync", "entity_ms", strconv.Itoa(cfg.Net.SyncEntityMs))
	d.set("sync", "state_ms", strconv.Itoa(cfg.Net.SyncStateMs))
	d.set("sync", "udp_retry_count", strconv.Itoa(cfg.Net.UdpRetryCount))
	d.set("sync", "udp_retry_delay_ms", strconv.Itoa(cfg.Net.UdpRetryDelayMs))
	d.set("sync", "udp_fallback_tcp", boolToIni(cfg.Net.UdpFallbackTCP))
	d.set("sync", "use_map_sync_data_fallback", boolToIni(cfg.Sync.UseMapSyncDataFallback))
	d.set("sync", "block_sync_logs_enabled", boolToIni(cfg.Sync.BlockSyncLogsEnabled))

	d.set("data", "mode", cfg.Storage.Mode)
	d.set("data", "directory", cfg.Storage.Directory)
	d.set("data", "database_enabled", boolToIni(cfg.Storage.DatabaseEnabled))
	d.set("data", "dsn", cfg.Storage.DSN)

	d.set("mods", "enabled", boolToIni(cfg.Mods.Enabled))
	d.set("mods", "directory", cfg.Mods.Directory)
	d.set("mods", "java_home", cfg.Mods.JavaHome)
	d.set("mods", "js_dir", cfg.Mods.JSDir)
	d.set("mods", "go_dir", cfg.Mods.GoDir)
	d.set("mods", "node_dir", cfg.Mods.NodeDir)
	d.set("mods", "expected_client_mods", strings.Join(cfg.Mods.ExpectedClientMods, ","))

	d.set("persist", "enabled", boolToIni(cfg.Persist.Enabled))
	d.set("persist", "directory", cfg.Persist.Directory)
	d.set("persist", "file", cfg.Persist.File)
	d.set("persist", "interval_sec", strconv.Itoa(cfg.Persist.IntervalSec))
	d.set("persist", "save_msav", boolToIni(cfg.Persist.SaveMSAV))
	d.set("persist", "msav_dir", cfg.Persist.MSAVDir)
	d.set("persist", "msav_file", cfg.Persist.MSAVFile)

	d.set("script", "file", cfg.Script.File)
	d.set("script", "daily_gc_time", cfg.Script.DailyGCTime)

	d.set("admin", "ops_file", cfg.Admin.OpsFile)
	d.set("admin", "player_limit", strconv.Itoa(cfg.Admin.PlayerLimit))
	d.set("admin", "strict_identity", boolToIni(cfg.Admin.StrictIdentity))
	d.set("admin", "allow_custom_clients", boolToIni(cfg.Admin.AllowCustomClients))
	d.set("admin", "whitelist_enabled", boolToIni(cfg.Admin.WhitelistEnabled))
	d.set("admin", "whitelist_file", cfg.Admin.WhitelistFile)
	d.set("admin", "banned_names", strings.Join(cfg.Admin.BannedNames, ","))
	d.set("admin", "banned_subnets", strings.Join(cfg.Admin.BannedSubnets, ","))
	d.set("admin", "recent_kick_seconds", strconv.Itoa(cfg.Admin.RecentKickSeconds))

	d.set("api", "enabled", boolToIni(cfg.API.Enabled))
	d.set("api", "bind", cfg.API.Bind)
	d.set("api", "key", cfg.API.Key)
	d.set("api", "keys", strings.Join(cfg.API.Keys, ","))
	d.set("api", "config_file", cfg.API.ConfigFile)

	d.set("paths", "assets_dir", cfg.Runtime.AssetsDir)
	d.set("paths", "worlds_dir", cfg.Runtime.WorldsDir)
	d.set("paths", "logs_dir", cfg.Runtime.LogsDir)

	return d
}

func writeINI(path string, sections []string, d iniData, header string) error {
	var buf bytes.Buffer
	if strings.TrimSpace(header) != "" {
		for _, ln := range strings.Split(header, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			buf.WriteString("; ")
			buf.WriteString(ln)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
	for i, sec := range sections {
		sec = strings.ToLower(strings.TrimSpace(sec))
		if sec == "" {
			continue
		}
		buf.WriteString("[")
		buf.WriteString(sec)
		buf.WriteString("]\n")
		m := d[sec]
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		for x := 0; x < len(keys); x++ {
			for y := x + 1; y < len(keys); y++ {
				if keys[y] < keys[x] {
					keys[x], keys[y] = keys[y], keys[x]
				}
			}
		}
		for _, k := range keys {
			buf.WriteString(k)
			buf.WriteString(" = ")
			buf.WriteString(m[k])
			buf.WriteString("\n")
		}
		if i < len(sections)-1 {
			buf.WriteString("\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

var tomlValueKinds = makeTOMLValueKinds()

func makeTOMLValueKinds() map[string]string {
	out := map[string]string{}
	add := func(kind, section string, keys ...string) {
		for _, key := range keys {
			out[section+"."+key] = kind
		}
	}

	add("int", "config", "reload_interval_sec")
	add("bool", "config",
		"reload_log_enabled",
		"translated_conn_log_enabled",
		"public_conn_uuid_enabled",
		"conn_uuid_auto_create",
		"player_identity_auto_create",
	)

	add("bool", "building", "log_enabled", "translated_enabled")
	add("bool", "authority_sync", "enabled")

	add("bool", "core", "dual_core_enabled")
	add("int", "core", "tps")
	add("int", "memory", "limit_mb", "startup_max_mb", "gc_trigger_mb", "check_interval_sec")
	add("bool", "memory", "free_os_memory")

	add("int", "server", "virtual_players")

	add("int", "sync", "entity_ms", "state_ms", "udp_retry_count", "udp_retry_delay_ms")
	add("bool", "sync", "udp_fallback_tcp", "use_map_sync_data_fallback", "block_sync_logs_enabled")

	add("bool", "data", "database_enabled")
	add("bool", "mods", "enabled")
	add("array", "mods", "expected_client_mods")
	add("bool", "persist", "enabled", "save_msav")
	add("int", "persist", "interval_sec")
	add("int", "admin", "player_limit", "recent_kick_seconds")
	add("bool", "admin", "strict_identity", "allow_custom_clients", "whitelist_enabled")
	add("array", "admin", "banned_names", "banned_subnets")

	add("bool", "api", "enabled")
	add("array", "api", "keys")

	add("int", "runtime", "cores")
	add("bool", "runtime", "scheduler_enabled", "devlog_enabled")

	add("bool", "development",
		"packet_events_enabled",
		"packet_recv_events_enabled",
		"packet_send_events_enabled",
		"terminal_player_logs_enabled",
		"terminal_player_uuid_enabled",
		"respawn_core_logs_enabled",
		"respawn_unit_logs_enabled",
		"respawn_packet_logs_enabled",
		"build_snapshot_logs_enabled",
		"build_place_logs_enabled",
		"build_finish_logs_enabled",
		"build_break_start_logs_enabled",
		"build_break_done_logs_enabled",
	)

	add("int", "sundries", "detailed_log_max_mb", "detailed_log_max_files")
	add("bool", "sundries",
		"net_event_logs_enabled",
		"chat_logs_enabled",
		"respawn_core_logs_enabled",
		"respawn_unit_logs_enabled",
		"build_place_logs_enabled",
		"build_finish_logs_enabled",
		"build_break_start_logs_enabled",
		"build_break_done_logs_enabled",
	)
	add("bool", "tracepoints",
		"enabled",
		"client_requests_enabled",
		"server_sends_enabled",
		"world_runtime_enabled",
		"state_build_enabled",
		"world_stream_enabled",
	)

	add("bool", "personalization",
		"startup_report_enabled",
		"map_load_details_enabled",
		"unit_id_list_enabled",
		"startup_current_map_line_enabled",
		"console_intro_enabled",
		"console_intro_server_name_enabled",
		"console_intro_current_map_enabled",
		"console_intro_listen_addr_enabled",
		"console_intro_local_ip_enabled",
		"console_intro_api_enabled",
		"console_intro_help_hint_enabled",
		"startup_help_enabled",
		"join_leave_chat_enabled",
		"player_name_color_enabled",
		"player_bind_prefix_enabled",
		"player_title_enabled",
		"player_conn_id_suffix_enabled",
	)
	add("int", "personalization", "player_bind_api_timeout_ms", "player_bind_api_cache_sec")

	add("bool", "join_popup", "enabled")
	add("int", "join_popup", "delay_ms")

	add("bool", "status_bar",
		"enabled",
		"header_enabled",
		"server_name_enabled",
		"performance_enabled",
		"current_map_enabled",
		"game_time_enabled",
		"player_count_enabled",
		"welcome_enabled",
		"qq_group_enabled",
		"custom_message_enabled",
	)
	add("int", "status_bar", "refresh_interval_sec", "popup_duration_ms", "top", "left", "bottom", "right")

	add("int", "map_vote", "duration_sec", "status_refresh_ms", "popup_duration_ms", "top", "left", "bottom", "right")

	return out
}

func writeTOML(path string, sections []string, d iniData, header string) error {
	var buf bytes.Buffer
	if strings.TrimSpace(header) != "" {
		for _, ln := range strings.Split(header, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			buf.WriteString("# ")
			buf.WriteString(ln)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
	for i, sec := range sections {
		sec = strings.ToLower(strings.TrimSpace(sec))
		if sec == "" {
			continue
		}
		buf.WriteString("[")
		buf.WriteString(sec)
		buf.WriteString("]\n")
		m := d[sec]
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		for x := 0; x < len(keys); x++ {
			for y := x + 1; y < len(keys); y++ {
				if keys[y] < keys[x] {
					keys[x], keys[y] = keys[y], keys[x]
				}
			}
		}
		for _, k := range keys {
			buf.WriteString(k)
			buf.WriteString(" = ")
			buf.WriteString(encodeTOMLValue(sec, k, m[k]))
			buf.WriteString("\n")
		}
		if i < len(sections)-1 {
			buf.WriteString("\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func encodeTOMLValue(section, key, value string) string {
	switch tomlValueKinds[section+"."+key] {
	case "bool":
		return strconv.FormatBool(asBool(value, false))
	case "int":
		return strconv.Itoa(asInt(value, 0))
	case "array":
		return encodeTOMLStringArray(asCSV(value))
	default:
		return encodeTOMLString(value)
	}
}

func encodeTOMLStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, encodeTOMLString(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func encodeTOMLString(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if strings.Contains(value, "\n") && !strings.Contains(value, `"""`) {
		return "\"\"\"\n" + value + "\n\"\"\""
	}
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(value) + `"`
}

func normalize(cfg *Config) {
	if cfg.Core.TPS <= 0 {
		cfg.Core.TPS = 60
	}
	if cfg.Core.TPS > 120 {
		cfg.Core.TPS = 120
	}
	cfg.Development.PacketEventsEnabled = cfg.Development.PacketRecvEventsEnabled || cfg.Development.PacketSendEventsEnabled
	cfg.Sync.Strategy = normalizeAuthoritySyncStrategy(string(cfg.Sync.Strategy))
	if strings.TrimSpace(cfg.Runtime.ServerName) == "" {
		cfg.Runtime.ServerName = "mdt-server"
	}
	if cfg.Runtime.VirtualPlayers < 0 {
		cfg.Runtime.VirtualPlayers = 0
	}
	if cfg.Net.SyncEntityMs <= 0 {
		cfg.Net.SyncEntityMs = 200
	}
	if cfg.Net.SyncStateMs <= 0 {
		cfg.Net.SyncStateMs = 200
	}
	if strings.TrimSpace(cfg.Runtime.AssetsDir) == "" {
		cfg.Runtime.AssetsDir = "assets"
	}
	if strings.TrimSpace(cfg.Runtime.WorldsDir) == "" {
		cfg.Runtime.WorldsDir = filepath.Join(cfg.Runtime.AssetsDir, "worlds")
	}
	if strings.TrimSpace(cfg.Runtime.LogsDir) == "" {
		cfg.Runtime.LogsDir = "logs"
	}
	if strings.TrimSpace(cfg.API.ConfigFile) == "" {
		cfg.API.ConfigFile = "api.toml"
	}
	if cfg.Core.MemoryCheckIntervalSec <= 0 {
		cfg.Core.MemoryCheckIntervalSec = 5
	}
	if cfg.Control.ReloadIntervalSec <= 0 {
		cfg.Control.ReloadIntervalSec = 5
	}
	if strings.TrimSpace(cfg.Tracepoints.File) == "" {
		cfg.Tracepoints.File = filepath.Join("logs", "tracepoints.jsonl")
	}
	if cfg.Admin.PlayerLimit < 0 {
		cfg.Admin.PlayerLimit = 0
	}
	if cfg.Admin.RecentKickSeconds < 0 {
		cfg.Admin.RecentKickSeconds = 0
	}
	if strings.TrimSpace(cfg.Admin.WhitelistFile) == "" {
		cfg.Admin.WhitelistFile = filepath.Join("data", "state", "whitelist.json")
	}
	if strings.TrimSpace(cfg.Control.PublicConnUUIDFile) == "" {
		cfg.Control.PublicConnUUIDFile = filepath.Join("json", "conn_uuid.json")
	}
	if strings.TrimSpace(cfg.Personalization.PlayerIdentityFile) == "" {
		cfg.Personalization.PlayerIdentityFile = filepath.Join("json", "player_identity.json")
	}
	if strings.TrimSpace(cfg.Personalization.PlayerBindSource) == "" {
		cfg.Personalization.PlayerBindSource = "internal"
	}
	cfg.Personalization.PlayerBindSource = strings.ToLower(strings.TrimSpace(cfg.Personalization.PlayerBindSource))
	if cfg.Personalization.PlayerBindSource != "api" {
		cfg.Personalization.PlayerBindSource = "internal"
	}
	if cfg.Personalization.PlayerBindAPITimeoutMs <= 0 {
		cfg.Personalization.PlayerBindAPITimeoutMs = 1500
	}
	if cfg.Personalization.PlayerBindAPICacheSec <= 0 {
		cfg.Personalization.PlayerBindAPICacheSec = 30
	}
	if strings.TrimSpace(cfg.Personalization.PlayerConnIDSuffixFormat) == "" {
		cfg.Personalization.PlayerConnIDSuffixFormat = " [gray]{id}[]"
	}
	cfg.Mods.ExpectedClientMods = asCSV(strings.Join(cfg.Mods.ExpectedClientMods, ","))
	cfg.Personalization.PlayerBoundPrefix = normalizeWrappedMindustryLiteral(cfg.Personalization.PlayerBoundPrefix)
	cfg.Personalization.PlayerUnboundPrefix = normalizeWrappedMindustryLiteral(cfg.Personalization.PlayerUnboundPrefix)
	if cfg.JoinPopup.DelayMs < 0 {
		cfg.JoinPopup.DelayMs = 0
	}
	if cfg.StatusBar.RefreshIntervalSec <= 0 {
		cfg.StatusBar.RefreshIntervalSec = 2
	}
	if cfg.StatusBar.PopupDurationMs <= 0 {
		cfg.StatusBar.PopupDurationMs = 2200
	}
	if strings.TrimSpace(cfg.StatusBar.Align) == "" {
		cfg.StatusBar.Align = "top_left"
	}
	if strings.TrimSpace(cfg.StatusBar.PopupID) == "" {
		cfg.StatusBar.PopupID = "server-status-bar"
	}
	if strings.TrimSpace(cfg.StatusBar.HeaderText) == "" {
		cfg.StatusBar.HeaderText = "[accent]服务器状态[]"
	}
	if strings.TrimSpace(cfg.StatusBar.ServerNameFormat) == "" {
		cfg.StatusBar.ServerNameFormat = "[green]服务器: [white]{server_name}[]"
	}
	if strings.TrimSpace(cfg.StatusBar.PerformanceFormat) == "" {
		cfg.StatusBar.PerformanceFormat = "[green]性能: [white]CPU {cpu_percent}%[] [white]进程内存 {memory_mb} MB[]"
	}
	if strings.TrimSpace(cfg.StatusBar.CurrentMapFormat) == "" {
		cfg.StatusBar.CurrentMapFormat = "[green]当前地图: [white]{current_map}[]"
	}
	if strings.TrimSpace(cfg.StatusBar.GameTimeFormat) == "" {
		cfg.StatusBar.GameTimeFormat = "[green]本局时间: [white]{game_time}[]"
	}
	if strings.TrimSpace(cfg.StatusBar.PlayerCountFormat) == "" {
		cfg.StatusBar.PlayerCountFormat = "[green]在线人数: [white]{players}[]"
	}
	if strings.TrimSpace(cfg.StatusBar.WelcomeFormat) == "" {
		cfg.StatusBar.WelcomeFormat = "[gold]欢迎玩家 {player_name} 来到镜像物语[]"
	}
	if strings.TrimSpace(cfg.StatusBar.QQGroupFormat) == "" {
		cfg.StatusBar.QQGroupFormat = "[green]QQ群: [white]{qq_group}[]"
	}
	if strings.TrimSpace(cfg.StatusBar.CustomMessageFormat) == "" {
		cfg.StatusBar.CustomMessageFormat = "[gold]{message}[]"
	}
	if cfg.MapVote.DurationSec <= 0 {
		cfg.MapVote.DurationSec = 15
	}
	if cfg.MapVote.StatusRefreshMs <= 0 {
		cfg.MapVote.StatusRefreshMs = 1500
	}
	if cfg.MapVote.PopupDurationMs <= 0 {
		cfg.MapVote.PopupDurationMs = cfg.MapVote.StatusRefreshMs + 300
	}
	if cfg.MapVote.PopupDurationMs < cfg.MapVote.StatusRefreshMs {
		cfg.MapVote.PopupDurationMs = cfg.MapVote.StatusRefreshMs + 300
	}
	cfg.MapVote.HomeLinkURL = strings.TrimSpace(cfg.MapVote.HomeLinkURL)
	if strings.TrimSpace(cfg.MapVote.Align) == "" {
		cfg.MapVote.Align = "top_left"
	}
	if cfg.Sundries.DetailedLogMaxMB <= 0 {
		cfg.Sundries.DetailedLogMaxMB = 2
	}
	if cfg.Sundries.DetailedLogMaxFiles <= 0 {
		cfg.Sundries.DetailedLogMaxFiles = 100
	}
}

func normalizeWrappedMindustryLiteral(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.Contains(v, "][") || !strings.HasSuffix(v, "[]") {
		return v
	}
	firstEnd := strings.Index(v, "]")
	if firstEnd <= 0 || firstEnd >= len(v)-3 {
		return v
	}
	colorTag := v[:firstEnd+1]
	body := strings.TrimSpace(v[firstEnd+1:])
	if strings.HasPrefix(body, "[") && strings.HasSuffix(body, "[]") {
		secondEnd := strings.Index(body, "]")
		if secondEnd > 0 && secondEnd < len(body)-2 {
			text := body[1:secondEnd]
			rest := body[secondEnd+1:]
			if rest == "[]" {
				return colorTag + text + "[]"
			}
		}
	}
	return v
}

func sidecarPaths(cfgPath string, cfg Config) map[string]string {
	dir := filepath.Dir(cfgPath)
	apiPath := cfg.API.ConfigFile
	if strings.TrimSpace(apiPath) == "" {
		apiPath = "api.toml"
	}
	if !filepath.IsAbs(apiPath) {
		apiPath = filepath.Join(dir, apiPath)
	}
	return map[string]string{
		"core":            filepath.Join(dir, "core.toml"),
		"server":          filepath.Join(dir, "server.toml"),
		"sync":            filepath.Join(dir, "sync.toml"),
		"misc":            filepath.Join(dir, "misc.toml"),
		"sundries":        filepath.Join(dir, "sundries.toml"),
		"development":     filepath.Join(dir, "development.toml"),
		"tracepoints":     filepath.Join(dir, "tracepoints.toml"),
		"personalization": filepath.Join(dir, "personalization.toml"),
		"join_popup":      filepath.Join(dir, "join_popup.toml"),
		"status_bar":      filepath.Join(dir, "status_bar.toml"),
		"map_vote":        filepath.Join(dir, "map_vote.toml"),
		"data":            filepath.Join(dir, "data.toml"),
		"paths":           filepath.Join(dir, "paths.toml"),
		"api":             apiPath,
	}
}

func loadSidecars(cfgPath string, cfg *Config) error {
	paths := sidecarPaths(cfgPath, *cfg)
	loadOne := func(path string) error {
		if strings.TrimSpace(path) == "" {
			return nil
		}
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if st.IsDir() {
			return nil
		}
		d, err := parseConfigData(path)
		if err != nil {
			return fmt.Errorf("parse config %s: %w", path, err)
		}
		applyINI(cfg, d)
		return nil
	}
	for _, key := range []string{"core", "server", "sync", "misc", "sundries", "development", "tracepoints", "personalization", "join_popup", "status_bar", "map_vote", "data", "paths", "api"} {
		if err := loadOne(paths[key]); err != nil {
			return err
		}
	}
	return nil
}

func saveSidecars(cfgPath string, cfg Config, d iniData) error {
	paths := sidecarPaths(cfgPath, cfg)
	if err := writeTOML(paths["core"], []string{"core", "memory"}, d, "核心配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["server"], []string{"server"}, d, "服务器基础配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["sync"], []string{"sync"}, d, "同步配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["misc"], []string{"data", "paths", "mods", "persist", "script", "admin"}, d, "杂项配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["sundries"], []string{"sundries"}, d, "附加日志配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["development"], []string{"development"}, d, "开发调试配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["tracepoints"], []string{"tracepoints"}, d, "断点追踪配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["personalization"], []string{"personalization"}, d, "个性化显示配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["join_popup"], []string{"join_popup"}, d, "入服公告配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["status_bar"], []string{"status_bar"}, d, "状态栏配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["map_vote"], []string{"map_vote"}, d, "地图投票配置"); err != nil {
		return err
	}
	if err := writeTOML(paths["api"], []string{"api"}, d, "API 配置"); err != nil {
		return err
	}
	return nil
}

func Default() Config {
	return Config{
		Control: ControlConfig{
			ReloadIntervalSec:               5,
			ReloadLogEnabled:                false,
			TranslatedConnLogEnabled:        true,
			PublicConnUUIDEnabled:           true,
			PublicConnUUIDFile:              filepath.Join("json", "conn_uuid.json"),
			ConnUUIDAutoCreateEnabled:       true,
			PlayerIdentityAutoCreateEnabled: true,
		},
		Development: DevelopmentConfig{
			PacketEventsEnabled:        false,
			PacketRecvEventsEnabled:    false,
			PacketSendEventsEnabled:    false,
			TerminalPlayerLogsEnabled:  true,
			TerminalPlayerUUIDEnabled:  false,
			RespawnCoreLogsEnabled:     false,
			RespawnUnitLogsEnabled:     false,
			RespawnPacketLogsEnabled:   false,
			BuildSnapshotLogsEnabled:   false,
			BuildPlaceLogsEnabled:      false,
			BuildFinishLogsEnabled:     false,
			BuildBreakStartLogsEnabled: false,
			BuildBreakDoneLogsEnabled:  false,
		},
		Personalization: PersonalizationConfig{
			StartupReportEnabled:          true,
			MapLoadDetailsEnabled:         true,
			UnitIDListEnabled:             true,
			StartupCurrentMapLineEnabled:  true,
			ConsoleIntroEnabled:           true,
			ConsoleIntroServerNameEnabled: true,
			ConsoleIntroCurrentMapEnabled: true,
			ConsoleIntroListenAddrEnabled: true,
			ConsoleIntroLocalIPEnabled:    true,
			ConsoleIntroAPIEnabled:        true,
			ConsoleIntroHelpHintEnabled:   true,
			StartupHelpEnabled:            true,
			JoinLeaveChatEnabled:          true,
			PlayerNameColorEnabled:        true,
			PlayerNamePrefix:              "",
			PlayerNameSuffix:              "",
			PlayerBindPrefixEnabled:       true,
			PlayerBoundPrefix:             "[green]（已绑定）[]",
			PlayerUnboundPrefix:           "[scarlet]（未绑定）[]",
			PlayerTitleEnabled:            true,
			PlayerIdentityFile:            filepath.Join("json", "player_identity.json"),
			PlayerBindSource:              "internal",
			PlayerBindAPIURL:              "",
			PlayerBindAPITimeoutMs:        1500,
			PlayerBindAPICacheSec:         30,
			PlayerConnIDSuffixEnabled:     true,
			PlayerConnIDSuffixFormat:      " [gray]{id}[]",
			MainConsoleTitle:              "mdt-server | 主进程 | {server_name}",
			Core2ConsoleTitle:             "mdt-server | Core2 | IO",
			Core3ConsoleTitle:             "mdt-server | Core3 | Snapshot",
			Core4ConsoleTitle:             "mdt-server | Core4 | Policy",
		},
		JoinPopup: JoinPopupConfig{
			Enabled:          true,
			DelayMs:          300,
			Title:            "[accent]服务器公告[]",
			Message:          "欢迎 [green]{player_name}[] 来到 [white]{server_name}[]\n当前地图: [white]{current_map}[]\n请选择下方按钮。",
			AnnouncementText: "[accent]服务器公告[]\n\n1. 请遵守服务器规则。\n2. 如有问题可先查看快速帮助。\n3. 需要联系管理时请使用群或外部链接。",
			LinkURL:          "https://example.com",
			HelpText:         "[accent]帮助为分页弹窗[]\n第 1 页显示基础指令与投票换图；\n第 2 页显示常用与 OP 指令；\n第 3 页显示单位控制。",
		},
		StatusBar: StatusBarConfig{
			Enabled:              true,
			RefreshIntervalSec:   2,
			PopupDurationMs:      2200,
			Align:                "top_left",
			Top:                  155,
			Left:                 0,
			Bottom:               0,
			Right:                0,
			PopupID:              "server-status-bar",
			HeaderEnabled:        true,
			HeaderText:           "[accent]服务器状态[]",
			ServerNameEnabled:    true,
			ServerNameFormat:     "[green]服务器: [white]{server_name}[]",
			PerformanceEnabled:   true,
			PerformanceFormat:    "[green]性能: [white]CPU {cpu_percent}%[] [white]进程内存 {memory_mb} MB[]",
			CurrentMapEnabled:    true,
			CurrentMapFormat:     "[green]当前地图: [white]{current_map}[]",
			GameTimeEnabled:      true,
			GameTimeFormat:       "[green]本局时间: [white]{game_time}[]",
			PlayerCountEnabled:   true,
			PlayerCountFormat:    "[green]在线人数: [white]{players}[]",
			WelcomeEnabled:       true,
			WelcomeFormat:        "[gold]欢迎玩家 {player_name} 来到镜像物语[]",
			QQGroupEnabled:       true,
			QQGroupText:          "请在这里填写QQ群",
			QQGroupFormat:        "[green]QQ群: [white]{qq_group}[]",
			CustomMessageEnabled: true,
			CustomMessageText:    "请在这里填写服务器公告",
			CustomMessageFormat:  "[gold]{message}[]",
		},
		MapVote: MapVoteConfig{
			DurationSec:     15,
			StatusRefreshMs: 1500,
			PopupDurationMs: 1800,
			HomeLinkURL:     "https://example.com/votemap",
			Align:           "top_left",
			Top:             220,
			Left:            0,
			Bottom:          0,
			Right:           0,
		},
		Building: BuildingLogConfig{
			Enabled:    true,
			Translated: true,
		},
		Tracepoints: TracepointsConfig{
			Enabled:               false,
			File:                  filepath.Join("logs", "tracepoints.jsonl"),
			ClientRequestsEnabled: true,
			ServerSendsEnabled:    true,
			WorldRuntimeEnabled:   true,
			StateBuildEnabled:     true,
			WorldStreamEnabled:    true,
		},
		Sundries: SundriesConfig{
			DetailedLogMaxMB:           2,
			DetailedLogMaxFiles:        100,
			NetEventLogsEnabled:        true,
			ChatLogsEnabled:            true,
			RespawnCoreLogsEnabled:     false,
			RespawnUnitLogsEnabled:     false,
			BuildPlaceLogsEnabled:      false,
			BuildFinishLogsEnabled:     false,
			BuildBreakStartLogsEnabled: false,
			BuildBreakDoneLogsEnabled:  false,
		},
		Runtime: RuntimeConfig{
			Cores:            6,
			SchedulerEnabled: false,
			VanillaProfiles:  "data/vanilla/profiles.json",
			AssetsDir:        "assets",
			WorldsDir:        "assets/worlds",
			LogsDir:          "logs",
			ServerName:       "mdt-server",
			ServerDesc:       "",
			VirtualPlayers:   0,
			DevLogEnabled:    true,
		},
		Core: CoreConfig{
			DualCoreEnabled:        true,
			TPS:                    60,
			MemoryLimitMB:          0,
			MemoryStartupMaxMB:     0,
			MemoryGCTriggerMB:      0,
			MemoryCheckIntervalSec: 5,
			MemoryFreeOSMemory:     false,
		},
		API: APIConfig{
			Enabled:    true,
			Key:        "",
			Keys:       nil,
			Bind:       "0.0.0.0:8090",
			ConfigFile: "api.toml",
		},
		Storage: StorageConfig{
			Mode:            "file",
			Directory:       "data/events",
			DatabaseEnabled: false,
			DSN:             "",
		},
		Net: NetConfig{
			UdpRetryCount:   2,
			UdpRetryDelayMs: 5,
			UdpFallbackTCP:  true,
			SyncEntityMs:    200,
			SyncStateMs:     200,
		},
		Sync: SyncConfig{
			Strategy:               AuthoritySyncDynamic,
			UseMapSyncDataFallback: false,
		},
		Persist: PersistConfig{
			Enabled:     true,
			Directory:   "data/state",
			File:        "server-state.json",
			IntervalSec: 30,
			SaveMSAV:    true,
			MSAVDir:     "data/snapshots",
			MSAVFile:    "",
		},
		Mods: ModsConfig{
			Enabled:            false,
			Directory:          "mods",
			JavaHome:           "",
			JSDir:              "mods/js",
			GoDir:              "mods/go",
			NodeDir:            "mods/node",
			ExpectedClientMods: nil,
		},
		Script: ScriptConfig{
			File:         "data/state/scripts.json",
			StartupTasks: nil,
			DailyGCTime:  "",
		},
		Admin: AdminConfig{
			OpsFile:            filepath.Join("configs", "json", "ops.json"),
			PlayerLimit:        0,
			StrictIdentity:     true,
			AllowCustomClients: false,
			WhitelistEnabled:   false,
			WhitelistFile:      "data/state/whitelist.json",
			BannedNames:        nil,
			BannedSubnets:      nil,
			RecentKickSeconds:  30,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 主配置不存在时，仍然允许同目录下的侧配置文件独立生效。
			if err := loadSidecars(path, &cfg); err != nil {
				return cfg, err
			}
			normalize(&cfg)
			keys := append([]string{}, cfg.API.Keys...)
			if strings.TrimSpace(cfg.API.Key) != "" {
				keys = append(keys, cfg.API.Key)
			}
			if err := ValidateAPIKeys(keys); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		return cfg, err
	}
	if st.IsDir() {
		return cfg, os.ErrInvalid
	}

	d, err := parseConfigData(path)
	if err != nil {
		return cfg, err
	}
	applyINI(&cfg, d)
	normalize(&cfg)
	if err := loadSidecars(path, &cfg); err != nil {
		return cfg, err
	}
	normalize(&cfg)
	keys := append([]string{}, cfg.API.Keys...)
	if strings.TrimSpace(cfg.API.Key) != "" {
		keys = append(keys, cfg.API.Key)
	}
	if err := ValidateAPIKeys(keys); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func ApplyBaseDir(cfg *Config, baseDir string) {
	if cfg == nil {
		return
	}
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return
	}
	resolve := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}
	cfg.Runtime.AssetsDir = resolve(cfg.Runtime.AssetsDir)
	cfg.Runtime.WorldsDir = resolve(cfg.Runtime.WorldsDir)
	cfg.Runtime.LogsDir = resolve(cfg.Runtime.LogsDir)
	cfg.Runtime.VanillaProfiles = resolve(cfg.Runtime.VanillaProfiles)
	cfg.Storage.Directory = resolve(cfg.Storage.Directory)
	cfg.Persist.Directory = resolve(cfg.Persist.Directory)
	cfg.Persist.MSAVDir = resolve(cfg.Persist.MSAVDir)
	cfg.Mods.Directory = resolve(cfg.Mods.Directory)
	cfg.Mods.JSDir = resolve(cfg.Mods.JSDir)
	cfg.Mods.GoDir = resolve(cfg.Mods.GoDir)
	cfg.Mods.NodeDir = resolve(cfg.Mods.NodeDir)
	cfg.Script.File = resolve(cfg.Script.File)
	cfg.Admin.OpsFile = resolve(cfg.Admin.OpsFile)
	cfg.Admin.WhitelistFile = resolve(cfg.Admin.WhitelistFile)
	cfg.Tracepoints.File = resolve(cfg.Tracepoints.File)
}

func relativizeConfigPath(baseDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return clean
	}
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return clean
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return clean
	}
	pathAbs, err := filepath.Abs(clean)
	if err != nil {
		return clean
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return clean
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == "" {
		return rel
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return clean
	}
	return rel
}

func configRootDir(cfgPath string) string {
	dir := filepath.Dir(strings.TrimSpace(cfgPath))
	if strings.TrimSpace(dir) == "" || dir == "." {
		return "."
	}
	root := filepath.Dir(dir)
	if strings.TrimSpace(root) == "" || root == dir {
		return "."
	}
	return root
}

func relativizeForSave(cfgPath string, cfg *Config) {
	if cfg == nil {
		return
	}
	configDir := filepath.Dir(strings.TrimSpace(cfgPath))
	if strings.TrimSpace(configDir) == "" {
		configDir = "."
	}
	rootDir := configRootDir(cfgPath)
	rootRel := func(p string) string { return relativizeConfigPath(rootDir, p) }
	configRel := func(p string) string { return relativizeConfigPath(configDir, p) }

	cfg.Runtime.AssetsDir = rootRel(cfg.Runtime.AssetsDir)
	cfg.Runtime.WorldsDir = rootRel(cfg.Runtime.WorldsDir)
	cfg.Runtime.LogsDir = rootRel(cfg.Runtime.LogsDir)
	cfg.Runtime.VanillaProfiles = rootRel(cfg.Runtime.VanillaProfiles)
	cfg.Storage.Directory = rootRel(cfg.Storage.Directory)
	cfg.Persist.Directory = rootRel(cfg.Persist.Directory)
	cfg.Persist.MSAVDir = rootRel(cfg.Persist.MSAVDir)
	cfg.Mods.Directory = rootRel(cfg.Mods.Directory)
	cfg.Mods.JSDir = rootRel(cfg.Mods.JSDir)
	cfg.Mods.GoDir = rootRel(cfg.Mods.GoDir)
	cfg.Mods.NodeDir = rootRel(cfg.Mods.NodeDir)
	cfg.Script.File = rootRel(cfg.Script.File)
	cfg.Admin.OpsFile = rootRel(cfg.Admin.OpsFile)
	cfg.Admin.WhitelistFile = rootRel(cfg.Admin.WhitelistFile)
	cfg.Tracepoints.File = rootRel(cfg.Tracepoints.File)

	cfg.Control.PublicConnUUIDFile = configRel(cfg.Control.PublicConnUUIDFile)
	cfg.Personalization.PlayerIdentityFile = configRel(cfg.Personalization.PlayerIdentityFile)
	cfg.API.ConfigFile = configRel(cfg.API.ConfigFile)
}

func Save(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return os.ErrInvalid
	}
	normalize(&cfg)
	relativizeForSave(path, &cfg)
	d := makeINI(cfg)

	if err := saveMainConfig(path, d); err != nil {
		return err
	}
	return saveSidecars(path, cfg, d)
}

func SaveSidecars(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return os.ErrInvalid
	}
	normalize(&cfg)
	relativizeForSave(path, &cfg)
	return saveSidecars(path, cfg, makeINI(cfg))
}

func saveMainConfig(path string, d iniData) error {
	return writeTOML(path,
		[]string{"config", "authority_sync", "building"},
		d,
		"mdt-server 主配置",
	)
}
