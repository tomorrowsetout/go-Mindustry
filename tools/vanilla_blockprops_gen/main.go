package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
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

func main() {
	in := flag.String("in", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/block_props.json", "output json path")
	flag.Parse()
	if strings.TrimSpace(*in) == "" {
		fmt.Println("missing -in Blocks.java")
		os.Exit(2)
	}
	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Println("read Blocks.java:", err)
		os.Exit(1)
	}
	defs := parseBlocks(string(src))
	if err := os.MkdirAll(filepathDir(*out), 0755); err != nil {
		fmt.Println("mkdir:", err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(defs, "", "  ")
	if err != nil {
		fmt.Println("json:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		fmt.Println("write:", err)
		os.Exit(1)
	}
	fmt.Printf("block props: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var blockStartRe = regexp.MustCompile(`new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var itemCapacityRe = regexp.MustCompile(`itemCapacity\s*=\s*([^;]+);`)
var liquidCapacityRe = regexp.MustCompile(`liquidCapacity\s*=\s*([^;]+);`)
var liquidPressureRe = regexp.MustCompile(`liquidPressure\s*=\s*([^;]+);`)
var powerCapacityRe = regexp.MustCompile(`powerCapacity\s*=\s*([^;]+);`)
var powerProductionRe = regexp.MustCompile(`powerProduction\s*=\s*([^;]+);`)
var powerOutputRe = regexp.MustCompile(`powerOutput\s*=\s*([^;]+);`)
var powerUseRe = regexp.MustCompile(`consumePower\s*\(\s*([^)]+)\)`)
var linkRangeRe = regexp.MustCompile(`laserRange\s*=\s*([^;]+);`)
var maxLinksRe = regexp.MustCompile(`maxNodes\s*=\s*([^;]+);`)
var drillTimeRe = regexp.MustCompile(`drillTime\s*=\s*([^;]+);`)
var drillTierRe = regexp.MustCompile(`(?m)\btier\s*=\s*([^;]+);`)
var pumpAmountRe = regexp.MustCompile(`pumpAmount\s*=\s*([^;]+);`)
var healthRe = regexp.MustCompile(`(?m)\bhealth\s*=\s*([^;]+);`)
var scaledHealthRe = regexp.MustCompile(`(?m)\bscaledHealth\s*=\s*([^;]+);`)
var armorRe = regexp.MustCompile(`(?m)\barmor\s*=\s*([^;]+);`)
var rotateSpeedRe = regexp.MustCompile(`(?m)\brotateSpeed\s*=\s*([^;]+);`)
var coolantMultiplierRe = regexp.MustCompile(`(?m)\bcoolantMultiplier\s*=\s*([^;]+);`)
var coolantAmountRe = regexp.MustCompile(`consumeCoolant\s*\(\s*([^,)]+)`)
var damageRe = regexp.MustCompile(`(?m)\bdamage\s*=\s*([^;]+);`)
var tileDamageRe = regexp.MustCompile(`(?m)\btileDamage\s*=\s*([^;]+);`)
var bulletDamageRe = regexp.MustCompile(`(?m)\bbulletDamage\s*=\s*([^;]+);`)
var inaccuracyRe = regexp.MustCompile(`(?m)\binaccuracy\s*=\s*([^;]+);`)
var velocityRndRe = regexp.MustCompile(`(?m)\bvelocityRnd\s*=\s*([^;]+);`)
var tractorForceRe = regexp.MustCompile(`(?m)\bforce\s*=\s*([^;]+);`)
var tractorScaledForceRe = regexp.MustCompile(`(?m)\bscaledForce\s*=\s*([^;]+);`)
var shootShotsRe = regexp.MustCompile(`(?m)\bshoot\.shots\s*=\s*([^;]+);`)
var shootShotDelayRe = regexp.MustCompile(`(?m)\bshoot\.shotDelay\s*=\s*([^;]+);`)
var shootSpreadRe = regexp.MustCompile(`(?m)\bshoot\.spread\s*=\s*([^;]+);`)
var repairSpeedRe = regexp.MustCompile(`(?m)\brepairSpeed\s*=\s*([^;]+);`)
var repairRadiusRe = regexp.MustCompile(`(?m)\brepairRadius\s*=\s*([^;]+);`)
var healPercentRe = regexp.MustCompile(`(?m)\bhealPercent\s*=\s*([^;]+);`)
var rangeRe = regexp.MustCompile(`(?m)\brange\s*=\s*([^;]+);`)
var reloadRe = regexp.MustCompile(`(?m)\breload\s*=\s*([^;]+);`)
var phaseBoostRe = regexp.MustCompile(`(?m)\bphaseBoost\s*=\s*([^;]+);`)
var speedBoostRe = regexp.MustCompile(`(?m)\bspeedBoost\s*=\s*([^;]+);`)
var speedBoostPhaseRe = regexp.MustCompile(`(?m)\bspeedBoostPhase\s*=\s*([^;]+);`)
var useTimeRe = regexp.MustCompile(`(?m)\buseTime\s*=\s*([^;]+);`)
var phaseRangeBoostRe = regexp.MustCompile(`(?m)\bphaseRangeBoost\s*=\s*([^;]+);`)
var shieldHealthRe = regexp.MustCompile(`(?m)\bshieldHealth\s*=\s*([^;]+);`)
var radiusRe = regexp.MustCompile(`(?m)\bradius\s*=\s*([^;]+);`)
var cooldownNormalRe = regexp.MustCompile(`(?m)\bcooldownNormal\s*=\s*([^;]+);`)
var cooldownLiquidRe = regexp.MustCompile(`(?m)\bcooldownLiquid\s*=\s*([^;]+);`)
var cooldownBrokenRe = regexp.MustCompile(`(?m)\bcooldownBrokenBase\s*=\s*([^;]+);`)
var phaseRadiusBoostRe = regexp.MustCompile(`(?m)\bphaseRadiusBoost\s*=\s*([^;]+);`)
var phaseShieldBoostRe = regexp.MustCompile(`(?m)\bphaseShieldBoost\s*=\s*([^;]+);`)
var minRangeRe = regexp.MustCompile(`(?m)\bminRange\s*=\s*([^;]+);`)
var shootConeRe = regexp.MustCompile(`(?m)\bshootCone\s*=\s*([^;]+);`)
var itemDropRe = regexp.MustCompile(`itemDrop\s*=\s*Items\.([A-Za-z0-9_]+);`)
var liquidDropRe = regexp.MustCompile(`liquidDrop\s*=\s*Liquids\.([A-Za-z0-9_]+);`)
var liquidBoostMulRe = regexp.MustCompile(`liquidBoostIntensity\s*=\s*([^;]+);`)
var liquidBoostRe = regexp.MustCompile(`consumeLiquid\s*\(\s*Liquids\.([A-Za-z0-9_]+)\s*,\s*([^)]+)\)\s*\.boost\(\)`)
var itemBoostRe = regexp.MustCompile(`consumeItem\s*\(\s*Items\.([A-Za-z0-9_]+)\s*\)\s*\.boost\(\)`)
var itemsBoostRe = regexp.MustCompile(`consumeItems\s*\(\s*with\(\s*Items\.([A-Za-z0-9_]+)\s*,\s*([^,)\s]+)`)
var oreBlockNoNameRe = regexp.MustCompile(`([A-Za-z0-9_]+)\s*=\s*new\s+OreBlock\s*\(\s*Items\.([A-Za-z0-9_]+)\s*\)`)
var oreBlockNamedRe = regexp.MustCompile(`new\s+OreBlock\s*\(\s*"([^"]+)"\s*,\s*Items\.([A-Za-z0-9_]+)\s*\)`)

func parseBlocks(src string) []BlockPropsDef {
	out := []BlockPropsDef{}
	seen := map[string]bool{}
	matches := blockStartRe.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(src[m[2]:m[3]]))
		if name == "" {
			continue
		}
		start := strings.Index(src[m[0]:m[1]], "{")
		if start < 0 {
			continue
		}
		start = m[0] + start
		body, ok := extractBlockBody(src, start)
		if !ok {
			continue
		}
		def := parseBlockBody(name, body)
		if def.Block == "" {
			continue
		}
		if defIsEmpty(def) {
			continue
		}
		out = append(out, def)
		seen[def.Block] = true
	}
	for _, m := range oreBlockNoNameRe.FindAllStringSubmatch(src, -1) {
		if len(m) < 3 {
			continue
		}
		item := strings.ToLower(strings.TrimSpace(m[2]))
		if item == "" {
			continue
		}
		name := "ore-" + item
		if seen[name] {
			continue
		}
		out = append(out, BlockPropsDef{Block: name, ItemDrop: item})
		seen[name] = true
	}
	for _, m := range oreBlockNamedRe.FindAllStringSubmatch(src, -1) {
		if len(m) < 3 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(m[1]))
		item := strings.ToLower(strings.TrimSpace(m[2]))
		if name == "" || item == "" {
			continue
		}
		if seen[name] {
			continue
		}
		out = append(out, BlockPropsDef{Block: name, ItemDrop: item})
		seen[name] = true
	}
	return out
}

func defIsEmpty(d BlockPropsDef) bool {
	return d.ItemCapacity == 0 && d.LiquidCapacity == 0 && d.LiquidPressure == 0 &&
		d.PowerCapacity == 0 && d.PowerProduction == 0 && d.PowerOutput == 0 && d.PowerUse == 0 &&
		d.LinkRange == 0 && d.MaxLinks == 0 && d.DrillTime == 0 && d.DrillTier == 0 && d.PumpAmount == 0 &&
		d.Health == 0 && d.ScaledHealth == 0 && d.Armor == 0 && d.RotateSpeed == 0 && d.Damage == 0 && d.TileDamage == 0 &&
		d.BulletDamage == 0 && d.Inaccuracy == 0 && d.VelocityRnd == 0 && d.TractorForce == 0 && d.TractorForceScale == 0 &&
		d.ShootShots == 0 && d.ShootShotDelay == 0 && d.ShootSpread == 0 &&
		d.CoolantMultiplier == 0 && d.CoolantAmount == 0 &&
		d.RepairSpeed == 0 && d.RepairRadius == 0 && d.HealPercent == 0 &&
		d.Range == 0 && d.Reload == 0 && d.PhaseBoost == 0 && d.SpeedBoost == 0 && d.SpeedBoostPhase == 0 && d.UseTime == 0 &&
		d.PhaseRangeBoost == 0 && d.ShieldHealth == 0 && d.Radius == 0 && d.CooldownNormal == 0 && d.CooldownLiquid == 0 &&
		d.CooldownBroken == 0 && d.PhaseRadiusBoost == 0 && d.PhaseShieldBoost == 0 &&
		d.MinRange == 0 && d.ShootCone == 0 &&
		d.BoostItemName == "" && d.BoostItemAmount == 0 &&
		d.ItemDrop == "" && d.LiquidDrop == "" && d.LiquidBoostName == "" && d.LiquidBoostMul == 0
}

func extractBlockBody(src string, start int) (string, bool) {
	depth := 0
	end := -1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 || start+1 > end-1 {
		return "", false
	}
	return src[start+1 : end-1], true
}

func parseBlockBody(name, body string) BlockPropsDef {
	def := BlockPropsDef{Block: strings.ToLower(strings.TrimSpace(name))}
	if m := itemCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ItemCapacity = v
		}
	}
	if m := liquidCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidCapacity = v
		}
	}
	if m := liquidPressureRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidPressure = v
		}
	}
	if m := powerCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerCapacity = v
		}
	}
	if m := powerProductionRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerProduction = v
		}
	}
	if m := powerOutputRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerOutput = v
		}
	}
	if m := powerUseRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerUse = v
		}
	}
	if m := linkRangeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LinkRange = v
		}
	}
	if m := maxLinksRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.MaxLinks = v
		}
	}
	if m := drillTimeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.DrillTime = v
		}
	}
	if m := drillTierRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.DrillTier = v
		}
	}
	if m := pumpAmountRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PumpAmount = v
		}
	}
	if m := healthRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Health = v
		}
	}
	if m := scaledHealthRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ScaledHealth = v
		}
	}
	if m := armorRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Armor = v
		}
	}
	if m := rotateSpeedRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.RotateSpeed = v
		}
	}
	if m := coolantMultiplierRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CoolantMultiplier = v
		}
	}
	if m := coolantAmountRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CoolantAmount = v
		}
	}
	if m := damageRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Damage = v
		}
	}
	if m := bulletDamageRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.BulletDamage = v
		}
	}
	if m := tileDamageRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.TileDamage = v
		}
	}
	if m := inaccuracyRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Inaccuracy = v
		}
	}
	if m := velocityRndRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.VelocityRnd = v
		}
	}
	if m := tractorForceRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.TractorForce = v
		}
	}
	if m := tractorScaledForceRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.TractorForceScale = v
		}
	}
	if m := shootShotsRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ShootShots = v
		}
	}
	if m := shootShotDelayRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ShootShotDelay = v
		}
	}
	if m := shootSpreadRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ShootSpread = v
		}
	}
	if m := repairSpeedRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.RepairSpeed = v
		}
	}
	if m := repairRadiusRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.RepairRadius = v
		}
	}
	if m := healPercentRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.HealPercent = v
		}
	}
	if m := rangeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Range = v
		}
	}
	if m := reloadRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Reload = v
		}
	}
	if m := phaseBoostRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PhaseBoost = v
		}
	}
	if m := speedBoostRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.SpeedBoost = v
		}
	}
	if m := speedBoostPhaseRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.SpeedBoostPhase = v
		}
	}
	if m := useTimeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.UseTime = v
		}
	}
	if m := phaseRangeBoostRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PhaseRangeBoost = v
		}
	}
	if m := shieldHealthRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ShieldHealth = v
		}
	}
	if m := radiusRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Radius = v
		}
	}
	if m := cooldownNormalRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CooldownNormal = v
		}
	}
	if m := cooldownLiquidRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CooldownLiquid = v
		}
	}
	if m := cooldownBrokenRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CooldownBroken = v
		}
	}
	if m := phaseRadiusBoostRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PhaseRadiusBoost = v
		}
	}
	if m := phaseShieldBoostRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PhaseShieldBoost = v
		}
	}
	if m := minRangeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.MinRange = v
		}
	}
	if m := shootConeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ShootCone = v
		}
	}
	if m := itemBoostRe.FindStringSubmatch(body); len(m) > 1 {
		def.BoostItemName = strings.ToLower(strings.TrimSpace(m[1]))
		def.BoostItemAmount = 1
	}
	if m := itemsBoostRe.FindStringSubmatch(body); len(m) > 2 {
		def.BoostItemName = strings.ToLower(strings.TrimSpace(m[1]))
		if v, ok := evalExpr(m[2]); ok {
			def.BoostItemAmount = v
		} else {
			def.BoostItemAmount = 1
		}
	}
	if m := itemDropRe.FindStringSubmatch(body); len(m) > 1 {
		def.ItemDrop = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := liquidDropRe.FindStringSubmatch(body); len(m) > 1 {
		def.LiquidDrop = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := liquidBoostMulRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidBoostMul = v
		}
	}
	if m := liquidBoostRe.FindStringSubmatch(body); len(m) > 2 {
		def.LiquidBoostName = strings.ToLower(strings.TrimSpace(m[1]))
		if v, ok := evalExpr(m[2]); ok {
			def.LiquidBoostAmount = v
		}
	}
	return def
}

func evalExpr(expr string) (float32, bool) {
	clean := strings.ReplaceAll(expr, "f", "")
	clean = strings.ReplaceAll(clean, "F", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, false
	}
	for _, r := range clean {
		if (r >= '0' && r <= '9') || r == '.' || r == '+' || r == '-' || r == '*' || r == '/' || r == '(' || r == ')' || r == 'e' || r == 'E' || r == ' ' {
			continue
		}
		return 0, false
	}
	tokens, ok := tokenize(clean)
	if !ok {
		return 0, false
	}
	out, ok := shuntingEval(tokens)
	if !ok {
		return 0, false
	}
	if math.IsNaN(float64(out)) || math.IsInf(float64(out), 0) {
		return 0, false
	}
	return out, true
}

func tokenize(s string) ([]string, bool) {
	var out []string
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			i++
		case ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '(' || ch == ')':
			out = append(out, string(ch))
			i++
		case (ch >= '0' && ch <= '9') || ch == '.':
			j := i + 1
			for j < len(s) {
				c := s[j]
				if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
					j++
					continue
				}
				break
			}
			out = append(out, strings.TrimSpace(s[i:j]))
			i = j
		default:
			return nil, false
		}
	}
	return out, true
}

func shuntingEval(tokens []string) (float32, bool) {
	var vals []float32
	var ops []string
	prec := func(op string) int {
		switch op {
		case "+", "-":
			return 1
		case "*", "/":
			return 2
		default:
			return 0
		}
	}
	apply := func() bool {
		if len(ops) == 0 || len(vals) < 2 {
			return false
		}
		op := ops[len(ops)-1]
		ops = ops[:len(ops)-1]
		b := vals[len(vals)-1]
		a := vals[len(vals)-2]
		vals = vals[:len(vals)-2]
		switch op {
		case "+":
			vals = append(vals, a+b)
		case "-":
			vals = append(vals, a-b)
		case "*":
			vals = append(vals, a*b)
		case "/":
			if b == 0 {
				return false
			}
			vals = append(vals, a/b)
		default:
			return false
		}
		return true
	}
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "+", "-", "*", "/":
			for len(ops) > 0 && prec(ops[len(ops)-1]) >= prec(t) {
				if !apply() {
					return 0, false
				}
			}
			ops = append(ops, t)
		case "(":
			ops = append(ops, t)
		case ")":
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				if !apply() {
					return 0, false
				}
			}
			if len(ops) == 0 {
				return 0, false
			}
			ops = ops[:len(ops)-1]
		default:
			v, err := strconv.ParseFloat(t, 32)
			if err != nil {
				return 0, false
			}
			vals = append(vals, float32(v))
		}
	}
	for len(ops) > 0 {
		if ops[len(ops)-1] == "(" {
			return 0, false
		}
		if !apply() {
			return 0, false
		}
	}
	if len(vals) != 1 {
		return 0, false
	}
	return vals[0], true
}
