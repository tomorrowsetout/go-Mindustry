package mods

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ====================
// ModManager - Mod Manager
// ====================

// ModManager manages all loaded Mods
// provides Mod loading, unloading, enabling, disabling, etc.
type ModManager struct {
	mods        []*LoadedMod
	directories []string
	mu          sync.RWMutex
	properties  map[string]interface{}
	loader      *ModLoader
}

// NewModManager create new ModManager instance
func NewModManager() *ModManager {
	return &ModManager{
		mods:        make([]*LoadedMod, 0),
		directories: make([]string, 0),
		properties:  make(map[string]interface{}),
		loader:      NewModLoader(),
	}
}

// ====================
// Directory Management
// ====================

// AddDirectory add Mod directory
// dir: directory path
// return error if directory is invalid
func (m *ModManager) AddDirectory(dir string) error {
	if dir == "" {
		return errors.New("mod: invalid directory path")
	}

	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// directory not exists, try to create
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			st, err = os.Stat(dir)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if !st.IsDir() {
		return errors.New("mod: path is not a directory")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// check if already added
	for _, d := range m.directories {
		if d == dir {
			return nil
		}
	}

	m.directories = append(m.directories, dir)
	return nil
}

// RemoveDirectory remove Mod directory
func (m *ModManager) RemoveDirectory(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, d := range m.directories {
		if d == dir {
			m.directories = append(m.directories[:i], m.directories[i+1:]...)
			return nil
		}
	}
	return errors.New("mod: directory not found")
}

// GetDirectories get all Mod directories
func (m *ModManager) GetDirectories() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dirs := make([]string, len(m.directories))
	copy(dirs, m.directories)
	return dirs
}

// LoadAll load all mods from directories
func (m *ModManager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// collect all mod files
	var modFiles []string
	for _, dir := range m.directories {
		// skip non-existent directories
		if _, err := os.Stat(dir); err != nil {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				ext := strings.ToLower(filepath.Ext(path))
				if ext == ".jar" || ext == ".js" || ext == ".go" || ext == ".node" {
					modFiles = append(modFiles, path)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// load each mod
	for _, path := range modFiles {
		if err := m.loader.Load(path); err != nil {
			ModLog.Warn("failed to load mod: %s (%v)", path, err)
		}
	}
	m.mods = m.loader.LoadedMods()

	return nil
}

// UnloadAll unload all mods
func (m *ModManager) UnloadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error
	for _, mod := range m.mods {
		if err := m.loader.Unload(mod.Name); err != nil {
			errors = append(errors, err)
			ModLog.Warn("failed to unload mod: %s (%v)", mod.Name, err)
		}
	}
	m.mods = m.loader.LoadedMods()

	if len(errors) > 0 {
		return fmt.Errorf("failed to unload some mods: %v", errors)
	}

	return nil
}

// ReloadAll reload all mods
func (m *ModManager) ReloadAll() error {
	if err := m.UnloadAll(); err != nil {
		return err
	}
	return m.LoadAll()
}

// StartAll start all mods
func (m *ModManager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loader.Start(); err != nil {
		return err
	}
	m.mods = m.loader.LoadedMods()

	return nil
}

// StopAll stop all mods
func (m *ModManager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loader.Stop(); err != nil {
		return err
	}
	m.mods = m.loader.LoadedMods()

	return nil
}

// GetMods get all loaded mods
func (m *ModManager) GetMods() []*LoadedMod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mods := make([]*LoadedMod, len(m.mods))
	copy(mods, m.mods)
	return mods
}

// GetMod get a loaded mod by name
func (m *ModManager) GetMod(name string) (*LoadedMod, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, mod := range m.mods {
		if mod.Name == name {
			return mod, nil
		}
	}
	return nil, errors.New("mod: mod not found")
}

// HasMod check if a mod is loaded
func (m *ModManager) HasMod(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, mod := range m.mods {
		if mod.Name == name {
			return true
		}
	}
	return false
}

// SetProperty set a property
func (m *ModManager) SetProperty(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.properties[key] = value
}

// GetProperty get a property
func (m *ModManager) GetProperty(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.properties[key]
	if !exists {
		return nil, errors.New("mod: property not found")
	}
	return value, nil
}

// GetLoader return mod loader
func (m *ModManager) GetLoader() *ModLoader {
	return m.loader
}
