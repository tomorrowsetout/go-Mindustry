package world

func (w *World) stepBuildAIFillCoresLocked() {
	if w == nil || w.model == nil || w.rulesMgr == nil || len(w.teamPrimaryCore) == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.BuildAi || rules.Pvp || rules.Editor {
		return
	}
	for team, corePos := range w.teamPrimaryCore {
		if team == 0 || corePos < 0 || int(corePos) >= len(w.model.Tiles) {
			continue
		}
		coreTile := &w.model.Tiles[corePos]
		if coreTile.Block == 0 || coreTile.Build == nil || coreTile.Team != team {
			continue
		}
		capacity := w.itemCapacityAtLocked(corePos)
		if capacity <= 0 {
			continue
		}
		changed := make([]ItemID, 0, len(buildAIFillCoreItemIDs()))
		for _, item := range buildAIFillCoreItemIDs() {
			if item < 0 || coreTile.Build.ItemAmount(item) == capacity {
				continue
			}
			coreTile.Build.SetItemAmount(item, capacity)
			changed = append(changed, item)
		}
		if len(changed) > 0 {
			w.emitTeamCoreItemsLocked(team, changed)
		}
	}
}

func buildAIFillCoreItemIDs() []ItemID {
	return []ItemID{
		copperItemID,
		leadItemID,
		metaglassItemID,
		graphiteItemID,
		sandItemID,
		coalItemID,
		titaniumItemID,
		thoriumItemID,
		scrapItemID,
		siliconItemID,
		plastaniumItemID,
		phaseFabricItemID,
		surgeAlloyItemID,
		sporePodItemID,
		blastCompoundItemID,
		pyratiteItemID,
		berylliumItemID,
		tungstenItemID,
		oxideItemID,
		carbideItemID,
		fissileMatterItemID,
	}
}
