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
	DrillTime         float32 `json:"drillTime,omitempty"`
	PumpAmount        float32 `json:"pumpAmount,omitempty"`
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
	DrillTimeSec      float32
	PumpAmount        float32
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
			ItemCapacity:    int32(def.ItemCapacity),
			LiquidCapacity:  def.LiquidCapacity,
			LiquidPressure:  def.LiquidPressure,
			PowerCapacity:   def.PowerCapacity,
			PowerProduction: def.PowerProduction,
			PowerUse:        def.PowerUse,
			PumpAmount:      def.PumpAmount * 60.0,
			BoostMultiplier: def.LiquidBoostMul,
		}
		if def.LinkRange > 0 {
			props.LinkRangeTiles = def.LinkRange
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
		w.blockPropsByName[name] = props
	}
}
