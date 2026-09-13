package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mdt-server/internal/config"
)

type OpsState struct {
	Ops     []string `json:"ops"`
	SavedAt string   `json:"saved_at"`
}

func LoadOps(cfg config.AdminConfig) ([]string, bool, error) {
	path := opsPath(cfg)
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if st.IsDir() {
		return nil, false, os.ErrInvalid
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var out OpsState
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false, err
	}
	ops := normalizeOps(out.Ops)
	return ops, true, nil
}

func SaveOps(cfg config.AdminConfig, ops []string) error {
	path := opsPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	state := OpsState{
		Ops:     normalizeOps(ops),
		SavedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func opsPath(cfg config.AdminConfig) string {
	path := strings.TrimSpace(cfg.OpsFile)
	if path == "" {
		return filepath.Join("data", "state", "ops.json")
	}
	return path
}

func normalizeOps(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
