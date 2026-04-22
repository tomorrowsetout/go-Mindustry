package world

import (
	"time"
)

type unitPlan struct {
	UnitType     int16
	Requirements []ItemStack
	Time         float32
}

type factoryProfile struct {
	Plans []unitPlan
}

var factoryProfilesByBlockName = map[string]factoryProfile{
	"ground-factory": {
		Plans: []unitPlan{
			{UnitType: 0, Requirements: []ItemStack{{Item: copperItemID, Amount: 10}}, Time: 600},  // dagger
			{UnitType: 1, Requirements: []ItemStack{{Item: siliconItemID, Amount: 15}}, Time: 900}, // nova
			{UnitType: 2, Requirements: []ItemStack{{Item: leadItemID, Amount: 10}}, Time: 600},    // crawler
		},
	},
	"air-factory": {
		Plans: []unitPlan{
			{UnitType: 10, Requirements: []ItemStack{{Item: copperItemID, Amount: 15}}, Time: 700},  // flare
			{UnitType: 11, Requirements: []ItemStack{{Item: siliconItemID, Amount: 20}}, Time: 1000}, // horizon
		},
	},
	"naval-factory": {
		Plans: []unitPlan{
			{UnitType: 20, Requirements: []ItemStack{{Item: copperItemID, Amount: 20}, {Item: leadItemID, Amount: 10}}, Time: 900}, // risso
		},
	},
	"additive-reconstructor": {
		Plans: []unitPlan{},
	},
	"multiplicative-reconstructor": {
		Plans: []unitPlan{},
	},
	"exponential-reconstructor": {
		Plans: []unitPlan{},
	},
	"tetrative-reconstructor": {
		Plans: []unitPlan{},
	},
}

type reconstructorUpgrade struct {
	From int16
	To   int16
	Time float32
}

var reconstructorUpgradesByBlockName = map[string][]reconstructorUpgrade{
	"additive-reconstructor": {
		{From: 0, To: 3, Time: 1800},  // dagger -> mace
		{From: 1, To: 4, Time: 2400},  // nova -> pulsar
		{From: 2, To: 5, Time: 1800},  // crawler -> atrax
		{From: 10, To: 13, Time: 2100}, // flare -> horizon
		{From: 20, To: 23, Time: 2700}, // risso -> minke
	},
	"multiplicative-reconstructor": {
		{From: 3, To: 6, Time: 3000},  // mace -> fortress
		{From: 4, To: 7, Time: 3600},  // pulsar -> quasar
		{From: 5, To: 8, Time: 3000},  // atrax -> spiroct
		{From: 13, To: 16, Time: 3300}, // horizon -> zenith
		{From: 23, To: 26, Time: 3900}, // minke -> bryde
	},
	"exponential-reconstructor": {
		{From: 6, To: 9, Time: 4200},  // fortress -> scepter
		{From: 7, To: 10, Time: 4800}, // quasar -> vela
		{From: 8, To: 11, Time: 4200}, // spiroct -> arkyid
		{From: 16, To: 19, Time: 4500}, // zenith -> antumbra
		{From: 26, To: 29, Time: 5100}, // bryde -> sei
	},
	"tetrative-reconstructor": {
		{From: 9, To: 12, Time: 6000},  // scepter -> reign
		{From: 10, To: 13, Time: 6600}, // vela -> corvus
		{From: 11, To: 14, Time: 6000}, // arkyid -> toxopid
		{From: 19, To: 22, Time: 6300}, // antumbra -> eclipse
		{From: 29, To: 32, Time: 6900}, // sei -> omura
	},
}


func (w *World) stepFactoriesLocked(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}

	dt := float32(delta.Seconds())
	deltaFrames := dt * 60.0

	if dt <= 0 {
		return
	}

	for _, pos := range w.factoryTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Team == 0 || tile.Block == 0 {
			continue
		}

		name := w.blockNameByID(int16(tile.Block))

		// Check if it's a reconstructor
		if upgrades, ok := reconstructorUpgradesByBlockName[name]; ok {
			w.stepReconstructorFactoryLocked(pos, tile, name, upgrades, dt, deltaFrames)
		} else if prof, ok := factoryProfilesByBlockName[name]; ok {
			w.stepFactoryLocked(pos, tile, name, prof, dt, deltaFrames)
		}
	}
}

func (w *World) stepFactoryLocked(pos int32, tile *Tile, name string, prof factoryProfile, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil {
		return
	}

	state := w.factoryStates[pos]

	// Validate current plan
	if int(state.CurrentPlan) < 0 || int(state.CurrentPlan) >= len(prof.Plans) {
		state.CurrentPlan = 0
		state.Progress = 0
	}

	if len(prof.Plans) == 0 {
		return
	}

	plan := prof.Plans[state.CurrentPlan]

	// Check if can produce
	canProduce := tile.Build.Health > 0

	// Check if has required items
	if canProduce {
		for _, req := range plan.Requirements {
			if tile.Build.ItemAmount(req.Item) < req.Amount {
				canProduce = false
				break
			}
		}
	}

	// Update progress
	if canProduce {
		state.Progress += deltaFrames
	}

	// Check if production complete
	if state.Progress >= plan.Time {
		// Consume items
		for _, req := range plan.Requirements {
			tile.Build.RemoveItem(req.Item, req.Amount)
		}

		// Create unit
		w.spawnUnitAtFactoryLocked(pos, tile, plan.UnitType)

		state.Progress = 0
	}

	w.factoryStates[pos] = state
}

func (w *World) stepReconstructorFactoryLocked(pos int32, tile *Tile, name string, upgrades []reconstructorUpgrade, dt, deltaFrames float32) {
	if tile == nil || tile.Build == nil {
		return
	}

	state := w.reconstructorStates[pos]

	// Check if has payload (unit to upgrade)
	if tile.Build.Payload == nil || len(tile.Build.Payload) == 0 {
		state.Progress = 0
		w.reconstructorStates[pos] = state
		return
	}

	// For now, skip upgrade logic as payload system needs integration
	w.reconstructorStates[pos] = state
}

func (w *World) spawnUnitAtFactoryLocked(pos int32, tile *Tile, unitType int16) {
	if w == nil || tile == nil {
		return
	}

	// Calculate spawn position (in front of factory)
	dx, dy := dirDelta(tile.Rotation)
	spawnX := float32(tile.X)*8 + float32(dx)*8
	spawnY := float32(tile.Y)*8 + float32(dy)*8

	// Create unit - actual implementation depends on unit system
	// For now, just a placeholder
	_ = spawnX
	_ = spawnY
	_ = unitType
}

func (w *World) factoryAcceptItemLocked(pos int32, tile *Tile, item ItemID) bool {
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}

	name := w.blockNameByID(int16(tile.Block))
	prof, ok := factoryProfilesByBlockName[name]
	if !ok || len(prof.Plans) == 0 {
		return false
	}

	state := w.factoryStates[pos]
	if int(state.CurrentPlan) < 0 || int(state.CurrentPlan) >= len(prof.Plans) {
		return false
	}

	plan := prof.Plans[state.CurrentPlan]

	// Check if this item is required
	for _, req := range plan.Requirements {
		if req.Item == item {
			return tile.Build.ItemAmount(item) < req.Amount*2
		}
	}

	return false
}

func (w *World) factoryHandleItemLocked(pos int32, tile *Tile, item ItemID) bool {
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}

	if !w.factoryAcceptItemLocked(pos, tile, item) {
		return false
	}

	tile.Build.AddItem(item, 1)
	return true
}
