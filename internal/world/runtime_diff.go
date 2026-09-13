package world

import (
	"bytes"
	"math"
	"sort"
)

// RuntimeChangedBlockSyncPackedPositions returns center-tile packed positions
// whose structure still matches the provided base model, but whose visible
// runtime state differs enough to need a writeSync/blockSnapshot correction.
func (w *World) RuntimeChangedBlockSyncPackedPositions(base *WorldModel) []int32 {
	if w == nil || base == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 || len(base.Tiles) != len(w.model.Tiles) {
		return nil
	}
	out := make([]int32, 0, 64)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) || int(pos) >= len(base.Tiles) {
			continue
		}
		if w.blockSyncSuppressedLocked(pos) {
			continue
		}
		liveTile := &w.model.Tiles[pos]
		if liveTile == nil || !isCenterBuildingTile(liveTile) {
			continue
		}
		baseTile := &base.Tiles[pos]
		if baseTile == nil || !isCenterBuildingTile(baseTile) {
			continue
		}
		if liveTile.Block != baseTile.Block || liveTile.Team != baseTile.Team || liveTile.Rotation != baseTile.Rotation {
			continue
		}
		if buildingRuntimeStateEqual(baseTile.Build, liveTile.Build) {
			continue
		}
		out = append(out, packTilePos(liveTile.X, liveTile.Y))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func buildingRuntimeStateEqual(a, b *Building) bool {
	if a == nil || b == nil {
		return a == b
	}
	if math.Abs(float64(a.Health-b.Health)) >= 0.01 {
		return false
	}
	if math.Abs(float64(a.MaxHealth-b.MaxHealth)) >= 0.01 {
		return false
	}
	if !bytes.Equal(a.Config, b.Config) || !bytes.Equal(a.Payload, b.Payload) {
		return false
	}
	if !itemStacksEqual(a.Items, b.Items) {
		return false
	}
	if !liquidStacksEqual(a.Liquids, b.Liquids) {
		return false
	}
	return true
}

func itemStacksEqual(a, b []ItemStack) bool {
	am := compactItemStacks(a)
	bm := compactItemStacks(b)
	if len(am) != len(bm) {
		return false
	}
	for item, amount := range am {
		if bm[item] != amount {
			return false
		}
	}
	return true
}

func compactItemStacks(stacks []ItemStack) map[ItemID]int32 {
	out := make(map[ItemID]int32, len(stacks))
	for _, stack := range stacks {
		if stack.Amount == 0 {
			continue
		}
		out[stack.Item] += stack.Amount
		if out[stack.Item] == 0 {
			delete(out, stack.Item)
		}
	}
	return out
}

func liquidStacksEqual(a, b []LiquidStack) bool {
	am := compactLiquidStacks(a)
	bm := compactLiquidStacks(b)
	if len(am) != len(bm) {
		return false
	}
	for liquid, amount := range am {
		if math.Abs(float64(bm[liquid]-amount)) >= 0.001 {
			return false
		}
	}
	return true
}

func compactLiquidStacks(stacks []LiquidStack) map[LiquidID]float32 {
	out := make(map[LiquidID]float32, len(stacks))
	for _, stack := range stacks {
		if math.Abs(float64(stack.Amount)) < 0.001 {
			continue
		}
		out[stack.Liquid] += stack.Amount
		if math.Abs(float64(out[stack.Liquid])) < 0.001 {
			delete(out, stack.Liquid)
		}
	}
	return out
}
