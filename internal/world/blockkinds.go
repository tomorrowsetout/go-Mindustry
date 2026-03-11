package world

import (
	"encoding/json"
	"os"
	"strings"
)

type BlockKindDef struct {
	Block string   `json:"block"`
	Class string   `json:"class"`
	Speed float32  `json:"speed,omitempty"`
	Group string   `json:"group,omitempty"`
	Kind  string   `json:"kind,omitempty"`
	Flags []string `json:"flags,omitempty"`
}

type BlockKind struct {
	Block string
	Class string
	Speed float32
	Group string
	Kind  string
	Flags map[string]struct{}
}

func (w *World) LoadBlockKinds(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []BlockKindDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.blockKindsByName = map[string]BlockKind{}
	for _, d := range defs {
		name := strings.ToLower(strings.TrimSpace(d.Block))
		if name == "" {
			continue
		}
		flags := map[string]struct{}{}
		for _, f := range d.Flags {
			f = strings.ToLower(strings.TrimSpace(f))
			if f == "" {
				continue
			}
			flags[f] = struct{}{}
		}
		w.blockKindsByName[name] = BlockKind{
			Block: name,
			Class: strings.TrimSpace(d.Class),
			Speed: d.Speed,
			Group: strings.ToLower(strings.TrimSpace(d.Group)),
			Kind:  strings.ToLower(strings.TrimSpace(d.Kind)),
			Flags: flags,
		}
	}
	w.resolveBlockKindsLocked()
	return nil
}

func (w *World) resolveBlockKindsLocked() {
	w.blockKindsByID = nil
	if w.blockKindsByName == nil || w.blockNamesByID == nil {
		return
	}
	out := make(map[int16]BlockKind, len(w.blockNamesByID))
	for id, name := range w.blockNamesByID {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		if k, ok := w.blockKindsByName[n]; ok {
			out[id] = k
		}
	}
	w.blockKindsByID = out
}

func (w *World) blockKindByName(name string) (BlockKind, bool) {
	if w == nil || w.blockKindsByName == nil {
		return BlockKind{}, false
	}
	k, ok := w.blockKindsByName[strings.ToLower(strings.TrimSpace(name))]
	return k, ok
}

func (w *World) blockKindByID(id int16) (BlockKind, bool) {
	if w == nil || w.blockKindsByID == nil {
		return BlockKind{}, false
	}
	k, ok := w.blockKindsByID[id]
	return k, ok
}
