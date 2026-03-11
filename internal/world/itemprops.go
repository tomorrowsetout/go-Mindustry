package world

import (
	"encoding/json"
	"os"
	"strings"
)

type ItemPropsDef struct {
	Item        string  `json:"item"`
	Hardness    float32 `json:"hardness,omitempty"`
	LowPriority bool    `json:"lowPriority,omitempty"`
}

type ItemProps struct {
	Hardness    float32
	LowPriority bool
}

func (w *World) LoadItemProps(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []ItemPropsDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.itemPropsDefs = defs
	w.resolveItemPropsLocked()
	return nil
}

func (w *World) resolveItemPropsLocked() {
	w.itemPropsByName = map[string]ItemProps{}
	if len(w.itemPropsDefs) == 0 {
		w.itemPropsByID = nil
		return
	}
	for _, def := range w.itemPropsDefs {
		name := strings.ToLower(strings.TrimSpace(def.Item))
		if name == "" {
			continue
		}
		w.itemPropsByName[name] = ItemProps{
			Hardness:    def.Hardness,
			LowPriority: def.LowPriority,
		}
	}
	w.resolveItemPropsByIDLocked()
}

func (w *World) resolveItemPropsByIDLocked() {
	w.itemPropsByID = nil
	if w.itemPropsByName == nil || w.itemNamesByID == nil {
		return
	}
	out := make(map[int16]ItemProps, len(w.itemNamesByID))
	for id, name := range w.itemNamesByID {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		if p, ok := w.itemPropsByName[n]; ok {
			out[id] = p
		}
	}
	w.itemPropsByID = out
}
