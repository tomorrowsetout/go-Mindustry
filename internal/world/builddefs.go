package world

import (
	"encoding/json"
	"os"
	"strings"
)

type BlockBuildDef struct {
	Block               string          `json:"block"`
	BuildTime           float32         `json:"buildTime,omitempty"`           // ticks
	BuildCostMultiplier float32         `json:"buildCostMultiplier,omitempty"` // block-level multiplier
	Requirements        []BuildReqEntry `json:"requirements,omitempty"`
}

type BuildReqEntry struct {
	Item   string `json:"item"`
	Amount int32  `json:"amount"`
}

type BlockBuildResolved struct {
	BuildTime    float32
	Requirements []ItemStack
}

func (w *World) LoadBlockBuild(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var defs []BlockBuildDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	w.blockBuildDefs = defs
	w.blockBuildByName = make(map[string]BlockBuildDef, len(defs))
	for _, d := range defs {
		name := strings.ToLower(strings.TrimSpace(d.Block))
		if name == "" {
			continue
		}
		w.blockBuildByName[name] = d
	}
	w.resolveBlockBuildsLocked()
	return nil
}

func (w *World) resolveBlockBuildsLocked() {
	w.blockBuildByID = nil
	if len(w.blockBuildByName) == 0 || w.blockNamesByID == nil || w.itemIDsByName == nil {
		return
	}
	nameToID := make(map[string]int16, len(w.blockNamesByID))
	for id, name := range w.blockNamesByID {
		n := strings.ToLower(strings.TrimSpace(name))
		if n != "" {
			nameToID[n] = id
		}
	}
	resolved := make(map[int16]BlockBuildResolved, len(w.blockBuildByName))
	for name, def := range w.blockBuildByName {
		blockID, ok := nameToID[name]
		if !ok {
			continue
		}
		reqs := make([]ItemStack, 0, len(def.Requirements))
		for _, r := range def.Requirements {
			itemName := strings.ToLower(strings.TrimSpace(r.Item))
			if itemName == "" || r.Amount <= 0 {
				continue
			}
			itemID, ok := w.itemIDsByName[itemName]
			if !ok {
				continue
			}
			reqs = append(reqs, ItemStack{Item: ItemID(itemID), Amount: r.Amount})
		}
		bt := def.BuildTime
		if bt <= 0 {
			bt = 20
		}
		resolved[blockID] = BlockBuildResolved{
			BuildTime:    bt,
			Requirements: reqs,
		}
	}
	w.blockBuildByID = resolved
}

func (w *World) blockBuildForID(blockID int16) (BlockBuildResolved, bool) {
	if w == nil {
		return BlockBuildResolved{}, false
	}
	if w.blockBuildByID == nil {
		return BlockBuildResolved{}, false
	}
	def, ok := w.blockBuildByID[blockID]
	return def, ok
}
