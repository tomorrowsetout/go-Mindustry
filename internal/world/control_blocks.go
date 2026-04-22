package world

type controlledBuildState struct {
	PlayerID int32
	AimX     float32
	AimY     float32
	Shooting bool
}

func canControlSelectBuildingName(name string) bool {
	switch name {
	case "payload-conveyor", "reinforced-payload-conveyor",
		"payload-router", "reinforced-payload-router",
		"payload-void",
		"small-deconstructor", "deconstructor", "payload-deconstructor":
		return true
	}
	return isCoreBlockName(name) || isReconstructorBlockName(name)
}

func (w *World) canControlBuildingLocked(pos int32, tile *Tile) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	if tile == nil {
		tile = &w.model.Tiles[pos]
	}
	if tile.Block == 0 || tile.Build == nil || tile.Build.Health <= 0 || tile.Build.Team == 0 {
		return false
	}
	_, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	return ok
}

func (w *World) controlledBuildingInfoLocked(pos int32, tile *Tile) (BuildingInfo, bool) {
	if !w.canControlBuildingLocked(pos, tile) {
		return BuildingInfo{}, false
	}
	if tile == nil {
		tile = &w.model.Tiles[pos]
	}
	team := tile.Team
	if tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	return BuildingInfo{
		Pos:      packTilePos(tile.X, tile.Y),
		X:        int32(tile.X),
		Y:        int32(tile.Y),
		BlockID:  int16(tile.Block),
		Name:     w.blockNameByID(int16(tile.Block)),
		Team:     team,
		Rotation: tile.Rotation,
	}, true
}

func (w *World) clearControlledBuildingLocked(pos int32) {
	if w == nil {
		return
	}
	if st, ok := w.controlledBuilds[pos]; ok {
		delete(w.controlledBuilds, pos)
		if st.PlayerID != 0 {
			if cur, exists := w.controlledBuildByPlayer[st.PlayerID]; exists && cur == pos {
				delete(w.controlledBuildByPlayer, st.PlayerID)
			}
		}
	}
}

func (w *World) CanControlBuildingPacked(pos int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return false
	}
	return w.canControlBuildingLocked(index, nil)
}

func (w *World) CanControlSelectBuildingPacked(pos int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok || w.model == nil || index < 0 || int(index) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[index]
	if tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 || tile.Build.Health <= 0 {
		return false
	}
	return canControlSelectBuildingName(w.blockNameByID(int16(tile.Block)))
}

func (w *World) ClaimControlledBuildingPacked(playerID, pos int32) (BuildingInfo, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || playerID == 0 {
		return BuildingInfo{}, false
	}
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return BuildingInfo{}, false
	}
	tile := &w.model.Tiles[index]
	info, ok := w.controlledBuildingInfoLocked(index, tile)
	if !ok {
		return BuildingInfo{}, false
	}
	if st, exists := w.controlledBuilds[index]; exists && st.PlayerID != 0 && st.PlayerID != playerID {
		return BuildingInfo{}, false
	}
	if prev, exists := w.controlledBuildByPlayer[playerID]; exists && prev != index {
		w.clearControlledBuildingLocked(prev)
	}
	state := w.controlledBuilds[index]
	state.PlayerID = playerID
	if state.AimX == 0 && state.AimY == 0 {
		state.AimX = float32(tile.X*8 + 4)
		state.AimY = float32(tile.Y*8 + 4)
	}
	state.Shooting = false
	w.controlledBuilds[index] = state
	w.controlledBuildByPlayer[playerID] = index
	return info, true
}

func (w *World) ControlledBuildingInfoPacked(playerID, pos int32) (BuildingInfo, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || playerID == 0 {
		return BuildingInfo{}, false
	}
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return BuildingInfo{}, false
	}
	state, exists := w.controlledBuilds[index]
	if !exists || state.PlayerID != playerID {
		return BuildingInfo{}, false
	}
	tile := &w.model.Tiles[index]
	info, ok := w.controlledBuildingInfoLocked(index, tile)
	if !ok {
		w.clearControlledBuildingLocked(index)
		return BuildingInfo{}, false
	}
	return info, true
}

func (w *World) ReleaseControlledBuildingPacked(playerID, pos int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || playerID == 0 {
		return false
	}
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return false
	}
	state, exists := w.controlledBuilds[index]
	if !exists || state.PlayerID != playerID {
		return false
	}
	w.clearControlledBuildingLocked(index)
	return true
}

func (w *World) ReleaseControlledBuildingByPlayer(playerID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || playerID == 0 {
		return false
	}
	pos, ok := w.controlledBuildByPlayer[playerID]
	if !ok {
		return false
	}
	w.clearControlledBuildingLocked(pos)
	return true
}

func (w *World) SetControlledBuildingInputPacked(playerID, pos int32, aimX, aimY float32, shooting bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || playerID == 0 {
		return false
	}
	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return false
	}
	state, exists := w.controlledBuilds[index]
	if !exists || state.PlayerID != playerID {
		return false
	}
	tile := &w.model.Tiles[index]
	if !w.canControlBuildingLocked(index, tile) {
		w.clearControlledBuildingLocked(index)
		return false
	}
	if aimX == 0 && aimY == 0 {
		aimX = float32(tile.X*8 + 4)
		aimY = float32(tile.Y*8 + 4)
	}
	state.AimX = aimX
	state.AimY = aimY
	state.Shooting = shooting
	w.controlledBuilds[index] = state
	return true
}

func (w *World) controlledBuildStateLocked(pos int32) (controlledBuildState, bool) {
	if w == nil {
		return controlledBuildState{}, false
	}
	state, ok := w.controlledBuilds[pos]
	if !ok || state.PlayerID == 0 {
		return controlledBuildState{}, false
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return controlledBuildState{}, false
	}
	if !w.canControlBuildingLocked(pos, &w.model.Tiles[pos]) {
		w.clearControlledBuildingLocked(pos)
		return controlledBuildState{}, false
	}
	return state, true
}

func (w *World) controlledBuildingAimLocked(pos int32) (bool, bool, float32, float32) {
	state, ok := w.controlledBuildStateLocked(pos)
	if !ok {
		return false, false, 0, 0
	}
	return true, state.Shooting, state.AimX, state.AimY
}
