package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mdt-server/internal/config"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type versionedFile struct {
	path string
	mod  time.Time
}

func SaveVersionedState(cfg config.PersistConfig, state State) error {
	if !cfg.Enabled || cfg.VersionMax <= 0 || !cfg.VersionState {
		return nil
	}
	dir := cfg.VersionDir
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("data", "snapshots", "versions")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	base := versionBaseFromMap(state.MapPath, "server-state")
	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.json", base, ts)
	outPath := filepath.Join(dir, name)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	return rotateVersionedFiles(dir, base, ".json", cfg.VersionMax)
}

func SaveVersionedMSAVFromModel(cfg config.PersistConfig, snap world.Snapshot, model *world.WorldModel, mapPath string) error {
	if !cfg.Enabled || !cfg.SaveMSAV || cfg.VersionMax <= 0 || !cfg.VersionMSAV {
		return nil
	}
	dir := cfg.VersionDir
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("data", "snapshots", "versions")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	base := versionBaseFromMap(mapPath, "snapshot")
	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.msav", base, ts)
	outPath := filepath.Join(dir, name)
	tags := map[string]string{
		"wave":     fmt.Sprintf("%d", snap.Wave),
		"wavetime": fmt.Sprintf("%.2f", snap.WaveTime),
		"tick":     fmt.Sprintf("%.0f", float64(snap.Tick)),
	}
	if model != nil {
		if err := worldstream.WriteMSAVFromModel(outPath, model, tags); err == nil {
			return rotateVersionedFiles(dir, base, ".msav", cfg.VersionMax)
		}
	}
	if mapPath != "" {
		if err := worldstream.WriteMSAVSnapshot(mapPath, outPath, tags); err != nil {
			return err
		}
	}
	return rotateVersionedFiles(dir, base, ".msav", cfg.VersionMax)
}

func versionBaseFromMap(mapPath, fallback string) string {
	mapPath = strings.TrimSpace(mapPath)
	if mapPath == "" {
		return fallback
	}
	base := worldstream.TrimMapName(filepath.Base(mapPath))
	if base == "" {
		return fallback
	}
	return base
}

func rotateVersionedFiles(dir, base, ext string, max int) error {
	if max <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	prefix := base + "-"
	candidates := make([]versionedFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, versionedFile{
			path: filepath.Join(dir, name),
			mod:  info.ModTime(),
		})
	}
	if len(candidates) <= max {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].mod.After(candidates[j].mod)
	})
	for i := max; i < len(candidates); i++ {
		_ = os.Remove(candidates[i].path)
	}
	return nil
}
