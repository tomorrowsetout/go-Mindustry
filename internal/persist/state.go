package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/config"
)

const (
	SnapshotVersion   = 1
	hotSnapshotKind   = "hot"
	coldSnapshotKind  = "cold"
	coldArchivePrefix = "runtime-"
	coldArchiveSuffix = ".json"
)

type State struct {
	Version    int     `json:"version"`
	Kind       string  `json:"kind,omitempty"`
	MapPath    string  `json:"map_path"`
	WaveTime   float32 `json:"wave_time"`
	Wave       int32   `json:"wave"`
	Enemies    int32   `json:"enemies"`
	Paused     bool    `json:"paused"`
	GameOver   bool    `json:"game_over"`
	Tick       uint64  `json:"tick"`
	TimeData   int32   `json:"time_data"`
	Tps        int8    `json:"tps"`
	Rand0      int64   `json:"rand0"`
	Rand1      int64   `json:"rand1"`
	CapturedAt string  `json:"captured_at,omitempty"`
	SavedAt    string  `json:"saved_at,omitempty"`
}

type HotSnapshotStore struct {
	mu    sync.RWMutex
	state State
	ok    bool
}

func NewHotSnapshotStore() *HotSnapshotStore {
	return &HotSnapshotStore{}
}

func (s *HotSnapshotStore) Update(state State) State {
	state = normalizeSnapshotState(state, hotSnapshotKind, time.Now().UTC())
	if s == nil {
		return state
	}
	s.mu.Lock()
	s.state = state
	s.ok = true
	s.mu.Unlock()
	return state
}

func (s *HotSnapshotStore) Get() (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ok {
		return State{}, false
	}
	return s.state, true
}

func (s *HotSnapshotStore) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.state = State{}
	s.ok = false
	s.mu.Unlock()
}

func Load(cfg config.PersistConfig) (State, bool, error) {
	path, err := filePath(cfg)
	if err != nil {
		return State{}, false, err
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, err
	}
	if st.IsDir() {
		return State{}, false, errors.New("persist file is a directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false, err
	}
	var out State
	if err := json.Unmarshal(data, &out); err != nil {
		return State{}, false, err
	}
	out = normalizeLoadedSnapshotState(out)
	if !out.Valid() {
		return State{}, false, nil
	}
	return out, true, nil
}

func Save(cfg config.PersistConfig, state State) error {
	if !cfg.Enabled {
		return nil
	}
	path, err := filePath(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	now := time.Now().UTC()
	state = normalizeSnapshotState(state, coldSnapshotKind, now)
	state.SavedAt = now.Format(time.RFC3339Nano)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data, 0644); err != nil {
		return err
	}
	if archivePath := archiveFilePath(cfg, now); archivePath != "" && filepath.Clean(archivePath) != filepath.Clean(path) {
		if err := writeFileAtomic(archivePath, data, 0644); err != nil {
			return err
		}
	}
	return cleanupColdSnapshots(filepath.Dir(path), cfg.RetentionDays)
}

func filePath(cfg config.PersistConfig) (string, error) {
	dir := cfg.Directory
	if dir == "" {
		dir = filepath.Join("data", "snapshots", "runtime")
	}
	name := cfg.File
	if name == "" {
		name = "latest.json"
	}
	return filepath.Join(dir, name), nil
}

func archiveFilePath(cfg config.PersistConfig, now time.Time) string {
	dir := cfg.Directory
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("data", "snapshots", "runtime")
	}
	name := coldArchivePrefix + now.UTC().Format("20060102-150405.000000000") + "Z" + coldArchiveSuffix
	return filepath.Join(dir, name)
}

func normalizeSnapshotState(state State, kind string, now time.Time) State {
	if state.Version <= 0 {
		state.Version = SnapshotVersion
	}
	if strings.TrimSpace(kind) != "" {
		state.Kind = strings.TrimSpace(kind)
	}
	if strings.TrimSpace(state.CapturedAt) == "" {
		state.CapturedAt = now.Format(time.RFC3339Nano)
	}
	return state
}

func normalizeLoadedSnapshotState(state State) State {
	if state.Version <= 0 && state.Valid() {
		state.Version = 1
	}
	return state
}

func (s State) Valid() bool {
	return strings.TrimSpace(s.MapPath) != "" ||
		s.WaveTime != 0 ||
		s.Wave > 0 ||
		s.Tick > 0 ||
		s.TimeData > 0 ||
		s.Rand0 != 0 ||
		s.Rand1 != 0
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return os.ErrInvalid
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d.%d", filepath.Base(path), os.Getpid(), time.Now().UnixNano()))
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func cleanupColdSnapshots(dir string, retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, coldArchivePrefix) || !strings.HasSuffix(name, coldArchiveSuffix) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}
