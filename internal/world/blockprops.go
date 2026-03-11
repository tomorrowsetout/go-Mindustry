package world

import (
	"encoding/json"
	"os"
	"strings"
)

type BlockPropsDef struct {
	Block             string  `json:"block"`
	ItemCapacity      float32 `json:"itemCapacity,omitempty"`
	LiquidCapacity    float32 `json:"liquidCapacity,omitempty"`
	LiquidPressure    float32 `json:"liquidPressure,omitempty"`
	PowerCapacity     float32 `json:"powerCapacity,omitempty"`
	PowerProduction   float32 `json:"powerProduction,omitempty"`
	PowerOutput       float32 `json:"powerOutput,omitempty"`
	PowerUse          float32 `json:"powerUse,omitempty"`
	LinkRange         float32 `json:"linkRange,omitempty"`
	MaxLinks          float32 `json:"maxLinks,omitempty"`
	DrillTime         float32 `json:"drillTime,omitempty"`
	DrillTier         float32 `json:"drillTier,omitempty"`
	PumpAmount        float32 `json:"pumpAmount,omitempty"`
	Health            float32 `json:"health,omitempty"`
	ScaledHealth      float32 `json:"scaledHealth,omitempty"`
	Armor             float32 `json:"armor,omitempty"`
	RotateSpeed       float32 `json:"rotateSpeed,omitempty"`
	Damage            float32 `json:"damage,omitempty"`
	TileDamage        float32 `json:"tileDamage,omitempty"`
	BulletDamage      float32 `json:"bulletDamage,omitempty"`
	Inaccuracy        float32 `json:"inaccuracy,omitempty"`
	VelocityRnd       float32 `json:"velocityRnd,omitempty"`
	TractorForce      float32 `json:"force,omitempty"`
	TractorForceScale float32 `json:"scaledForce,omitempty"`
	ShootShots        float32 `json:"shootShots,omitempty"`
	ShootShotDelay    float32 `json:"shootShotDelay,omitempty"`
	ShootSpread       float32 `json:"shootSpread,omitempty"`
	CoolantMultiplier float32 `json:"coolantMultiplier,omitempty"`
	CoolantAmount     float32 `json:"coolantAmount,omitempty"`
	RepairSpeed       float32 `json:"repairSpeed,omitempty"`
	RepairRadius      float32 `json:"repairRadius,omitempty"`
	HealPercent       float32 `json:"healPercent,omitempty"`
	Range             float32 `json:"range,omitempty"`
	Reload            float32 `json:"reload,omitempty"`
	PhaseBoost        float32 `json:"phaseBoost,omitempty"`
	SpeedBoost        float32 `json:"speedBoost,omitempty"`
	SpeedBoostPhase   float32 `json:"speedBoostPhase,omitempty"`
	UseTime           float32 `json:"useTime,omitempty"`
	PhaseRangeBoost   float32 `json:"phaseRangeBoost,omitempty"`
	ShieldHealth      float32 `json:"shieldHealth,omitempty"`
	Radius            float32 `json:"radius,omitempty"`
	CooldownNormal    float32 `json:"cooldownNormal,omitempty"`
	CooldownLiquid    float32 `json:"cooldownLiquid,omitempty"`
	CooldownBroken    float32 `json:"cooldownBroken,omitempty"`
	PhaseRadiusBoost  float32 `json:"phaseRadiusBoost,omitempty"`
	PhaseShieldBoost  float32 `json:"phaseShieldBoost,omitempty"`
	MinRange          float32 `json:"minRange,omitempty"`
	ShootCone         float32 `json:"shootCone,omitempty"`
	BoostItemName     string  `json:"boostItem,omitempty"`
	BoostItemAmount   float32 `json:"boostItemAmount,omitempty"`
	ItemDrop          string  `json:"itemDrop,omitempty"`
	LiquidDrop        string  `json:"liquidDrop,omitempty"`
	LiquidBoostName   string  `json:"boostLiquid,omitempty"`
	LiquidBoostAmount float32 `json:"boostAmount,omitempty"`
	LiquidBoostMul    float32 `json:"boostMultiplier,omitempty"`
}

type BlockProps struct {
	ItemCapacity      int32
	LiquidCapacity    float32
	LiquidPressure    float32
	PowerCapacity     float32
	PowerProduction   float32
	PowerUse          float32
	LinkRangeTiles    float32
	MaxLinks          int
	DrillTimeSec      float32
	DrillTier         int
	HardnessDrillMul  float32
	PumpAmount        float32
	Health            float32
	Armor             float32
	RotateSpeed       float32
	Damage            float32
	TileDamage        float32
	Inaccuracy        float32
	VelocityRnd       float32
	TractorForce      float32
	TractorForceScale float32
	ShootShots        int32
	ShootShotDelaySec float32
	ShootSpread       float32
	CoolantMultiplier float32
	CoolantAmountPerS float32
	RepairSpeed       float32
	RepairRadius      float32
	HealPercent       float32
	EffectRange       float32
	HealReloadSec     float32
	PhaseBoost        float32
	OverdriveBoost    float32
	OverdriveBoostPh  float32
	UseTimeSec        float32
	OverdriveRange    float32
	OverdrivePhaseRng float32
	ShieldHealth      float32
	ShieldRadius      float32
	ShieldRegenPerS   float32
	ShieldCooldownBrk float32
	ShieldCooldownLiq float32
	PhaseRadiusBoost  float32
	PhaseShieldBoost  float32
	MinRange          float32
	ShootCone         float32
	BoostItem         ItemID
	BoostItemAmount   int32
	ItemDrop          ItemID
	LiquidDrop        LiquidID
	BoostLiquid       LiquidID
	BoostAmountPerSec float32
	BoostMultiplier   float32
}

func (w *World) LoadBlockProps(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []BlockPropsDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.blockPropsDefs = defs
	w.resolveBlockPropsLocked()
	return nil
}

func (w *World) resolveBlockPropsLocked() {
	w.blockPropsByName = map[string]BlockProps{}
	if len(w.blockPropsDefs) == 0 {
		return
	}
	for _, def := range w.blockPropsDefs {
		name := strings.ToLower(strings.TrimSpace(def.Block))
		if name == "" {
			continue
		}
		props := BlockProps{
			ItemCapacity:      int32(def.ItemCapacity),
			LiquidCapacity:    def.LiquidCapacity,
			LiquidPressure:    def.LiquidPressure,
			PowerCapacity:     def.PowerCapacity,
			PowerProduction:   def.PowerProduction,
			PowerUse:          def.PowerUse,
			PumpAmount:        def.PumpAmount * 60.0,
			BoostMultiplier:   def.LiquidBoostMul,
			Health:            def.Health,
			Armor:             def.Armor,
			RotateSpeed:       def.RotateSpeed,
			Damage:            def.Damage,
			TileDamage:        def.TileDamage,
			Inaccuracy:        def.Inaccuracy,
			VelocityRnd:       def.VelocityRnd,
			TractorForce:      def.TractorForce,
			TractorForceScale: def.TractorForceScale,
			ShootShots:        int32(def.ShootShots),
			ShootSpread:       def.ShootSpread,
			DrillTier:         int(def.DrillTier),
			RepairSpeed:       def.RepairSpeed,
			RepairRadius:      def.RepairRadius,
			HealPercent:       def.HealPercent,
			EffectRange:       def.Range,
			PhaseBoost:        def.PhaseBoost,
			OverdriveBoost:    def.SpeedBoost,
			OverdriveBoostPh:  def.SpeedBoostPhase,
			UseTimeSec:        def.UseTime / 60.0,
			OverdriveRange:    def.Range,
			OverdrivePhaseRng: def.PhaseRangeBoost,
			ShieldHealth:      def.ShieldHealth,
			ShieldRadius:      def.Radius,
			PhaseRadiusBoost:  def.PhaseRadiusBoost,
			PhaseShieldBoost:  def.PhaseShieldBoost,
			MinRange:          def.MinRange,
			ShootCone:         def.ShootCone,
		}
		if def.LinkRange > 0 {
			props.LinkRangeTiles = def.LinkRange
		}
		if def.MaxLinks > 0 {
			props.MaxLinks = int(def.MaxLinks)
		}
		if def.ScaledHealth > 0 && props.Health <= 0 {
			props.Health = def.ScaledHealth
		}
		if def.CoolantMultiplier > 0 {
			props.CoolantMultiplier = def.CoolantMultiplier
		}
		if def.CoolantAmount > 0 {
			props.CoolantAmountPerS = def.CoolantAmount * 60.0
		}
		if props.CoolantAmountPerS > 0 && props.CoolantMultiplier == 0 {
			props.CoolantMultiplier = 5
		}
		if def.Reload > 0 {
			props.HealReloadSec = def.Reload / 60.0
		}
		if def.CooldownNormal > 0 {
			props.ShieldRegenPerS = def.CooldownNormal * 60.0
		}
		if def.CooldownLiquid > 0 {
			props.ShieldCooldownLiq = def.CooldownLiquid
		}
		if def.CooldownBroken > 0 {
			props.ShieldCooldownBrk = def.CooldownBroken * 60.0
		}
		if props.ShieldRadius <= 0 {
			switch name {
			case "shield-projector":
				props.ShieldRadius = 200
			case "large-shield-projector":
				props.ShieldRadius = 400
			}
		}
		if props.PowerProduction == 0 && def.PowerOutput != 0 {
			props.PowerProduction = def.PowerOutput
		}
		if def.DrillTime > 0 {
			props.DrillTimeSec = def.DrillTime / 60.0
			if props.DrillTimeSec < 0.05 {
				props.DrillTimeSec = 0.05
			}
		}
		if props.Damage <= 0 && def.BulletDamage > 0 {
			props.Damage = def.BulletDamage
		}
		if def.ShootShotDelay > 0 {
			props.ShootShotDelaySec = def.ShootShotDelay / 60.0
		}
		if props.DrillTier > 0 && props.HardnessDrillMul == 0 {
			switch {
			case strings.Contains(name, "impact-drill"), strings.Contains(name, "eruption-drill"):
				props.HardnessDrillMul = 0
			default:
				props.HardnessDrillMul = 50
			}
		}
		if def.ItemDrop != "" && w.itemIDsByName != nil {
			if id, ok := w.itemIDsByName[strings.ToLower(strings.TrimSpace(def.ItemDrop))]; ok {
				props.ItemDrop = ItemID(id)
			}
		}
		if def.LiquidDrop != "" && w.liquidIDsByName != nil {
			if id, ok := w.liquidIDsByName[strings.ToLower(strings.TrimSpace(def.LiquidDrop))]; ok {
				props.LiquidDrop = LiquidID(id)
			}
		}
		if def.LiquidBoostName != "" && w.liquidIDsByName != nil {
			if id, ok := w.liquidIDsByName[strings.ToLower(strings.TrimSpace(def.LiquidBoostName))]; ok {
				props.BoostLiquid = LiquidID(id)
				props.BoostAmountPerSec = def.LiquidBoostAmount * 60.0
			}
		}
		if def.BoostItemName != "" && w.itemIDsByName != nil {
			if id, ok := w.itemIDsByName[strings.ToLower(strings.TrimSpace(def.BoostItemName))]; ok {
				props.BoostItem = ItemID(id)
				if def.BoostItemAmount > 0 {
					props.BoostItemAmount = int32(def.BoostItemAmount)
				} else {
					props.BoostItemAmount = 1
				}
			}
		}
		w.blockPropsByName[name] = props
	}
}
