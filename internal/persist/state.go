package persist

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"mdt-server/internal/config"
)

type State struct {
	MapPath  string  `json:"map_path"`
	WaveTime float32 `json:"wave_time"`
	Wave     int32   `json:"wave"`
	Tick     uint64  `json:"tick"`
	TimeData int32   `json:"time_data"`
	Rand0    int64   `json:"rand0"`
	Rand1    int64   `json:"rand1"`
	SavedAt  string  `json:"saved_at"`
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
	return out, true, nil
}

func Save(cfg config.PersistConfig, state State) error {
	path, err := filePath(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	state.SavedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func filePath(cfg config.PersistConfig) (string, error) {
	dir := cfg.Directory
	if dir == "" {
		dir = "data/state"
	}
	name := cfg.File
	if name == "" {
		name = "server-state.json"
	}
	return filepath.Join(dir, name), nil
}
