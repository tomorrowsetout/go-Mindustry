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
	Enabled   bool
	Directory string
	JavaHome  string
	JSDir     string
	GoDir     string
	NodeDir   string
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
	OpsFile string
}

type SundriesConfig struct {
	DetailedLogMaxMB    int
	DetailedLogMaxFiles int
}

type ControlConfig struct {
	ReloadIntervalSec        int
	ReloadLogEnabled         bool
	NetworkVerboseLogEnabled bool
	TranslatedConnLogEnabled bool
}

type BuildingLogConfig struct {
	Enabled    bool
	Translated bool
}

type Config struct {
	Source   string
	Control  ControlConfig
	Building BuildingLogConfig
	Sundries SundriesConfig
	Runtime  RuntimeConfig
	Core     CoreConfig
	API      APIConfig
	Storage  StorageConfig
	Net      NetConfig
	Persist  PersistConfig
	Mods     ModsConfig
	Script   ScriptConfig
	Admin    AdminConfig
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
		if i := strings.IndexAny(s, ";#"); i >= 0 {
			s = strings.TrimSpace(s[:i])
			if s == "" {
				continue
			}
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
	if v, ok := d.get("config", "network_verbose_log_enabled"); ok {
		cfg.Control.NetworkVerboseLogEnabled = asBool(v, cfg.Control.NetworkVerboseLogEnabled)
	}
	if v, ok := d.get("config", "translated_conn_log_enabled"); ok {
		cfg.Control.TranslatedConnLogEnabled = asBool(v, cfg.Control.TranslatedConnLogEnabled)
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
	d.set("config", "network_verbose_log_enabled", boolToIni(cfg.Control.NetworkVerboseLogEnabled))
	d.set("config", "translated_conn_log_enabled", boolToIni(cfg.Control.TranslatedConnLogEnabled))
	d.set("config", "api_file", cfg.API.ConfigFile)
	d.set("building", "log_enabled", boolToIni(cfg.Building.Enabled))
	d.set("building", "translated_enabled", boolToIni(cfg.Building.Translated))
	d.set("sundries", "detailed_log_max_mb", strconv.Itoa(cfg.Sundries.DetailedLogMaxMB))
	d.set("sundries", "detailed_log_max_files", strconv.Itoa(cfg.Sundries.DetailedLogMaxFiles))

	d.set("runtime", "cores", strconv.Itoa(cfg.Runtime.Cores))
	d.set("runtime", "scheduler_enabled", boolToIni(cfg.Runtime.SchedulerEnabled))
	d.set("runtime", "devlog_enabled", boolToIni(cfg.Runtime.DevLogEnabled))
	d.set("runtime", "vanilla_profiles", cfg.Runtime.VanillaProfiles)

	d.set("core", "dual_core_enabled", boolToIni(cfg.Core.DualCoreEnabled))

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

func normalize(cfg *Config) {
	if strings.TrimSpace(cfg.Runtime.ServerName) == "" {
		cfg.Runtime.ServerName = "mdt-server"
	}
	if cfg.Runtime.VirtualPlayers < 0 {
		cfg.Runtime.VirtualPlayers = 0
	}
	if cfg.Net.SyncEntityMs <= 0 {
		cfg.Net.SyncEntityMs = 100
	}
	if cfg.Net.SyncStateMs <= 0 {
		cfg.Net.SyncStateMs = 250
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
		cfg.API.ConfigFile = "api.ini"
	}
	if cfg.Core.MemoryCheckIntervalSec <= 0 {
		cfg.Core.MemoryCheckIntervalSec = 5
	}
	if cfg.Control.ReloadIntervalSec <= 0 {
		cfg.Control.ReloadIntervalSec = 5
	}
	if cfg.Sundries.DetailedLogMaxMB <= 0 {
		cfg.Sundries.DetailedLogMaxMB = 2
	}
	if cfg.Sundries.DetailedLogMaxFiles <= 0 {
		cfg.Sundries.DetailedLogMaxFiles = 100
	}
}

func sidecarPaths(cfgPath string, cfg Config) map[string]string {
	dir := filepath.Dir(cfgPath)
	apiPath := cfg.API.ConfigFile
	if strings.TrimSpace(apiPath) == "" {
		apiPath = "api.ini"
	}
	if !filepath.IsAbs(apiPath) {
		apiPath = filepath.Join(dir, apiPath)
	}
	return map[string]string{
		"core":     filepath.Join(dir, "core.ini"),
		"server":   filepath.Join(dir, "server.ini"),
		"sync":     filepath.Join(dir, "sync.ini"),
		"misc":     filepath.Join(dir, "misc.ini"),
		"sundries": filepath.Join(dir, "Sundries.ini"),
		"data":     filepath.Join(dir, "data.ini"),  // backward compatibility
		"paths":    filepath.Join(dir, "paths.ini"), // backward compatibility
		"api":      apiPath,
	}
}

func loadSidecars(cfgPath string, cfg *Config) error {
	paths := sidecarPaths(cfgPath, *cfg)
	loadOne := func(path string) error {
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
		d, err := parseINI(path)
		if err != nil {
			return fmt.Errorf("parse ini %s: %w", path, err)
		}
		applyINI(cfg, d)
		return nil
	}
	for _, key := range []string{"core", "server", "sync", "misc", "sundries", "data", "paths", "api"} {
		if err := loadOne(paths[key]); err != nil {
			return err
		}
	}
	return nil
}

func saveSidecars(cfgPath string, cfg Config, d iniData) error {
	paths := sidecarPaths(cfgPath, cfg)
	if err := writeINI(paths["core"], []string{"core", "memory"}, d, "core settings"); err != nil {
		return err
	}
	if err := writeINI(paths["server"], []string{"server"}, d, "server settings"); err != nil {
		return err
	}
	if err := writeINI(paths["sync"], []string{"sync"}, d, "snapshot sync settings"); err != nil {
		return err
	}
	if err := writeINI(paths["misc"], []string{"data", "paths", "mods", "persist", "script", "admin"}, d, "misc settings"); err != nil {
		return err
	}
	if err := writeINI(paths["sundries"], []string{"sundries"}, d, "sundries settings"); err != nil {
		return err
	}
	if err := writeINI(paths["api"], []string{"api"}, d, "api settings"); err != nil {
		return err
	}
	return nil
}

func Default() Config {
	return Config{
		Control: ControlConfig{
			ReloadIntervalSec:        5,
			ReloadLogEnabled:         false,
			NetworkVerboseLogEnabled: false,
			TranslatedConnLogEnabled: true,
		},
		Building: BuildingLogConfig{
			Enabled:    true,
			Translated: true,
		},
		Sundries: SundriesConfig{
			DetailedLogMaxMB:    2,
			DetailedLogMaxFiles: 100,
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
			ConfigFile: "api.ini",
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
			SyncEntityMs:    100,
			SyncStateMs:     250,
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
			Enabled:   false,
			Directory: "mods",
			JavaHome:  "",
			JSDir:     "mods/js",
			GoDir:     "mods/go",
			NodeDir:   "mods/node",
		},
		Script: ScriptConfig{
			File:         "data/state/scripts.json",
			StartupTasks: nil,
			DailyGCTime:  "",
		},
		Admin: AdminConfig{OpsFile: "data/state/ops.json"},
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
			return cfg, nil
		}
		return cfg, err
	}
	if st.IsDir() {
		return cfg, os.ErrInvalid
	}

	d, err := parseINI(path)
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
}

func Save(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return os.ErrInvalid
	}
	normalize(&cfg)
	d := makeINI(cfg)

	if err := writeINI(path,
		[]string{"config", "building"},
		d,
		"mdt-server main config (INI)",
	); err != nil {
		return err
	}
	return saveSidecars(path, cfg, d)
}
