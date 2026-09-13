package world

import (
	"strings"
	"time"
)

var vanillaNonOverdrivableBlocks = map[string]struct{}{
	"battery":                     {},
	"battery-large":               {},
	"canvas":                      {},
	"landing-pad":                 {},
	"liquid-container":            {},
	"liquid-router":               {},
	"liquid-tank":                 {},
	"large-canvas":                {},
	"memory-bank":                 {},
	"memory-cell":                 {},
	"overdrive-dome":              {},
	"overdrive-projector":         {},
	"overflow-gate":               {},
	"payload-loader":              {},
	"payload-unloader":            {},
	"power-node":                  {},
	"power-node-large":            {},
	"power-source":                {},
	"reinforced-liquid-container": {},
	"reinforced-liquid-router":    {},
	"reinforced-liquid-tank":      {},
	"underflow-gate":              {},
	"beam-link":                   {},
	"electric-heater":             {},
	"heat-reactor":                {},
	"heat-source":                 {},
	"neoplasia-reactor":           {},
	"oxidation-chamber":           {},
	"phase-heater":                {},
	"slag-heater":                 {},
	"surge-tower":                 {},
	"world-cell":                  {},
}

func vanillaBlockCanOverdrive(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, "core-") {
		return false
	}
	if strings.HasSuffix(name, "-conduit") {
		return false
	}
	if strings.HasSuffix(name, "-wall") ||
		strings.HasSuffix(name, "-wall-large") ||
		strings.HasSuffix(name, "-wall-huge") ||
		strings.HasSuffix(name, "-wall-gigantic") {
		return false
	}
	if strings.HasSuffix(name, "logic-display") {
		return false
	}
	_, blocked := vanillaNonOverdrivableBlocks[name]
	return !blocked
}

func (w *World) buildingCanOverdriveLocked(pos int32) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return false
	}
	return vanillaBlockCanOverdrive(w.blockNameByID(int16(tile.Block)))
}

type buildingBoostState struct {
	TimeScale float32
	Duration  float32
}

func (w *World) stepBuildingBoosts(delta time.Duration) {
	if w == nil || len(w.buildingBoostStates) == 0 {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	if deltaFrames <= 0 {
		return
	}
	for pos, st := range w.buildingBoostStates {
		if !w.buildingCanOverdriveLocked(pos) {
			delete(w.buildingBoostStates, pos)
			continue
		}
		st.Duration -= deltaFrames
		if st.Duration <= 0 {
			delete(w.buildingBoostStates, pos)
			continue
		}
		if st.TimeScale < 1 {
			st.TimeScale = 1
		}
		w.buildingBoostStates[pos] = st
	}
}

func (w *World) applyBuildingBoostLocked(pos int32, intensity, duration float32) {
	if w == nil || intensity <= 0 || duration <= 0 || !w.buildingCanOverdriveLocked(pos) {
		return
	}
	if w.buildingBoostStates == nil {
		w.buildingBoostStates = map[int32]buildingBoostState{}
	}
	st := w.buildingBoostStates[pos]
	if intensity >= st.TimeScale-0.001 {
		st.Duration = maxf(st.Duration, duration)
	}
	if intensity > st.TimeScale {
		st.TimeScale = intensity
	}
	if st.TimeScale < 1 {
		st.TimeScale = 1
	}
	if st.Duration <= 0 {
		st.Duration = duration
	}
	w.buildingBoostStates[pos] = st
}

func (w *World) buildingTimeScaleLocked(pos int32) float32 {
	if w == nil || len(w.buildingBoostStates) == 0 {
		return 1
	}
	st, ok := w.buildingBoostStates[pos]
	if !ok || st.Duration <= 0 || st.TimeScale <= 0 || !w.buildingCanOverdriveLocked(pos) {
		return 1
	}
	if st.TimeScale < 1 {
		return 1
	}
	return st.TimeScale
}

func (w *World) scaledBuildingDeltaLocked(pos int32, deltaFrames, deltaSeconds float32) (float32, float32) {
	scale := w.buildingTimeScaleLocked(pos)
	return deltaFrames * scale, deltaSeconds * scale
}
