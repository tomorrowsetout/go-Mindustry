package persist

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"mdt-server/internal/config"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

func SaveMSAVSnapshot(cfg config.PersistConfig, mapPath string, snap world.Snapshot) error {
	if !cfg.Enabled || !cfg.SaveMSAV {
		return nil
	}
	if mapPath == "" {
		return nil
	}
	lower := strings.ToLower(mapPath)
	if !strings.HasSuffix(lower, ".msav") && !strings.HasSuffix(lower, ".msav.msav") {
		return nil
	}
	dir := cfg.MSAVDir
	if dir == "" {
		dir = cfg.Directory
	}
	name := cfg.MSAVFile
	if name == "" {
		base := worldstream.TrimMapName(filepath.Base(mapPath))
		name = fmt.Sprintf("%s-snapshot.msav", base)
	}
	outPath := filepath.Join(dir, name)
	tags := map[string]string{
		"wave":     strconv.Itoa(int(snap.Wave)),
		"wavetime": fmt.Sprintf("%.2f", snap.WaveTime),
		"tick":     fmt.Sprintf("%.0f", float64(snap.Tick)),
	}
	return worldstream.WriteMSAVSnapshot(mapPath, outPath, tags)
}

func SaveMSAVSnapshotFromModel(cfg config.PersistConfig, snap world.Snapshot, model *world.WorldModel, fallbackMapPath string) error {
	if !cfg.Enabled || !cfg.SaveMSAV {
		return nil
	}
	dir := cfg.MSAVDir
	if dir == "" {
		dir = cfg.Directory
	}
	name := cfg.MSAVFile
	if name == "" && fallbackMapPath != "" {
		base := worldstream.TrimMapName(filepath.Base(fallbackMapPath))
		name = fmt.Sprintf("%s-snapshot.msav", base)
	}
	if name == "" {
		name = "snapshot.msav"
	}
	outPath := filepath.Join(dir, name)
	tags := map[string]string{
		"wave":     strconv.Itoa(int(snap.Wave)),
		"wavetime": fmt.Sprintf("%.2f", snap.WaveTime),
		"tick":     fmt.Sprintf("%.0f", float64(snap.Tick)),
	}
	if model != nil {
		if err := worldstream.WriteMSAVFromModel(outPath, model, tags); err == nil {
			return nil
		}
	}
	if fallbackMapPath != "" {
		return worldstream.WriteMSAVSnapshot(fallbackMapPath, outPath, tags)
	}
	return nil
}
