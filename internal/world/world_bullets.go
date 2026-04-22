package world

import (
	"math"
	"time"
)

func (w *World) stepBulletsLocked(delta time.Duration) {
	if w == nil {
		return
	}

	dt := float32(delta.Seconds())

	if dt <= 0 {
		return
	}

	// Update existing bullets using simBullet structure
	for i := len(w.bullets) - 1; i >= 0; i-- {
		b := &w.bullets[i]

		// Update age
		b.AgeSec += dt

		// Check lifetime
		if b.AgeSec >= b.LifeSec {
			w.despawnSimBulletLocked(i, b)
			continue
		}

		// Update position
		b.X += b.VX * dt
		b.Y += b.VY * dt

		// Check collisions
		if w.checkSimBulletCollisionsLocked(i, b) {
			continue
		}
	}
}

func (w *World) checkSimBulletCollisionsLocked(idx int, b *simBullet) bool {
	if w == nil || w.model == nil {
		return false
	}

	// Check unit collisions
	if b.HitUnits {
		for i := range w.model.Entities {
			entity := &w.model.Entities[i]
			if entity.Team == b.Team {
				continue
			}

			dx := entity.X - b.X
			dy := entity.Y - b.Y
			dist := sqrtf(dx*dx + dy*dy)

			if dist < b.Radius+4 {
				// Hit entity
				entity.Health -= b.Damage
				w.despawnSimBulletLocked(idx, b)
				return true
			}
		}
	}

	// Check building collisions
	if b.HitBuilds {
		tileX := int(b.X / 8)
		tileY := int(b.Y / 8)

		if w.model.InBounds(tileX, tileY) {
			pos := int32(tileY*w.model.Width + tileX)
			if pos >= 0 && int(pos) < len(w.model.Tiles) {
				tile := &w.model.Tiles[pos]
				if tile.Build != nil && tile.Build.Team != b.Team && tile.Build.Health > 0 {
					// Hit building
					tile.Build.Health -= b.BuildingDamage
					w.despawnSimBulletLocked(idx, b)
					return true
				}
			}
		}
	}

	return false
}

func (w *World) despawnSimBulletLocked(idx int, b *simBullet) {
	if w == nil {
		return
	}

	// Apply splash damage
	if b.SplashDamage > 0 && b.SplashRadius > 0 {
		w.applySplashDamageLocked(b.X, b.Y, b.SplashRadius, b.SplashDamage, b.Team)
	}

	// Remove bullet
	if idx >= 0 && idx < len(w.bullets) {
		w.bullets = append(w.bullets[:idx], w.bullets[idx+1:]...)
	}
}

func (w *World) applySplashDamageLocked(x, y, radius, damage float32, team TeamID) {
	if w == nil || w.model == nil {
		return
	}

	radiusSq := radius * radius

	// Damage entities
	for i := range w.model.Entities {
		entity := &w.model.Entities[i]
		if entity.Team == team {
			continue
		}

		dx := entity.X - x
		dy := entity.Y - y
		distSq := dx*dx + dy*dy

		if distSq < radiusSq {
			dist := sqrtf(distSq)
			falloff := 1 - dist/radius
			entity.Health -= damage * falloff
		}
	}

	// Damage buildings
	tileRadius := int(radius/8) + 1
	centerX := int(x / 8)
	centerY := int(y / 8)

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
			if tile.Build == nil || tile.Build.Team == team || tile.Build.Health <= 0 {
				continue
			}

			buildX := float32(tileX) * 8
			buildY := float32(tileY) * 8
			dx := buildX - x
			dy := buildY - y
			distSq := dx*dx + dy*dy

			if distSq < radiusSq {
				dist := sqrtf(distSq)
				falloff := 1 - dist/radius
				tile.Build.Health -= damage * falloff
			}
		}
	}
}

func (w *World) createBulletLocked(bulletType string, x, y, velX, velY float32, team TeamID, owner int32) {
	if w == nil {
		return
	}

	// Use simBullet structure directly
	b := simBullet{
		ID:             w.bulletNextID,
		Team:           team,
		X:              x,
		Y:              y,
		VX:             velX,
		VY:             velY,
		Damage:         10,  // Default damage
		SplashDamage:   0,
		LifeSec:        1.0, // Default 1 second lifetime
		AgeSec:         0,
		Radius:         4,
		HitUnits:       true,
		HitBuilds:      true,
		BulletClass:    bulletType,
		SplashRadius:   0,
		BuildingDamage: 10,
	}

	w.bullets = append(w.bullets, b)
	w.bulletNextID++
}

func sinf(x float32) float32 {
	return float32(math.Sin(float64(x)))
}

func cosf(x float32) float32 {
	return float32(math.Cos(float64(x)))
}

func atan2f(y, x float32) float32 {
	return float32(math.Atan2(float64(y), float64(x)))
}

func sqrtf(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}
