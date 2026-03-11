package world

import (
	"encoding/json"
	"os"
	"strings"
)

type BlockSizeDef struct {
	Block string `json:"block"`
	Size  int    `json:"size"`
}

func (w *World) LoadBlockSizes(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []BlockSizeDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.blockSizeDefs = defs
	w.resolveBlockSizesLocked()
	return nil
}

func (w *World) resolveBlockSizesLocked() {
	w.blockSizesByName = map[string]int{}
	if len(w.blockSizeDefs) == 0 {
		return
	}
	for _, def := range w.blockSizeDefs {
		name := strings.ToLower(strings.TrimSpace(def.Block))
		if name == "" || def.Size <= 1 {
			continue
		}
		w.blockSizesByName[name] = def.Size
	}
}
