package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mdt-server/internal/config"
	"mdt-server/internal/mods/go"
	"mdt-server/internal/mods/java"
	"mdt-server/internal/mods/js"
	"mdt-server/internal/mods/node"
)

// ModLoader - Mod Loader
// responsible for loading, unloading, and managing Mods
type ModLoader struct {
	cfg        config.ModsConfig
	loadedMods map[string]*LoadedMod
	managers   map[string]interface{}
}

// LoadedMod represents a loaded mod
type LoadedMod struct {
	Name     string
	Path     string
	DataType string // "java", "js", "go", "node"
	Mod      interface{}
	Manager  interface{}
}

// NewModLoader create new ModLoader instance
func NewModLoader() *ModLoader {
	return &ModLoader{
		cfg: config.ModsConfig{
			Enabled:   false,
			Directory: "mods",
			JavaHome:  "",
			JSDir:     "mods/js",
			GoDir:     "mods/go",
			NodeDir:   "mods/node",
		},
		loadedMods: make(map[string]*LoadedMod),
		managers:   make(map[string]interface{}),
	}
}

// Load loads a mod from the specified path
func (ml *ModLoader) Load(path string) error {
	if !ml.cfg.Enabled {
		return fmt.Errorf("mods not enabled")
	}

	// Check mod type and load accordingly
	ext := filepath.Ext(path)
	switch ext {
	case ".jar":
		manager, exists := ml.managers["java"]
		if !exists {
			manager = java.New(ml.cfg)
			ml.managers["java"] = manager
		}

		// For now, just track the jar file
		mod := &LoadedMod{
			Name:     filepath.Base(path),
			Path:     path,
			DataType: "java",
		}
		ml.loadedMods[mod.Name] = mod
		return nil

	case ".js":
		manager, exists := ml.managers["js"]
		if !exists {
			manager = js.NewJSModManager()
			ml.managers["js"] = manager
		}

		mod := &LoadedMod{
			Name:     filepath.Base(path),
			Path:     path,
			DataType: "js",
		}
		ml.loadedMods[mod.Name] = mod
		return nil

	case ".go":
		manager, exists := ml.managers["go"]
		if !exists {
			manager = go.NewGoModManager()
			ml.managers["go"] = manager
		}

		mod := &LoadedMod{
			Name:     filepath.Base(path),
			Path:     path,
			DataType: "go",
		}
		ml.loadedMods[mod.Name] = mod
		return nil

	default:
		return fmt.Errorf("mod loader: mod type not supported: %s", ext)
	}
}

// Unload unloads a mod by name
func (ml *ModLoader) Unload(name string) error {
	if mod, exists := ml.loadedMods[name]; exists {
		delete(ml.loadedMods, name)
		return nil
	}
	return fmt.Errorf("mod loader: mod not found: %s", name)
}

// GetMod gets a loaded mod by name
func (ml *ModLoader) GetMod(name string) (*LoadedMod, error) {
	if mod, exists := ml.loadedMods[name]; exists {
		return mod, nil
	}
	return nil, fmt.Errorf("mod loader: mod not found: %s", name)
}

// ListMods lists all loaded mods
func (ml *ModLoader) ListMods() []string {
	var mods []string
	for name := range ml.loadedMods {
		mods = append(mods, name)
	}
	return mods
}

// Start starts all loaded mods
func (ml *ModLoader) Start() error {
	// Placeholder for starting mods
	return nil
}

// Stop stops all loaded mods
func (ml *ModLoader) Stop() error {
	// Placeholder for stopping mods
	return nil
}

// Reload reloads all loaded mods
func (ml *ModLoader) Reload() error {
	ml.Stop()
	defer ml.Start()

	// Reload each mod from its path
	for name, mod := range ml.loadedMods {
		// For now, just re-add the mod
		ml.loadedMods[name] = mod
	}
	return nil
}

// IsEnabled checks if mod loading is enabled
func (ml *ModLoader) IsEnabled() bool {
	return ml.cfg.Enabled
}

// SetEnabled enables or disables mod loading
func (ml *ModLoader) SetEnabled(enabled bool) {
	ml.cfg.Enabled = enabled
}

// GetConfig returns the current mod configuration
func (ml *ModLoader) GetConfig() config.ModsConfig {
	return ml.cfg
}

// SetConfig sets the mod configuration
func (ml *ModLoader) SetConfig(cfg config.ModsConfig) {
	ml.cfg = cfg
}

// MarshalJSON implements json.Marshaler
func (ml *ModLoader) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Enabled      bool
		LoadedMods   []string
		JavaHome     string
		JSDir        string
		GoDir        string
		NodeDir      string
	}{
		Enabled:      ml.cfg.Enabled,
		LoadedMods:   ml.ListMods(),
		JavaHome:     ml.cfg.JavaHome,
		JSDir:        ml.cfg.JSDir,
		GoDir:        ml.cfg.GoDir,
		NodeDir:      ml.cfg.NodeDir,
	})
}

// UnmarshalJSON implements json.Unmarshaler
func (ml *ModLoader) UnmarshalJSON(data []byte) error {
	var cfg struct {
		Enabled      bool   `json:"enabled"`
		JavaHome     string `json:"java_home"`
		JSDir        string `json:"js_dir"`
		GoDir        string `json:"go_dir"`
		NodeDir      string `json:"node_dir"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	ml.cfg.Enabled = cfg.Enabled
	ml.cfg.JavaHome = cfg.JavaHome
	ml.cfg.JSDir = cfg.JSDir
	ml.cfg.GoDir = cfg.GoDir
	ml.cfg.NodeDir = cfg.NodeDir
	return nil
}
