package java

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mdt-server/internal/config"
)

var ErrJVMNotImplemented = errors.New("jvm bridge not implemented")

type Mod struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

type Event struct {
	Type string
	Time time.Time
	Meta map[string]string
}

type Bridge interface {
	Send(Event) error
}

type Manager struct {
	cfg    config.ModsConfig
	mods   []Mod
	bridge Bridge
}

func New(cfg config.ModsConfig) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) SetBridge(b Bridge) {
	m.bridge = b
}

func (m *Manager) Mods() []Mod {
	out := make([]Mod, len(m.mods))
	copy(out, m.mods)
	return out
}

func (m *Manager) Scan() error {
	if !m.cfg.Enabled {
		return nil
	}
	dir := strings.TrimSpace(m.cfg.Directory)
	if dir == "" {
		dir = "mods"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var mods []Mod
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jar") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			return err
		}
		mods = append(mods, Mod{
			Name:    strings.TrimSuffix(name, filepath.Ext(name)),
			Path:    path,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
	m.mods = mods
	return nil
}

func (m *Manager) Start() error {
	if !m.cfg.Enabled {
		return nil
	}
	if len(m.mods) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %d mods found", ErrJVMNotImplemented, len(m.mods))
}

func (m *Manager) Emit(event Event) {
	if m.bridge == nil {
		return
	}
	_ = m.bridge.Send(event)
}
