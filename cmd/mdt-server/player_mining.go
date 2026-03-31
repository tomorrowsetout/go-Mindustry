package main

import (
	"strings"
	"time"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

const (
	invalidMiningTilePos int32   = -1
	playerMineRange      float32 = 70
	mineTransferRange    float32 = 220
)

type playerMiningState struct {
	unitID        int32
	targetPos     int32
	progress      float32
	carriedItem   world.ItemID
	carriedAmount int32
}

type playerMiningProfile struct {
	speed               float32
	tier                int
	capacity            int32
	mineFloor           bool
	mineWalls           bool
	mineHardnessScaling bool
}

type playerMiningService struct {
	states map[int32]*playerMiningState
}

func newPlayerMiningService() *playerMiningService {
	return &playerMiningService{states: map[int32]*playerMiningState{}}
}

func (m *playerMiningService) Tick(wld *world.World, srv *netserver.Server, delta time.Duration) {
	if m == nil || wld == nil || srv == nil || delta <= 0 {
		return
	}
	unitMineMul := wld.UnitMineSpeedMultiplier()
	if unitMineMul <= 0 {
		return
	}
	conns := srv.ListConnectedConns()
	active := make(map[int32]struct{}, len(conns))
	for _, c := range conns {
		if c == nil || c.PlayerID() == 0 {
			continue
		}
		connID := c.ConnID()
		active[connID] = struct{}{}
		state := m.state(connID)
		unitID := c.UnitID()
		if state.unitID != unitID {
			state.unitID = unitID
			state.targetPos = invalidMiningTilePos
			state.progress = 0
			state.carriedItem = 0
			state.carriedAmount = 0
			srv.SetConnUnitStack(c, 0, 0)
		}
		if unitID == 0 {
			continue
		}
		ent, ok := wld.GetEntity(unitID)
		if !ok || ent.Health <= 0 {
			continue
		}

		corePos, coreInRange := nearestCoreInRange(wld, ent.Team, ent.X, ent.Y, mineTransferRange)
		if coreInRange && state.carriedAmount > 0 {
			accepted := wld.AcceptItemAt(corePos, state.carriedItem, state.carriedAmount)
			if accepted > 0 {
				state.carriedAmount -= accepted
				if state.carriedAmount <= 0 {
					state.carriedAmount = 0
					state.carriedItem = 0
				}
				srv.SetConnUnitStack(c, int16(state.carriedItem), state.carriedAmount)
			}
		}

		minePos, mining := c.MiningTilePos()
		if !mining {
			state.targetPos = invalidMiningTilePos
			state.progress = 0
			continue
		}
		profile := miningProfileForUnitType(ent.TypeID, wld.UnitNameByTypeID(ent.TypeID))
		if profile.speed <= 0 || profile.capacity <= 0 || profile.tier < 0 {
			state.targetPos = invalidMiningTilePos
			state.progress = 0
			continue
		}
		result, ok := wld.ResolveMineTile(minePos, profile.mineFloor, profile.mineWalls)
		if !ok || result.Hardness > profile.tier || !withinWorldRange(ent.X, ent.Y, result.WorldX, result.WorldY, playerMineRange) {
			state.targetPos = invalidMiningTilePos
			state.progress = 0
			continue
		}
		if state.targetPos != minePos {
			state.targetPos = minePos
			state.progress = 0
		}
		if !canCarryMinedItem(state, result.Item, profile.capacity) && !coreInRange {
			state.progress = 0
			continue
		}

		threshold := float32(65)
		if profile.mineHardnessScaling {
			threshold = 50 + float32(result.Hardness)*15
		}
		state.progress += float32(delta.Seconds()) * 60 * profile.speed * unitMineMul

		stackChanged := false
		for state.progress >= threshold {
			state.progress -= threshold
			if coreInRange && wld.AcceptItemAt(corePos, result.Item, 1) == 1 {
				continue
			}
			if !canCarryMinedItem(state, result.Item, profile.capacity) {
				state.progress = 0
				break
			}
			if state.carriedAmount == 0 {
				state.carriedItem = result.Item
			}
			state.carriedAmount++
			stackChanged = true
		}
		if stackChanged {
			srv.SetConnUnitStack(c, int16(state.carriedItem), state.carriedAmount)
		}
	}
	for connID := range m.states {
		if _, ok := active[connID]; !ok {
			delete(m.states, connID)
		}
	}
}

func (m *playerMiningService) state(connID int32) *playerMiningState {
	if st, ok := m.states[connID]; ok && st != nil {
		return st
	}
	st := &playerMiningState{targetPos: invalidMiningTilePos}
	m.states[connID] = st
	return st
}

func canCarryMinedItem(state *playerMiningState, item world.ItemID, capacity int32) bool {
	if state == nil || capacity <= 0 {
		return false
	}
	if state.carriedAmount <= 0 {
		return true
	}
	if state.carriedItem != item {
		return false
	}
	return state.carriedAmount < capacity
}

func nearestCoreInRange(wld *world.World, team world.TeamID, x, y, r float32) (int32, bool) {
	if wld == nil || team == 0 {
		return 0, false
	}
	bestPos := int32(0)
	bestDist := float32(-1)
	for _, packed := range wld.TeamCorePositions(team) {
		pt := protocol.UnpackPoint2(packed)
		cx := float32(pt.X*8 + 4)
		cy := float32(pt.Y*8 + 4)
		dist := squaredWorldDistance(x, y, cx, cy)
		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			bestPos = packed
		}
	}
	if bestDist < 0 || bestDist > r*r {
		return 0, false
	}
	return bestPos, true
}

func withinWorldRange(ax, ay, bx, by, r float32) bool {
	return squaredWorldDistance(ax, ay, bx, by) <= r*r
}

func squaredWorldDistance(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func miningProfileForUnitType(typeID int16, name string) playerMiningProfile {
	switch normalizeMiningUnitName(name, typeID) {
	case "alpha":
		return playerMiningProfile{speed: 6.5, tier: 1, capacity: 30, mineFloor: true, mineWalls: false, mineHardnessScaling: true}
	case "beta":
		return playerMiningProfile{speed: 7, tier: 1, capacity: 50, mineFloor: true, mineWalls: false, mineHardnessScaling: true}
	case "gamma":
		return playerMiningProfile{speed: 8, tier: 2, capacity: 70, mineFloor: true, mineWalls: false, mineHardnessScaling: true}
	case "evoke":
		return playerMiningProfile{speed: 6, tier: 3, capacity: 60, mineFloor: false, mineWalls: true, mineHardnessScaling: true}
	case "incite":
		return playerMiningProfile{speed: 8, tier: 3, capacity: 90, mineFloor: false, mineWalls: true, mineHardnessScaling: true}
	case "emanate":
		return playerMiningProfile{speed: 9, tier: 3, capacity: 110, mineFloor: false, mineWalls: true, mineHardnessScaling: true}
	default:
		return playerMiningProfile{}
	}
}

func normalizeMiningUnitName(name string, typeID int16) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name != "" {
		return name
	}
	switch typeID {
	case 35:
		return "alpha"
	case 36:
		return "beta"
	case 37:
		return "gamma"
	case 53:
		return "evoke"
	case 54:
		return "incite"
	case 55:
		return "emanate"
	default:
		return ""
	}
}
