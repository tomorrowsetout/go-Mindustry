package world

import (
	"log"
	"math/rand"
	"strings"
	"time"
)

const (
	turretRotateSpeed       = float32(5.0)
	turretActivationTime    = float32(0.0)
	turretCoolantMultiplier = float32(5.0)
)

type turretProfile struct {
	Range              float32
	RotateSpeed        float32
	Reload             float32
	ReloadTime         float32
	Recoil             float32
	RecoilTime         float32
	Cooldown           float32
	Inaccuracy         float32
	VelocityInaccuracy float32
	Shake              float32
	Shots              int
	Spread             float32
	BurstSpacing       float32
	ShootCone          float32
	ShootSound         string
	MinWarmup          float32
	AccurateShooting   bool
	TargetAir          bool
	TargetGround       bool
	TargetHealing      bool
	PredictTarget      bool
	AmmoPerShot        int
	MaxAmmo            int
	AmmoEjectBack      float32
	ChargeTime         float32
	ChargeEffects      int
	ChargeMaxDelay     float32
	ShootWarmupSpeed   float32
	MinRange           float32
	AlternateShoot     bool
	ShootX             float32
	ShootY             float32
	XRand              float32
}

var turretProfilesByBlockName = map[string]turretProfile{
	"duo": {
		Range: 120, RotateSpeed: 10, Reload: 20, Recoil: 1, RecoilTime: 10, Cooldown: 0.03,
		Inaccuracy: 2, Shake: 1, Shots: 1, ShootCone: 8, TargetAir: true, TargetGround: true,
		AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: true,
	},
	"scatter": {
		Range: 160, RotateSpeed: 10, Reload: 18, Recoil: 3, RecoilTime: 20, Cooldown: 0.03,
		Inaccuracy: 17, VelocityInaccuracy: 0.2, Shake: 2, Shots: 3, Spread: 25, ShootCone: 45,
		TargetAir: true, TargetGround: true, AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: true,
	},
	"scorch": {
		Range: 80, RotateSpeed: 5, Reload: 40, Recoil: 0, RecoilTime: 30, Cooldown: 0.06,
		Inaccuracy: 5, Shake: 2, Shots: 1, ShootCone: 50, TargetAir: false, TargetGround: true,
		AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: false,
	},
	"hail": {
		Range: 200, RotateSpeed: 7, Reload: 16, Recoil: 1, RecoilTime: 10, Cooldown: 0.03,
		Inaccuracy: 1, Shake: 1, Shots: 1, ShootCone: 8, TargetAir: true, TargetGround: true,
		AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: true,
	},
	"wave": {
		Range: 120, RotateSpeed: 3, Reload: 4, Recoil: 0, RecoilTime: 0, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 0, Shots: 1, ShootCone: 50, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: false, MinWarmup: 0.94,
	},
	"lancer": {
		Range: 165, RotateSpeed: 2, Reload: 80, Recoil: 2, RecoilTime: 30, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 2, Shots: 1, ShootCone: 10, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: true, ChargeTime: 60, ChargeEffects: 7, ChargeMaxDelay: 10,
		ShootWarmupSpeed: 0.1,
	},
	"arc": {
		Range: 190, RotateSpeed: 8, Reload: 35, Recoil: 1, RecoilTime: 5, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 1, Shots: 1, ShootCone: 8, TargetAir: true, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: false,
	},
	"swarmer": {
		Range: 240, RotateSpeed: 5, Reload: 60, Recoil: 0.5, RecoilTime: 8, Cooldown: 0.03,
		Inaccuracy: 10, VelocityInaccuracy: 0.1, Shake: 1, Shots: 4, BurstSpacing: 5, ShootCone: 30,
		TargetAir: true, TargetGround: false, AmmoPerShot: 1, MaxAmmo: 40, PredictTarget: true,
	},
	"salvo": {
		Range: 200, RotateSpeed: 4, Reload: 31, Recoil: 3, RecoilTime: 20, Cooldown: 0.03,
		Inaccuracy: 2, Shake: 1, Shots: 4, BurstSpacing: 5, ShootCone: 15, TargetAir: true, TargetGround: true,
		AmmoPerShot: 1, MaxAmmo: 40, PredictTarget: true, AlternateShoot: true, Spread: 3, ShootY: 3,
	},
	"ripple": {
		Range: 320, RotateSpeed: 3, Reload: 60, Recoil: 5, RecoilTime: 60, Cooldown: 0.03,
		Inaccuracy: 5, VelocityInaccuracy: 0.2, Shake: 3, Shots: 4, BurstSpacing: 5, ShootCone: 30,
		TargetAir: true, TargetGround: true, AmmoPerShot: 1, MaxAmmo: 50, PredictTarget: true,
	},
	"cyclone": {
		Range: 190, RotateSpeed: 8, Reload: 7, Recoil: 0.5, RecoilTime: 8, Cooldown: 0.03,
		Inaccuracy: 12, VelocityInaccuracy: 0.125, Shake: 0.5, Shots: 2, ShootCone: 20,
		TargetAir: true, TargetGround: false, AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: true,
	},
	"fuse": {
		Range: 240, RotateSpeed: 3, Reload: 35, Recoil: 3, RecoilTime: 30, Cooldown: 0.03,
		Inaccuracy: 1, Shake: 2, Shots: 1, ShootCone: 10, TargetAir: true, TargetGround: true,
		AmmoPerShot: 1, MaxAmmo: 30, PredictTarget: true,
	},
	"spectre": {
		Range: 260, RotateSpeed: 2, Reload: 6, Recoil: 2, RecoilTime: 15, Cooldown: 0.03,
		Inaccuracy: 3, VelocityInaccuracy: 0.1, Shake: 1, Shots: 2, ShootCone: 20,
		TargetAir: true, TargetGround: true, AmmoPerShot: 1, MaxAmmo: 50, PredictTarget: true,
		AlternateShoot: true, Spread: 4, ShootY: 3, XRand: 4,
	},
	"meltdown": {
		Range: 230, RotateSpeed: 1, Reload: 200, Recoil: 4, RecoilTime: 120, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 4, Shots: 1, ShootCone: 40, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: true, ChargeTime: 60, ChargeEffects: 10,
		ChargeMaxDelay: 30, ShootWarmupSpeed: 0.03,
	},
	"foreshadow": {
		Range: 500, RotateSpeed: 0.7, Reload: 400, Recoil: 5, RecoilTime: 240, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 4, Shots: 1, ShootCone: 2, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: true, ChargeTime: 180, ChargeEffects: 30,
		ChargeMaxDelay: 30, ShootWarmupSpeed: 0.019, MinRange: 140,
	},
	"segment": {
		Range: 220, RotateSpeed: 5, Reload: 30, Recoil: 2, RecoilTime: 20, Cooldown: 0.03,
		Inaccuracy: 3, Shake: 1, Shots: 1, ShootCone: 6, TargetAir: true, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: true,
	},
	"parallax": {
		Range: 280, RotateSpeed: 2, Reload: 60, Recoil: 4, RecoilTime: 30, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 3, Shots: 1, ShootCone: 40, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: true, ChargeTime: 50, ChargeEffects: 8,
		ChargeMaxDelay: 10, ShootWarmupSpeed: 0.08,
	},
	"tsunami": {
		Range: 190, RotateSpeed: 2, Reload: 4, Recoil: 0, RecoilTime: 0, Cooldown: 0.03,
		Inaccuracy: 0, Shake: 0, Shots: 1, ShootCone: 50, TargetAir: false, TargetGround: true,
		AmmoPerShot: 0, MaxAmmo: 0, PredictTarget: false, MinWarmup: 0.94,
	},
}

var turretItemAmmoMultipliersByName = map[string]map[ItemID]int32{
	"duo": {
		copperItemID:   2,
		graphiteItemID: 4,
		siliconItemID:  5,
	},
	"scatter": {
		scrapItemID:     5,
		leadItemID:      4,
		metaglassItemID: 5,
	},
	"scorch": {
		coalItemID:     3,
		pyratiteItemID: 10,
	},
	"hail": {
		graphiteItemID: 1,
		siliconItemID:  3,
		pyratiteItemID: 4,
	},
	"swarmer": {
		blastCompoundItemID: 5,
		pyratiteItemID:      5,
		surgeAlloyItemID:    4,
	},
	"salvo": {
		copperItemID:   5,
		graphiteItemID: 4,
		pyratiteItemID: 5,
		siliconItemID:  5,
		thoriumItemID:  4,
	},
	"fuse": {
		titaniumItemID: 4,
		thoriumItemID:  5,
	},
	"ripple": {
		graphiteItemID:      1,
		siliconItemID:       3,
		pyratiteItemID:      4,
		blastCompoundItemID: 4,
		plastaniumItemID:    1,
	},
	"cyclone": {
		metaglassItemID:     2,
		blastCompoundItemID: 5,
		plastaniumItemID:    4,
		surgeAlloyItemID:    5,
	},
	"foreshadow": {
		surgeAlloyItemID: 1,
	},
	"spectre": {
		graphiteItemID: 4,
		thoriumItemID:  1,
		pyratiteItemID: 3,
	},
	"breach": {
		berylliumItemID: 1,
		tungstenItemID:  2,
		carbideItemID:   2,
	},
	"diffuse": {
		graphiteItemID: 1,
		oxideItemID:    2,
		siliconItemID:  1,
	},
	"titan": {
		thoriumItemID: 1,
		carbideItemID: 1,
		oxideItemID:   1,
	},
	"disperse": {
		tungstenItemID:   3,
		thoriumItemID:    1,
		siliconItemID:    4,
		surgeAlloyItemID: 3,
	},
	"scathe": {
		carbideItemID:     1,
		phaseFabricItemID: 5,
		surgeAlloyItemID:  1,
	},
	"smite": {
		surgeAlloyItemID: 1,
	},
}

type turretRuntimeState struct {
	Rotation        float32
	ActivationTimer float32
	ReloadCounter   float32
	Heat            float32
	Recoil          float32
	Charge          float32
	Warmup          float32
	BurstCounter    int
	WasShooting     bool
	LastTarget      int32
}

func (w *World) stepTurretsLocked(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}

	dt := float32(delta.Seconds())
	deltaFrames := dt * 60.0

	if dt <= 0 {
		return
	}

	for _, pos := range w.turretTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Team == 0 || tile.Block == 0 {
			continue
		}

		name := w.blockNameByID(int16(tile.Block))
		prof, ok := turretProfilesByBlockName[name]
		if !ok {
			continue
		}

		state := w.turretStateLocked(pos)
		w.stepSingleTurretLocked(pos, tile, name, prof, state, dt, deltaFrames)
	}
}

func (w *World) stepSingleTurretLocked(pos int32, tile *Tile, name string, prof turretProfile, state *turretRuntimeState, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}

	// Activation timer
	if state.ActivationTimer > 0 {
		state.ActivationTimer -= deltaFrames
		if state.ActivationTimer < 0 {
			state.ActivationTimer = 0
		}
	}

	// Update heat
	state.Heat = approachf(state.Heat, 0, prof.Cooldown*deltaFrames)

	// Update recoil
	if state.Recoil > 0 {
		state.Recoil = approachf(state.Recoil, 0, deltaFrames/maxf(prof.RecoilTime, 1))
	}

	// Find target
	target := w.turretFindTargetLocked(pos, tile, prof)
	state.LastTarget = target

	// Update rotation
	if target >= 0 {
		w.turretTurnToTargetLocked(pos, tile, target, prof, state, deltaFrames)
	}

	// Check if can shoot
	canShoot := target >= 0 && state.ActivationTimer <= 0 && tile.Build.Health > 0

	// Update warmup
	targetWarmup := float32(0)
	if canShoot {
		targetWarmup = 1
	}

	if prof.ShootWarmupSpeed > 0 {
		state.Warmup = approachf(state.Warmup, targetWarmup, prof.ShootWarmupSpeed*deltaFrames)
	} else {
		state.Warmup = targetWarmup
	}

	// Update charge
	if prof.ChargeTime > 0 {
		if canShoot && state.Warmup >= prof.MinWarmup {
			state.Charge += deltaFrames
		} else {
			state.Charge = 0
		}
	}

	// Update reload
	if state.ReloadCounter < prof.Reload {
		state.ReloadCounter += deltaFrames

		// Coolant boost - simplified
		if tile.Build.LiquidAmount(waterLiquidID) > 0 {
			boost := minf(tile.Build.LiquidAmount(waterLiquidID), 0.2*deltaFrames)
			state.ReloadCounter += boost * 1.5 * deltaFrames
			tile.Build.RemoveLiquid(waterLiquidID, boost)
		}
	}

	// Try to shoot
	if canShoot && w.turretCanShootLocked(pos, tile, target, prof, state) {
		w.turretShootLocked(pos, tile, target, name, prof, state)
	}

	state.WasShooting = canShoot && state.Warmup >= prof.MinWarmup
}

func (w *World) turretFindTargetLocked(pos int32, tile *Tile, prof turretProfile) int32 {
	if w == nil || tile == nil || tile.Build == nil {
		return -1
	}

	// Simplified - no unit targeting for now
	// Actual implementation would iterate through w.model.Entities
	return -1
}

func (w *World) turretTurnToTargetLocked(pos int32, tile *Tile, target int32, prof turretProfile, state *turretRuntimeState, deltaFrames float32) {
	if w == nil || tile == nil || state == nil || target < 0 {
		return
	}

	// Simplified rotation logic
	// Actual implementation would calculate angle to target
}

func (w *World) turretCanShootLocked(pos int32, tile *Tile, target int32, prof turretProfile, state *turretRuntimeState) bool {
	if state == nil || target < 0 {
		return false
	}

	// Check warmup
	if state.Warmup < prof.MinWarmup {
		return false
	}

	// Check reload
	if state.ReloadCounter < prof.Reload {
		return false
	}

	// Check charge
	if prof.ChargeTime > 0 && state.Charge < prof.ChargeTime {
		return false
	}

	// Check ammo
	if prof.MaxAmmo > 0 && !w.turretHasAmmoLocked(tile) {
		return false
	}

	// Simplified - assume can shoot if all checks pass
	return true
}

func (w *World) turretShootLocked(pos int32, tile *Tile, target int32, name string, prof turretProfile, state *turretRuntimeState) {
	if state == nil {
		return
	}

	// Consume ammo
	if prof.MaxAmmo > 0 {
		w.turretConsumeAmmoLocked(tile, prof.AmmoPerShot)
	}

	// Reset reload
	state.ReloadCounter = 0
	state.Charge = 0

	// Add heat
	state.Heat = 1.0

	// Add recoil
	state.Recoil = prof.Recoil

	// Shoot bullets
	for i := 0; i < prof.Shots; i++ {
		w.turretShootBulletLocked(pos, tile, target, name, prof, state, i)
	}

	state.BurstCounter++
}

func (w *World) turretShootBulletLocked(pos int32, tile *Tile, target int32, name string, prof turretProfile, state *turretRuntimeState, shotIndex int) {
	if w == nil || tile == nil {
		return
	}

	// Calculate shoot angle
	angle := state.Rotation

	// Add spread
	if prof.Shots > 1 {
		if prof.AlternateShoot {
			side := float32(1)
			if shotIndex%2 == 0 {
				side = -1
			}
			angle += side * prof.Spread * float32(shotIndex/2)
		} else {
			offset := float32(shotIndex) - float32(prof.Shots-1)/2.0
			angle += offset * prof.Spread
		}
	}

	// Add inaccuracy
	angle += (rand.Float32()*2 - 1) * prof.Inaccuracy

	// Calculate bullet spawn position
	angleRad := angle * 3.14159 / 180.0
	spawnX := float32(tile.X)*8 + cosf(angleRad)*(prof.ShootY+state.Recoil)
	spawnY := float32(tile.Y)*8 + sinf(angleRad)*(prof.ShootY+state.Recoil)

	if prof.XRand > 0 {
		perpAngle := angleRad + 3.14159/2
		offset := (rand.Float32()*2 - 1) * prof.XRand
		spawnX += cosf(perpAngle) * offset
		spawnY += sinf(perpAngle) * offset
	}

	// Create bullet
	w.createTurretBulletLocked(spawnX, spawnY, angle, tile.Build.Team, name, prof)
}

func (w *World) createTurretBulletLocked(x, y, angle float32, team TeamID, turretName string, prof turretProfile) {
	// Create bullet based on turret type
	bulletType := "basic"

	// Map turret types to bullet types
	switch turretName {
	case "scatter":
		bulletType = "flak"
	case "hail", "swarmer", "ripple":
		bulletType = "missile"
	case "salvo", "fuse":
		bulletType = "artillery"
	case "lancer", "meltdown", "foreshadow":
		bulletType = "laser"
	case "arc", "parallax":
		bulletType = "arc"
	case "scorch":
		bulletType = "basic"
	case "wave", "tsunami":
		bulletType = "water"
	}

	// Calculate velocity
	angleRad := angle * 3.14159 / 180.0
	speed := float32(5.0) // Default bullet speed
	velX := cosf(angleRad) * speed
	velY := sinf(angleRad) * speed

	w.createBulletLocked(bulletType, x, y, velX, velY, team, -1)
}

// Item turret ammo now lives only in tile.Build.Items.
func (w *World) turretHasAmmoLocked(tile *Tile) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
		return w.totalBuildingAmmoLocked(tile, prof) > 0
	}
	for _, stack := range tile.Build.Items {
		if stack.Amount > 0 {
			return true
		}
	}
	return false
}

func (w *World) turretConsumeAmmoLocked(tile *Tile, amount int) {
	if tile == nil || tile.Build == nil || amount <= 0 {
		return
	}
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
		w.normalizeTurretAmmoEntriesLocked(tile, prof)
		remaining := int32(amount)
		index := w.currentBuildingAmmoIndexLocked(tile, prof)
		if index < 0 || index >= len(tile.Build.Items) || tile.Build.Items[index].Amount < remaining {
			return
		}
		tile.Build.Items[index].Amount -= remaining
		if tile.Build.Items[index].Amount <= 0 {
			tile.Build.Items = append(tile.Build.Items[:index], tile.Build.Items[index+1:]...)
		}
		w.emitTurretAmmoItemSyncLocked(tile)
		return
	}

	remaining := int32(amount)
	for i := len(tile.Build.Items) - 1; i >= 0 && remaining > 0; i-- {
		take := tile.Build.Items[i].Amount
		if take > remaining {
			take = remaining
		}
		tile.Build.Items[i].Amount -= take
		remaining -= take
		if tile.Build.Items[i].Amount <= 0 {
			tile.Build.Items = append(tile.Build.Items[:i], tile.Build.Items[i+1:]...)
		}
	}
	if remaining < int32(amount) {
		w.emitTurretAmmoItemSyncLocked(tile)
	}
}

func (w *World) turretStateByTileLocked(tile *Tile) *turretRuntimeState {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil {
		return nil
	}
	if !w.model.InBounds(tile.X, tile.Y) {
		return nil
	}
	return w.turretStateLocked(int32(tile.Y*w.model.Width + tile.X))
}

func (w *World) setTurretAmmoEntriesLocked(tile *Tile, prof buildingWeaponProfile, stacks []ItemStack) {
	if w == nil || tile == nil || tile.Build == nil {
		return
	}
	capacity := w.buildingItemAmmoCapacityLocked(tile, prof)
	remainingCap := capacity
	items := make([]ItemStack, 0, len(stacks))
	for _, stack := range stacks {
		amount := stack.Amount
		if amount <= 0 || remainingCap <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, stack.Item) {
			continue
		}
		if amount > remainingCap {
			amount = remainingCap
		}
		items = append(items, ItemStack{Item: stack.Item, Amount: amount})
		remainingCap -= amount
	}
	tile.Build.Items = items
}

func (w *World) totalBuildingAmmoLocked(tile *Tile, prof buildingWeaponProfile) int32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	w.normalizeTurretAmmoEntriesLocked(tile, prof)
	total := int32(0)
	for _, stack := range tile.Build.Items {
		if stack.Amount <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, stack.Item) {
			continue
		}
		total += stack.Amount
	}
	return total
}

func (w *World) turretAcceptItemLocked(pos int32, tile *Tile, item ItemID) bool {
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}

	prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	if !ok || !w.buildingUsesItemAmmoLocked(tile, prof) {
		return false
	}
	ammoUnits := w.turretAmmoUnitsPerItemLocked(tile, item)
	if ammoUnits <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, item) {
		return false
	}
	capacity := w.buildingItemAmmoCapacityLocked(tile, prof)
	return capacity > 0 && w.totalBuildingAmmoLocked(tile, prof)+ammoUnits <= capacity
}

func (w *World) turretHandleItemLocked(pos int32, tile *Tile, item ItemID, amount int32) bool {
	if w == nil || tile == nil || tile.Build == nil || amount <= 0 {
		return false
	}

	prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	if !ok || !w.buildingUsesItemAmmoLocked(tile, prof) {
		return false
	}
	ammoUnits := w.turretAmmoUnitsPerItemLocked(tile, item)
	if ammoUnits <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, item) {
		return false
	}
	w.normalizeTurretAmmoEntriesLocked(tile, prof)
	capacity := w.buildingItemAmmoCapacityLocked(tile, prof)
	totalAdded := ammoUnits * amount
	space := capacity - w.totalBuildingAmmoLocked(tile, prof)
	if space <= 0 || totalAdded > space {
		return false
	}
	for i := range tile.Build.Items {
		if tile.Build.Items[i].Item != item {
			continue
		}
		tile.Build.Items[i].Amount += totalAdded
		last := len(tile.Build.Items) - 1
		if i != last {
			tile.Build.Items[i], tile.Build.Items[last] = tile.Build.Items[last], tile.Build.Items[i]
		}
		if w.blockSyncLogsEnabled {
			log.Printf("[turret-ammo] accept pos=%d (%d,%d) block=%s tileTeam=%d buildTeam=%d item=%d add=%d totalAmmo=%d stacks=%s",
				pos, tile.X, tile.Y, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block)))),
				tile.Team, tile.Build.Team, item, totalAdded, w.totalBuildingAmmoLocked(tile, prof), w.debugItemStacksLocked(tile.Build.Items))
		}
		w.emitBlockItemSyncLocked(pos)
		return true
	}
	tile.Build.Items = append(tile.Build.Items, ItemStack{Item: item, Amount: totalAdded})
	if w.blockSyncLogsEnabled {
		log.Printf("[turret-ammo] accept pos=%d (%d,%d) block=%s tileTeam=%d buildTeam=%d item=%d add=%d totalAmmo=%d stacks=%s",
			pos, tile.X, tile.Y, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block)))),
			tile.Team, tile.Build.Team, item, totalAdded, w.totalBuildingAmmoLocked(tile, prof), w.debugItemStacksLocked(tile.Build.Items))
	}
	w.emitBlockItemSyncLocked(pos)
	return true
}

func (w *World) turretAmmoUnitsPerItemLocked(tile *Tile, item ItemID) int32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block))))
	if ammoByItem, ok := turretItemAmmoMultipliersByName[name]; ok {
		return ammoByItem[item]
	}
	return 0
}

func (w *World) hydrateTurretAmmoFromMapSyncLocked(tile *Tile, prof buildingWeaponProfile) {
	_ = tile
	_ = prof
}

func (w *World) normalizeTurretAmmoEntriesLocked(tile *Tile, prof buildingWeaponProfile) {
	if w == nil || tile == nil || tile.Build == nil {
		return
	}
	filtered := tile.Build.Items[:0]
	for _, entry := range tile.Build.Items {
		if entry.Amount <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, entry.Item) {
			continue
		}
		filtered = append(filtered, entry)
	}
	tile.Build.Items = filtered
	required := buildingAmmoPerShotCount(prof)
	if required <= 0 || len(tile.Build.Items) < 2 {
		return
	}
	last := len(tile.Build.Items) - 1
	if tile.Build.Items[last].Amount >= required {
		return
	}
	for i := 0; i < last; i++ {
		if tile.Build.Items[i].Amount >= required {
			tile.Build.Items[i], tile.Build.Items[last] = tile.Build.Items[last], tile.Build.Items[i]
			return
		}
	}
}

func (w *World) currentBuildingAmmoIndexLocked(tile *Tile, prof buildingWeaponProfile) int {
	if w == nil || tile == nil || tile.Build == nil {
		return -1
	}
	w.normalizeTurretAmmoEntriesLocked(tile, prof)
	for i := len(tile.Build.Items) - 1; i >= 0; i-- {
		if tile.Build.Items[i].Amount > 0 && w.buildingAcceptsAmmoItemLocked(tile, prof, tile.Build.Items[i].Item) {
			return i
		}
	}
	return -1
}

func (w *World) currentBuildingAmmoItemLocked(tile *Tile, prof buildingWeaponProfile) (ItemID, bool) {
	index := w.currentBuildingAmmoIndexLocked(tile, prof)
	if index < 0 || tile == nil || tile.Build == nil || index >= len(tile.Build.Items) {
		return 0, false
	}
	return tile.Build.Items[index].Item, true
}

func (w *World) currentBuildingAmmoAmountLocked(tile *Tile, prof buildingWeaponProfile) (int32, bool) {
	index := w.currentBuildingAmmoIndexLocked(tile, prof)
	if index < 0 || tile == nil || tile.Build == nil || index >= len(tile.Build.Items) {
		return 0, false
	}
	return tile.Build.Items[index].Amount, true
}

func (w *World) turretStateLocked(pos int32) *turretRuntimeState {
	if w.turretStates == nil {
		w.turretStates = map[int32]*turretRuntimeState{}
	}
	st := w.turretStates[pos]
	if st == nil {
		st = &turretRuntimeState{
			Rotation:   90,
			LastTarget: -1,
		}
		w.turretStates[pos] = st
	}
	return st
}

func (w *World) emitTurretAmmoItemSyncLocked(tile *Tile) {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil {
		return
	}
	if !w.model.InBounds(tile.X, tile.Y) {
		return
	}
	w.emitBlockItemSyncLocked(int32(tile.Y*w.model.Width + tile.X))
}

func normalizeAngle(angle float32) float32 {
	for angle > 180 {
		angle -= 360
	}
	for angle < -180 {
		angle += 360
	}
	return angle
}
