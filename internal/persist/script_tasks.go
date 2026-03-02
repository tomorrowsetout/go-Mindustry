package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mdt-server/internal/config"
)

type ScriptState struct {
	Version      int                 `json:"version"`
	StartupTasks []config.ScriptTask `json:"startup_tasks"`
	DailyGCTime  string              `json:"daily_gc_time"`
	UpdatedAt    string              `json:"updated_at"`
}

func LoadScriptConfig(cfg config.ScriptConfig) (ScriptState, bool, error) {
	path := scriptPath(cfg)
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ScriptState{}, false, nil
		}
		return ScriptState{}, false, err
	}
	if st.IsDir() {
		return ScriptState{}, false, os.ErrInvalid
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ScriptState{}, false, err
	}
	var out ScriptState
	if err := json.Unmarshal(data, &out); err != nil {
		return ScriptState{}, false, err
	}
	if out.Version <= 0 {
		out.Version = 1
	}
	return out, true, nil
}

func SaveScriptConfig(cfg config.ScriptConfig, state ScriptState) error {
	path := scriptPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if state.Version <= 0 {
		state.Version = 1
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func scriptPath(cfg config.ScriptConfig) string {
	path := strings.TrimSpace(cfg.File)
	if path == "" {
		return filepath.Join("data", "state", "scripts.json")
	}
	return path
}
