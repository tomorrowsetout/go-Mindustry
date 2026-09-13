package oracle

import (
	"fmt"
	"math"
	"sort"
)

type Difference struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func CompareTraces(expected, actual Trace) []Difference {
	var diffs []Difference
	if expected.Scenario.Ticks != actual.Scenario.Ticks {
		diffs = append(diffs, Difference{
			Path:    "scenario.ticks",
			Message: fmt.Sprintf("expected %d, got %d", expected.Scenario.Ticks, actual.Scenario.Ticks),
		})
	}
	if len(expected.Ticks) != len(actual.Ticks) {
		diffs = append(diffs, Difference{
			Path:    "ticks.length",
			Message: fmt.Sprintf("expected %d, got %d", len(expected.Ticks), len(actual.Ticks)),
		})
	}
	limit := len(expected.Ticks)
	if len(actual.Ticks) < limit {
		limit = len(actual.Ticks)
	}
	tol := expected.Tolerances
	if tol.Position == 0 {
		tol.Position = PositionTolerance
	}
	if tol.Angle == 0 {
		tol.Angle = AngleTolerance
	}
	if tol.Scalar == 0 {
		tol.Scalar = ScalarTolerance
	}
	for i := 0; i < limit; i++ {
		diffs = append(diffs, compareTickTrace(i, expected.Ticks[i], actual.Ticks[i], tol)...)
	}
	return diffs
}

func compareTickTrace(index int, expected, actual TickTrace, tol Tolerances) []Difference {
	var diffs []Difference
	if expected.Index != actual.Index {
		diffs = append(diffs, Difference{
			Path:    fmt.Sprintf("ticks[%d].index", index),
			Message: fmt.Sprintf("expected %d, got %d", expected.Index, actual.Index),
		})
	}
	expectedTiles := make(map[string]TileState, len(expected.Tiles))
	actualTiles := make(map[string]TileState, len(actual.Tiles))
	for _, tile := range expected.Tiles {
		expectedTiles[tileKey(tile.X, tile.Y)] = tile
	}
	for _, tile := range actual.Tiles {
		actualTiles[tileKey(tile.X, tile.Y)] = tile
	}
	diffs = append(diffs, compareTileMaps(index, expectedTiles, actualTiles, tol)...)

	expectedUnits := make(map[int]UnitState, len(expected.Units))
	actualUnits := make(map[int]UnitState, len(actual.Units))
	for _, unit := range expected.Units {
		expectedUnits[unit.ID] = unit
	}
	for _, unit := range actual.Units {
		actualUnits[unit.ID] = unit
	}
	diffs = append(diffs, compareUnitMaps(index, expectedUnits, actualUnits, tol)...)
	return diffs
}

func compareTileMaps(index int, expected, actual map[string]TileState, tol Tolerances) []Difference {
	var diffs []Difference
	keys := unionStringKeys(expected, actual)
	for _, key := range keys {
		e, eok := expected[key]
		a, aok := actual[key]
		path := fmt.Sprintf("ticks[%d].tiles[%s]", index, key)
		if !eok || !aok {
			diffs = append(diffs, Difference{
				Path:    path,
				Message: presenceMessage(eok, aok),
			})
			continue
		}
		checkInt(&diffs, path+".blockId", e.BlockID, a.BlockID)
		checkInt(&diffs, path+".teamId", e.TeamID, a.TeamID)
		checkInt(&diffs, path+".rotation", e.Rotation, a.Rotation)
		checkInt(&diffs, path+".constructBlockId", e.ConstructBlockID, a.ConstructBlockID)
		checkFloat(&diffs, path+".constructProgress", e.ConstructProgress, a.ConstructProgress, tol.Scalar)
		checkFloat(&diffs, path+".buildHealth", e.BuildHealth, a.BuildHealth, tol.Scalar)
		checkString(&diffs, path+".controllerType", e.ControllerType, a.ControllerType)
		checkFloat(&diffs, path+".powerStored", e.PowerStored, a.PowerStored, tol.Scalar)
		checkFloat(&diffs, path+".powerBalance", e.PowerBalance, a.PowerBalance, tol.Scalar)
		checkFloat(&diffs, path+".heat", e.Heat, a.Heat, tol.Scalar)
		checkFloat(&diffs, path+".reload", e.Reload, a.Reload, tol.Scalar)
		diffs = append(diffs, compareSlots(path+".items", e.Items, a.Items, tol.Scalar)...)
		diffs = append(diffs, compareSlots(path+".liquids", e.Liquids, a.Liquids, tol.Scalar)...)
	}
	return diffs
}

func compareUnitMaps(index int, expected, actual map[int]UnitState, tol Tolerances) []Difference {
	var diffs []Difference
	keys := unionIntKeys(expected, actual)
	for _, key := range keys {
		e, eok := expected[key]
		a, aok := actual[key]
		path := fmt.Sprintf("ticks[%d].units[%d]", index, key)
		if !eok || !aok {
			diffs = append(diffs, Difference{
				Path:    path,
				Message: presenceMessage(eok, aok),
			})
			continue
		}
		checkInt(&diffs, path+".typeId", e.TypeID, a.TypeID)
		checkInt(&diffs, path+".teamId", e.TeamID, a.TeamID)
		checkString(&diffs, path+".controllerType", e.ControllerType, a.ControllerType)
		checkFloat(&diffs, path+".x", e.X, a.X, tol.Position)
		checkFloat(&diffs, path+".y", e.Y, a.Y, tol.Position)
		checkFloat(&diffs, path+".rotation", e.Rotation, a.Rotation, tol.Angle)
		checkFloat(&diffs, path+".velocityX", e.VelocityX, a.VelocityX, tol.Position)
		checkFloat(&diffs, path+".velocityY", e.VelocityY, a.VelocityY, tol.Position)
		checkFloat(&diffs, path+".health", e.Health, a.Health, tol.Scalar)
		checkFloat(&diffs, path+".shield", e.Shield, a.Shield, tol.Scalar)
	}
	return diffs
}

func compareSlots(path string, expected, actual []SlotState, tol float64) []Difference {
	var diffs []Difference
	expectedMap := make(map[int]float64, len(expected))
	actualMap := make(map[int]float64, len(actual))
	for _, slot := range expected {
		expectedMap[slot.ID] = slot.Amount
	}
	for _, slot := range actual {
		actualMap[slot.ID] = slot.Amount
	}
	keys := make([]int, 0, len(expectedMap)+len(actualMap))
	for key := range expectedMap {
		keys = append(keys, key)
	}
	for key := range actualMap {
		if _, ok := expectedMap[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for _, key := range keys {
		e, eok := expectedMap[key]
		a, aok := actualMap[key]
		if !eok || !aok {
			diffs = append(diffs, Difference{
				Path:    fmt.Sprintf("%s[%d]", path, key),
				Message: presenceMessage(eok, aok),
			})
			continue
		}
		checkFloat(&diffs, fmt.Sprintf("%s[%d]", path, key), e, a, tol)
	}
	return diffs
}

func tileKey(x, y int) string {
	return fmt.Sprintf("%d,%d", x, y)
}

func unionStringKeys(left, right map[string]TileState) []string {
	keys := make([]string, 0, len(left)+len(right))
	seen := map[string]struct{}{}
	for key := range left {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range right {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unionIntKeys(left, right map[int]UnitState) []int {
	keys := make([]int, 0, len(left)+len(right))
	seen := map[int]struct{}{}
	for key := range left {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range right {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func presenceMessage(expected, actual bool) string {
	switch {
	case expected && !actual:
		return "missing from actual trace"
	case !expected && actual:
		return "unexpected extra state in actual trace"
	default:
		return "presence mismatch"
	}
}

func checkInt(diffs *[]Difference, path string, expected, actual int) {
	if expected == actual {
		return
	}
	*diffs = append(*diffs, Difference{
		Path:    path,
		Message: fmt.Sprintf("expected %d, got %d", expected, actual),
	})
}

func checkString(diffs *[]Difference, path, expected, actual string) {
	if expected == actual {
		return
	}
	*diffs = append(*diffs, Difference{
		Path:    path,
		Message: fmt.Sprintf("expected %q, got %q", expected, actual),
	})
}

func checkFloat(diffs *[]Difference, path string, expected, actual, tolerance float64) {
	if math.Abs(expected-actual) <= tolerance {
		return
	}
	*diffs = append(*diffs, Difference{
		Path:    path,
		Message: fmt.Sprintf("expected %.6f, got %.6f (tol=%.6f)", expected, actual, tolerance),
	})
}
