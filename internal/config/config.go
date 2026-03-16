package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type RuntimeConfig struct {
	Cores            int    `json:"cores"`
	SchedulerEnabled bool   `json:"scheduler_enabled"`
	VanillaProfiles  string `json:"vanilla_profiles"`
	ServerName       string `json:"server_name"`
	ServerDesc       string `json:"server_desc"`
	VirtualPlayers   int    `json:"virtual_players"`
	DevLogEnabled    bool   `json:"devlog_enabled"`
}

type APIConfig struct {
	Enabled bool     `json:"enabled"`
	Key     string   `json:"key"`
	Keys    []string `json:"keys"`
	Bind    string   `json:"bind"`
}

type StorageConfig struct {
	Mode            string `json:"mode"`
	Directory       string `json:"directory"`
	DatabaseEnabled bool   `json:"database_enabled"`
	DSN             string `json:"dsn"`
}

type NetConfig struct {
	UdpRetryCount   int  `json:"udp_retry_count"`
	UdpRetryDelayMs int  `json:"udp_retry_delay_ms"`
	UdpFallbackTCP  bool `json:"udp_fallback_tcp"`
	SyncEntityMs    int  `json:"sync_entity_ms"`
	SyncStateMs     int  `json:"sync_state_ms"`
}

type PersistConfig struct {
	Enabled     bool   `json:"enabled"`
	Directory   string `json:"directory"`
	File        string `json:"file"`
	IntervalSec int    `json:"interval_sec"`
	SaveMSAV    bool   `json:"save_msav"`
	MSAVDir     string `json:"msav_dir"`
	MSAVFile    string `json:"msav_file"`
}

type ModsConfig struct {
	Enabled   bool   `json:"enabled"`
	Directory string `json:"directory"`
	JavaHome  string `json:"java_home"`
	JSDir     string `json:"js_dir"`
	GoDir     string `json:"go_dir"`
	NodeDir   string `json:"node_dir"`
}

type ScriptTask struct {
	DelaySec int      `json:"delay_sec"`
	Runtime  string   `json:"runtime"`
	Target   string   `json:"target"`
	Args     []string `json:"args"`
}

type ScriptConfig struct {
	File         string       `json:"file"`
	StartupTasks []ScriptTask `json:"startup_tasks"`
	DailyGCTime  string       `json:"daily_gc_time"`
}

type AdminConfig struct {
	OpsFile string `json:"ops_file"`
}

type Config struct {
	Source  string        `json:"-"`
	Runtime RuntimeConfig `json:"runtime"`
	API     APIConfig     `json:"api"`
	Storage StorageConfig `json:"storage"`
	Net     NetConfig     `json:"net"`
	Persist PersistConfig `json:"persist"`
	Mods    ModsConfig    `json:"mods"`
	Script  ScriptConfig  `json:"script"`
	Admin   AdminConfig   `json:"admin"`
}

func Default() Config {
	return Config{
		Runtime: RuntimeConfig{
			Cores:            6,
			SchedulerEnabled: false,
			VanillaProfiles:  "data/vanilla/profiles.json",
			ServerName:       "mdt-server",
			ServerDesc:       "",
			VirtualPlayers:   0,
			DevLogEnabled:    true,
		},
		API: APIConfig{
			Enabled: true,
			Key:     "",
			Keys:    nil,
			Bind:    "0.0.0.0:8090",
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
		Admin: AdminConfig{
			OpsFile: "data/state/ops.json",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
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
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, err
	}
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
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}
