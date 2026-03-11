package world

import (
	"encoding/json"
	"os"
	"strings"
)

type LiquidPropsDef struct {
	Liquid       string  `json:"liquid"`
	Coolant      bool    `json:"coolant,omitempty"`
	HeatCapacity float32 `json:"heatCapacity,omitempty"`
	Temperature  float32 `json:"temperature,omitempty"`
	Flammability float32 `json:"flammability,omitempty"`
	Gas          bool    `json:"gas,omitempty"`
}

type LiquidProps struct {
	Coolant      bool
	HeatCapacity float32
	Temperature  float32
	Flammability float32
	Gas          bool
}

func (w *World) LoadLiquidProps(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []LiquidPropsDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.liquidPropsDefs = defs
	w.resolveLiquidPropsLocked()
	return nil
}

func (w *World) resolveLiquidPropsLocked() {
	w.liquidPropsByName = map[string]LiquidProps{}
	if len(w.liquidPropsDefs) == 0 {
		w.liquidPropsByID = nil
		return
	}
	for _, def := range w.liquidPropsDefs {
		name := strings.ToLower(strings.TrimSpace(def.Liquid))
		if name == "" {
			continue
		}
		w.liquidPropsByName[name] = LiquidProps{
			Coolant:      def.Coolant,
			HeatCapacity: def.HeatCapacity,
			Temperature:  def.Temperature,
			Flammability: def.Flammability,
			Gas:          def.Gas,
		}
	}
	w.resolveLiquidPropsByIDLocked()
}

func (w *World) resolveLiquidPropsByIDLocked() {
	w.liquidPropsByID = nil
	if w.liquidPropsByName == nil || w.liquidNamesByID == nil {
		return
	}
	out := make(map[int16]LiquidProps, len(w.liquidNamesByID))
	for id, name := range w.liquidNamesByID {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		if p, ok := w.liquidPropsByName[n]; ok {
			out[id] = p
		}
	}
	w.liquidPropsByID = out
}
