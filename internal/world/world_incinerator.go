package world

import (
	"math/rand"
	"time"
)

func (w *World) stepIncinerators(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 || w.blockNameByID(int16(tile.Block)) != "incinerator" {
			continue
		}
		target := float32(0)
		if w.requirePowerAtLocked(pos, tile.Build.Team, 0.5*deltaSeconds) {
			target = 1
		}
		w.incineratorStates[pos] = approachf(w.incineratorStates[pos], target, 0.04*deltaFrames)
	}
}

func (w *World) incineratorAcceptsItemLocked(pos int32) bool {
	if w == nil {
		return false
	}
	return w.incineratorStates[pos] > 0.5
}

func (w *World) incineratorAcceptsLiquidLocked(pos int32, liquid LiquidID) bool {
	if w == nil || !w.incineratorAcceptsItemLocked(pos) {
		return false
	}
	return liquidIncinerable(liquid)
}

func (w *World) incineratorBurnItemLocked(pos int32) {
	if !w.incineratorAcceptsItemLocked(pos) {
		return
	}
	if rand.Float32() < 0.3 && w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		tile := &w.model.Tiles[pos]
		w.emitEffectLocked("fuelburn", float32(tile.X*8+4), float32(tile.Y*8+4), 0)
	}
}

func (w *World) incineratorBurnLiquidLocked(pos int32) {
	if !w.incineratorAcceptsItemLocked(pos) {
		return
	}
	if rand.Float32() < 0.02 && w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		tile := &w.model.Tiles[pos]
		w.emitEffectLocked("fuelburn", float32(tile.X*8+4), float32(tile.Y*8+4), 0)
	}
}

func liquidIncinerable(liquid LiquidID) bool {
	switch liquid {
	default:
		return true
	}
}
