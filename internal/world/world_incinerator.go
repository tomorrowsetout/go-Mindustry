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
	for _, pos := range w.incineratorTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		target := float32(0)
		switch name {
		case "incinerator":
			if w.requirePowerAtLocked(pos, tile.Build.Team, 0.5*deltaSeconds) {
				target = 1
			}
		case "slag-incinerator":
			if tile.Build.LiquidAmount(slagLiquidID) > 0.0001 {
				target = 1
			}
		default:
			continue
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
	if w == nil {
		return false
	}
	if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		if w.blockNameByID(int16(w.model.Tiles[pos].Block)) == "slag-incinerator" {
			return liquid == slagLiquidID
		}
	}
	if !w.incineratorAcceptsItemLocked(pos) {
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
	if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		if w.blockNameByID(int16(w.model.Tiles[pos].Block)) == "slag-incinerator" {
			return
		}
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
