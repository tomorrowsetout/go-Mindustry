package world

import "mdt-server/internal/protocol"

const invalidEntityTilePos int32 = -1

func cloneEntityPlanConfig(config any) any {
	switch typed := config.(type) {
	case []byte:
		return append([]byte(nil), typed...)
	case *string:
		if typed == nil {
			return nil
		}
		copy := *typed
		return &copy
	default:
		return typed
	}
}

func cloneEntityBuildPlansFromProtocol(plans []*protocol.BuildPlan) []entityBuildPlan {
	if len(plans) == 0 {
		return nil
	}
	out := make([]entityBuildPlan, 0, len(plans))
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		blockID := int16(0)
		if !plan.Breaking && plan.Block != nil {
			blockID = plan.Block.ID()
		}
		out = append(out, entityBuildPlan{
			Breaking: plan.Breaking,
			Pos:      protocol.PackPoint2(plan.X, plan.Y),
			Rotation: plan.Rotation,
			BlockID:  blockID,
			Config:   cloneEntityPlanConfig(plan.Config),
		})
	}
	return out
}

func (w *World) SetEntityRuntimeState(id int32, shooting, boosting, updateBuilding bool, mineTilePos int32, plans []*protocol.BuildPlan) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.Shooting = shooting
		e.UpdateBuilding = updateBuilding
		if mineTilePos < 0 {
			e.MineTilePos = invalidEntityTilePos
		} else {
			e.MineTilePos = mineTilePos
		}
		e.Plans = cloneEntityBuildPlansFromProtocol(plans)
		switch {
		case isEntityFlying(*e):
			e.Elevation = 1
		case boosting:
			e.Elevation = 1
		default:
			e.Elevation = 0
		}
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityBuildState(id int32, updateBuilding bool, plans []*protocol.BuildPlan) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.UpdateBuilding = updateBuilding
		e.Plans = cloneEntityBuildPlansFromProtocol(plans)
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityStack(id int32, item ItemID, amount int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		if amount <= 0 {
			e.Stack = ItemStack{}
		} else {
			e.Stack = ItemStack{Item: item, Amount: amount}
		}
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityMineTile(id int32, mineTilePos int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		if mineTilePos < 0 {
			e.MineTilePos = invalidEntityTilePos
		} else {
			e.MineTilePos = mineTilePos
		}
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}

func (w *World) SetEntitySpawnedByCore(id int32, spawnedByCore bool) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.SpawnedByCore = spawnedByCore
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityFlag(id int32, flag float64) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.Flag = flag
		w.model.EntitiesRev++
		return cloneRawEntity(*e), true
	}
	return RawEntity{}, false
}
