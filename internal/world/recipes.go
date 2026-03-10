package world

import (
	"encoding/json"
	"os"
	"strings"
)

type RecipeItem struct {
	Name   string `json:"name"`
	ID     int16  `json:"id,omitempty"`
	Amount int32  `json:"amount"`
}

type RecipeLiquid struct {
	Name   string  `json:"name"`
	ID     int16   `json:"id,omitempty"`
	Amount float32 `json:"amount"`
}

type RecipeDef struct {
	Block         string         `json:"block"`
	CraftTime     float32        `json:"craftTime"`
	Power         float32        `json:"power,omitempty"`
	PowerBuffered float32        `json:"powerBuffered,omitempty"`
	InputItems    []RecipeItem   `json:"inputItems,omitempty"`
	InputLiquids  []RecipeLiquid `json:"inputLiquids,omitempty"`
	OutputItems   []RecipeItem   `json:"outputItems,omitempty"`
	OutputLiquids []RecipeLiquid `json:"outputLiquids,omitempty"`
}

type CraftRecipe struct {
	BlockName     string
	CraftTime     float32
	Power         float32
	PowerBuffered float32
	InputItems    []ItemStack
	InputLiquids  []LiquidStack
	OutputItems   []ItemStack
	OutputLiquids []LiquidStack
}

func (w *World) LoadRecipes(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []RecipeDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.recipeDefs = defs
	w.resolveRecipesLocked()
	return nil
}

func (w *World) resolveRecipesLocked() {
	w.recipesByBlockName = map[string]CraftRecipe{}
	if len(w.recipeDefs) == 0 {
		return
	}
	for _, def := range w.recipeDefs {
		block := strings.ToLower(strings.TrimSpace(def.Block))
		if block == "" {
			continue
		}
		craftTime := def.CraftTime
		if craftTime > 0 {
			craftTime = craftTime / 60.0
			if craftTime < 0.05 {
				craftTime = 0.05
			}
		}
		recipe := CraftRecipe{
			BlockName:     block,
			CraftTime:     craftTime,
			Power:         def.Power,
			PowerBuffered: def.PowerBuffered,
		}
		for _, it := range def.InputItems {
			name := strings.ToLower(strings.TrimSpace(it.Name))
			id := it.ID
			if id == 0 && name != "" && w.itemIDsByName != nil {
				if v, ok := w.itemIDsByName[name]; ok {
					id = v
				}
			}
			if id <= 0 || it.Amount <= 0 {
				continue
			}
			recipe.InputItems = append(recipe.InputItems, ItemStack{Item: ItemID(id), Amount: it.Amount})
		}
		for _, liq := range def.InputLiquids {
			name := strings.ToLower(strings.TrimSpace(liq.Name))
			id := liq.ID
			if id == 0 && name != "" && w.liquidIDsByName != nil {
				if v, ok := w.liquidIDsByName[name]; ok {
					id = v
				}
			}
			if id <= 0 || liq.Amount <= 0 {
				continue
			}
			recipe.InputLiquids = append(recipe.InputLiquids, LiquidStack{Liquid: LiquidID(id), Amount: liq.Amount})
		}
		for _, it := range def.OutputItems {
			name := strings.ToLower(strings.TrimSpace(it.Name))
			id := it.ID
			if id == 0 && name != "" && w.itemIDsByName != nil {
				if v, ok := w.itemIDsByName[name]; ok {
					id = v
				}
			}
			if id <= 0 || it.Amount <= 0 {
				continue
			}
			recipe.OutputItems = append(recipe.OutputItems, ItemStack{Item: ItemID(id), Amount: it.Amount})
		}
		for _, liq := range def.OutputLiquids {
			name := strings.ToLower(strings.TrimSpace(liq.Name))
			id := liq.ID
			if id == 0 && name != "" && w.liquidIDsByName != nil {
				if v, ok := w.liquidIDsByName[name]; ok {
					id = v
				}
			}
			if id <= 0 || liq.Amount <= 0 {
				continue
			}
			recipe.OutputLiquids = append(recipe.OutputLiquids, LiquidStack{Liquid: LiquidID(id), Amount: liq.Amount})
		}
		if len(recipe.OutputItems) == 0 && len(recipe.OutputLiquids) == 0 {
			continue
		}
		w.recipesByBlockName[block] = recipe
	}
}
