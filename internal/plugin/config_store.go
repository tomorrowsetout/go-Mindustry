package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	hjson "github.com/hjson/hjson-go/v4"
)

// ConfigStore mirrors YZFModuleConfigStore: runtime values live in
// <root>/data/config/config.hjson while <root>/config.hjson is kept as
// a portable index/descriptor for the web UI.
type ConfigStore struct {
	mu   sync.Mutex
	def  *Definition
	file string
	root map[string]any
}

// NewConfigStore opens (and migrates, when needed) the config store
// for a module definition.
func NewConfigStore(def *Definition) *ConfigStore {
	store := &ConfigStore{
		def:  def,
		file: filepath.Join(def.DataDir, "config", "config.hjson"),
	}
	store.migrateRootConfigIndex()
	store.load()
	return store
}

// Path returns the runtime config file location.
func (s *ConfigStore) Path() string { return s.file }

func (s *ConfigStore) migrateRootConfigIndex() {
	rootFile := filepath.Join(s.def.Root, "config.hjson")
	legacyFile := filepath.Join(s.def.DataDir, "config.hjson")

	if !fileExists(s.file) && fileExists(legacyFile) {
		_ = os.MkdirAll(filepath.Dir(s.file), 0o755)
		if data, err := os.ReadFile(legacyFile); err == nil {
			_ = os.WriteFile(s.file, data, 0o644)
		}
	}
	if !fileExists(rootFile) {
		s.writeRootConfigIndex()
		return
	}
	portableIndex := false
	if raw, err := os.ReadFile(rootFile); err == nil {
		var root map[string]any
		if hjson.Unmarshal(raw, &root) == nil {
			fileVal, _ := root["configFile"].(string)
			pathVal, _ := root["configPath"].(string)
			portableIndex = fileVal == "data/config/config.hjson" &&
				pathVal == "data/config/config.hjson"
		}
	}
	if !fileExists(s.file) && !portableIndex {
		_ = os.MkdirAll(filepath.Dir(s.file), 0o755)
		if data, err := os.ReadFile(rootFile); err == nil {
			_ = os.WriteFile(s.file, data, 0o644)
		}
	}
	if !portableIndex {
		s.writeRootConfigIndex()
	}
}

func (s *ConfigStore) writeRootConfigIndex() {
	index := map[string]any{
		"configFile": "data/config/config.hjson",
		"configPath": "data/config/config.hjson",
		"configType": "runtime",
		"note":       "插件运行时配置",
		"tags":       []any{"runtime"},
		"links": []any{
			map[string]any{
				"path": "data/config/config.hjson",
				"note": "插件运行时配置",
				"tags": []any{"runtime"},
			},
		},
	}
	payload, err := hjson.Marshal(index)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.def.Root, "config.hjson"), payload, 0o644)
}

func (s *ConfigStore) load() {
	s.root = map[string]any{}
	raw, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var decoded map[string]any
	if err := hjson.Unmarshal(raw, &decoded); err == nil && decoded != nil {
		s.root = decoded
	}
}

// Save writes the runtime config back as formatted hjson.
func (s *ConfigStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *ConfigStore) saveLocked() error {
	payload, err := hjson.Marshal(s.root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.file, payload, 0o644)
}

// GetString mirrors YZFModuleConfigStore.getString.
func (s *ConfigStore) GetString(key, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.root[key]; ok {
		switch typed := v.(type) {
		case string:
			return typed
		default:
			return fmt.Sprintf("%v", typed)
		}
	}
	return def
}

// GetBool mirrors YZFModuleConfigStore.getBool.
func (s *ConfigStore) GetBool(key string, def bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.root[key]; ok {
		switch typed := v.(type) {
		case bool:
			return typed
		case string:
			parsed, err := strconv.ParseBool(typed)
			if err == nil {
				return parsed
			}
		case float64:
			return typed != 0
		}
	}
	return def
}

// GetInt mirrors YZFModuleConfigStore.getInt.
func (s *ConfigStore) GetInt(key string, def int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.root[key]; ok {
		switch typed := v.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case string:
			parsed, err := strconv.Atoi(typed)
			if err == nil {
				return parsed
			}
		}
	}
	return def
}

// SetString mirrors YZFModuleConfigStore.setString (persists to disk).
func (s *ConfigStore) SetString(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root[key] = value
	return s.saveLocked()
}

// SetBool mirrors YZFModuleConfigStore.setBool.
func (s *ConfigStore) SetBool(key string, value bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root[key] = value
	return s.saveLocked()
}

// SetInt mirrors YZFModuleConfigStore.setInt.
func (s *ConfigStore) SetInt(key string, value int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root[key] = value
	return s.saveLocked()
}
