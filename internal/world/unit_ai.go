package world

import (
	"math"
	"strings"
	"time"
)

type unitAIKind string

const (
	unitAIGround       unitAIKind = "ground"
	unitAIFlying       unitAIKind = "flying"
	unitAIFlyingFollow unitAIKind = "flying-follow"
	unitAIDefender     unitAIKind = "defender"
	unitAISuicide      unitAIKind = "suicide"
	unitAIHug          unitAIKind = "hug"
	unitAIBuilder      unitAIKind = "builder"
	unitAICargo        unitAIKind = "cargo"
	unitAIAssembler    unitAIKind = "assembler"
	unitAIMissile      unitAIKind = "missile"
)

const (
	unitAIWaypointReach = float32(4)
	unitAIStuckRange    = float32(12)

	builderAIBuildRadius       = float32(1500)
	builderAIRetreatDst        = float32(110)
	builderAIRetreatDelaySec   = float32(2)
	builderAIEnemyScanSec      = float32(40.0 / 60.0)
	builderAIAssistScanSec     = float32(20.0 / 60.0)
	builderAIBreakCheckSec     = float32(40.0 / 60.0)
	builderAIRebuildPeriodSec  = float32(2)
	builderAIBuildAIRebuildSec = float32(10.0 / 60.0)
)

type unitAIState struct {
	WaypointX  float32
	WaypointY  float32
	GoalX      float32
	GoalY      float32
	GoalRadius float32
	RepathCD   float32
	StuckX     float32
	StuckY     float32
	StuckTime  float32

	BuilderFollowingID     int32
	BuilderAssistFollowing int32
	BuilderEnemyScanCD     float32
	BuilderAssistScanCD    float32
	BuilderRebuildScanCD   float32
	BuilderBreakCheckCD    float32
	BuilderRetreatTimer    float32
	BuilderThreatEntityID  int32
	BuilderThreatBuildPos  int32
	BuilderLastPlanQueued  bool
	BuilderLastPlanX       int32
	BuilderLastPlanY       int32
	BuilderLastPlanBlockID int16

	PrebuildCollectingItems bool
	PrebuildMining          bool
	PrebuildLastTargetItem  ItemID
	PrebuildHasTargetItem   bool
	PrebuildOreTilePos      int32
	PrebuildPlanScanCD      float32
	PrebuildOreScanCD       float32
	PrebuildLastPlanQueued  bool
	PrebuildLastPlanX       int32
	PrebuildLastPlanY       int32
	PrebuildLastPlanBlockID int16
}

type builderAIProfile struct {
	AlwaysFlee       bool
	OnlyAssist       bool
	FleeRange        float32
	RebuildPeriodSec float32
}

type unitAITarget struct {
	EntityID int32
	BuildPos int32
	X        float32
	Y        float32
	Radius   float32
	IsCore   bool
}

func defaultUnitAIKindByName(name string, prof unitRuntimeProfile) unitAIKind {
	name = normalizeUnitName(name)
	switch name {
	case "crawler":
		return unitAISuicide
	case "oct":
		return unitAIDefender
	case "quell", "disrupt":
		return unitAIFlyingFollow
	case "renale", "latum":
		return unitAIHug
	case "alpha", "beta", "gamma", "evoke", "incite", "emanate":
		return unitAIBuilder
	case "manifold":
		return unitAICargo
	case "assemblydrone":
		return unitAIAssembler
	}
	if strings.Contains(name, "missile") {
		return unitAIMissile
	}
	if prof.Flying {
		return unitAIFlying
	}
	return unitAIGround
}

func (w *World) unitAIKindForEntityLocked(e RawEntity) unitAIKind {
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[e.TypeID]
	}
	if strings.TrimSpace(name) == "" {
		name = fallbackUnitNameByTypeID(e.TypeID)
	}
	prof, _ := w.unitRuntimeProfileForEntityLocked(e)
	kind := defaultUnitAIKindByName(name, prof)
	if kind == "" {
		if isEntityFlying(e) {
			return unitAIFlying
		}
		return unitAIGround
	}
	return kind
}

func (w *World) stepEntityAutonomousAILocked(e *RawEntity, dt float32, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) {
	if w == nil || w.model == nil || e == nil || e.Health <= 0 {
		return
	}
	kind := w.unitAIKindForEntityLocked(*e)
	if e.PlayerID != 0 || e.Behavior != "" {
		delete(w.unitAIStates, e.ID)
		return
	}
	if e.CommandID != 0 && !(kind == unitAIBuilder && (e.UpdateBuilding || len(e.Plans) > 0)) {
		delete(w.unitAIStates, e.ID)
		return
	}

	speed := e.MoveSpeed
	if speed <= 0 {
		speed = 18
	}
	speed *= entitySpeedMultiplier(*e)

	switch kind {
	case unitAIBuilder:
		if w.builderAIUsesPrebuildFallbackLocked(*e) {
			w.applyPrebuildAIMovementLocked(e, speed, dt)
			return
		}
		if fallbackKind, ok := w.builderAIFallbackKindLocked(*e); ok {
			kind = fallbackKind
			break
		}
		w.applyBuilderAIMovementLocked(e, speed, dt, spatial, teamSpatial)
		return
	case unitAICargo, unitAIAssembler:
		if !canEntityAttack(*e) {
			e.VelX, e.VelY = 0, 0
			delete(w.unitAIStates, e.ID)
			return
		}
		if isEntityFlying(*e) {
			kind = unitAIFlying
		} else {
			kind = unitAIGround
		}
	}

	target, ok := w.selectUnitAITargetLocked(*e, kind, spatial, teamSpatial)
	if !ok {
		e.VelX, e.VelY = 0, 0
		delete(w.unitAIStates, e.ID)
		return
	}

	switch kind {
	case unitAIFlying:
		w.applyFlyingAIMovementLocked(e, target, speed)
	case unitAIFlyingFollow:
		w.applyFlyingFollowAIMovementLocked(e, target, speed)
	case unitAIDefender:
		w.applyDefenderAIMovementLocked(e, target, speed)
	case unitAISuicide:
		w.applySuicideAIMovementLocked(e, target, speed, dt)
	case unitAIHug:
		w.applyHugAIMovementLocked(e, target, speed, dt)
	case unitAIMissile:
		w.applyMissileAIMovementLocked(e, target, speed)
	default:
		w.applyGroundAIMovementLocked(e, target, speed, dt)
	}
}

func (w *World) builderAIFallbackKindLocked(e RawEntity) (unitAIKind, bool) {
	if w == nil || e.Team == 0 || w.rulesMgr == nil {
		return "", false
	}
	rules := w.rulesMgr.Get()
	if rules == nil {
		return "", false
	}
	if rules.Waves && w.isWaveTeamLocked(e.Team) && !rules.RtsAi {
		if isEntityFlying(e) {
			return unitAIFlying, true
		}
		return unitAIGround, true
	}
	return "", false
}

func entityBuildPlansToOps(plans []entityBuildPlan) []BuildPlanOp {
	if len(plans) == 0 {
		return nil
	}
	out := make([]BuildPlanOp, 0, len(plans))
	for _, plan := range plans {
		x, y := unpackTilePos(plan.Pos)
		out = append(out, BuildPlanOp{
			Breaking: plan.Breaking,
			X:        int32(x),
			Y:        int32(y),
			Rotation: int8(plan.Rotation),
			BlockID:  plan.BlockID,
			Config:   cloneEntityPlanConfig(plan.Config),
		})
	}
	return out
}

func (w *World) syncEntityBuilderRuntimeLocked(e *RawEntity) {
	if w == nil || e == nil || e.ID == 0 || e.Team == 0 {
		return
	}
	if w.builderStates == nil {
		w.builderStates = map[int32]builderRuntimeState{}
	}
	w.builderStates[e.ID] = builderRuntimeState{
		Owner:      e.ID,
		Team:       e.Team,
		UnitID:     e.ID,
		X:          e.X,
		Y:          e.Y,
		Active:     true,
		BuildRange: vanillaBuilderRange,
		UpdatedAt:  time.Now(),
	}
	e.UpdateBuilding = true
	w.applyBuildPlanSnapshotForOwnerLocked(e.ID, e.Team, entityBuildPlansToOps(e.Plans), true)
}

func (w *World) builderPrimaryPlanLocked(owner int32, team TeamID) (unitAITarget, bool) {
	if w == nil || w.model == nil || owner == 0 {
		return unitAITarget{}, false
	}
	bestBuild := pendingBuildState{}
	bestBuildPos := int32(-1)
	for pos, st := range w.pendingBuilds {
		if st.Owner != owner || st.Team != team {
			continue
		}
		if bestBuildPos < 0 || st.QueueOrder < bestBuild.QueueOrder {
			bestBuildPos = pos
			bestBuild = st
		}
	}
	bestBreak := pendingBreakState{}
	bestBreakPos := int32(-1)
	for pos, st := range w.pendingBreaks {
		if st.Owner != owner || st.Team != team {
			continue
		}
		if bestBreakPos < 0 || st.QueueOrder < bestBreak.QueueOrder {
			bestBreakPos = pos
			bestBreak = st
		}
	}
	useBreak := bestBreakPos >= 0 && (bestBuildPos < 0 || bestBreak.QueueOrder < bestBuild.QueueOrder)
	if useBreak {
		tile := &w.model.Tiles[bestBreakPos]
		tx, ty := tileCenterWorld(tile.X, tile.Y)
		return unitAITarget{BuildPos: bestBreakPos, X: tx, Y: ty, Radius: 4}, true
	}
	if bestBuildPos < 0 {
		return unitAITarget{}, false
	}
	x := int(bestBuildPos % int32(w.model.Width))
	y := int(bestBuildPos / int32(w.model.Width))
	tx, ty := tileCenterWorld(x, y)
	return unitAITarget{BuildPos: bestBuildPos, X: tx, Y: ty, Radius: 4}, true
}

func (w *World) builderAIProfileLocked(e RawEntity) builderAIProfile {
	profile := builderAIProfile{
		FleeRange:        370,
		RebuildPeriodSec: builderAIRebuildPeriodSec,
	}
	if w != nil && w.rulesMgr != nil {
		if rules := w.rulesMgr.Get(); rules != nil && rules.BuildAi {
			profile.RebuildPeriodSec = builderAIBuildAIRebuildSec
		}
	}
	name := ""
	if w != nil && w.unitNamesByID != nil {
		name = w.unitNamesByID[e.TypeID]
	}
	if strings.TrimSpace(name) == "" {
		name = fallbackUnitNameByTypeID(e.TypeID)
	}
	switch normalizeUnitName(name) {
	case "alpha", "beta", "gamma":
		profile.AlwaysFlee = true
		profile.FleeRange = 400
	case "evoke", "incite", "emanate":
		profile.AlwaysFlee = true
		profile.FleeRange = 500
	}
	return profile
}

func entityActivelyBuilding(entity RawEntity) bool {
	return entity.Health > 0 && entity.UpdateBuilding && len(entity.Plans) > 0
}

func cloneEntityBuildPlan(plan entityBuildPlan) entityBuildPlan {
	return entityBuildPlan{
		Breaking: plan.Breaking,
		Pos:      plan.Pos,
		Rotation: plan.Rotation,
		BlockID:  plan.BlockID,
		Config:   cloneEntityPlanConfig(plan.Config),
	}
}

func buildPlanEntityFromOp(op BuildPlanOp) entityBuildPlan {
	return entityBuildPlan{
		Breaking: op.Breaking,
		Pos:      packTilePos(int(op.X), int(op.Y)),
		Rotation: byte(op.Rotation),
		BlockID:  op.BlockID,
		Config:   cloneEntityPlanConfig(op.Config),
	}
}

func entityPrimaryPlanLocked(entity RawEntity) (entityBuildPlan, bool) {
	if len(entity.Plans) == 0 {
		return entityBuildPlan{}, false
	}
	return cloneEntityBuildPlan(entity.Plans[0]), true
}

func (w *World) setEntityBuilderPlansLocked(e *RawEntity, plans []entityBuildPlan) {
	if w == nil || e == nil {
		return
	}
	if len(plans) == 0 {
		e.Plans = nil
	} else {
		e.Plans = make([]entityBuildPlan, len(plans))
		for i, plan := range plans {
			e.Plans[i] = cloneEntityBuildPlan(plan)
		}
	}
	e.UpdateBuilding = true
	w.applyBuildPlanSnapshotForOwnerLocked(e.ID, e.Team, entityBuildPlansToOps(e.Plans), true)
}

func (w *World) clearEntityBuilderPlansLocked(e *RawEntity, state *unitAIState) {
	if w == nil || e == nil {
		return
	}
	w.setEntityBuilderPlansLocked(e, nil)
	if state != nil {
		state.BuilderLastPlanQueued = false
	}
}

func (w *World) removeFirstBuilderPlanLocked(e *RawEntity, state *unitAIState) {
	if w == nil || e == nil {
		return
	}
	if len(e.Plans) <= 1 {
		w.clearEntityBuilderPlansLocked(e, state)
		return
	}
	out := make([]entityBuildPlan, 0, len(e.Plans)-1)
	for _, plan := range e.Plans[1:] {
		out = append(out, cloneEntityBuildPlan(plan))
	}
	w.setEntityBuilderPlansLocked(e, out)
	if state != nil {
		state.BuilderLastPlanQueued = false
	}
}

func builderPlanOpFromEntity(plan entityBuildPlan) BuildPlanOp {
	x, y := unpackTilePos(plan.Pos)
	return BuildPlanOp{
		Breaking: plan.Breaking,
		X:        int32(x),
		Y:        int32(y),
		Rotation: int8(plan.Rotation),
		BlockID:  plan.BlockID,
		Config:   cloneEntityPlanConfig(plan.Config),
	}
}

func (w *World) builderPlanTargetLocked(plan entityBuildPlan) (unitAITarget, bool) {
	if w == nil || w.model == nil {
		return unitAITarget{}, false
	}
	x, y := unpackTilePos(plan.Pos)
	if !w.model.InBounds(x, y) {
		return unitAITarget{}, false
	}
	tx, ty := tileCenterWorld(x, y)
	return unitAITarget{
		BuildPos: int32(y*w.model.Width + x),
		X:        tx,
		Y:        ty,
		Radius:   4,
	}, true
}

func (w *World) builderLastQueuedPlanStillPresentLocked(team TeamID, state unitAIState) bool {
	if w == nil || team == 0 || !state.BuilderLastPlanQueued {
		return true
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.BuildAi {
		return w.teamBuildPlanStillPresentLocked(team, state.BuilderLastPlanX, state.BuilderLastPlanY, state.BuilderLastPlanBlockID)
	}
	for _, plan := range w.teamRebuildPlans[team] {
		if plan.X == state.BuilderLastPlanX && plan.Y == state.BuilderLastPlanY && plan.BlockID == state.BuilderLastPlanBlockID {
			return true
		}
	}
	return false
}

func (w *World) clearQueuedRebuildPlanAtLocked(team TeamID, x, y int32) {
	if w == nil || team == 0 {
		return
	}
	w.clearTeamBuildPlanAtLocked(team, x, y)
	plans := w.teamRebuildPlans[team]
	if len(plans) == 0 {
		return
	}
	out := plans[:0]
	for _, plan := range plans {
		if plan.X == x && plan.Y == y {
			continue
		}
		out = append(out, plan)
	}
	if len(out) == 0 {
		delete(w.teamRebuildPlans, team)
		return
	}
	w.teamRebuildPlans[team] = out
}

func (w *World) builderPlanValidLocked(team TeamID, plan entityBuildPlan, state unitAIState) bool {
	if w == nil || w.model == nil || team == 0 {
		return false
	}
	if !w.builderLastQueuedPlanStillPresentLocked(team, state) {
		return false
	}
	op := builderPlanOpFromEntity(plan)
	if !w.model.InBounds(int(op.X), int(op.Y)) {
		return false
	}
	pos := int32(int(op.Y)*w.model.Width + int(op.X))
	if op.Breaking {
		if st, ok := w.pendingBreaks[pos]; ok && st.Team == team {
			return true
		}
		tile := &w.model.Tiles[pos]
		targetTeam := tile.Team
		if tile.Build != nil && tile.Build.Team != 0 {
			targetTeam = tile.Build.Team
		}
		return tile.Block != 0 && (targetTeam == team || targetTeam == 0)
	}
	if st, ok := w.pendingBuilds[pos]; ok && st.Team == team && st.BlockID == op.BlockID {
		return true
	}
	return w.evaluateBuildPlanPlacementLocked(team, op) == BuildPlanPlacementReady
}

func (w *World) builderAIThreatNearbyLocked(src RawEntity, fleeRange float32, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) (unitAITarget, bool) {
	if w == nil || w.model == nil || src.Team == 0 {
		return unitAITarget{}, false
	}
	if target, ok := w.findAutonomousEnemyEntityLocked(src, fleeRange, true, true, spatial, teamSpatial); ok {
		return target, true
	}
	return w.findAutonomousEnemyBuildingLocked(src, fleeRange)
}

func (w *World) builderAINearEnemyPlanLocked(team TeamID, x, y int32, fleeRange float32) bool {
	if w == nil || w.model == nil || team == 0 || fleeRange <= 0 {
		return false
	}
	cx := float32(x*8 + 4)
	cy := float32(y*8 + 4)
	half := fleeRange * 0.5
	minX := cx - half
	maxX := cx + half
	minY := cy - half
	maxY := cy + half
	for _, other := range w.model.Entities {
		if other.ID == 0 || other.Team == 0 || other.Team == team || other.Health <= 0 {
			continue
		}
		if other.X < minX || other.X > maxX || other.Y < minY || other.Y > maxY {
			continue
		}
		return true
	}
	for _, pos := range w.turretTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Build.Team == team || tile.Build.Health <= 0 {
			continue
		}
		prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
		if !ok || prof.Range <= 0 {
			continue
		}
		tx := float32(tile.X*8 + 4)
		ty := float32(tile.Y*8 + 4)
		rangeHalf := prof.Range
		if tx+rangeHalf < minX || tx-rangeHalf > maxX || ty+rangeHalf < minY || ty-rangeHalf > maxY {
			continue
		}
		return true
	}
	return false
}

func (w *World) builderAIPlayerBreakingSamePosLocked(selfID int32, team TeamID, plan entityBuildPlan) bool {
	if w == nil || w.model == nil || plan.Breaking {
		return false
	}
	for _, entity := range w.model.Entities {
		if entity.ID == 0 || entity.ID == selfID || entity.Team != team || entity.PlayerID == 0 || !entityActivelyBuilding(entity) {
			continue
		}
		other, ok := entityPrimaryPlanLocked(entity)
		if !ok || !other.Breaking || other.Pos != plan.Pos {
			continue
		}
		return true
	}
	return false
}

func (w *World) applyBuilderAIMovementLocked(e *RawEntity, speed, dt float32, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) bool {
	if w == nil || e == nil {
		return false
	}
	w.syncEntityBuilderRuntimeLocked(e)
	state := w.unitAIStates[e.ID]
	profile := w.builderAIProfileLocked(*e)
	if state.BuilderEnemyScanCD > 0 {
		state.BuilderEnemyScanCD -= dt
		if state.BuilderEnemyScanCD < 0 {
			state.BuilderEnemyScanCD = 0
		}
	}
	if state.BuilderAssistScanCD > 0 {
		state.BuilderAssistScanCD -= dt
		if state.BuilderAssistScanCD < 0 {
			state.BuilderAssistScanCD = 0
		}
	}
	if state.BuilderRebuildScanCD > 0 {
		state.BuilderRebuildScanCD -= dt
		if state.BuilderRebuildScanCD < 0 {
			state.BuilderRebuildScanCD = 0
		}
	}
	if state.BuilderBreakCheckCD > 0 {
		state.BuilderBreakCheckCD -= dt
		if state.BuilderBreakCheckCD < 0 {
			state.BuilderBreakCheckCD = 0
		}
	}

	if state.BuilderAssistFollowing != 0 {
		leader, ok := w.entityByIDLocked(state.BuilderAssistFollowing)
		if !ok || leader.Team != e.Team || leader.Health <= 0 {
			state.BuilderAssistFollowing = 0
		}
	}
	if state.BuilderFollowingID != 0 {
		leader, ok := w.entityByIDLocked(state.BuilderFollowingID)
		if !ok || leader.Team != e.Team || leader.Health <= 0 {
			state.BuilderFollowingID = 0
		}
	}
	if state.BuilderAssistFollowing != 0 {
		if assist, ok := w.entityByIDLocked(state.BuilderAssistFollowing); ok && entityActivelyBuilding(assist) {
			state.BuilderFollowingID = state.BuilderAssistFollowing
		}
	}

	if state.BuilderFollowingID != 0 {
		leader, ok := w.entityByIDLocked(state.BuilderFollowingID)
		if !ok || !entityActivelyBuilding(leader) {
			state.BuilderFollowingID = 0
			w.clearEntityBuilderPlansLocked(e, &state)
			e.VelX, e.VelY = 0, 0
			w.unitAIStates[e.ID] = state
			return true
		}
		if leaderPlan, ok := entityPrimaryPlanLocked(leader); ok {
			w.setEntityBuilderPlansLocked(e, []entityBuildPlan{leaderPlan})
			state.BuilderLastPlanQueued = false
		}
		state.BuilderRetreatTimer = 0
	}

	if len(e.Plans) == 0 || profile.AlwaysFlee {
		if state.BuilderEnemyScanCD <= 0 {
			state.BuilderEnemyScanCD = builderAIEnemyScanSec
			state.BuilderThreatEntityID = 0
			state.BuilderThreatBuildPos = 0
			if threat, ok := w.builderAIThreatNearbyLocked(*e, profile.FleeRange, spatial, teamSpatial); ok {
				state.BuilderThreatEntityID = threat.EntityID
				state.BuilderThreatBuildPos = threat.BuildPos
			}
		}
		state.BuilderRetreatTimer += dt
		if (state.BuilderRetreatTimer >= builderAIRetreatDelaySec || profile.AlwaysFlee) &&
			(state.BuilderThreatEntityID != 0 || state.BuilderThreatBuildPos != 0) {
			w.clearEntityBuilderPlansLocked(e, &state)
			if core, ok := w.findNearestFriendlyCoreLocked(*e); ok {
				if !reachedTarget(e.X, e.Y, core.X, core.Y, builderAIRetreatDst) {
					if isEntityFlying(*e) {
						setVelocityToTarget(e, core.X, core.Y, speed, builderAIRetreatDst)
					} else {
						wx, wy, ok := w.nextGroundWaypointLocked(*e, core, builderAIRetreatDst, dt, false)
						if !ok {
							e.VelX, e.VelY = 0, 0
						} else {
							setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
						}
					}
					w.unitAIStates[e.ID] = state
					return true
				}
			}
		}
	}

	if plan, ok := entityPrimaryPlanLocked(*e); ok {
		if !profile.AlwaysFlee {
			state.BuilderRetreatTimer = 0
		}
		if !plan.Breaking && state.BuilderBreakCheckCD <= 0 {
			state.BuilderBreakCheckCD = builderAIBreakCheckSec
			if w.builderAIPlayerBreakingSamePosLocked(e.ID, e.Team, plan) {
				x, y := unpackTilePos(plan.Pos)
				w.clearQueuedRebuildPlanAtLocked(e.Team, int32(x), int32(y))
				w.removeFirstBuilderPlanLocked(e, &state)
				w.unitAIStates[e.ID] = state
				return true
			}
		}
		if w.builderPlanValidLocked(e.Team, plan, state) {
			target, ok := w.builderPlanTargetLocked(plan)
			if !ok {
				w.removeFirstBuilderPlanLocked(e, &state)
				w.unitAIStates[e.ID] = state
				return true
			}
			stopRadius := minf(vanillaBuilderRange-maxf(e.HitRadius*2, 8), builderAIBuildRadius)
			if stopRadius < 24 {
				stopRadius = 24
			}
			if reachedTarget(e.X, e.Y, target.X, target.Y, stopRadius) {
				e.VelX, e.VelY = 0, 0
				e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
				w.unitAIStates[e.ID] = state
				return true
			}
			if isEntityFlying(*e) {
				setVelocityToTarget(e, target.X, target.Y, speed, stopRadius)
				w.unitAIStates[e.ID] = state
				return true
			}
			wx, wy, ok := w.nextGroundWaypointLocked(*e, target, stopRadius, dt, false)
			if !ok {
				e.VelX, e.VelY = 0, 0
				w.unitAIStates[e.ID] = state
				return true
			}
			setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
			w.unitAIStates[e.ID] = state
			return true
		}
		w.removeFirstBuilderPlanLocked(e, &state)
		w.unitAIStates[e.ID] = state
		return true
	}

	if state.BuilderAssistScanCD <= 0 {
		state.BuilderAssistScanCD = builderAIAssistScanSec
		if leader, ok := w.findNearestAssistConstructBuilderLocked(e.Team, e.ID, e.X, e.Y, vanillaBuilderRange, speed, builderAIBuildRadius); ok {
			state.BuilderFollowingID = leader.ID
		}
	}

	if !profile.OnlyAssist && state.BuilderFollowingID == 0 && state.BuilderRebuildScanCD <= 0 {
		state.BuilderRebuildScanCD = profile.RebuildPeriodSec
		acquire := w.acquireNextRebuildPlanLocked
		if rules := w.rulesMgr.Get(); rules != nil && rules.BuildAi {
			acquire = func(team TeamID) (BuildPlanOp, bool) {
				return w.acquireNextBuildAIPlanLocked(team, profile)
			}
		}
		if op, ok := acquire(e.Team); ok {
			if !profile.AlwaysFlee || !w.builderAINearEnemyPlanLocked(e.Team, op.X, op.Y, profile.FleeRange) {
				w.setEntityBuilderPlansLocked(e, []entityBuildPlan{buildPlanEntityFromOp(op)})
				state.BuilderLastPlanQueued = true
				state.BuilderLastPlanX = op.X
				state.BuilderLastPlanY = op.Y
				state.BuilderLastPlanBlockID = op.BlockID
			}
		}
	}

	e.VelX, e.VelY = 0, 0
	w.unitAIStates[e.ID] = state
	return true
}

func (w *World) acquireNextBuildAIPlanLocked(team TeamID, profile builderAIProfile) (BuildPlanOp, bool) {
	if w == nil || w.model == nil || team == 0 {
		return BuildPlanOp{}, false
	}
	plans := w.teamAIBuildPlans[team]
	for len(plans) > 0 {
		head := plans[0]
		if w.teamBuildPlanAlreadyBuiltLocked(head) {
			plans = plans[1:]
			continue
		}
		op := buildPlanOpFromTeamBuildPlan(head)
		if w.evaluateBuildPlanPlacementLocked(team, op) == BuildPlanPlacementReady &&
			(!profile.AlwaysFlee || !w.builderAINearEnemyPlanLocked(team, op.X, op.Y, profile.FleeRange)) {
			if len(plans) > 1 {
				plans = append(plans[1:], head)
			}
			w.teamAIBuildPlans[team] = plans
			return op, true
		}
		if len(plans) > 1 {
			plans = append(plans[1:], head)
		}
		w.teamAIBuildPlans[team] = plans
		return BuildPlanOp{}, false
	}
	delete(w.teamAIBuildPlans, team)
	return BuildPlanOp{}, false
}

func (w *World) selectUnitAITargetLocked(src RawEntity, kind unitAIKind, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) (unitAITarget, bool) {
	if w == nil || w.model == nil || src.Team == 0 {
		return unitAITarget{}, false
	}

	switch kind {
	case unitAIDefender:
		if target, ok := w.selectDefenderTargetLocked(src, spatial, teamSpatial); ok {
			return target, true
		}
	case unitAISuicide, unitAIHug, unitAIGround, unitAIFlying, unitAIFlyingFollow, unitAIMissile:
	}

	allowAir, allowGround := entityAITargetFlags(src)
	localEntityRange := maxf(src.AttackRange*1.35, 120)
	localBuildingRange := maxf(src.AttackRange*1.05, 64)
	entityTarget, entityOK := w.findAutonomousEnemyEntityLocked(src, localEntityRange, allowAir, allowGround, spatial, teamSpatial)
	buildingTarget := unitAITarget{}
	buildingOK := false
	if kind == unitAISuicide || kind == unitAIHug {
		buildingTarget, buildingOK = w.findAutonomousEnemyBuildingLocked(src, localBuildingRange)
		if src.AttackPreferBuildings && buildingOK {
			return buildingTarget, true
		}
	}
	if entityOK {
		return entityTarget, true
	}
	if buildingOK {
		return buildingTarget, true
	}
	if coreTarget, ok := w.findNearestEnemyCoreLocked(src); ok {
		return coreTarget, true
	}
	globalRange := w.maxWorldSeekRangeLocked()
	if buildingTarget, ok := w.findAutonomousEnemyBuildingLocked(src, globalRange); ok {
		return buildingTarget, true
	}
	if entityTarget, ok := w.findAutonomousEnemyEntityLocked(src, globalRange, allowAir, allowGround, spatial, teamSpatial); ok {
		return entityTarget, true
	}
	return unitAITarget{}, false
}

func (w *World) selectDefenderTargetLocked(src RawEntity, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) (unitAITarget, bool) {
	rangeLimit := maxf(src.AttackRange, 400)
	bestID := int32(0)
	bestD2 := float32(math.MaxFloat32)
	visit := func(i int) {
		e := w.model.Entities[i]
		if e.ID == src.ID || e.Health <= 0 || e.Team == 0 || e.Team == src.Team || e.PlayerID == 0 {
			return
		}
		dx := e.X - src.X
		dy := e.Y - src.Y
		d2 := dx*dx + dy*dy
		if d2 > rangeLimit*rangeLimit {
			return
		}
		score := -e.MaxHealth + d2/6400
		if bestID == 0 || score < bestD2 {
			bestID = e.ID
			bestD2 = score
		}
	}
	if len(teamSpatial) != 0 {
		for team, idx := range teamSpatial {
			if team == 0 || team == src.Team || idx == nil {
				continue
			}
			idx.forEachInRange(src.X, src.Y, rangeLimit, visit)
		}
	} else if spatial != nil {
		spatial.forEachInRange(src.X, src.Y, rangeLimit, visit)
	} else {
		for i := range w.model.Entities {
			visit(i)
		}
	}
	if bestID != 0 {
		for i := range w.model.Entities {
			e := w.model.Entities[i]
			if e.ID != bestID {
				continue
			}
			return unitAITarget{
				EntityID: e.ID,
				X:        e.X,
				Y:        e.Y,
				Radius:   maxf(e.HitRadius*0.5, 4),
			}, true
		}
	}
	if core, ok := w.findNearestFriendlyCoreLocked(src); ok {
		return core, true
	}
	if w.isWaveTeamLocked(src.Team) {
		return w.findNearestEnemyCoreLocked(src)
	}
	return unitAITarget{}, false
}

func entityAITargetFlags(e RawEntity) (allowAir, allowGround bool) {
	allowAir = e.AttackTargetAir
	allowGround = e.AttackTargetGround
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	return allowAir, allowGround
}

func (w *World) findAutonomousEnemyEntityLocked(src RawEntity, rangeLimit float32, allowAir, allowGround bool, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) (unitAITarget, bool) {
	targetID, ok := findNearestEnemyEntity(src, w.model.Entities, spatial, teamSpatial, rangeLimit, allowAir, allowGround, src.AttackTargetPriority)
	if !ok {
		return unitAITarget{}, false
	}
	for i := range w.model.Entities {
		e := w.model.Entities[i]
		if e.ID != targetID {
			continue
		}
		return unitAITarget{
			EntityID: e.ID,
			X:        e.X,
			Y:        e.Y,
			Radius:   maxf(e.HitRadius*0.5, 4),
		}, true
	}
	return unitAITarget{}, false
}

func (w *World) findAutonomousEnemyBuildingLocked(src RawEntity, rangeLimit float32) (unitAITarget, bool) {
	pos, tx, ty, ok := w.findNearestEnemyBuilding(src, rangeLimit)
	if !ok || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return unitAITarget{}, false
	}
	tile := &w.model.Tiles[pos]
	size := w.blockSizeForTileLocked(tile)
	return unitAITarget{
		BuildPos: pos,
		X:        tx,
		Y:        ty,
		Radius:   float32(size) * 4,
		IsCore:   isCoreBlockName(w.blockNameByID(int16(tile.Block))),
	}, true
}

func (w *World) findNearestEnemyCoreLocked(src RawEntity) (unitAITarget, bool) {
	bestD2 := float32(math.MaxFloat32)
	bestPos := int32(-1)
	bestX, bestY, bestRadius := float32(0), float32(0), float32(0)
	for team, positions := range w.teamCoreTiles {
		if team == 0 || team == src.Team {
			continue
		}
		for _, pos := range positions {
			if pos < 0 || int(pos) >= len(w.model.Tiles) {
				continue
			}
			tile := &w.model.Tiles[pos]
			if tile.Block == 0 || tile.Build == nil {
				continue
			}
			tx := float32(tile.X*8 + 4)
			ty := float32(tile.Y*8 + 4)
			dx := tx - src.X
			dy := ty - src.Y
			d2 := dx*dx + dy*dy
			if bestPos < 0 || d2 < bestD2 {
				bestD2 = d2
				bestPos = pos
				bestX = tx
				bestY = ty
				bestRadius = float32(w.blockSizeForTileLocked(tile)) * 4
			}
		}
	}
	if bestPos < 0 {
		return unitAITarget{}, false
	}
	return unitAITarget{
		BuildPos: bestPos,
		X:        bestX,
		Y:        bestY,
		Radius:   bestRadius,
		IsCore:   true,
	}, true
}

func (w *World) findNearestFriendlyCoreLocked(src RawEntity) (unitAITarget, bool) {
	bestD2 := float32(math.MaxFloat32)
	bestPos := int32(-1)
	bestX, bestY, bestRadius := float32(0), float32(0), float32(0)
	for _, pos := range w.teamCoreTiles[src.Team] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Block == 0 || tile.Build == nil || tile.Team != src.Team {
			continue
		}
		tx := float32(tile.X*8 + 4)
		ty := float32(tile.Y*8 + 4)
		dx := tx - src.X
		dy := ty - src.Y
		d2 := dx*dx + dy*dy
		if bestPos < 0 || d2 < bestD2 {
			bestD2 = d2
			bestPos = pos
			bestX = tx
			bestY = ty
			bestRadius = float32(w.blockSizeForTileLocked(tile)) * 4
		}
	}
	if bestPos < 0 {
		return unitAITarget{}, false
	}
	return unitAITarget{
		BuildPos: bestPos,
		X:        bestX,
		Y:        bestY,
		Radius:   bestRadius,
		IsCore:   true,
	}, true
}

func (w *World) applyFlyingAIMovementLocked(e *RawEntity, target unitAITarget, speed float32) {
	stopRadius := maxf(maxf(e.AttackRange*0.8, target.Radius+4), 8)
	if reachedTarget(e.X, e.Y, target.X, target.Y, stopRadius) {
		e.VelX, e.VelY = 0, 0
		e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
		return
	}
	setVelocityToTarget(e, target.X, target.Y, speed, stopRadius)
}

func (w *World) applyFlyingFollowAIMovementLocked(e *RawEntity, target unitAITarget, speed float32) {
	if follow, ok := w.selectFlyingFollowLeaderLocked(*e); ok {
		stopRadius := follow.Radius + maxf(e.HitRadius*0.5, 4) + 15
		if !reachedTarget(e.X, e.Y, follow.X, follow.Y, stopRadius) {
			setVelocityToTarget(e, follow.X, follow.Y, speed, stopRadius)
		} else {
			e.VelX, e.VelY = 0, 0
		}
		if target.X != 0 || target.Y != 0 {
			if reachedTarget(e.X, e.Y, target.X, target.Y, maxf(e.AttackRange, 80)) {
				e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
			} else {
				e.Rotation = lookAt(e.X, e.Y, follow.X, follow.Y)
			}
		} else {
			e.Rotation = lookAt(e.X, e.Y, follow.X, follow.Y)
		}
		return
	}
	stopRadius := maxf(target.Radius+4, 80)
	if reachedTarget(e.X, e.Y, target.X, target.Y, stopRadius) {
		e.VelX, e.VelY = 0, 0
		e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
		return
	}
	setVelocityToTarget(e, target.X, target.Y, speed, stopRadius)
}

func (w *World) applyDefenderAIMovementLocked(e *RawEntity, target unitAITarget, speed float32) {
	stopRadius := target.Radius + maxf(e.HitRadius*0.5, 4) + 15
	if reachedTarget(e.X, e.Y, target.X, target.Y, stopRadius) {
		e.VelX, e.VelY = 0, 0
		e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
		return
	}
	setVelocityToTarget(e, target.X, target.Y, speed, stopRadius)
}

func (w *World) applyGroundAIMovementLocked(e *RawEntity, target unitAITarget, speed, dt float32) {
	stopRadius := w.groundAIStopRadiusLocked(*e, target)
	if reachedTarget(e.X, e.Y, target.X, target.Y, stopRadius) {
		e.VelX, e.VelY = 0, 0
		e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
		delete(w.unitAIStates, e.ID)
		return
	}
	wx, wy, ok := w.nextGroundWaypointLocked(*e, target, stopRadius, dt, false)
	if !ok {
		e.VelX, e.VelY = 0, 0
		return
	}
	setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
}

func (w *World) applySuicideAIMovementLocked(e *RawEntity, target unitAITarget, speed, dt float32) {
	stopRadius := maxf(target.Radius+2, 4)
	if w.groundLineClearLocked(e.X, e.Y, target.X, target.Y, stopRadius) {
		setVelocityToTarget(e, target.X, target.Y, speed, stopRadius)
		return
	}
	wx, wy, ok := w.nextGroundWaypointLocked(*e, target, stopRadius, dt, true)
	if !ok {
		e.VelX, e.VelY = 0, 0
		return
	}
	setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
}

func (w *World) applyHugAIMovementLocked(e *RawEntity, target unitAITarget, speed, dt float32) {
	circleRadius := maxf(target.Radius+maxf(e.HitRadius*0.5, 4), 8)
	if w.groundLineClearLocked(e.X, e.Y, target.X, target.Y, circleRadius) {
		if reachedTarget(e.X, e.Y, target.X, target.Y, circleRadius) {
			dx := target.X - e.X
			dy := target.Y - e.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			if dist <= 0.001 {
				e.VelX, e.VelY = 0, 0
			} else {
				sign := float32(1)
				if e.ID%2 == 0 {
					sign = -1
				}
				e.VelX = -dy / dist * speed * sign
				e.VelY = dx / dist * speed * sign
			}
			e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
			return
		}
		setVelocityToTarget(e, target.X, target.Y, speed, circleRadius)
		return
	}
	wx, wy, ok := w.nextGroundWaypointLocked(*e, target, circleRadius, dt, false)
	if !ok {
		e.VelX, e.VelY = 0, 0
		return
	}
	setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
}

func (w *World) applyMissileAIMovementLocked(e *RawEntity, target unitAITarget, speed float32) {
	if target.X != 0 || target.Y != 0 {
		e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
	}
	rad := float64(e.Rotation) * math.Pi / 180
	e.VelX = float32(math.Cos(rad)) * speed
	e.VelY = float32(math.Sin(rad)) * speed
}

func (w *World) selectFlyingFollowLeaderLocked(src RawEntity) (unitAITarget, bool) {
	bestID := int32(0)
	bestScore := float32(math.MaxFloat32)
	searchRange := maxf(src.AttackRange, 400)
	for i := range w.model.Entities {
		other := w.model.Entities[i]
		if other.ID == src.ID || other.Health <= 0 || other.Team != src.Team || other.TypeID == src.TypeID {
			continue
		}
		if w.unitAIKindForEntityLocked(other) == unitAIFlyingFollow {
			continue
		}
		dx := other.X - src.X
		dy := other.Y - src.Y
		d2 := dx*dx + dy*dy
		if d2 > searchRange*searchRange {
			continue
		}
		score := -other.MaxHealth + d2/6400
		if bestID == 0 || score < bestScore {
			bestID = other.ID
			bestScore = score
		}
	}
	if bestID == 0 {
		return unitAITarget{}, false
	}
	for i := range w.model.Entities {
		other := w.model.Entities[i]
		if other.ID != bestID {
			continue
		}
		return unitAITarget{
			EntityID: other.ID,
			X:        other.X,
			Y:        other.Y,
			Radius:   maxf(other.HitRadius*0.5, 4),
		}, true
	}
	return unitAITarget{}, false
}

func (w *World) groundAIStopRadiusLocked(src RawEntity, target unitAITarget) float32 {
	stopRadius := maxf(src.AttackRange*0.75, 12)
	if target.BuildPos != 0 || target.IsCore {
		stopRadius = maxf(stopRadius, target.Radius+4)
	}
	return stopRadius
}

func (w *World) nextGroundWaypointLocked(src RawEntity, target unitAITarget, goalRadius, dt float32, aggressive bool) (float32, float32, bool) {
	state := w.unitAIStates[src.ID]
	speed := maxf(src.MoveSpeed*entitySpeedMultiplier(src), 1)
	stuckThreshold := maxf(1, unitAIStuckRange*2/speed)

	if state.RepathCD > 0 {
		state.RepathCD -= dt
		if state.RepathCD < 0 {
			state.RepathCD = 0
		}
	}
	if reachedTarget(src.X, src.Y, state.StuckX, state.StuckY, unitAIStuckRange) {
		state.StuckTime += dt
	} else {
		state.StuckX = src.X
		state.StuckY = src.Y
		state.StuckTime = 0
	}

	goalChanged := math.Abs(float64(state.GoalX-target.X)) > 4 || math.Abs(float64(state.GoalY-target.Y)) > 4 || math.Abs(float64(state.GoalRadius-goalRadius)) > 2
	waypointReached := state.WaypointX == 0 && state.WaypointY == 0 || reachedTarget(src.X, src.Y, state.WaypointX, state.WaypointY, unitAIWaypointReach)
	forceRepath := aggressive || goalChanged || waypointReached || state.RepathCD <= 0 || state.StuckTime >= stuckThreshold
	if !forceRepath {
		w.unitAIStates[src.ID] = state
		return state.WaypointX, state.WaypointY, true
	}

	state.GoalX = target.X
	state.GoalY = target.Y
	state.GoalRadius = goalRadius
	state.RepathCD = 0.3
	if state.StuckTime >= stuckThreshold {
		state.RepathCD = 0
	}

	if w.groundLineClearLocked(src.X, src.Y, target.X, target.Y, goalRadius) {
		state.WaypointX = target.X
		state.WaypointY = target.Y
		state.StuckX = src.X
		state.StuckY = src.Y
		state.StuckTime = 0
		w.unitAIStates[src.ID] = state
		return state.WaypointX, state.WaypointY, true
	}

	wx, wy, ok := w.findGroundWaypointLocked(src.X, src.Y, target.X, target.Y, goalRadius)
	if !ok {
		state.WaypointX = 0
		state.WaypointY = 0
		w.unitAIStates[src.ID] = state
		return 0, 0, false
	}
	state.WaypointX = wx
	state.WaypointY = wy
	state.StuckX = src.X
	state.StuckY = src.Y
	state.StuckTime = 0
	w.unitAIStates[src.ID] = state
	return wx, wy, true
}

func (w *World) findGroundWaypointLocked(fromX, fromY, toX, toY, goalRadius float32) (float32, float32, bool) {
	if w == nil || w.model == nil || w.model.Width <= 0 || w.model.Height <= 0 {
		return 0, 0, false
	}
	startX := int(clampf(float32(int(fromX/8)), 0, float32(w.model.Width-1)))
	startY := int(clampf(float32(int(fromY/8)), 0, float32(w.model.Height-1)))
	start := int32(startY*w.model.Width + startX)
	if reachedTarget(fromX, fromY, toX, toY, goalRadius) {
		return toX, toY, true
	}

	prev := make([]int32, len(w.model.Tiles))
	for i := range prev {
		prev[i] = -2
	}
	queue := make([]int32, 0, 64)
	queue = append(queue, start)
	prev[start] = -1

	found := int32(-1)
	head := 0
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for head < len(queue) {
		cur := queue[head]
		head++
		cx := int(cur % int32(w.model.Width))
		cy := int(cur / int32(w.model.Width))
		wx, wy := tileCenterWorld(cx, cy)
		if (cur == start || !w.groundCellBlockedLocked(cx, cy)) && reachedTarget(wx, wy, toX, toY, goalRadius) {
			found = cur
			break
		}
		for _, dir := range dirs {
			nx := cx + dir[0]
			ny := cy + dir[1]
			if nx < 0 || ny < 0 || nx >= w.model.Width || ny >= w.model.Height {
				continue
			}
			next := int32(ny*w.model.Width + nx)
			if prev[next] != -2 {
				continue
			}
			if next != start && w.groundCellBlockedLocked(nx, ny) {
				continue
			}
			prev[next] = cur
			queue = append(queue, next)
		}
	}
	if found < 0 {
		return 0, 0, false
	}

	step := found
	for prev[step] >= 0 && prev[step] != start {
		step = prev[step]
	}
	if prev[step] == -1 {
		return toX, toY, true
	}
	x := int(step % int32(w.model.Width))
	y := int(step / int32(w.model.Width))
	wx, wy := tileCenterWorld(x, y)
	return wx, wy, true
}

func (w *World) groundLineClearLocked(fromX, fromY, toX, toY, ignoreRadius float32) bool {
	dx := toX - fromX
	dy := toY - fromY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist <= 0.001 {
		return true
	}
	steps := int(dist/4) + 1
	for i := 1; i < steps; i++ {
		t := float32(i) / float32(steps)
		x := fromX + dx*t
		y := fromY + dy*t
		if reachedTarget(x, y, toX, toY, ignoreRadius) {
			return true
		}
		tx := int(x / 8)
		ty := int(y / 8)
		if w.groundCellBlockedLocked(tx, ty) {
			return false
		}
	}
	return true
}

func (w *World) groundCellBlockedLocked(x, y int) bool {
	if w == nil || w.model == nil || !w.model.InBounds(x, y) {
		return true
	}
	if _, ok := w.buildingOccupyingCellLocked(x, y); ok {
		return true
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	return tile.Block != 0 && tile.Build == nil
}

func tileCenterWorld(x, y int) (float32, float32) {
	return float32(x*8 + 4), float32(y*8 + 4)
}

func (w *World) maxWorldSeekRangeLocked() float32 {
	if w == nil || w.model == nil {
		return 0
	}
	width := float32(w.model.Width * 8)
	height := float32(w.model.Height * 8)
	return float32(math.Sqrt(float64(width*width + height*height)))
}

func (w *World) teamsFromRulesLocked() (defaultTeam, waveTeam TeamID) {
	defaultTeam = 1
	waveTeam = 2
	if w == nil || w.rulesMgr == nil {
		return defaultTeam, waveTeam
	}
	rules := w.rulesMgr.Get()
	if rules == nil {
		return defaultTeam, waveTeam
	}
	if parsed, ok := parseTeamKey(rules.DefaultTeam); ok && parsed != 0 {
		defaultTeam = parsed
	}
	if parsed, ok := parseTeamKey(rules.WaveTeam); ok && parsed != 0 {
		waveTeam = parsed
	}
	if waveTeam == defaultTeam {
		if defaultTeam != 2 {
			waveTeam = 2
		} else {
			waveTeam = 3
		}
	}
	return defaultTeam, waveTeam
}

func (w *World) isWaveTeamLocked(team TeamID) bool {
	_, waveTeam := w.teamsFromRulesLocked()
	return team != 0 && team == waveTeam
}

func (w *World) pickWaveSpawnPositionLocked(unitType int16, team TeamID) (float32, float32, bool) {
	if w == nil || w.model == nil || w.model.Width <= 0 || w.model.Height <= 0 {
		return 0, 0, false
	}
	prof, _ := w.unitRuntimeProfileForTypeLocked(unitType)
	flying := prof.Flying
	bestX, bestY := -1, -1
	bestScore := float32(-1)
	tryCell := func(x, y int) {
		if x < 0 || y < 0 || x >= w.model.Width || y >= w.model.Height {
			return
		}
		if !flying && w.groundCellBlockedLocked(x, y) {
			return
		}
		wx, wy := tileCenterWorld(x, y)
		score := float32(math.MaxFloat32)
		foundEnemyCore := false
		for otherTeam, positions := range w.teamCoreTiles {
			if otherTeam == 0 || otherTeam == team {
				continue
			}
			for _, pos := range positions {
				if pos < 0 || int(pos) >= len(w.model.Tiles) {
					continue
				}
				tile := &w.model.Tiles[pos]
				if tile.Build == nil || tile.Block == 0 {
					continue
				}
				foundEnemyCore = true
				cx, cy := tileCenterWorld(tile.X, tile.Y)
				dx := cx - wx
				dy := cy - wy
				d2 := dx*dx + dy*dy
				if d2 < score {
					score = d2
				}
			}
		}
		if !foundEnemyCore {
			score = -float32(math.Abs(float64(wx-float32(w.model.Width*4)))) - float32(math.Abs(float64(wy-float32(w.model.Height*4))))
		}
		if bestX < 0 || score > bestScore {
			bestX = x
			bestY = y
			bestScore = score
		}
	}
	for x := 0; x < w.model.Width; x++ {
		tryCell(x, 0)
		tryCell(x, w.model.Height-1)
	}
	for y := 1; y < w.model.Height-1; y++ {
		tryCell(0, y)
		tryCell(w.model.Width-1, y)
	}
	if bestX < 0 {
		bestX = w.model.Width - 1
		bestY = w.model.Height / 2
	}
	wx, wy := tileCenterWorld(bestX, bestY)
	return wx, wy, true
}
