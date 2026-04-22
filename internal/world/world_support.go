package world

import (
	"time"
)

// Support building profiles
type mendProjectorProfile struct {
	Range           float32
	Reload          float32
	HealPercent     float32
	PhaseBoost      float32
	PhaseRangeBoost float32
	UseTime         float32
}

type overdriveProjectorProfile struct {
	Range           float32
	Reload          float32
	SpeedBoost      float32
	SpeedBoostPhase float32
	PhaseRangeBoost float32
	UseTime         float32
}

type forceProjectorProfile struct {
	Radius                float32
	PhaseRadiusBoost      float32
	PhaseShieldBoost      float32
	ShieldHealth          float32
	CooldownNormal        float32
	CooldownLiquid        float32
	CooldownBrokenBase    float32
	CoolantConsumption    float32
	CrashDamageMultiplier float32
	PhaseUseTime          float32
	Sides                 int
}

var mendProjectorProfiles = map[string]mendProjectorProfile{
	"mend-projector": {
		Range:           85,  // Java: range = 85f
		Reload:          250, // Java: reload = 250f
		HealPercent:     11,  // Java: healPercent = 11f
		PhaseBoost:      15,  // Java: phaseBoost = 15f
		PhaseRangeBoost: 50,  // Default from MendProjector.java
		UseTime:         400, // Default from MendProjector.java
	},
}

var overdriveProjectorProfiles = map[string]overdriveProjectorProfile{
	"overdrive-projector": {
		Range:           80,
		Reload:          60,
		SpeedBoost:      1.5,
		SpeedBoostPhase: 0.75,
		PhaseRangeBoost: 20,
		UseTime:         400,
	},
	"overdrive-dome": {
		Range:           200,
		Reload:          60,
		SpeedBoost:      2.0,
		SpeedBoostPhase: 1.0,
		PhaseRangeBoost: 50,
		UseTime:         480,
	},
}

var forceProjectorProfiles = map[string]forceProjectorProfile{
	"force-projector": {
		Radius:                101.7, // Java: radius = 101.7f
		PhaseRadiusBoost:      80,    // Java: phaseRadiusBoost = 80f
		PhaseShieldBoost:      400,   // Default from ForceProjector.java
		ShieldHealth:          750,   // Java: shieldHealth = 750f
		CooldownNormal:        1.5,   // Java: cooldownNormal = 1.5f
		CooldownLiquid:        1.2,   // Java: cooldownLiquid = 1.2f
		CooldownBrokenBase:    0.35,  // Java: cooldownBrokenBase = 0.35f
		CoolantConsumption:    0.1,   // Default from ForceProjector.java
		CrashDamageMultiplier: 2.0,   // Default from ForceProjector.java
		PhaseUseTime:          350,   // Default from ForceProjector.java
		Sides:                 6,     // Default from ForceProjector.java
	},
}

type mendProjectorState struct {
	Heat             float32
	Charge           float32
	PhaseHeat        float32
	SmoothEfficiency float32
}

type overdriveProjectorState struct {
	Heat             float32
	Charge           float32
	PhaseHeat        float32
	SmoothEfficiency float32
	UseProgress      float32
}

type forceProjectorState struct {
	Broken    bool
	Buildup   float32
	Radscl    float32
	Hit       float32
	Warmup    float32
	PhaseHeat float32
}

func (w *World) stepSupportBuildingsLocked(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}

	dt := float32(delta.Seconds())
	deltaFrames := dt * 60.0

	if dt <= 0 {
		return
	}

	// Step mend projectors
	for _, pos := range w.mendProjectorPositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Team == 0 || tile.Block == 0 {
			continue
		}

		name := w.blockNameByID(int16(tile.Block))
		prof, ok := mendProjectorProfiles[name]
		if !ok {
			continue
		}

		state := w.mendProjectorStateLocked(pos)
		w.stepMendProjectorLocked(pos, tile, prof, state, dt, deltaFrames)
	}

	// Step overdrive projectors
	for _, pos := range w.overdriveProjectorPositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Team == 0 || tile.Block == 0 {
			continue
		}

		name := w.blockNameByID(int16(tile.Block))
		prof, ok := overdriveProjectorProfiles[name]
		if !ok {
			continue
		}

		state := w.overdriveProjectorStateLocked(pos)
		w.stepOverdriveProjectorLocked(pos, tile, prof, state, dt, deltaFrames)
	}

	// Step force projectors
	for _, pos := range w.forceProjectorPositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Team == 0 || tile.Block == 0 {
			continue
		}

		name := w.blockNameByID(int16(tile.Block))
		prof, ok := forceProjectorProfiles[name]
		if !ok {
			continue
		}

		state := w.forceProjectorStateLocked(pos)
		w.stepForceProjectorLocked(pos, tile, prof, state, dt, deltaFrames)
	}
}

func (w *World) stepMendProjectorLocked(pos int32, tile *Tile, prof mendProjectorProfile, state *mendProjectorState, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	canHeal := !w.isBuildingHealSuppressedLocked(tile.Build)

	efficiency := float32(1.0)
	if tile.Build.Health <= 0 {
		efficiency = 0
	}

	state.SmoothEfficiency = approachf(state.SmoothEfficiency, efficiency, 0.08*deltaFrames)

	targetHeat := float32(0)
	if efficiency > 0 && canHeal {
		targetHeat = 1
	}
	state.Heat = approachf(state.Heat, targetHeat, 0.08*deltaFrames)

	state.Charge += state.Heat * deltaFrames

	// Phase heat (simplified - assume no phase fabric for now)
	state.PhaseHeat = approachf(state.PhaseHeat, 0, 0.1*deltaFrames)

	if state.Charge >= prof.Reload && canHeal {
		realRange := prof.Range + state.PhaseHeat*prof.PhaseRangeBoost
		state.Charge = 0

		// Heal nearby buildings
		w.healBuildingsInRangeLocked(tile, realRange, prof.HealPercent, prof.PhaseBoost, state.PhaseHeat, efficiency)
	}
}

func (w *World) healBuildingsInRangeLocked(source *Tile, radius, healPercent, phaseBoost, phaseHeat, efficiency float32) {
	if w == nil || w.model == nil || source == nil || source.Build == nil {
		return
	}

	radiusSq := radius * radius
	sourceX := float32(source.X) * 8
	sourceY := float32(source.Y) * 8

	tileRadius := int(radius/8) + 1
	centerX := source.X
	centerY := source.Y

	for dy := -tileRadius; dy <= tileRadius; dy++ {
		for dx := -tileRadius; dx <= tileRadius; dx++ {
			tileX := centerX + dx
			tileY := centerY + dy

			if !w.model.InBounds(tileX, tileY) {
				continue
			}

			pos := int32(tileY*w.model.Width + tileX)
			if pos < 0 || int(pos) >= len(w.model.Tiles) {
				continue
			}

			tile := &w.model.Tiles[pos]
			// CRITICAL: Only heal buildings, NOT units
			// MendProjector should only affect tile.Build, never entities
			if tile.Build == nil || tile.Build.Team != source.Build.Team {
				continue
			}
			if w.isBuildingHealSuppressedLocked(tile.Build) {
				continue
			}

			if tile.Build.Health >= tile.Build.MaxHealth {
				continue
			}

			buildX := float32(tileX) * 8
			buildY := float32(tileY) * 8
			dx2 := buildX - sourceX
			dy2 := buildY - sourceY
			distSq := dx2*dx2 + dy2*dy2

			if distSq < radiusSq {
				healAmount := tile.Build.MaxHealth * (healPercent + phaseHeat*phaseBoost) / 100.0 * efficiency
				_ = w.healBuilding(pos, healAmount)
			}
		}
	}
}

func (w *World) stepOverdriveProjectorLocked(pos int32, tile *Tile, prof overdriveProjectorProfile, state *overdriveProjectorState, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}

	efficiency := float32(1.0)
	if tile.Build.Health <= 0 {
		efficiency = 0
	}

	state.SmoothEfficiency = approachf(state.SmoothEfficiency, efficiency, 0.08*deltaFrames)

	targetHeat := float32(0)
	if efficiency > 0 {
		targetHeat = 1
	}
	state.Heat = approachf(state.Heat, targetHeat, 0.08*deltaFrames)

	state.Charge += state.Heat * deltaFrames

	// Phase heat (simplified)
	state.PhaseHeat = approachf(state.PhaseHeat, 0, 0.1*deltaFrames)

	if state.Charge >= prof.Reload {
		realRange := prof.Range + state.PhaseHeat*prof.PhaseRangeBoost
		state.Charge = 0

		realBoost := (prof.SpeedBoost + state.PhaseHeat*prof.SpeedBoostPhase) * efficiency

		// Apply boost to nearby buildings
		w.applyBoostToBuildingsLocked(tile, realRange, realBoost, prof.Reload+1)
	}

	if efficiency > 0 {
		state.UseProgress += deltaFrames
	}

	if state.UseProgress >= prof.UseTime {
		state.UseProgress = 0
	}
}

func (w *World) applyBoostToBuildingsLocked(source *Tile, radius, boost, duration float32) {
	if w == nil || w.model == nil || source == nil {
		return
	}

	radiusSq := radius * radius
	sourceX := float32(source.X) * 8
	sourceY := float32(source.Y) * 8

	tileRadius := int(radius/8) + 1
	centerX := source.X
	centerY := source.Y

	for dy := -tileRadius; dy <= tileRadius; dy++ {
		for dx := -tileRadius; dx <= tileRadius; dx++ {
			tileX := centerX + dx
			tileY := centerY + dy

			if !w.model.InBounds(tileX, tileY) {
				continue
			}

			pos := int32(tileY*w.model.Width + tileX)
			if pos < 0 || int(pos) >= len(w.model.Tiles) {
				continue
			}

			tile := &w.model.Tiles[pos]
			if tile.Build == nil || tile.Build.Team != source.Build.Team {
				continue
			}

			buildX := float32(tileX) * 8
			buildY := float32(tileY) * 8
			dx2 := buildX - sourceX
			dy2 := buildY - sourceY
			distSq := dx2*dx2 + dy2*dy2

			if distSq < radiusSq {
				// Apply speed boost (simplified - would need boost tracking in Building)
				// For now, just mark that building received boost
			}
		}
	}
}

func (w *World) stepForceProjectorLocked(pos int32, tile *Tile, prof forceProjectorProfile, state *forceProjectorState, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}

	efficiency := float32(1.0)
	if tile.Build.Health <= 0 {
		efficiency = 0
	}

	// Phase heat (simplified)
	state.PhaseHeat = approachf(state.PhaseHeat, 0, 0.1*deltaFrames)

	// Update radscl
	targetRadscl := float32(0)
	if !state.Broken {
		targetRadscl = state.Warmup
	}
	state.Radscl = approachf(state.Radscl, targetRadscl, 0.05*deltaFrames)

	state.Warmup = approachf(state.Warmup, efficiency, 0.1*deltaFrames)

	// Cooldown buildup
	if state.Buildup > 0 {
		scale := prof.CooldownNormal
		if state.Broken {
			scale = prof.CooldownBrokenBase
		}

		// Coolant boost (simplified)
		if tile.Build.LiquidAmount(waterLiquidID) > 0 {
			liquidHeat := float32(1.0 + (1.0-0.4)*0.9) // water heatCapacity approximation
			scale *= prof.CooldownLiquid * liquidHeat

			consume := minf(prof.CoolantConsumption*dt, tile.Build.LiquidAmount(waterLiquidID))
			tile.Build.RemoveLiquid(waterLiquidID, consume)
		}

		state.Buildup -= deltaFrames * scale
		if state.Buildup < 0 {
			state.Buildup = 0
		}
	}

	// Repair shield
	if state.Broken && state.Buildup <= 0 {
		state.Broken = false
	}

	// Break shield
	maxShield := prof.ShieldHealth + prof.PhaseShieldBoost*state.PhaseHeat
	if state.Buildup >= maxShield && !state.Broken {
		state.Broken = true
		state.Buildup = prof.ShieldHealth
	}

	// Decay hit effect
	if state.Hit > 0 {
		state.Hit -= deltaFrames / 5.0
		if state.Hit < 0 {
			state.Hit = 0
		}
	}

	// Deflect bullets
	if !state.Broken {
		w.deflectBulletsLocked(tile, prof, state)
	}
}

func (w *World) deflectBulletsLocked(tile *Tile, prof forceProjectorProfile, state *forceProjectorState) {
	if w == nil || tile == nil || state == nil {
		return
	}

	realRadius := (prof.Radius + state.PhaseHeat*prof.PhaseRadiusBoost) * state.Radscl
	if realRadius <= 0 {
		return
	}

	centerX := float32(tile.X) * 8
	centerY := float32(tile.Y) * 8

	// Check bullets
	for i := len(w.bullets) - 1; i >= 0; i-- {
		b := &w.bullets[i]

		if b.Team == tile.Build.Team {
			continue
		}

		dx := b.X - centerX
		dy := b.Y - centerY
		dist := sqrtf(dx*dx + dy*dy)

		if dist < realRadius {
			// Absorb bullet
			state.Hit = 1.0
			state.Buildup += b.Damage

			// Remove bullet
			w.bullets = append(w.bullets[:i], w.bullets[i+1:]...)
		}
	}
}

func (w *World) mendProjectorStateLocked(pos int32) *mendProjectorState {
	if w.mendProjectorStates == nil {
		w.mendProjectorStates = map[int32]*mendProjectorState{}
	}
	st := w.mendProjectorStates[pos]
	if st == nil {
		st = &mendProjectorState{
			Charge: 0,
		}
		w.mendProjectorStates[pos] = st
	}
	return st
}

func (w *World) overdriveProjectorStateLocked(pos int32) *overdriveProjectorState {
	if w.overdriveProjectorStates == nil {
		w.overdriveProjectorStates = map[int32]*overdriveProjectorState{}
	}
	st := w.overdriveProjectorStates[pos]
	if st == nil {
		st = &overdriveProjectorState{
			Charge: 0,
		}
		w.overdriveProjectorStates[pos] = st
	}
	return st
}

func (w *World) forceProjectorStateLocked(pos int32) *forceProjectorState {
	if w.forceProjectorStates == nil {
		w.forceProjectorStates = map[int32]*forceProjectorState{}
	}
	st := w.forceProjectorStates[pos]
	if st == nil {
		st = &forceProjectorState{
			Broken: true,
			Radscl: 0,
		}
		w.forceProjectorStates[pos] = st
	}
	return st
}
