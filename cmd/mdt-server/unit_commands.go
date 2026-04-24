package main

import (
	"strings"
	"sync"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

const (
	moveCommandID            int16   = 0
	repairCommandID          int16   = 1
	rebuildCommandID         int16   = 2
	assistCommandID          int16   = 3
	mineCommandID            int16   = 4
	stopUnitStanceID         int16   = 0
	holdPositionStanceID     int16   = 6
	mineAutoStanceID         int16   = 7
	defaultCommandMoveSpeed  float32 = 18
	builderCommandRange      float32 = 220
	commandAssistBuildRadius float32 = 1500
	commandMineRange         float32 = 70
	commandMineTransferRange float32 = 220
	commandRepairRange       float32 = 120
	commandAssistFollowRange float32 = 72
	rebuildRetryTicksDefault uint64  = 120
	rebuildRetryTicksBuildAI uint64  = 10
	maxQueuedUnitCommands            = 16

	// 修复：添加超时和卡住检测常量
	commandTimeoutTicks    uint64  = 300 // 5分钟无响应则取消命令
	stuckDistanceThreshold float32 = 2.0 // 单位移动距离小于这个值持续过久，视为卡住
	stuckDetectionTicks    uint64  = 60  // 每分钟检查一次是否卡住
	maxStuckTicks          uint64  = 30  // 卡住超过30秒则重置
)

type unitCommandService struct {
	mu      sync.RWMutex
	byUnit  map[int32]*protocol.ControllerState
	runtime map[int32]unitCommandRuntime
}

type unitCommandTargetSpec struct {
	hasAttack  bool
	attack     protocol.CommandTarget
	hasPos     bool
	pos        protocol.Vec2
	queue      protocol.CommandTarget
	hasQueue   bool
	followUnit int32
	worldPos   protocol.Vec2
	worldMode  string
}

type unitCommandRuntime struct {
	rebuildRetryTick uint64
	// 修复：添加命令执行时间和超时检测
	lastActionTick uint64
	stuckCheckTick uint64
	lastX, lastY   float32
	stuckTicks     int
}

func newUnitCommandService() *unitCommandService {
	return &unitCommandService{
		byUnit:  make(map[int32]*protocol.ControllerState),
		runtime: make(map[int32]unitCommandRuntime),
	}
}

func cloneControllerStateDeep(src *protocol.ControllerState) *protocol.ControllerState {
	if src == nil {
		return nil
	}
	out := *src
	out.Command.Queue = append([]protocol.CommandTarget(nil), src.Command.Queue...)
	out.Command.Stances = append([]protocol.UnitStance(nil), src.Command.Stances...)
	return &out
}

func (s *unitCommandService) ensureLocked(unitID int32) *protocol.ControllerState {
	state, ok := s.byUnit[unitID]
	if !ok || state == nil {
		state = &protocol.ControllerState{
			Type: protocol.ControllerCommand9,
		}
		state.Command.CommandID = int8(moveCommandID)
		s.byUnit[unitID] = state
	}
	if state.Type != protocol.ControllerCommand9 {
		state.Type = protocol.ControllerCommand9
	}
	if state.Command.CommandID < 0 {
		state.Command.CommandID = int8(moveCommandID)
	}
	// 修复：初始化运行时状态
	if _, ok := s.runtime[unitID]; !ok {
		s.runtime[unitID] = unitCommandRuntime{
			lastActionTick: 0,
			stuckCheckTick: 0,
			stuckTicks:     0,
		}
	}
	return state
}

func (s *unitCommandService) remove(unitID int32) {
	if s == nil || unitID == 0 {
		return
	}
	s.mu.Lock()
	delete(s.byUnit, unitID)
	delete(s.runtime, unitID)
	s.mu.Unlock()
}

// 修复：改进的超时检测和卡住检测
func (s *unitCommandService) checkAndUpdateRuntime(tick uint64, unitID int32, x, y float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runtime, ok := s.runtime[unitID]
	if !ok {
		return
	}

	// 检查超时
	if tick > runtime.lastActionTick+commandTimeoutTicks {
		delete(s.byUnit, unitID)
		delete(s.runtime, unitID)
		return
	}

	// 检查卡住状态
	if tick > runtime.stuckCheckTick+stuckDetectionTicks {
		distance := (x-runtime.lastX)*(x-runtime.lastX) + (y-runtime.lastY)*(y-runtime.lastY)
		if distance < stuckDistanceThreshold*stuckDistanceThreshold {
			runtime.stuckTicks++
			if uint64(runtime.stuckTicks) > maxStuckTicks {
				// 单位卡住，重新设置为空闲
				if state, ok := s.byUnit[unitID]; ok {
					state.Command.HasAttack = false
					state.Command.HasPos = false
					state.Command.Queue = nil
					runtime.stuckTicks = 0
				}
			}
		} else {
			runtime.stuckTicks = 0
		}
		runtime.stuckCheckTick = tick
	}

	runtime.lastX = x
	runtime.lastY = y
}

func (s *unitCommandService) overlay(units []protocol.UnitSyncEntity) []protocol.UnitSyncEntity {
	if s == nil || len(units) == 0 {
		return units
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]protocol.UnitSyncEntity, 0, len(units))
	for _, ent := range units {
		unit, ok := ent.(*protocol.UnitEntitySync)
		if !ok || unit == nil {
			out = append(out, ent)
			continue
		}
		state, ok := s.byUnit[unit.ID()]
		if !ok || state == nil {
			out = append(out, ent)
			continue
		}
		copy := *unit
		copy.Controller = cloneControllerStateDeep(state)
		out = append(out, &copy)
	}
	return out
}

func (s *unitCommandService) step(wld *world.World) {
	if s == nil || wld == nil {
		return
	}
	tick := wld.Snapshot().Tick
	defaultMineUnits := discoverDefaultMineCommandUnits(wld)
	defaultRebuildUnits := discoverDefaultRebuildCommandUnits(wld)
	defaultRepairUnits := discoverDefaultRepairCommandUnits(wld)
	s.mu.Lock()
	for _, unitID := range defaultMineUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(mineCommandID)
		ensureMineAutoStance(state)
	}
	for _, unitID := range defaultRebuildUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(rebuildCommandID)
	}
	for _, unitID := range defaultRepairUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(repairCommandID)
	}
	defer s.mu.Unlock()
	for unitID, state := range s.byUnit {
		if state == nil {
			delete(s.byUnit, unitID)
			delete(s.runtime, unitID)
			continue
		}
		entity, ok := wld.GetEntity(unitID)
		if !ok {
			delete(s.byUnit, unitID)
			delete(s.runtime, unitID)
			continue
		}
		if entity.PlayerID != 0 {
			continue
		}
		if state.Command.CommandID == int8(mineCommandID) {
			ensureMineAutoStance(state)
			stepMineCommand(wld, unitID, entity, state)
			continue
		}
		if state.Command.CommandID == int8(repairCommandID) {
			stepRepairCommand(wld, unitID, entity, state)
			continue
		}
		if state.Command.CommandID == int8(rebuildCommandID) {
			runtime := s.runtime[unitID]
			stepRebuildCommand(wld, unitID, entity, state, tick, &runtime)
			s.runtime[unitID] = runtime
			continue
		}
		if state.Command.CommandID == int8(assistCommandID) {
			stepAssistCommand(wld, unitID, entity, state)
			continue
		}
		if entity.Behavior != "" && entity.Behavior != "command" {
			continue
		}
		if len(state.Command.Queue) == 0 {
			if state.Command.HasPos {
				state.Command.HasPos = false
				state.Command.TargetPos = protocol.Vec2{}
			}
			if entity.Behavior == "" {
				_, _ = wld.SetEntityCommandIdle(unitID)
			}
			continue
		}
		spec, ok := queuedUnitCommandTargetSpec(wld, state.Command.Queue[0])
		state.Command.Queue = state.Command.Queue[1:]
		if !ok {
			if entity.Behavior == "" {
				_, _ = wld.SetEntityCommandIdle(unitID)
			}
			continue
		}
		applyActiveCommandTarget(state, spec)
		speed := entity.MoveSpeed
		if speed <= 0 {
			speed = defaultCommandMoveSpeed
		}
		startWorldUnitCommand(wld, unitID, spec, speed)
	}
}

func (s *unitCommandService) applySetUnitCommand(c *netserver.Conn, wld *world.World, unitIDs []int32, command *protocol.UnitCommand) {
	if s == nil || c == nil || wld == nil || len(unitIDs) == 0 || command == nil {
		return
	}
	team := world.TeamID(c.TeamID())
	updated := make([]int32, 0, len(unitIDs))
	s.mu.Lock()
	for _, unitID := range unitIDs {
		entity, ok := wld.GetEntity(unitID)
		if !ok || entity.Team != team {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(command.ID)
		state.Command.HasAttack = false
		state.Command.HasPos = false
		state.Command.Target = protocol.CommandTarget{}
		state.Command.TargetPos = protocol.Vec2{}
		state.Command.Queue = nil
		if command.ID == mineCommandID {
			ensureMineAutoStance(state)
		}
		updated = append(updated, unitID)
	}
	s.mu.Unlock()
	for _, unitID := range updated {
		s.mu.Lock()
		s.runtime[unitID] = unitCommandRuntime{}
		s.mu.Unlock()
		clearUnitBuilderCommandState(wld, unitID, team)
		_, _ = wld.SetEntityMineTile(unitID, -1)
		_, _ = wld.SetEntityCommandIdle(unitID)
	}
}

func (s *unitCommandService) applySetUnitStance(c *netserver.Conn, wld *world.World, unitIDs []int32, stance protocol.UnitStance, enable bool) {
	if s == nil || c == nil || wld == nil || len(unitIDs) == 0 {
		return
	}
	team := world.TeamID(c.TeamID())
	for _, unitID := range unitIDs {
		entity, ok := wld.GetEntity(unitID)
		if !ok || entity.Team != team {
			continue
		}
		if stance.ID == stopUnitStanceID {
			_, _ = wld.ClearEntityBehavior(unitID)
			s.mu.Lock()
			state := s.ensureLocked(unitID)
			state.Command.HasAttack = false
			state.Command.HasPos = false
			state.Command.Target = protocol.CommandTarget{}
			state.Command.TargetPos = protocol.Vec2{}
			state.Command.Queue = nil
			s.mu.Unlock()
			continue
		}

		s.mu.Lock()
		state := s.ensureLocked(unitID)
		if enable {
			if stance.ID == mineAutoStanceID {
				filtered := state.Command.Stances[:0]
				for _, current := range state.Command.Stances {
					if _, itemStance := stanceItemID(current); itemStance {
						continue
					}
					if current.ID == mineAutoStanceID {
						continue
					}
					filtered = append(filtered, current)
				}
				state.Command.Stances = filtered
			} else if _, itemStance := stanceItemID(stance); itemStance {
				filtered := state.Command.Stances[:0]
				for _, current := range state.Command.Stances {
					if current.ID == mineAutoStanceID {
						continue
					}
					filtered = append(filtered, current)
				}
				state.Command.Stances = filtered
			}
			found := false
			for _, current := range state.Command.Stances {
				if current.ID == stance.ID {
					found = true
					break
				}
			}
			if !found {
				state.Command.Stances = append(state.Command.Stances, stance)
			}
		} else {
			filtered := state.Command.Stances[:0]
			for _, current := range state.Command.Stances {
				if current.ID != stance.ID {
					filtered = append(filtered, current)
				}
			}
			state.Command.Stances = filtered
		}
		s.mu.Unlock()
	}
}

func (s *unitCommandService) applyCommandUnits(c *netserver.Conn, wld *world.World, unitIDs []int32, buildTarget any, unitTarget any, posTarget any, queueCommand bool) {
	if s == nil || c == nil || wld == nil || len(unitIDs) == 0 {
		return
	}
	team := world.TeamID(c.TeamID())
	spec, ok := resolveUnitCommandTarget(wld, team, buildTarget, unitTarget, posTarget)
	if !ok {
		return
	}
	for _, unitID := range unitIDs {
		entity, ok := wld.GetEntity(unitID)
		if !ok || entity.Team != team {
			continue
		}
		speed := entity.MoveSpeed
		if speed <= 0 {
			speed = defaultCommandMoveSpeed
		}

		activeNow := false
		s.mu.Lock()
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(moveCommandID)
		_, currentIsItemStance := activeMineTargetItem(state)
		if currentIsItemStance || hasMineAutoStance(state) {
			filtered := state.Command.Stances[:0]
			for _, stance := range state.Command.Stances {
				if stance.ID == mineAutoStanceID {
					continue
				}
				if _, itemStance := stanceItemID(stance); itemStance {
					continue
				}
				filtered = append(filtered, stance)
			}
			state.Command.Stances = filtered
		}
		if !queueCommand {
			state.Command.Queue = nil
			applyActiveCommandTarget(state, spec)
			activeNow = true
		} else if !state.Command.HasAttack && !state.Command.HasPos {
			applyActiveCommandTarget(state, spec)
			activeNow = true
		} else if spec.hasQueue && len(state.Command.Queue) < maxQueuedUnitCommands {
			state.Command.Queue = append(state.Command.Queue, spec.queue)
		}
		s.mu.Unlock()
		s.mu.Lock()
		s.runtime[unitID] = unitCommandRuntime{}
		s.mu.Unlock()
		clearUnitBuilderCommandState(wld, unitID, team)
		_, _ = wld.SetEntityMineTile(unitID, -1)

		if activeNow {
			startWorldUnitCommand(wld, unitID, spec, speed)
		} else if entity.Behavior == "" {
			_, _ = wld.SetEntityCommandIdle(unitID)
		}
	}
}

func applyActiveCommandTarget(state *protocol.ControllerState, spec unitCommandTargetSpec) {
	if state == nil {
		return
	}
	if spec.hasAttack {
		state.Command.HasAttack = true
		state.Command.Target = spec.attack
		state.Command.HasPos = false
		state.Command.TargetPos = protocol.Vec2{}
	} else {
		state.Command.HasAttack = false
		state.Command.Target = protocol.CommandTarget{}
		state.Command.HasPos = spec.hasPos
		state.Command.TargetPos = spec.pos
	}
}

func startWorldUnitCommand(wld *world.World, unitID int32, spec unitCommandTargetSpec, speed float32) {
	if wld == nil || unitID == 0 {
		return
	}
	switch spec.worldMode {
	case "follow":
		_, _ = wld.SetEntityFollow(unitID, spec.followUnit, speed)
	case "move":
		_, _ = wld.SetEntityMoveTo(unitID, spec.worldPos.X, spec.worldPos.Y, speed)
	default:
		_, _ = wld.SetEntityCommandIdle(unitID)
	}
}

func resolveUnitCommandTarget(wld *world.World, team world.TeamID, buildTarget any, unitTarget any, posTarget any) (unitCommandTargetSpec, bool) {
	if buildPos, ok := extractBuildingPos(buildTarget); ok {
		if info, exists := wld.BuildingInfoPacked(buildPos); exists {
			if info.Team != 0 && info.Team != team {
				worldPos := protocol.Vec2{X: float32(info.X*8 + 4), Y: float32(info.Y*8 + 4)}
				return unitCommandTargetSpec{
					hasAttack: true,
					attack:    protocol.CommandTarget{Type: 1, Pos: buildPos},
					hasQueue:  true,
					queue:     protocol.CommandTarget{Type: 0, Pos: buildPos},
					worldPos:  worldPos,
					worldMode: "move",
				}, true
			}
		}
	}

	if targetUnitID, ok := extractUnitID(unitTarget); ok {
		if target, exists := wld.GetEntity(targetUnitID); exists && target.Team != 0 && target.Team != team {
			return unitCommandTargetSpec{
				hasAttack:  true,
				attack:     protocol.CommandTarget{Type: 0, Pos: targetUnitID},
				hasQueue:   true,
				queue:      protocol.CommandTarget{Type: 1, Pos: targetUnitID},
				followUnit: targetUnitID,
				worldMode:  "follow",
			}, true
		}
	}

	if vec, ok := extractVec2(posTarget); ok {
		return unitCommandTargetSpec{
			hasPos:    true,
			pos:       vec,
			hasQueue:  true,
			queue:     protocol.CommandTarget{Type: 2, Vec: vec},
			worldPos:  vec,
			worldMode: "move",
		}, true
	}

	return unitCommandTargetSpec{}, false
}

func queuedUnitCommandTargetSpec(wld *world.World, target protocol.CommandTarget) (unitCommandTargetSpec, bool) {
	switch target.Type {
	case 0:
		if target.Pos < 0 {
			return unitCommandTargetSpec{}, false
		}
		packed := protocol.UnpackPoint2(target.Pos)
		worldPos := protocol.Vec2{X: float32(packed.X*8 + 4), Y: float32(packed.Y*8 + 4)}
		return unitCommandTargetSpec{
			hasAttack: true,
			attack:    protocol.CommandTarget{Type: 1, Pos: target.Pos},
			hasQueue:  true,
			queue:     protocol.CommandTarget{Type: 0, Pos: target.Pos},
			worldPos:  worldPos,
			worldMode: "move",
		}, true
	case 1:
		return unitCommandTargetSpec{
			hasAttack:  true,
			attack:     protocol.CommandTarget{Type: 0, Pos: target.Pos},
			hasQueue:   true,
			queue:      protocol.CommandTarget{Type: 1, Pos: target.Pos},
			followUnit: target.Pos,
			worldMode:  "follow",
		}, true
	case 2:
		return unitCommandTargetSpec{
			hasPos:    true,
			pos:       target.Vec,
			hasQueue:  true,
			queue:     protocol.CommandTarget{Type: 2, Vec: target.Vec},
			worldPos:  target.Vec,
			worldMode: "move",
		}, true
	default:
		return unitCommandTargetSpec{}, false
	}
}

func discoverDefaultMineCommandUnits(wld *world.World) []int32 {
	return discoverDefaultCommandUnitsByName(wld, "mono", 20)
}

func discoverDefaultRebuildCommandUnits(wld *world.World) []int32 {
	return discoverDefaultCommandUnitsByName(wld, "poly", 21)
}

func discoverDefaultRepairCommandUnits(wld *world.World) []int32 {
	return discoverDefaultCommandUnitsByName(wld, "mega", 22)
}

func discoverDefaultCommandUnitsByName(wld *world.World, unitName string, fallbackTypeID int16) []int32 {
	if wld == nil {
		return nil
	}
	typeID, ok := wld.ResolveUnitTypeID(unitName)
	if !ok {
		typeID = fallbackTypeID
	}
	waveTeam := inferWaveTeamID(wld.GetRulesManager().Get())
	return wld.UnitIDsByType(typeID, waveTeam)
}

func inferWaveTeamID(rules *world.Rules) world.TeamID {
	if rules == nil {
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(rules.WaveTeam)) {
	case "", "crux":
		return 2
	case "derelict":
		return 0
	case "sharded":
		return 1
	case "malis":
		return 3
	case "green":
		return 4
	case "blue":
		return 5
	default:
		return 2
	}
}

func stepMineCommand(wld *world.World, unitID int32, entity world.RawEntity, state *protocol.ControllerState) {
	if wld == nil || state == nil || unitID == 0 || entity.Team == 0 {
		return
	}
	profile, ok := commandUnitMiningProfile(entity)
	if !ok {
		_, _ = wld.SetEntityMineTile(unitID, -1)
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	corePos, coreX, coreY, coreOK := nearestFriendlyCore(wld, entity.Team, entity.X, entity.Y)
	if !coreOK {
		_, _ = wld.SetEntityMineTile(unitID, -1)
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}

	targetItem, orePos, oreX, oreY, hasTarget := selectMineTarget(wld, entity, profile, state, coreX, coreY)
	shouldOffload := entity.Stack.Amount > 0 && (!hasTarget || entity.Stack.Amount >= profile.Capacity || entity.Stack.Item != targetItem)
	if shouldOffload {
		driveUnitToCoreForOffload(wld, entity, corePos, coreX, coreY)
		return
	}
	if !hasTarget {
		_, _ = wld.SetEntityMineTile(unitID, -1)
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}

	_, _ = wld.SetEntityMineTile(unitID, orePos)
	if squaredWorldDistance(entity.X, entity.Y, oreX, oreY) <= commandMineRange*commandMineRange {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, oreX, oreY)
}

func stepRepairCommand(wld *world.World, unitID int32, entity world.RawEntity, state *protocol.ControllerState) {
	if wld == nil || unitID == 0 || entity.Team == 0 {
		return
	}
	hold := hasHoldPositionStance(state)
	clearUnitBuilderCommandState(wld, unitID, entity.Team)
	target, ok := wld.FindNearestDamagedFriendlyBuilding(entity.Team, entity.X, entity.Y)
	if !ok {
		if hold {
			_, _ = wld.SetEntityMineTile(unitID, -1)
			setUnitCommandIdleIfNeeded(wld, entity)
			return
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}
	rangeLimit := repairCommandMoveRange(entity)
	if squaredWorldDistance(entity.X, entity.Y, target.X, target.Y) <= rangeLimit*rangeLimit {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	if hold {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, target.X, target.Y)
}

func stepRebuildCommand(wld *world.World, unitID int32, entity world.RawEntity, state *protocol.ControllerState, tick uint64, runtime *unitCommandRuntime) {
	if wld == nil || unitID == 0 || entity.Team == 0 {
		return
	}
	hold := hasHoldPositionStance(state)
	ignoreRange := builderCommandIgnoresRange(wld, entity.Team)
	retryTicks := rebuildCommandRetryTicks(wld)
	if !commandUnitCanBuild(entity) {
		clearUnitBuilderCommandState(wld, unitID, entity.Team)
		if runtime != nil {
			runtime.rebuildRetryTick = tick + retryTicks
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}

	if op, ok := primaryEntityBuildPlan(entity); ok && !op.Breaking {
		if hold && !ignoreRange && !builderPlanWithinRange(entity, op, builderCommandRange) {
			clearUnitBuilderCommandState(wld, unitID, entity.Team)
		} else {
			switch wld.EvaluateBuildPlanPlacement(entity.Team, op) {
			case world.BuildPlanPlacementReady:
				applyUnitCommandBuildPlan(wld, unitID, entity, op, !hold)
				return
			case world.BuildPlanPlacementBuilt:
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				if runtime != nil {
					runtime.rebuildRetryTick = tick + retryTicks
				}
				setUnitMineIdleAndRetreatIfNeeded(wld, entity)
				return
			default:
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				if runtime != nil {
					runtime.rebuildRetryTick = tick + retryTicks
				}
				setUnitMineIdleAndRetreatIfNeeded(wld, entity)
				return
			}
		}
	}

	if runtime != nil && tick < runtime.rebuildRetryTick {
		clearUnitBuilderCommandState(wld, unitID, entity.Team)
		if hold {
			_, _ = wld.SetEntityMineTile(unitID, -1)
			setUnitCommandIdleIfNeeded(wld, entity)
			return
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}

	if leader, ok := wld.FindNearestAssistConstructBuilder(entity.Team, entity.ID, entity.X, entity.Y, builderCommandRange, unitCommandMoveSpeed(entity), commandAssistBuildRadius); ok {
		if plan, planOK := primaryEntityBuildPlan(leader); planOK {
			if hold && !ignoreRange && !builderPlanWithinRange(entity, plan, builderCommandRange) {
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				setUnitCommandIdleIfNeeded(wld, entity)
				return
			}
			applyUnitCommandBuildPlan(wld, unitID, entity, plan, !hold)
			return
		}
	}

	var (
		op world.BuildPlanOp
		ok bool
	)
	if hold && !ignoreRange {
		op, ok = wld.AcquireNextRebuildPlanInRange(entity.Team, entity.X, entity.Y, builderCommandRange)
	} else {
		op, ok = wld.AcquireNextRebuildPlan(entity.Team)
	}
	if !ok || wld.EvaluateBuildPlanPlacement(entity.Team, op) != world.BuildPlanPlacementReady {
		clearUnitBuilderCommandState(wld, unitID, entity.Team)
		if runtime != nil {
			runtime.rebuildRetryTick = tick + retryTicks
		}
		if hold {
			_, _ = wld.SetEntityMineTile(unitID, -1)
			setUnitCommandIdleIfNeeded(wld, entity)
			return
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}
	if runtime != nil {
		runtime.rebuildRetryTick = tick + retryTicks
	}
	applyUnitCommandBuildPlan(wld, unitID, entity, op, !hold)
}

func stepAssistCommand(wld *world.World, unitID int32, entity world.RawEntity, state *protocol.ControllerState) {
	if wld == nil || unitID == 0 || entity.Team == 0 {
		return
	}
	hold := hasHoldPositionStance(state)
	if !commandUnitCanBuild(entity) {
		clearUnitBuilderCommandState(wld, unitID, entity.Team)
		if hold {
			_, _ = wld.SetEntityMineTile(unitID, -1)
			setUnitCommandIdleIfNeeded(wld, entity)
			return
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}
	assistFollowing, assistFollowingOK := selectAssistFollowing(wld, entity, state)
	leader, ok := selectAssistConstructLeader(wld, entity, assistFollowing, assistFollowingOK)
	if ok {
		plan, planOK := primaryEntityBuildPlan(leader)
		if !planOK {
			ok = false
		} else {
			if hold && !builderCommandIgnoresRange(wld, entity.Team) && !builderPlanWithinRange(entity, plan, builderCommandRange) {
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				setUnitCommandIdleIfNeeded(wld, entity)
				return
			}
			applyUnitCommandBuildPlan(wld, unitID, entity, plan, !hold)
			return
		}
	}
	clearUnitBuilderCommandState(wld, unitID, entity.Team)
	_, _ = wld.SetEntityBuildState(unitID, false, nil)
	if hold {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	if !assistFollowingOK {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	if squaredWorldDistance(entity.X, entity.Y, assistFollowing.X, assistFollowing.Y) <= commandAssistFollowRange*commandAssistFollowRange {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, assistFollowing.X, assistFollowing.Y)
}

func selectMineTarget(wld *world.World, entity world.RawEntity, profile world.UnitMiningProfile, state *protocol.ControllerState, coreX, coreY float32) (world.ItemID, int32, float32, float32, bool) {
	items := desiredMineItems(state)
	if len(items) == 0 {
		return 0, 0, 0, 0, false
	}
	coreItems := wld.TeamItems(entity.Team)
	bestItem := world.ItemID(0)
	bestPos := int32(-1)
	bestX, bestY := float32(0), float32(0)
	bestCount := int32(0)
	bestDist := float32(0)
	for _, item := range items {
		pos, wx, wy, ok := wld.FindClosestMineTileForItem(coreX, coreY, item, profile.MineFloor, profile.MineWalls, profile.Tier)
		if !ok {
			continue
		}
		count := coreItems[item]
		dist := squaredWorldDistance(coreX, coreY, wx, wy)
		if bestPos < 0 || count < bestCount || (count == bestCount && dist < bestDist) {
			bestItem = item
			bestPos = pos
			bestX = wx
			bestY = wy
			bestCount = count
			bestDist = dist
		}
	}
	if bestPos < 0 {
		return 0, 0, 0, 0, false
	}
	return bestItem, bestPos, bestX, bestY, true
}

func desiredMineItems(state *protocol.ControllerState) []world.ItemID {
	if state == nil {
		return defaultMineAutoItems()
	}
	items := make([]world.ItemID, 0, len(state.Command.Stances))
	for _, stance := range state.Command.Stances {
		if item, ok := stanceItemID(stance); ok {
			items = append(items, item)
		}
	}
	if len(items) > 0 && !hasMineAutoStance(state) {
		return items
	}
	return defaultMineAutoItems()
}

func clearUnitBuilderCommandState(wld *world.World, unitID int32, team world.TeamID) {
	if wld == nil || unitID == 0 {
		return
	}
	wld.ClearBuilderState(unitID)
	_ = wld.ApplyBuildPlanSnapshotForOwner(unitID, team, nil)
	_, _ = wld.SetEntityBuildState(unitID, false, nil)
}

func commandUnitCanBuild(entity world.RawEntity) bool {
	return entity.BuildSpeed > 0
}

func applyUnitCommandBuildPlan(wld *world.World, unitID int32, entity world.RawEntity, op world.BuildPlanOp, allowMove bool) {
	if wld == nil || unitID == 0 {
		return
	}
	_ = wld.ApplyBuildPlanSnapshotForOwner(unitID, entity.Team, []world.BuildPlanOp{op})
	wld.UpdateBuilderState(unitID, entity.Team, unitID, entity.X, entity.Y, true, builderCommandRange)
	_, _ = wld.SetEntityBuildState(unitID, true, []*protocol.BuildPlan{buildPlanFromOp(op)})
	if !allowMove {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	targetX := float32(op.X*8 + 4)
	targetY := float32(op.Y*8 + 4)
	stopRadius := maxFloat32(builderCommandRange-maxFloat32(entity.HitRadius*2, 8), 24)
	if squaredWorldDistance(entity.X, entity.Y, targetX, targetY) <= stopRadius*stopRadius {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, targetX, targetY)
}

func buildPlanFromOp(op world.BuildPlanOp) *protocol.BuildPlan {
	plan := &protocol.BuildPlan{
		Breaking: op.Breaking,
		X:        op.X,
		Y:        op.Y,
		Rotation: byte(op.Rotation),
		Config:   op.Config,
	}
	if !op.Breaking {
		plan.Block = protocol.BlockRef{BlkID: op.BlockID}
	}
	return plan
}

func selectAssistFollowing(wld *world.World, entity world.RawEntity, state *protocol.ControllerState) (world.RawEntity, bool) {
	if wld == nil || entity.Team == 0 {
		return world.RawEntity{}, false
	}
	if leader, ok := preferredAssistLeader(wld, entity, state); ok {
		return leader, true
	}
	return wld.FindNearestPlayerBuilder(entity.Team, entity.ID, entity.X, entity.Y)
}

func selectAssistConstructLeader(wld *world.World, entity world.RawEntity, assistFollowing world.RawEntity, assistFollowingOK bool) (world.RawEntity, bool) {
	if wld == nil || entity.Team == 0 {
		return world.RawEntity{}, false
	}
	speed := unitCommandMoveSpeed(entity)
	if assistFollowingOK && wld.CanAssistFollowBuilder(entity.Team, assistFollowing.ID, entity.ID, entity.X, entity.Y, builderCommandRange, speed, 0) {
		return assistFollowing, true
	}
	return wld.FindNearestAssistConstructBuilder(entity.Team, entity.ID, entity.X, entity.Y, builderCommandRange, speed, commandAssistBuildRadius)
}

func preferredAssistLeader(wld *world.World, entity world.RawEntity, state *protocol.ControllerState) (world.RawEntity, bool) {
	if wld == nil || state == nil || !state.Command.HasAttack || state.Command.Target.Type != 0 {
		return world.RawEntity{}, false
	}
	leader, ok := wld.GetEntity(state.Command.Target.Pos)
	if !ok || leader.Team != entity.Team || leader.ID == entity.ID || leader.Health <= 0 {
		return world.RawEntity{}, false
	}
	if leader.BuildSpeed <= 0 && leader.PlayerID == 0 {
		return world.RawEntity{}, false
	}
	return leader, true
}

func primaryEntityBuildPlan(entity world.RawEntity) (world.BuildPlanOp, bool) {
	if len(entity.Plans) == 0 {
		return world.BuildPlanOp{}, false
	}
	plan := entity.Plans[0]
	pos := protocol.UnpackPoint2(plan.Pos)
	return world.BuildPlanOp{
		Breaking: plan.Breaking,
		X:        pos.X,
		Y:        pos.Y,
		Rotation: int8(plan.Rotation),
		BlockID:  plan.BlockID,
		Config:   plan.Config,
	}, true
}

func setUnitMineIdleAndRetreatIfNeeded(wld *world.World, entity world.RawEntity) {
	if wld == nil {
		return
	}
	_, _ = wld.SetEntityMineTile(entity.ID, -1)
	if corePos, coreX, coreY, ok := nearestFriendlyCore(wld, entity.Team, entity.X, entity.Y); ok {
		_ = corePos
		stopRadius := float32(110)
		if squaredWorldDistance(entity.X, entity.Y, coreX, coreY) > stopRadius*stopRadius {
			setUnitMoveIfNeeded(wld, entity, coreX, coreY)
			return
		}
	}
	setUnitCommandIdleIfNeeded(wld, entity)
}

func hasHoldPositionStance(state *protocol.ControllerState) bool {
	if state == nil {
		return false
	}
	for _, stance := range state.Command.Stances {
		if stance.ID == holdPositionStanceID || strings.EqualFold(strings.TrimSpace(stance.Name), "holdposition") {
			return true
		}
	}
	return false
}

func builderCommandIgnoresRange(wld *world.World, team world.TeamID) bool {
	if wld == nil {
		return false
	}
	if rules := wld.GetRulesManager().Get(); rules != nil {
		return rules.Editor || rules.InfiniteResources || rules.TeamInfiniteResources(team)
	}
	return false
}

func builderPlanWithinRange(entity world.RawEntity, op world.BuildPlanOp, buildRange float32) bool {
	if buildRange <= 0 {
		buildRange = builderCommandRange
	}
	targetX := float32(op.X*8 + 4)
	targetY := float32(op.Y*8 + 4)
	return squaredWorldDistance(entity.X, entity.Y, targetX, targetY) <= buildRange*buildRange
}

func repairCommandMoveRange(entity world.RawEntity) float32 {
	if entity.AttackRange > 0 {
		return maxFloat32(entity.AttackRange*0.65, 24)
	}
	return commandRepairRange
}

func defaultMineAutoItems() []world.ItemID {
	return []world.ItemID{0, 1, 6, 7, 16, 17}
}

func hasMineAutoStance(state *protocol.ControllerState) bool {
	if state == nil {
		return false
	}
	for _, stance := range state.Command.Stances {
		if stance.ID == mineAutoStanceID || strings.EqualFold(strings.TrimSpace(stance.Name), "mineauto") {
			return true
		}
	}
	return false
}

func ensureMineAutoStance(state *protocol.ControllerState) {
	if state == nil || hasMineAutoStance(state) {
		return
	}
	if _, ok := activeMineTargetItem(state); ok {
		return
	}
	state.Command.Stances = append(state.Command.Stances, protocol.UnitStance{ID: mineAutoStanceID, Name: "mineauto"})
}

func activeMineTargetItem(state *protocol.ControllerState) (world.ItemID, bool) {
	if state == nil {
		return 0, false
	}
	for _, stance := range state.Command.Stances {
		if item, ok := stanceItemID(stance); ok {
			return item, true
		}
	}
	return 0, false
}

func stanceItemID(stance protocol.UnitStance) (world.ItemID, bool) {
	switch strings.ToLower(strings.TrimSpace(stance.Name)) {
	case "item-copper":
		return 0, true
	case "item-lead":
		return 1, true
	case "item-sand":
		return 4, true
	case "item-coal":
		return 5, true
	case "item-titanium":
		return 6, true
	case "item-thorium":
		return 7, true
	case "item-scrap":
		return 8, true
	case "item-beryllium":
		return 16, true
	case "item-tungsten":
		return 17, true
	}
	switch stance.ID {
	case 8:
		return 0, true
	case 9:
		return 1, true
	case 12:
		return 4, true
	case 13:
		return 5, true
	case 14:
		return 6, true
	case 15:
		return 7, true
	case 16:
		return 8, true
	case 23:
		return 16, true
	case 24:
		return 17, true
	}
	return 0, false
}

func nearestFriendlyCore(wld *world.World, team world.TeamID, x, y float32) (int32, float32, float32, bool) {
	if wld == nil || team == 0 {
		return 0, 0, 0, false
	}
	bestPos := int32(0)
	bestX, bestY := float32(0), float32(0)
	bestDist := float32(-1)
	for _, packed := range wld.TeamCorePositions(team) {
		pt := protocol.UnpackPoint2(packed)
		cx := float32(pt.X*8 + 4)
		cy := float32(pt.Y*8 + 4)
		dist := squaredWorldDistance(x, y, cx, cy)
		if bestDist < 0 || dist < bestDist {
			bestPos = packed
			bestX = cx
			bestY = cy
			bestDist = dist
		}
	}
	if bestDist < 0 {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func driveUnitToCoreForOffload(wld *world.World, entity world.RawEntity, corePos int32, coreX, coreY float32) {
	if wld == nil {
		return
	}
	_, _ = wld.SetEntityMineTile(entity.ID, -1)
	if entity.Stack.Amount <= 0 || squaredWorldDistance(entity.X, entity.Y, coreX, coreY) <= commandMineTransferRange*commandMineTransferRange {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	_ = corePos
	setUnitMoveIfNeeded(wld, entity, coreX, coreY)
}

func setUnitMoveIfNeeded(wld *world.World, entity world.RawEntity, x, y float32) {
	if wld == nil {
		return
	}
	if entity.Behavior == "move" && entity.PatrolAX == x && entity.PatrolAY == y {
		return
	}
	_, _ = wld.SetEntityMoveTo(entity.ID, x, y, unitCommandMoveSpeed(entity))
}

func setUnitCommandIdleIfNeeded(wld *world.World, entity world.RawEntity) {
	if wld == nil {
		return
	}
	if entity.Behavior == "command" && entity.VelX == 0 && entity.VelY == 0 && entity.RotVel == 0 {
		return
	}
	_, _ = wld.SetEntityCommandIdle(entity.ID)
}

func unitCommandMoveSpeed(entity world.RawEntity) float32 {
	if entity.MoveSpeed > 0 {
		return entity.MoveSpeed
	}
	return defaultCommandMoveSpeed
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func rebuildCommandRetryTicks(wld *world.World) uint64 {
	if wld != nil {
		if rules := wld.GetRulesManager().Get(); rules != nil && rules.BuildAi {
			return rebuildRetryTicksBuildAI
		}
	}
	return rebuildRetryTicksDefault
}

func commandUnitMiningProfile(entity world.RawEntity) (world.UnitMiningProfile, bool) {
	profile := world.UnitMiningProfile{
		Speed:     entity.MineSpeed,
		Tier:      int(entity.MineTier),
		Capacity:  entity.ItemCapacity,
		MineFloor: entity.MineFloor,
		MineWalls: entity.MineWalls,
	}
	return profile, profile.Speed > 0 && profile.Capacity > 0 && profile.Tier >= 0
}

func squaredWorldDistance(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func extractBuildingPos(value any) (int32, bool) {
	switch v := value.(type) {
	case protocol.BuildingBox:
		return v.Pos(), true
	case protocol.Building:
		if v == nil {
			return 0, false
		}
		return v.Pos(), true
	default:
		return 0, false
	}
}

func extractUnitID(value any) (int32, bool) {
	switch v := value.(type) {
	case protocol.UnitBox:
		return v.ID(), true
	case protocol.Unit:
		if v == nil {
			return 0, false
		}
		return v.ID(), true
	default:
		return 0, false
	}
}

func extractVec2(value any) (protocol.Vec2, bool) {
	switch v := value.(type) {
	case protocol.Vec2:
		return v, true
	case *protocol.Vec2:
		if v == nil {
			return protocol.Vec2{}, false
		}
		return *v, true
	default:
		return protocol.Vec2{}, false
	}
}
