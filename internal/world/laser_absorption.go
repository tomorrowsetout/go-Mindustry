package world

func blockAbsorbsLasers(name string) bool {
	switch normalizeBlockLookupName(name) {
	case "plastaniumwall", "plastaniumwalllarge":
		return true
	default:
		return false
	}
}

func (w *World) findLaserAbsorberLocked(team TeamID, x1, y1, x2, y2 float32) (int32, bool) {
	if w == nil || w.model == nil || team == 0 {
		return 0, false
	}
	tx0 := int(x1 / 8)
	ty0 := int(y1 / 8)
	tx1 := int(x2 / 8)
	ty1 := int(y2 / 8)
	if !w.model.InBounds(tx0, ty0) || !w.model.InBounds(tx1, ty1) {
		return 0, false
	}
	dx := absInt(tx1 - tx0)
	dy := absInt(ty1 - ty0)
	sx := 1
	if tx0 > tx1 {
		sx = -1
	}
	sy := 1
	if ty0 > ty1 {
		sy = -1
	}
	err := dx - dy
	for {
		if tx0 != int(x1/8) || ty0 != int(y1/8) {
			if pos, ok := w.buildingOccupyingCellLocked(tx0, ty0); ok && pos >= 0 && int(pos) < len(w.model.Tiles) {
				tile := &w.model.Tiles[pos]
				if tile.Build != nil && tile.Build.Team != team && blockAbsorbsLasers(w.blockNameByID(int16(tile.Block))) {
					return pos, true
				}
			}
		}
		if tx0 == tx1 && ty0 == ty1 {
			break
		}
		e2 := err * 2
		if e2 > -dy {
			err -= dy
			tx0 += sx
		}
		if e2 < dx {
			err += dx
			ty0 += sy
		}
	}
	return 0, false
}
