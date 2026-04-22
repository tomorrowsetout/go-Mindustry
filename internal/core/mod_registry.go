package core

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mdt-server/internal/storage"
)

type managedMod struct {
	ID      int64
	Name    string
	Path    string
	ModType string
	Loaded  bool
	Running bool
	Size    int64
	ModTime time.Time
}

func modStableID(path, modType string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(modType))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(path))))
	return int64(h.Sum64())
}

func modKey(path, modType string) string {
	return strings.ToLower(strings.TrimSpace(modType)) + "|" + filepath.Clean(path)
}

func defaultModRoots() []string {
	return []string{"mods", filepath.Join("bin", "mods")}
}

func inferModType(path, hint string) string {
	if strings.TrimSpace(hint) != "" {
		return strings.ToLower(strings.TrimSpace(hint))
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".jar", ".zip":
		return "java"
	case ".js":
		base := strings.ToLower(filepath.Base(filepath.Dir(path)))
		if base == "node" {
			return "node"
		}
		return "js"
	}
	base := strings.ToLower(filepath.Base(filepath.Dir(path)))
	switch base {
	case "go", "js", "node", "java":
		return base
	}
	return "unknown"
}

func isModFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".js", ".jar", ".zip":
		return true
	default:
		return false
	}
}

func isManifestFileName(name string) bool {
	switch strings.ToLower(name) {
	case "mod.json", "mod.hjson", "plugin.json", "meta.json":
		return true
	default:
		return false
	}
}

func normalizeModName(name, path string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = filepath.Base(filepath.Dir(path))
	}
	return base
}

func modPathInfo(path string) (string, fs.FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, errors.New("empty mod path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, err
	}
	return abs, info, nil
}

func newManagedMod(path, name, modType string, info fs.FileInfo) *managedMod {
	if info == nil {
		info = fakeFileInfo{name: filepath.Base(path)}
	}
	modType = inferModType(path, modType)
	return &managedMod{
		ID:      modStableID(path, modType),
		Name:    normalizeModName(name, path),
		Path:    filepath.Clean(path),
		ModType: modType,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
}

func (c2 *Core2) recordModEvent(kind string, mod *managedMod, detail string) {
	if c2 == nil {
		return
	}
	name := ""
	if mod != nil {
		name = mod.Name
	}
	c2.recordEvent(storage.Event{
		Timestamp: time.Now().UTC(),
		Kind:      kind,
		Name:      name,
		Detail:    detail,
	})
}

func (c2 *Core2) cloneModLocked(mod *managedMod) *managedMod {
	if mod == nil {
		return nil
	}
	clone := *mod
	return &clone
}

func (c2 *Core2) findModLocked(name, path, modType string) *managedMod {
	if c2 == nil {
		return nil
	}
	if strings.TrimSpace(path) != "" {
		if abs, info, err := modPathInfo(path); err == nil {
			key := modKey(abs, inferModType(abs, modType))
			if mod, ok := c2.mods[key]; ok {
				mod.Size = info.Size()
				mod.ModTime = info.ModTime()
				return mod
			}
		}
	}
	wantName := strings.TrimSpace(name)
	wantType := strings.ToLower(strings.TrimSpace(modType))
	for _, mod := range c2.mods {
		if mod == nil {
			continue
		}
		if wantName != "" && !strings.EqualFold(mod.Name, wantName) {
			continue
		}
		if wantType != "" && mod.ModType != wantType {
			continue
		}
		return mod
	}
	return nil
}

func (c2 *Core2) ensureModRegistered(name, path, modType string) (*managedMod, error) {
	abs, info, err := modPathInfo(path)
	if err != nil {
		return nil, err
	}
	c2.modMu.Lock()
	defer c2.modMu.Unlock()
	if existing := c2.findModLocked(name, abs, modType); existing != nil {
		existing.Name = normalizeModName(name, abs)
		existing.ModType = inferModType(abs, modType)
		existing.Size = info.Size()
		existing.ModTime = info.ModTime()
		return c2.cloneModLocked(existing), nil
	}
	mod := newManagedMod(abs, name, modType, info)
	c2.mods[modKey(abs, mod.ModType)] = mod
	return c2.cloneModLocked(mod), nil
}

func (c2 *Core2) scanOneRoot(path string, seen map[string]struct{}) (int, error) {
	abs, info, err := modPathInfo(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	register := func(modPath, modType string, info fs.FileInfo) int {
		key := modKey(modPath, inferModType(modPath, modType))
		if _, ok := seen[key]; ok {
			return 0
		}
		seen[key] = struct{}{}
		c2.modMu.Lock()
		if mod, ok := c2.mods[key]; ok {
			mod.Size = info.Size()
			mod.ModTime = info.ModTime()
			mod.Name = normalizeModName(mod.Name, modPath)
			mod.ModType = inferModType(modPath, modType)
		} else {
			c2.mods[key] = newManagedMod(modPath, "", modType, info)
		}
		c2.modMu.Unlock()
		return 1
	}

	if !info.IsDir() {
		if isModFileName(info.Name()) {
			return register(abs, "", info), nil
		}
		return 0, nil
	}

	count := 0
	err = filepath.WalkDir(abs, func(entryPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		name := strings.ToLower(d.Name())
		switch {
		case isManifestFileName(name):
			count += register(filepath.Dir(entryPath), "", info)
		case isModFileName(name):
			count += register(entryPath, "", info)
		}
		return nil
	})
	return count, err
}

func (c2 *Core2) scanModDirectory(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil {
		result.Error = errors.New("nil core2")
		return result
	}
	roots := make([]string, 0, 2)
	if m != nil && strings.TrimSpace(m.Path) != "" {
		roots = append(roots, m.Path)
	} else {
		roots = append(roots, defaultModRoots()...)
	}
	seen := make(map[string]struct{})
	count := 0
	for _, root := range roots {
		scanned, err := c2.scanOneRoot(root, seen)
		if err != nil {
			result.Error = err
			return result
		}
		count += scanned
	}
	result.Success = true
	result.Name = fmt.Sprintf("scanned:%d", count)
	c2.recordModEvent("mod_scan", nil, result.Name)
	return result
}

func (c2 *Core2) loadMod(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil || m == nil {
		result.Error = errors.New("invalid mod load request")
		return result
	}
	mod, err := c2.ensureModRegistered(m.Name, m.Path, m.ModType)
	if err != nil {
		result.Error = err
		return result
	}
	c2.modMu.Lock()
	target := c2.findModLocked(mod.Name, mod.Path, mod.ModType)
	target.Loaded = true
	target.Running = false
	mod = c2.cloneModLocked(target)
	c2.modMu.Unlock()
	result.Success = true
	result.Name = mod.Name
	result.ModID = mod.ID
	c2.recordModEvent("mod_load", mod, mod.Path)
	return result
}

func (c2 *Core2) unloadMod(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil || m == nil {
		result.Error = errors.New("invalid mod unload request")
		return result
	}
	c2.modMu.Lock()
	target := c2.findModLocked(m.Name, m.Path, m.ModType)
	if target == nil {
		c2.modMu.Unlock()
		result.Error = fmt.Errorf("mod not found")
		return result
	}
	target.Loaded = false
	target.Running = false
	mod := c2.cloneModLocked(target)
	c2.modMu.Unlock()
	result.Success = true
	result.Name = mod.Name
	result.ModID = mod.ID
	c2.recordModEvent("mod_unload", mod, mod.Path)
	return result
}

func (c2 *Core2) startMod(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil || m == nil {
		result.Error = errors.New("invalid mod start request")
		return result
	}
	c2.modMu.Lock()
	target := c2.findModLocked(m.Name, m.Path, m.ModType)
	c2.modMu.Unlock()
	if target == nil {
		loaded := c2.loadMod(m)
		if loaded.Error != nil {
			return loaded
		}
		c2.modMu.Lock()
		target = c2.findModLocked(m.Name, m.Path, m.ModType)
		c2.modMu.Unlock()
	}
	if target == nil {
		result.Error = fmt.Errorf("mod not found after load")
		return result
	}
	c2.modMu.Lock()
	target.Loaded = true
	target.Running = true
	mod := c2.cloneModLocked(target)
	c2.modMu.Unlock()
	result.Success = true
	result.Name = mod.Name
	result.ModID = mod.ID
	c2.recordModEvent("mod_start", mod, mod.Path)
	return result
}

func (c2 *Core2) stopMod(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil || m == nil {
		result.Error = errors.New("invalid mod stop request")
		return result
	}
	c2.modMu.Lock()
	target := c2.findModLocked(m.Name, m.Path, m.ModType)
	if target == nil {
		c2.modMu.Unlock()
		result.Error = fmt.Errorf("mod not found")
		return result
	}
	target.Running = false
	mod := c2.cloneModLocked(target)
	c2.modMu.Unlock()
	result.Success = true
	result.Name = mod.Name
	result.ModID = mod.ID
	c2.recordModEvent("mod_stop", mod, mod.Path)
	return result
}

func (c2 *Core2) reloadMod(m *ModMessage) ModResult {
	result := ModResult{ID: m.ID}
	if c2 == nil || m == nil {
		result.Error = errors.New("invalid mod reload request")
		return result
	}
	c2.modMu.RLock()
	prev := c2.findModLocked(m.Name, m.Path, m.ModType)
	wasRunning := prev != nil && prev.Running
	c2.modMu.RUnlock()
	mod, err := c2.ensureModRegistered(m.Name, chooseNonEmpty(m.Path, pathOfManagedMod(prev)), chooseNonEmpty(m.ModType, typeOfManagedMod(prev)))
	if err != nil {
		result.Error = err
		return result
	}
	c2.modMu.Lock()
	target := c2.findModLocked(mod.Name, mod.Path, mod.ModType)
	target.Loaded = true
	target.Running = wasRunning
	mod = c2.cloneModLocked(target)
	c2.modMu.Unlock()
	result.Success = true
	result.Name = mod.Name
	result.ModID = mod.ID
	c2.recordModEvent("mod_reload", mod, mod.Path)
	return result
}

func pathOfManagedMod(mod *managedMod) string {
	if mod == nil {
		return ""
	}
	return mod.Path
}

func typeOfManagedMod(mod *managedMod) string {
	if mod == nil {
		return ""
	}
	return mod.ModType
}

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }
