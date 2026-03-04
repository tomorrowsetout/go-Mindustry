package js

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mdt-server/internal/config"
	"mdt-server/internal/world"
)

// ModInfo 表示 Mod 的元信息
type ModInfo struct {
	Name         string            `json:"name"`         // Mod 名称
	Version      string            `json:"version"`      // 版本
	Author       string            `json:"author"`       // 作者
	Description  string            `json:"description"`  // 描述
	Icon         string            `json:"icon"`         // 图标路径
	Files        map[string]bool   `json:"files"`        // Mod 包含的文件
	Dependencies []string          `json:"dependencies"` // 依赖列表
}

// JSMod 表示一个 JavaScript Mod 实例
type JSMod struct {
	Info     *ModInfo          `json:"info"`      // Mod 信息
	Path     string            `json:"path"`      // Mod 文件路径
	Enabled  bool              `json:"enabled"`   // 是否启用
	Runtime  *JSRuntime        `json:"-"`         // JavaScript 运行时
	loader   *JSModManager     `json:"-"`         // 关联的管理器
	loaded   bool              `json:"-"`         // 是否已加载
	metadata map[string]string `json:"-"`         // 元数据
}

// JSModManager 管理 JavaScript Mod 的加载和运行
type JSModManager struct {
	mu       sync.RWMutex
	cfg      config.ModsConfig
	mods     map[string]*JSMod
	runtimes map[string]*JSRuntime
	enabled  map[string]bool
}

// ErrModNotFound 表示 Mod 未找到
var ErrModNotFound = errors.New("mod not found")

// ErrModAlreadyLoaded 表示 Mod 已加载
var ErrModAlreadyLoaded = errors.New("mod already loaded")

// ErrModNotLoaded 表示 Mod 未加载
var ErrModNotLoaded = errors.New("mod not loaded")

// ErrModNotEnabled 表示 Mod 未启用
var ErrModNotEnabled = errors.New("mod not enabled")

// NewJSModManager 创建新的 JS Mod 管理器
func NewJSModManager() *JSModManager {
	return &JSModManager{
		cfg:      config.ModsConfig{},
		mods:     make(map[string]*JSMod),
		runtimes: make(map[string]*JSRuntime),
		enabled:  make(map[string]bool),
	}
}

// SetConfig 设置配置
func (m *JSModManager) SetConfig(cfg config.ModsConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// LoadMods 从指定目录加载所有 Mod
func (m *JSModManager) LoadMods(directory string) ([]*JSMod, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cfg.Enabled {
		return nil, nil
	}

	// 创建目录（如果不存在）
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mods directory: %w", err)
	}

	// 读取目录
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read mods directory: %w", err)
	}

	var loadedMods []*JSMod

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// 只加载 .js 和 .mjs 文件
		if ext != ".js" && ext != ".mjs" {
			continue
		}

		modName := strings.TrimSuffix(name, ext)
		if _, exists := m.mods[modName]; exists {
			continue
		}

		// 创建 Mod
		mod := &JSMod{
			Info: &ModInfo{
				Name:    modName,
				Version: "1.0.0",
				Files:   make(map[string]bool),
			},
			Path:     filepath.Join(directory, name),
			Enabled:  true,
			metadata: make(map[string]string),
		}

		// 读取 Mod 信息（如果存在）
		if err := m.parseModInfo(mod); err != nil {
			return nil, fmt.Errorf("failed to parse mod info for %s: %w", modName, err)
		}

		// 创建运行时
		runtime := NewJSRuntime()
		if err := runtime.Init(); err != nil {
			return nil, fmt.Errorf("failed to init runtime for %s: %w", modName, err)
		}

		mod.Runtime = runtime
		mod.loader = m

		// 加载 Mod 脚本
		if err := m.loadModScript(mod); err != nil {
			runtime.Close()
			return nil, fmt.Errorf("failed to load script for %s: %w", modName, err)
		}

		// 调用 onLoad 方法
		if err := m.CallOnLoad(modName); err != nil {
			runtime.Close()
			return nil, fmt.Errorf("failed to call onLoad for %s: %w", modName, err)
		}

		m.mods[modName] = mod
		m.runtimes[modName] = runtime
		m.enabled[modName] = true
		loadedMods = append(loadedMods, mod)

		mod.loaded = true
	}

	return loadedMods, nil
}

// parseModInfo 解析 Mod 信息
func (m *JSModManager) parseModInfo(mod *JSMod) error {
	mod.Info.Files[mod.Path] = true

	// 尝试读取 package.json（如果存在）
	dir := filepath.Dir(mod.Path)
	packageJSON := filepath.Join(dir, "package.json")

	_, err := os.ReadFile(packageJSON)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	// 这里是简化实现，实际应该解析 JSON
	mod.Info.Author = "Unknown"
	mod.Info.Description = "A JavaScript Mod"

	return nil
}

// loadModScript 加载 Mod 脚本
func (m *JSModManager) loadModScript(mod *JSMod) error {
	data, err := os.ReadFile(mod.Path)
	if err != nil {
		return err
	}

	return mod.Runtime.Run(string(data))
}

// UnloadMod 卸载指定名称的 Mod
func (m *JSModManager) UnloadMod(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if !mod.loaded {
		return nil
	}

	// 调用 onUnload 方法
	if err := m.CallOnUnload(name); err != nil {
		return fmt.Errorf("failed to call onUnload: %w", err)
	}

	// 关闭运行时
	if mod.Runtime != nil {
		if err := mod.Runtime.Close(); err != nil {
			return fmt.Errorf("failed to close runtime: %w", err)
		}
	}

	delete(m.mods, name)
	delete(m.runtimes, name)
	delete(m.enabled, name)
	mod.loaded = false

	return nil
}

// EnableMod 启用指定名称的 Mod
func (m *JSModManager) EnableMod(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if mod.Enabled {
		return nil
	}

	mod.Enabled = true
	m.enabled[name] = true

	// 重新加载脚本
	if mod.loaded {
		if err := m.loadModScript(mod); err != nil {
			return fmt.Errorf("failed to reload script: %w", err)
		}
	}

	return nil
}

// DisableMod 禁用指定名称的 Mod
func (m *JSModManager) DisableMod(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if !mod.Enabled {
		return nil
	}

	mod.Enabled = false
	m.enabled[name] = false

	return nil
}

// GetMod 获取指定名称的 Mod
func (m *JSModManager) GetMod(name string) *JSMod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.mods[name]
}

// GetActiveMods 获取所有已启用的 Mod
func (m *JSModManager) GetActiveMods() []*JSMod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var activeMods []*JSMod
	for name, mod := range m.mods {
		if m.enabled[name] {
			activeMods = append(activeMods, mod)
		}
	}

	return activeMods
}

// CallMethod 调用 Mod 的指定方法
func (m *JSModManager) CallMethod(modName, method string, args ...interface{}) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod, exists := m.mods[modName]
	if !exists {
		return nil, ErrModNotFound
	}

	if !m.enabled[modName] {
		return nil, ErrModNotEnabled
	}

	if !mod.loaded {
		return nil, ErrModNotLoaded
	}

	return mod.Runtime.CallFunction(method, args...)
}

// CallOnLoad 调用 Mod 的 onLoad 方法
func (m *JSModManager) CallOnLoad(name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if !m.enabled[name] {
		return ErrModNotEnabled
	}

	_, err := mod.Runtime.CallFunction("onLoad")
	return err
}

// CallOnUnload 调用 Mod 的 onUnload 方法
func (m *JSModManager) CallOnUnload(name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if !m.enabled[name] {
		return ErrModNotEnabled
	}

	_, err := mod.Runtime.CallFunction("onUnload")
	return err
}

// CallOnWorldLoad 调用 Mod 的 onWorldLoad 方法
func (m *JSModManager) CallOnWorldLoad(name string, world *world.World) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod, exists := m.mods[name]
	if !exists {
		return ErrModNotFound
	}

	if !m.enabled[name] {
		return ErrModNotEnabled
	}

	// 将 world 对象传递给 JavaScript
	if err := mod.Runtime.SetGlobal("__world__", world); err != nil {
		return fmt.Errorf("failed to set world: %w", err)
	}

	_, err := mod.Runtime.CallFunction("onWorldLoad")
	return err
}

// GetModInfo 获取指定 Mod 的信息
func (m *JSModManager) GetModInfo(name string) (*ModInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod, exists := m.mods[name]
	if !exists {
		return nil, ErrModNotFound
	}

	info := &ModInfo{
		Name:         mod.Info.Name,
		Version:      mod.Info.Version,
		Author:       mod.Info.Author,
		Description:  mod.Info.Description,
		Icon:         mod.Info.Icon,
		Files:        make(map[string]bool),
		Dependencies: make([]string, len(mod.Info.Dependencies)),
	}

	// 复制 Files
	for k, v := range mod.Info.Files {
		info.Files[k] = v
	}

	// 复制 Dependencies
	copy(info.Dependencies, mod.Info.Dependencies)

	return info, nil
}

// GetAllMods 获取所有 Mod（包括未启用的）
func (m *JSModManager) GetAllMods() []*JSMod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allMods []*JSMod
	for _, mod := range m.mods {
		allMods = append(allMods, mod)
	}

	return allMods
}

// GetRuntime 获取指定 Mod 的运行时
func (m *JSModManager) GetRuntime(name string) (*JSRuntime, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	runtime, exists := m.runtimes[name]
	if !exists {
		return nil, ErrModNotFound
	}

	return runtime, nil
}
