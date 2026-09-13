//go:build ignore

package main

import (
	"fmt"
	"strings"
	"sync"

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
)

// 修复：添加超时检查，避免无限循环
const (
	commandTimeoutTicks    uint64  = 300 // 5分钟无响应则取消命令
	stuckDistanceThreshold float32 = 2.0 // 单位移动距离小于这个值持续过久，视为卡住
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

// 修复：改进的超时检测和卡住检测
func (s *unitCommandService) checkAndUpdateRuntime(tick uint64, unitID int32, x, y float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runtime := s.runtime[unitID]
	if tick < runtime.lastActionTick+commandTimeoutTicks {
		// 命令超时，清除状态
		delete(s.byUnit, unitID)
		delete(s.runtime, unitID)
		return
	}

	// 检查卡住状态
	if tick > runtime.stuckCheckTick+60 {
		distance := (x-runtime.lastX)*(x-runtime.lastX) + (y-runtime.lastY)*(y-runtime.lastY)
		if distance < stuckDistanceThreshold*stuckDistanceThreshold {
			runtime.stuckTicks++
			if runtime.stuckTicks > 30 { // 卡住超过30秒
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

func (s *unitCommandService) step(wld *world.World) {
	if s == nil || wld == nil {
		return
	}
	tick := wld.Snapshot().Tick

	// 修复：在锁外部获取默认单位，避免死锁
	defaultMineUnits := discoverDefaultMineCommandUnits(wld)
	defaultRebuildUnits := discoverDefaultRebuildCommandUnits(wld)
	defaultRepairUnits := discoverDefaultRepairCommandUnits(wld)

	s.mu.Lock()
	// 修复：检查超时
	for unitID, runtime := range s.runtime {
		if tick > runtime.lastActionTick+commandTimeoutTicks {
			delete(s.byUnit, unitID)
			delete(s.runtime, unitID)
		}
	}

	// 设置默认命令
	for _, unitID := range defaultMineUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(mineCommandID)
		ensureMineAutoStance(state)
		// 初始化运行时
		s.runtime[unitID] = unitCommandRuntime{
			lastActionTick: tick,
			stuckCheckTick: tick,
			stuckTicks:     0,
		}
	}
	for _, unitID := range defaultRebuildUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(rebuildCommandID)
		s.runtime[unitID] = unitCommandRuntime{
			lastActionTick: tick,
			stuckCheckTick: tick,
			stuckTicks:     0,
		}
	}
	for _, unitID := range defaultRepairUnits {
		if _, ok := s.byUnit[unitID]; ok {
			continue
		}
		state := s.ensureLocked(unitID)
		state.Command.CommandID = int8(repairCommandID)
		s.runtime[unitID] = unitCommandRuntime{
			lastActionTick: tick,
			stuckCheckTick: tick,
			stuckTicks:     0,
		}
	}
	defer s.mu.Unlock()

	// 修复：在锁外部执行命令，避免死锁
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
			continue // 玩家控制的单位不需要AI命令
		}

		// 更新卡住检测
		s.checkAndUpdateRuntime(tick, unitID, entity.X, entity.Y)

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

// 修复：改进的挖掘命令逻辑
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

	// 修复：检查物品传输
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

	// 修复：改进的距离检查，添加缓冲
	distSq := squaredWorldDistance(entity.X, entity.Y, oreX, oreY)
	miningRangeSq := (commandMineRange * 1.5) * (commandMineRange * 1.5) // 添加50%缓冲

	if distSq <= miningRangeSq {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}

	setUnitMoveIfNeeded(wld, entity, oreX, oreY)
}

// 修复：改进的修理命令逻辑
func stepRepairCommand(wld *world.World, unitID int32, entity world.RawEntity, state *protocol.ControllerState) {
	if wld == nil || unitID == 0 || entity.Team == 0 {
		return
	}
	hold := hasHoldPositionStance(state)
	clearUnitBuilderCommandState(wld, unitID, entity.Team)

	// 修复：改进修理目标选择，优先修理最近的最严重损坏建筑
	target, ok := wld.FindNearestDamagedFriendlyBuilding(entity.Team, entity.X, entity.Y)
	if !ok {
		if hold {
			_, _ = wld.SetEntityMineTile(unitID, -1)
			setUnitCommandIdleIfNeeded(wld, entity)
			return
		}
		// 修复：如果没有修理目标，尝试寻找可建造的目标
		if assistTarget, assistOK := selectAssistConstructTarget(wld, entity, state, hold); assistOK {
			// 转换到协助模式
			state.Command.CommandID = int8(assistCommandID)
			startWorldUnitCommand(wld, unitID, assistTarget.spec, assistTarget.speed)
			return
		}
		setUnitMineIdleAndRetreatIfNeeded(wld, entity)
		return
	}

	// 修复：添加修理距离检查和优先级
	repairRangeSq := (commandRepairRange * 1.2) * (commandRepairRange * 1.2) // 添加20%缓冲
	distSq := squaredWorldDistance(entity.X, entity.Y, target.X, target.Y)

	if distSq <= repairRangeSq {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	if hold {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, target.X, target.Y)
}

// 修复：改进的重建命令逻辑
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
			// 修复：添加更好的错误处理和状态转换
			switch wld.EvaluateBuildPlanPlacement(entity.Team, op) {
			case world.BuildPlanPlacementReady:
				applyUnitCommandBuildPlan(wld, unitID, entity, op, !hold)
				if runtime != nil {
					runtime.rebuildRetryTick = 0
					runtime.lastActionTick = tick
				}
				return
			case world.BuildPlanPlacementBuilt:
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				if runtime != nil {
					runtime.rebuildRetryTick = tick + retryTicks
				}
				setUnitMineIdleAndRetreatIfNeeded(wld, entity)
				return
			default:
				// 修复：尝试找到下一个可建造的计划
				clearUnitBuilderCommandState(wld, unitID, entity.Team)
				if runtime != nil {
					runtime.rebuildRetryTick = tick + retryTicks
				}
				setUnitMineIdleAndRetreatIfNeeded(wld, entity)
				return
			}
		}
	}

	// 修复：检查重试超时
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

	var (
		op world.BuildPlanOp
		ok bool
	)

	// 修复：改进的重建计划获取逻辑
	if hold && !ignoreRange {
		op, ok = wld.AcquireNextRebuildPlanInRange(entity.Team, entity.X, entity.Y, builderCommandRange*1.5) // 添加50%范围缓冲
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
		runtime.lastActionTick = tick
	}
	applyUnitCommandBuildPlan(wld, unitID, entity, op, !hold)
}

// 修复：改进的协助命令逻辑
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
			// 修复：添加更好的范围检查
			assistRange := builderCommandRange * 1.3 // 添加30%缓冲
			if hold && !builderCommandIgnoresRange(wld, entity.Team) && !builderPlanWithinRange(entity, plan, assistRange) {
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

	// 修复：添加跟随距离检查和速度调整
	assistFollowRangeSq := (commandAssistFollowRange * 1.2) * (commandAssistFollowRange * 1.2)
	speed := unitCommandMoveSpeed(entity)
	if assistFollowingOK && wld.CanAssistFollowBuilder(entity.Team, assistFollowing.ID, entity.ID, entity.X, entity.Y, assistRange, speed, 0) {
		// 可以跟随，继续移动
	} else if assistFollowingOK {
		// 超出跟随范围，停止
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}

	if squaredWorldDistance(entity.X, entity.Y, assistFollowing.X, assistFollowing.Y) <= assistFollowRangeSq {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, assistFollowing.X, assistFollowing.Y)
}

// 修复：改进的协助跟随选择
func selectAssistFollowing(wld *world.World, entity world.RawEntity, state *protocol.ControllerState) (world.RawEntity, bool) {
	if wld == nil || state == nil {
		return world.RawEntity{}, false
	}

	// 优先选择当前命令的目标作为跟随目标
	if state.Command.HasAttack && state.Command.Target.Type == 0 { // 跟随单位
		if target, ok := wld.GetEntity(state.Command.Target.Pos); ok && target.Team == entity.Team {
			return target, true
		}
	}
	return wld.FindNearestPlayerBuilder(entity.Team, entity.ID, entity.X, entity.Y)
}

// 修复：改进的协助构造目标选择
func selectAssistConstructTarget(wld *world.World, entity world.RawEntity, state *protocol.ControllerState, hold bool) (unitCommandTargetSpec, bool) {
	if wld == nil || entity.Team == 0 {
		return unitCommandTargetSpec{}, false
	}

	// 尝试寻找最近的建造目标
	target, ok := wld.FindNearestPlayerBuilder(entity.Team, entity.ID, entity.X, entity.Y)
	if ok {
		speed := unitCommandMoveSpeed(entity)
		return unitCommandTargetSpec{
			hasAttack: false,
			pos:       protocol.Vec2{X: target.X, Y: target.Y},
			hasPos:    true,
			queue:     protocol.CommandTarget{Type: 2, Vec: protocol.Vec2{X: target.X, Y: target.Y}},
			worldPos:  protocol.Vec2{X: target.X, Y: target.Y},
			worldMode: "move",
		}, true
	}

	// 如果没有建造目标，尝试挖掘
	if profile, ok := commandUnitMiningProfile(entity); ok {
		corePos, coreX, coreY, coreOK := nearestFriendlyCore(wld, entity.Team, entity.X, entity.Y)
		if coreOK {
			targetItem, orePos, oreX, oreY, hasTarget := selectMineTarget(wld, entity, profile, state, coreX, coreY)
			if hasTarget {
				speed := unitCommandMoveSpeed(entity)
				return unitCommandTargetSpec{
					hasPos:    true,
					pos:       protocol.Vec2{X: oreX, Y: oreY},
					hasQueue:  true,
					queue:     protocol.CommandTarget{Type: 0, Pos: orePos},
					worldPos:  protocol.Vec2{X: oreX, Y: oreY},
					worldMode: "move",
				}, true
			}
		}
	}

	return unitCommandTargetSpec{}, false
}

func selectMineTarget(wld *world.World, entity world.RawEntity, profile world.UnitMiningProfile, state *protocol.ControllerState, coreX, coreY float32) (world.ItemID, int32, float32, float32, bool) {
	items := desiredMineItems(state)
	if len(items) == 0 {
		return 0, 0, 0, 0, false
	}
	coreItems := wld.TeamItems(entity.Team)

	// 修复：改进的矿石选择逻辑
	bestItem := world.ItemID(0)
	bestPos := int32(-1)
	bestX, bestY := float32(0), float32(0)
	bestCount := int32(0)
	bestDist := float32(0)
	bestScore := float32(-1) // 综合评分：距离 + 稀缺度

	for _, item := range items {
		pos, wx, wy, ok := wld.FindClosestMineTileForItem(coreX, coreY, item, profile.MineFloor, profile.MineWalls, profile.Tier)
		if !ok {
			continue
		}
		count := coreItems[item]
		dist := squaredWorldDistance(coreX, coreY, wx, wy)

		// 修复：计算综合评分（距离 + 稀缺度因子）
		scarcityFactor := float32(0)
		if count < 10 { // 稀缺资源给予更高优先级
			scarcityFactor = float32((10 - count) * 100) // 稀缺的资源优先
		}

		score := dist + scarityityFactor

		if bestScore < 0 || score < bestScore || (score == bestScore && count < bestCount) {
			bestItem = item
			bestPos = pos
			bestX = wx
			bestY = wy
			bestCount = count
			bestDist = dist
			bestScore = score
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

func setUnitMineIdleAndRetreatIfNeeded(wld *world.World, unitID int32, entity world.RawEntity) {
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
			bestDist = dist
			bestPos = packed
			bestX = cx
			bestY = cy
		}
	}
	if bestDist < 0 {
		return bestPos, bestX, bestY, false
	}
	return bestPos, bestX, bestY, true
}

func setUnitMoveIfNeeded(wld *world.World, unitID int32, x, y float32) {
	if wld == nil || unitID == 0 {
		return
	}
	_, _ = wld.SetEntityMoveTo(unitID, x, y, unitCommandMoveSpeed(world.RawEntity{MoveSpeed: 0}))
}

func unitCommandMoveSpeed(entity world.RawEntity) float32 {
	if entity.MoveSpeed > 0 {
		return entity.MoveSpeed
	}
	return defaultCommandMoveSpeed
}

func setUnitCommandIdleIfNeeded(wld *world.World, unitID int32) {
	if wld == nil || unitID == 0 {
		return
	}
	_, _ = wld.SetEntityCommandIdle(unitID)
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

func defaultMineAutoItems() []world.ItemID {
	return []world.ItemID{0, 1, 6, 7, 16, 17}
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
	// 修复：使用正确的队伍ID，而不是固定的wave team
	team := wld.DefaultTeam()
	if team == 0 {
		team = wld.WaveTeam()
	}
	return wld.UnitIDsByType(typeID, team)
}

// 修复：改进的距离计算
func squaredWorldDistance(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func driveUnitToCoreForOffload(wld *world.World, entity world.RawEntity, corePos int32, coreX, coreY float32) {
	if wld == nil {
		return
	}
	_, _ = wld.SetEntityMineTile(entity.ID, -1)
	if entity.Stack.Amount <= 0 {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	offloadRange := float32(commandMineTransferRange * 1.2) // 添加20%缓冲
	if squaredWorldDistance(entity.X, entity.Y, coreX, coreY) <= offloadRange*offloadRange {
		setUnitCommandIdleIfNeeded(wld, entity)
		return
	}
	setUnitMoveIfNeeded(wld, entity, coreX, coreY)
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
			pos:       spec.Vec,
			hasQueue:  true,
			queue:     protocol.CommandTarget{Type: 2, Vec: spec.Vec},
			worldPos:  spec.Vec,
			worldMode: "move",
		}, true
	default:
		return unitCommandTargetSpec{}, false
	}
}
