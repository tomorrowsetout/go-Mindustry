package main

import (
	"testing"
	"time"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func findOverlayUnit(t *testing.T, units []protocol.UnitSyncEntity, id int32) *protocol.UnitEntitySync {
	t.Helper()
	for _, ent := range units {
		unit, ok := ent.(*protocol.UnitEntitySync)
		if ok && unit != nil && unit.ID() == id {
			return unit
		}
	}
	t.Fatalf("unit %d not found in overlay snapshot", id)
	return nil
}

func primeAssistConstructLeader(t *testing.T, wld *world.World, leader world.RawEntity, op world.BuildPlanOp) {
	t.Helper()
	if wld == nil {
		t.Fatalf("expected world")
	}
	rules := wld.GetRulesManager().Get()
	rules.InfiniteResources = true
	wld.ApplyBuildPlanSnapshotForOwner(leader.ID, leader.Team, []world.BuildPlanOp{op})
	wld.UpdateBuilderState(leader.ID, leader.Team, leader.ID, leader.X, leader.Y, true, 220)
	if _, ok := wld.SetEntityBuildState(leader.ID, true, []*protocol.BuildPlan{buildPlanFromOp(op)}); !ok {
		t.Fatalf("expected leader build state to apply")
	}
	wld.Step(time.Second / 60)
	if !wld.CanAssistFollowBuilder(leader.Team, leader.ID, 0, leader.X, leader.Y, 220, 24, 0) {
		t.Fatalf("expected primed leader %d to qualify as active construct builder", leader.ID)
	}
}

func TestUnitCommandServiceApplyCommandUnitsMoveTarget(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	wld.SetModel(model)
	entity, err := wld.AddEntityWithID(36, 2001, 32, 32, 1)
	if err != nil {
		t.Fatalf("add entity: %v", err)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	target := protocol.Vec2{X: 120, Y: 88}
	service.applyCommandUnits(conn, wld, []int32{entity.ID}, nil, nil, target, false)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected move behavior, got %q", got.Behavior)
	}
	if got.PatrolAX != target.X || got.PatrolAY != target.Y {
		t.Fatalf("expected move target (%v,%v), got (%v,%v)", target.X, target.Y, got.PatrolAX, got.PatrolAY)
	}

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Type != protocol.ControllerCommand9 {
		t.Fatalf("expected command controller type 9, got %v", state.Type)
	}
	if !state.Command.HasPos || state.Command.TargetPos != target {
		t.Fatalf("expected command target pos %+v, got %+v", target, state.Command.TargetPos)
	}
	if state.Command.CommandID != int8(moveCommandID) {
		t.Fatalf("expected move command id %d, got %d", moveCommandID, state.Command.CommandID)
	}
}

func TestUnitCommandServiceApplyCommandUnitsBuildTargetUsesPackedTilePos(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		430: "laser-drill",
	}
	targetPos := protocol.PackPoint2(12, 15)
	tile := &model.Tiles[15*model.Width+12]
	tile.Block = 430
	tile.Team = 2
	tile.Build = &world.Building{Block: 430, Team: 2, X: 12, Y: 15, Health: 250, MaxHealth: 250}
	wld.SetModel(model)
	entity, err := wld.AddEntityWithID(36, 2101, 32, 32, 1)
	if err != nil {
		t.Fatalf("add entity: %v", err)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applyCommandUnits(conn, wld, []int32{entity.ID}, protocol.BuildingBox{PosValue: targetPos}, nil, nil, false)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected move behavior toward hostile building, got %q", got.Behavior)
	}
	if got.PatrolAX != float32(12*8+4) || got.PatrolAY != float32(15*8+4) {
		t.Fatalf("expected hostile building world target (%v,%v), got (%v,%v)", float32(12*8+4), float32(15*8+4), got.PatrolAX, got.PatrolAY)
	}

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if !state.Command.HasAttack || state.Command.Target.Pos != targetPos {
		t.Fatalf("expected packed hostile build target %d, got %+v", targetPos, state.Command.Target)
	}
}

func TestUnitCommandServiceSetCommandAndStopStance(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	wld.SetModel(model)
	entity, err := wld.AddEntityWithID(36, 2002, 32, 32, 1)
	if err != nil {
		t.Fatalf("add entity: %v", err)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applyCommandUnits(conn, wld, []int32{entity.ID}, nil, nil, protocol.Vec2{X: 96, Y: 96}, false)
	service.applySetUnitCommand(conn, wld, []int32{entity.ID}, &protocol.UnitCommand{ID: 2, Name: "rebuild"})
	service.applySetUnitStance(conn, wld, []int32{entity.ID}, protocol.UnitStance{ID: stopUnitStanceID, Name: "stop"}, true)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "" {
		t.Fatalf("expected stop stance to clear behavior, got %q", got.Behavior)
	}

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Command.CommandID != 2 {
		t.Fatalf("expected command id 2, got %d", state.Command.CommandID)
	}
	if state.Command.HasPos || state.Command.HasAttack {
		t.Fatalf("expected stop stance to clear active command target, got hasPos=%v hasAttack=%v", state.Command.HasPos, state.Command.HasAttack)
	}
	if len(state.Command.Queue) != 0 {
		t.Fatalf("expected stop stance to clear queue, got %d", len(state.Command.Queue))
	}
}

func TestUnitCommandServiceStepPromotesQueuedMoveCommand(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	wld.SetModel(model)
	entity, err := wld.AddEntityWithID(36, 2003, 32, 32, 1)
	if err != nil {
		t.Fatalf("add entity: %v", err)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	first := protocol.Vec2{X: 96, Y: 64}
	second := protocol.Vec2{X: 160, Y: 112}

	service.applyCommandUnits(conn, wld, []int32{entity.ID}, nil, nil, first, false)
	service.applyCommandUnits(conn, wld, []int32{entity.ID}, nil, nil, second, true)

	_, _ = wld.SetEntityPosition(entity.ID, first.X, first.Y, 0)
	_, _ = wld.ClearEntityBehavior(entity.ID)
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected queued command to restore move behavior, got %q", got.Behavior)
	}
	if got.PatrolAX != second.X || got.PatrolAY != second.Y {
		t.Fatalf("expected queued move target (%v,%v), got (%v,%v)", second.X, second.Y, got.PatrolAX, got.PatrolAY)
	}

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if !state.Command.HasPos || state.Command.TargetPos != second {
		t.Fatalf("expected active queued target %+v, got %+v", second, state.Command.TargetPos)
	}
	if len(state.Command.Queue) != 0 {
		t.Fatalf("expected queued command to be consumed, got %d remaining", len(state.Command.Queue))
	}
}

func TestUnitCommandServiceBootstrapsMonoMineCommand(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		20: "mono",
	}
	corePos := 0
	model.Tiles[corePos].Block = 339
	model.Tiles[corePos].Team = 1
	model.Tiles[corePos].Build = &world.Building{Block: 339, Team: 1, X: 0, Y: 0}
	orePos := 3*model.Width + 3
	model.Tiles[orePos].Floor = 1
	model.Tiles[orePos].Overlay = 2
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       20,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		MineSpeed:    10,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)
	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Command.CommandID != int8(mineCommandID) {
		t.Fatalf("expected mono default command id %d, got %d", mineCommandID, state.Command.CommandID)
	}

	for i := 0; i < 90; i++ {
		wld.Step(time.Second / 60)
		service.step(wld)
	}

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Stack.Item != 0 || got.Stack.Amount <= 0 {
		t.Fatalf("expected bootstrapped mono mine command to mine copper, got %+v", got.Stack)
	}
}

func TestUnitCommandServiceMineStanceTargetsRequestedOre(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		3:   "ore-lead",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		20: "mono",
	}
	model.Tiles[0].Block = 339
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &world.Building{Block: 339, Team: 1, X: 0, Y: 0}
	model.Tiles[2*model.Width+2].Floor = 1
	model.Tiles[2*model.Width+2].Overlay = 2
	model.Tiles[4*model.Width+4].Floor = 1
	model.Tiles[4*model.Width+4].Overlay = 3
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       20,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		MineSpeed:    10,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.step(wld)
	service.applySetUnitCommand(conn, wld, []int32{entity.ID}, &protocol.UnitCommand{ID: mineCommandID, Name: "mine"})
	service.applySetUnitStance(conn, wld, []int32{entity.ID}, protocol.UnitStance{ID: 9, Name: "item-lead"}, true)

	for i := 0; i < 90; i++ {
		wld.Step(time.Second / 60)
		service.step(wld)
	}

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Stack.Item != 1 || got.Stack.Amount <= 0 {
		t.Fatalf("expected lead mining stance to mine lead, got %+v", got.Stack)
	}
}

func TestUnitCommandServiceMoveCommandDoesNotFallBackToMonoMine(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		20: "mono",
	}
	model.Tiles[0].Block = 339
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &world.Building{Block: 339, Team: 1, X: 0, Y: 0}
	model.Tiles[2*model.Width+2].Floor = 1
	model.Tiles[2*model.Width+2].Overlay = 2
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       20,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		MineSpeed:    10,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.step(wld)
	target := protocol.Vec2{X: 96, Y: 96}
	service.applyCommandUnits(conn, wld, []int32{entity.ID}, nil, nil, target, false)

	for i := 0; i < 150; i++ {
		wld.Step(time.Second / 60)
		service.step(wld)
	}

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Stack.Amount != 0 {
		t.Fatalf("expected move command to suppress fallback mining, got stack %+v", got.Stack)
	}
	if got.MineTilePos >= 0 {
		t.Fatalf("expected move command to keep mining target cleared, got %d", got.MineTilePos)
	}

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Command.CommandID != int8(moveCommandID) {
		t.Fatalf("expected command id to stay on move, got %d", state.Command.CommandID)
	}
}

func TestUnitCommandServiceBootstrapsPolyRebuildCommand(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.UnitNames = map[int16]string{
		21: "poly",
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Command.CommandID != int8(rebuildCommandID) {
		t.Fatalf("expected poly default command id %d, got %d", rebuildCommandID, state.Command.CommandID)
	}
}

func TestUnitCommandServiceBootstrapsMegaRepairCommand(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.UnitNames = map[int16]string{
		22: "mega",
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       22,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   2.6,
		ItemCapacity: 70,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	overlay := service.overlay(wld.EntitySyncSnapshots(nil, nil))
	unit := findOverlayUnit(t, overlay, entity.ID)
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state overlay")
	}
	if state.Command.CommandID != int8(repairCommandID) {
		t.Fatalf("expected mega default command id %d, got %d", repairCommandID, state.Command.CommandID)
	}
}

func TestUnitCommandServiceRepairCommandMovesTowardDamagedFriendlyBuilding(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		22: "mega",
	}
	buildPos := 20*model.Width + 20
	model.Tiles[buildPos].Block = 339
	model.Tiles[buildPos].Team = 1
	model.Tiles[buildPos].Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         20,
		Y:         20,
		Health:    400,
		MaxHealth: 1000,
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       22,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   2.6,
		ItemCapacity: 70,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected repair command to move toward damaged building, got %q", got.Behavior)
	}
	if got.PatrolAX != float32(20*8+4) || got.PatrolAY != float32(20*8+4) {
		t.Fatalf("expected repair move target (164,164), got (%v,%v)", got.PatrolAX, got.PatrolAY)
	}
}

func TestUnitCommandServiceRepairCommandHoldPositionDoesNotMove(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		22: "mega",
	}
	buildPos := 20*model.Width + 20
	model.Tiles[buildPos].Block = 339
	model.Tiles[buildPos].Team = 1
	model.Tiles[buildPos].Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         20,
		Y:         20,
		Health:    400,
		MaxHealth: 1000,
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       22,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   2.6,
		ItemCapacity: 70,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{entity.ID}, &protocol.UnitCommand{ID: repairCommandID, Name: "repair"})
	service.applySetUnitStance(conn, wld, []int32{entity.ID}, protocol.UnitStance{ID: holdPositionStanceID, Name: "holdposition"}, true)
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior == "move" {
		t.Fatalf("expected holdposition repair command to avoid moving toward target")
	}
}

func TestUnitCommandServiceRepairCommandUsesUnitAttackRange(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		22: "mega",
	}
	buildPos := 1*model.Width + 16
	model.Tiles[buildPos].Block = 339
	model.Tiles[buildPos].Team = 1
	model.Tiles[buildPos].Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         16,
		Y:         1,
		Health:    400,
		MaxHealth: 1000,
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       22,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		AttackRange:  220,
		BuildSpeed:   2.6,
		ItemCapacity: 70,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior == "move" {
		t.Fatalf("expected repair command to idle once target is within unit attack range, got move toward (%v,%v)", got.PatrolAX, got.PatrolAY)
	}
}

func TestUnitCommandServiceRepairCommandIgnoresConstructBuildTargets(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		339: "core-shard",
		998: "build3",
	}
	model.UnitNames = map[int16]string{
		22: "mega",
	}

	constructPos := 10*model.Width + 10
	model.Tiles[constructPos].Block = 998
	model.Tiles[constructPos].Team = 1
	model.Tiles[constructPos].Build = &world.Building{
		Block:     998,
		Team:      1,
		X:         10,
		Y:         10,
		Health:    2,
		MaxHealth: 10,
	}

	buildPos := 20*model.Width + 20
	model.Tiles[buildPos].Block = 339
	model.Tiles[buildPos].Team = 1
	model.Tiles[buildPos].Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         20,
		Y:         20,
		Health:    400,
		MaxHealth: 1000,
	}
	wld.SetModel(model)

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       22,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   2.6,
		ItemCapacity: 70,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected repair command to move toward real damaged building, got %q", got.Behavior)
	}
	if got.PatrolAX != float32(20*8+4) || got.PatrolAY != float32(20*8+4) {
		t.Fatalf("expected construct build target to be ignored in favor of real building at (164,164), got (%v,%v)", got.PatrolAX, got.PatrolAY)
	}
}

func TestUnitCommandServiceRebuildCommandConsumesBrokenBlockQueue(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
	}
	wld.SetModel(model)
	rules := wld.GetRulesManager().Get()
	rules.InfiniteResources = true
	buildPos := 20*model.Width + 20
	model.Tiles[buildPos].Block = 257
	model.Tiles[buildPos].Team = 1
	model.Tiles[buildPos].Build = &world.Building{
		Block:     257,
		Team:      1,
		X:         20,
		Y:         20,
		Health:    1000,
		MaxHealth: 1000,
	}

	_ = wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	if !wld.DamageBuildingPacked(protocol.PackPoint2(20, 20), 2000) {
		t.Fatalf("expected damage to destroy queued rebuild target")
	}

	service := newUnitCommandService()
	service.step(wld)
	for i := 0; i < 480; i++ {
		wld.Step(time.Second / 60)
		service.step(wld)
	}

	gotModel := wld.CloneModel()
	tile := gotModel.Tiles[20*gotModel.Width+20]
	if tile.Block != 257 || tile.Build == nil || tile.Team != 1 {
		t.Fatalf("expected rebuild command to restore destroyed duo, got block=%d build=%v team=%d", tile.Block, tile.Build != nil, tile.Team)
	}
}

func TestUnitCommandServiceRebuildCommandFollowsNearbyConstructBuilderBeforeQueue(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)
	rules := wld.GetRulesManager().Get()
	rules.InfiniteResources = true
	rebuildPos := 5*model.Width + 5
	model.Tiles[rebuildPos].Block = 257
	model.Tiles[rebuildPos].Team = 1
	model.Tiles[rebuildPos].Build = &world.Building{
		Block:     257,
		Team:      1,
		X:         5,
		Y:         5,
		Health:    1000,
		MaxHealth: 1000,
	}

	leader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            8 * 8,
		Y:            8 * 8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            9 * 8,
		Y:            9 * 8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	if !wld.DamageBuildingPacked(protocol.PackPoint2(5, 5), 2000) {
		t.Fatalf("expected broken rebuild plan to queue")
	}

	primeAssistConstructLeader(t, wld, leader, world.BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 257})

	service := newUnitCommandService()
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if len(got.Plans) == 0 {
		t.Fatal("expected rebuild command to copy nearby construct leader plan before rebuild queue")
	}
	if got.Plans[0].Pos != protocol.PackPoint2(18, 18) {
		t.Fatalf("expected follower to mirror leader plan at (18,18), got %+v", got.Plans[0])
	}
}

func TestUnitCommandServiceRebuildCommandIgnoresPendingBuildPlans(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
	}
	wld.SetModel(model)

	wld.ApplyBuildPlans(1, []world.BuildPlanOp{{
		X:       20,
		Y:       20,
		BlockID: 257,
	}})

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	service := newUnitCommandService()
	service.step(wld)

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.UpdateBuilding || len(got.Plans) > 0 {
		t.Fatalf("expected rebuild command to ignore ordinary pending build plans, got update=%v plans=%d", got.UpdateBuilding, len(got.Plans))
	}
	if got.Behavior == "move" && got.PatrolAX == float32(20*8+4) && got.PatrolAY == float32(20*8+4) {
		t.Fatalf("expected rebuild command not to move toward ordinary pending build plan")
	}
}

func TestUnitCommandServiceRebuildCommandHoldPositionUsesInRangeBrokenBlockQueue(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
	}
	corePos := 1*model.Width + 1
	model.Tiles[corePos].Block = 339
	model.Tiles[corePos].Team = 1
	model.Tiles[corePos].Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         1,
		Y:         1,
		Health:    1000,
		MaxHealth: 1000,
		Items:     []world.ItemStack{{Item: 0, Amount: 200}},
	}
	wld.SetModel(model)

	for _, xy := range [][2]int{{12, 12}, {40, 40}} {
		pos := xy[1]*model.Width + xy[0]
		model.Tiles[pos].Block = 257
		model.Tiles[pos].Team = 1
		model.Tiles[pos].Build = &world.Building{
			Block:     257,
			Team:      1,
			X:         xy[0],
			Y:         xy[1],
			Health:    1000,
			MaxHealth: 1000,
		}
	}

	if !wld.DamageBuildingPacked(protocol.PackPoint2(12, 12), 2000) {
		t.Fatalf("expected near duo destruction to queue rebuild")
	}
	if !wld.DamageBuildingPacked(protocol.PackPoint2(40, 40), 2000) {
		t.Fatalf("expected far duo destruction to queue rebuild")
	}

	entity := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{entity.ID}, &protocol.UnitCommand{ID: rebuildCommandID, Name: "rebuild"})
	service.applySetUnitStance(conn, wld, []int32{entity.ID}, protocol.UnitStance{ID: holdPositionStanceID, Name: "holdposition"}, true)
	service.step(wld)
	for i := 0; i < 480; i++ {
		wld.Step(time.Second / 60)
		service.step(wld)
	}

	got, ok := wld.GetEntity(entity.ID)
	if !ok {
		t.Fatalf("expected entity to remain in world")
	}
	if got.Behavior == "move" {
		t.Fatalf("expected holdposition rebuild command to build in range without moving")
	}

	gotModel := wld.CloneModel()
	nearTile := gotModel.Tiles[12*gotModel.Width+12]
	farTile := gotModel.Tiles[40*gotModel.Width+40]
	if nearTile.Block != 257 || nearTile.Build == nil || nearTile.Team != 1 {
		t.Fatalf("expected holdposition rebuild to restore in-range duo, got block=%d build=%v team=%d", nearTile.Block, nearTile.Build != nil, nearTile.Team)
	}
	if farTile.Block != 0 || farTile.Build != nil {
		t.Fatalf("expected out-of-range broken block to remain queued, got block=%d build=%v", farTile.Block, farTile.Build != nil)
	}
}

func TestUnitCommandServiceAssistCommandFollowsPlayerBuilderBeforeIdleAIBuilder(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(32, 32)
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)

	idleAIBuilder := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            40,
		Y:            40,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})

	playerBuilder := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		PlayerID:     7,
		X:            120,
		Y:            120,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})

	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if got.Behavior != "move" {
		t.Fatalf("expected assist command to move toward player builder, got %q", got.Behavior)
	}
	if got.PatrolAX != playerBuilder.X || got.PatrolAY != playerBuilder.Y {
		t.Fatalf("expected assist command to follow player builder (%v,%v), got (%v,%v); idle ai builder was (%v,%v)", playerBuilder.X, playerBuilder.Y, got.PatrolAX, got.PatrolAY, idleAIBuilder.X, idleAIBuilder.Y)
	}
}

func TestUnitCommandServiceAssistCommandMirrorsNearbyConstructLeaderBeforeIdlePlayer(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)

	leader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            140,
		Y:            140,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	primeAssistConstructLeader(t, wld, leader, world.BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 257})

	idlePlayer := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		PlayerID:     7,
		X:            320,
		Y:            320,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	_ = idlePlayer

	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})
	if gotLeader, ok := wld.FindNearestAssistConstructBuilder(1, follower.ID, follower.X, follower.Y, 220, follower.MoveSpeed, 1500); !ok || gotLeader.ID != leader.ID {
		gotID := int32(0)
		if ok {
			gotID = gotLeader.ID
		}
		t.Fatalf("expected nearby construct helper to select leader %d, got %d ok=%v", leader.ID, gotID, ok)
	}
	if gotLeader, ok := selectAssistConstructLeader(wld, follower, idlePlayer, true); !ok || gotLeader.ID != leader.ID {
		gotID := int32(0)
		if ok {
			gotID = gotLeader.ID
		}
		t.Fatalf("expected command-layer assist selector to choose construct leader %d, got %d ok=%v", leader.ID, gotID, ok)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if !got.UpdateBuilding || len(got.Plans) == 0 {
		t.Fatalf("expected assist follower to mirror active construct leader plan, updateBuilding=%v plans=%d", got.UpdateBuilding, len(got.Plans))
	}
	pos := protocol.UnpackPoint2(got.Plans[0].Pos)
	if pos.X != 18 || pos.Y != 18 || got.Plans[0].BlockID != 257 {
		t.Fatalf("expected mirrored build plan at (18,18) duo, got (%d,%d) block=%d", pos.X, pos.Y, got.Plans[0].BlockID)
	}
}

func TestUnitCommandServiceAssistCommandDoesNotMirrorNonConstructLeader(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)

	nonConstructLeader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            40,
		Y:            40,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	if _, ok := wld.SetEntityBuildState(nonConstructLeader.ID, true, []*protocol.BuildPlan{{
		X:        18,
		Y:        18,
		Rotation: 0,
		Block:    protocol.BlockRef{BlkID: 257},
	}}); !ok {
		t.Fatalf("expected non-construct leader build state to apply")
	}

	playerBuilder := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		PlayerID:     7,
		X:            120,
		Y:            120,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})

	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if got.UpdateBuilding || len(got.Plans) != 0 {
		t.Fatalf("expected assist follower to ignore non-construct builder, updateBuilding=%v plans=%d", got.UpdateBuilding, len(got.Plans))
	}
	if got.Behavior != "move" {
		t.Fatalf("expected assist follower to move toward player builder instead, got %q", got.Behavior)
	}
	if got.PatrolAX != playerBuilder.X || got.PatrolAY != playerBuilder.Y {
		t.Fatalf("expected assist follower to move toward player builder (%v,%v), got (%v,%v)", playerBuilder.X, playerBuilder.Y, got.PatrolAX, got.PatrolAY)
	}
}

func TestUnitCommandServiceAssistCommandIgnoresConstructLeaderOutsideBuildRadius(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(256, 256)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)

	playerBuilder := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		PlayerID:     7,
		X:            120,
		Y:            120,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})

	farLeader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            1604,
		Y:            1604,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	primeAssistConstructLeader(t, wld, farLeader, world.BuildPlanOp{X: 200, Y: 200, Rotation: 0, BlockID: 257})

	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if got.UpdateBuilding || len(got.Plans) != 0 {
		t.Fatalf("expected out-of-radius construct leader to be ignored, updateBuilding=%v plans=%d", got.UpdateBuilding, len(got.Plans))
	}
	if got.Behavior != "move" {
		t.Fatalf("expected assist follower to move toward player builder, got %q", got.Behavior)
	}
	if got.PatrolAX != playerBuilder.X || got.PatrolAY != playerBuilder.Y {
		t.Fatalf("expected assist follower to move toward player builder (%v,%v), got (%v,%v)", playerBuilder.X, playerBuilder.Y, got.PatrolAX, got.PatrolAY)
	}
}

func TestUnitCommandServiceAssistCommandPrefersActivePlayerConstructLeader(t *testing.T) {
	wld := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	wld.SetModel(model)

	aiLeader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		X:            100,
		Y:            100,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	primeAssistConstructLeader(t, wld, aiLeader, world.BuildPlanOp{X: 13, Y: 13, Rotation: 0, BlockID: 257})

	playerLeader := wld.Model().AddEntity(world.RawEntity{
		TypeID:       35,
		PlayerID:     7,
		X:            200,
		Y:            200,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	primeAssistConstructLeader(t, wld, playerLeader, world.BuildPlanOp{X: 25, Y: 25, Rotation: 0, BlockID: 257})

	follower := wld.Model().AddEntity(world.RawEntity{
		TypeID:       21,
		X:            8,
		Y:            8,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		SlowMul:      1,
		MineTilePos:  -1,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 40,
		Flying:       true,
	})
	if gotLeader, ok := selectAssistConstructLeader(wld, follower, playerLeader, true); !ok || gotLeader.ID != playerLeader.ID {
		gotID := int32(0)
		if ok {
			gotID = gotLeader.ID
		}
		t.Fatalf("expected active player assistFollowing to override nearby ai leader %d, got %d ok=%v", playerLeader.ID, gotID, ok)
	}

	conn := &netserver.Conn{}
	conn.SetTeamID(1)

	service := newUnitCommandService()
	service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
	service.step(wld)

	got, ok := wld.GetEntity(follower.ID)
	if !ok {
		t.Fatalf("expected follower to remain in world")
	}
	if !got.UpdateBuilding || len(got.Plans) == 0 {
		t.Fatalf("expected assist follower to mirror active player leader plan, updateBuilding=%v plans=%d", got.UpdateBuilding, len(got.Plans))
	}
	pos := protocol.UnpackPoint2(got.Plans[0].Pos)
	if pos.X != 25 || pos.Y != 25 {
		t.Fatalf("expected active player leader plan at (25,25) to win over nearby ai builder, got (%d,%d)", pos.X, pos.Y)
	}
}

func TestUnitCommandServiceAssistCommandAddsFollowerConstructProgress(t *testing.T) {
	runBuildTicks := func(t *testing.T, withFollower bool) int {
		t.Helper()
		wld := world.New(world.Config{TPS: 60})
		model := world.NewWorldModel(64, 64)
		model.BlockNames = map[int16]string{
			0:   "air",
			257: "duo",
		}
		model.UnitNames = map[int16]string{
			35: "alpha",
		}
		wld.SetModel(model)

		leader := wld.Model().AddEntity(world.RawEntity{
			TypeID:       35,
			X:            140,
			Y:            140,
			Team:         1,
			Health:       100,
			MaxHealth:    100,
			SlowMul:      1,
			MineTilePos:  -1,
			MoveSpeed:    24,
			BuildSpeed:   0.5,
			ItemCapacity: 30,
			Flying:       true,
		})
		primeAssistConstructLeader(t, wld, leader, world.BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 257})

		var service *unitCommandService
		if withFollower {
			follower := wld.Model().AddEntity(world.RawEntity{
				TypeID:       35,
				X:            148,
				Y:            148,
				Team:         1,
				Health:       100,
				MaxHealth:    100,
				SlowMul:      1,
				MineTilePos:  -1,
				MoveSpeed:    24,
				BuildSpeed:   0.5,
				ItemCapacity: 30,
				Flying:       true,
			})
			conn := &netserver.Conn{}
			conn.SetTeamID(1)
			service = newUnitCommandService()
			service.applySetUnitCommand(conn, wld, []int32{follower.ID}, &protocol.UnitCommand{ID: assistCommandID, Name: "assist"})
			service.step(wld)
			gotFollower, ok := wld.GetEntity(follower.ID)
			if !ok || !gotFollower.UpdateBuilding || len(gotFollower.Plans) == 0 {
				t.Fatalf("expected assist follower to mirror construct leader before progress check, ok=%v updateBuilding=%v plans=%d", ok, gotFollower.UpdateBuilding, len(gotFollower.Plans))
			}
		}

		for i := 1; i <= 180; i++ {
			wld.Step(time.Second / 60)
			if service != nil {
				service.step(wld)
			}
			gotModel := wld.CloneModel()
			tile := gotModel.Tiles[18*gotModel.Width+18]
			if tile.Block == 257 && tile.Build != nil && tile.Team == 1 {
				return i
			}
		}
		return -1
	}

	soloTicks := runBuildTicks(t, false)
	assistTicks := runBuildTicks(t, true)
	if soloTicks <= 0 || assistTicks <= 0 {
		t.Fatalf("expected both solo and assisted builds to finish, soloTicks=%d assistTicks=%d", soloTicks, assistTicks)
	}
	if assistTicks >= soloTicks {
		t.Fatalf("expected mirrored assist follower to finish the same construct faster, soloTicks=%d assistTicks=%d", soloTicks, assistTicks)
	}
}
