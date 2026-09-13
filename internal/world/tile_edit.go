package world

// Server-authoritative tile editing primitives.
//
// Java's Tile class exposes setFloorNet / setOverlayNet / setNet / removeNet
// which the server uses to mutate tiles (map editors, world processors) and
// then broadcasts Remote_Tile_setFloor_137 / setOverlay_138 / setTile_140 /
// removeTile_139 to clients. The Go world model previously had no runtime
// tile mutation path at all, so these methods provide that foundation.
//
// The methods only mutate tile data under the world lock. Broadcasting the
// corresponding sync packets is the caller's responsibility (the net layer
// owns the wire protocol). This keeps the world package free of protocol
// concerns and lets callers batch several edits into one broadcast.

// SetFloorAt replaces the floor (and overlay) of the tile at (x, y).
// It mirrors Tile.setFloorNet(floor, overlay). Server-side there is no
// rendering or pathfinding cache to invalidate, so the mutation is a plain
// field write. Returns false when the position is out of bounds or no model
// is loaded.
func (w *World) SetFloorAt(x, y int, floor FloorID, overlay OverlayID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(x, y) {
		return false
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	tile.Floor = floor
	tile.Overlay = overlay
	return true
}

// SetOverlayAt replaces only the overlay of the tile at (x, y), leaving the
// floor untouched. Mirrors Tile.setOverlayNet(overlay).
func (w *World) SetOverlayAt(x, y int, overlay OverlayID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(x, y) {
		return false
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	tile.Overlay = overlay
	return true
}

// SetBlockAt places a block on the tile at (x, y) with the given team and
// rotation. Mirrors Tile.setNet(block). Any existing building runtime state
// on the tile is dropped, because the replaced block no longer corresponds to
// the old building; transient per-block runtime maps are rebuilt lazily by
// the simulation as needed. Returns false when out of bounds or unloaded.
func (w *World) SetBlockAt(x, y int, block BlockID, team TeamID, rotation int8) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(x, y) {
		return false
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	tile.Block = block
	tile.Team = team
	tile.Rotation = rotation
	// The previous building (if any) belongs to the replaced block; drop it so
	// snapshot senders do not emit stale runtime state for the new block.
	tile.Build = nil
	return true
}

// RemoveTileAt clears the block on the tile at (x, y), resetting it to air.
// Mirrors Tile.removeNet(). The floor and overlay are preserved, matching
// vanilla semantics where removing a block does not touch the ground.
func (w *World) RemoveTileAt(x, y int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(x, y) {
		return false
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	tile.Block = 0
	tile.Team = 0
	tile.Rotation = 0
	tile.Build = nil
	return true
}

// TileSnapshotAt returns a copy of the tile data at (x, y) for read-only
// inspection without exposing the internal slice. Returns false when the
// position is invalid.
func (w *World) TileSnapshotAt(x, y int) (Tile, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || !w.model.InBounds(x, y) {
		return Tile{}, false
	}
	return w.model.Tiles[y*w.model.Width+x], true
}
