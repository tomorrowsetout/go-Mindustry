package world

import (
	"testing"
)

func TestRulesDefault(t *testing.T) {
	rules := DefaultRules()
	if rules.UnitDamageMultiplier != 1.0 {
		t.Fatalf("expected default unitDamageMultiplier=1.0, got %f", rules.UnitDamageMultiplier)
	}
	if rules.UnitHealthMultiplier != 1.0 {
		t.Fatalf("expected default unitHealthMultiplier=1.0, got %f", rules.UnitHealthMultiplier)
	}
}

func TestRulesManagerClone(t *testing.T) {
	rm := NewRulesManager(nil)
	rm.SetField("unitDamageMultiplier", 1.5)

	cloned := rm.Clone()
	if cloned.UnitDamageMultiplier != 1.5 {
		t.Fatalf("expected cloned value=1.5, got %f", cloned.UnitDamageMultiplier)
	}

	// 修改克隆不影响原值
	cloned.UnitDamageMultiplier = 2.0
	original := rm.Get()
	if original.UnitDamageMultiplier != 1.5 {
		t.Fatalf("expected original unchanged=1.5, got %f", original.UnitDamageMultiplier)
	}
}

func TestRulesManagerFromTags(t *testing.T) {
	jsonData := `{"unitDamageMultiplier": 1.3}`
	tags := map[string]string{
		"rules": jsonData,
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rules := rm.Get()
	if rules.UnitDamageMultiplier != 1.3 {
		t.Fatalf("expected unitDamageMultiplier=1.3, got %f", rules.UnitDamageMultiplier)
	}
}

func TestRulesManagerFromTagsAppliesExplicitModeDefaultsFirst(t *testing.T) {
	tags := map[string]string{
		"mode":  "editor",
		"rules": `{"buildCostMultiplier":2}`,
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rules := rm.Get()
	if !rules.Editor || !rules.InstantBuild || !rules.InfiniteResources {
		t.Fatalf("expected editor defaults from explicit mode tag, got editor=%v instant=%v infinite=%v", rules.Editor, rules.InstantBuild, rules.InfiniteResources)
	}
	if rules.BuildCostMultiplier != 2 {
		t.Fatalf("expected overlay build cost multiplier to remain 2, got %f", rules.BuildCostMultiplier)
	}
}

func TestRulesManagerFromJSONInfersAttackDefaultsFromFlags(t *testing.T) {
	rm := NewRulesManager(nil)
	if err := rm.FromJSON([]byte(`{"attackMode":true}`)); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rules := rm.Get()
	if !rules.AttackMode {
		t.Fatalf("expected attack mode to be enabled")
	}
	if rules.WaveSpacing != 120 {
		t.Fatalf("expected attack defaults wave spacing=120, got %f", rules.WaveSpacing)
	}
	if !rules.teamInfiniteResources(2) {
		t.Fatalf("expected attack defaults to grant wave team infinite resources")
	}
}

func TestRulesManagerFromTagsAppliesExplicitPvpDefaults(t *testing.T) {
	tags := map[string]string{
		"mode": "pvp",
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rules := rm.Get()
	if !rules.Pvp || !rules.AttackMode {
		t.Fatalf("expected pvp defaults to enable pvp and attack, got pvp=%v attack=%v", rules.Pvp, rules.AttackMode)
	}
	if rules.EnemyCoreBuildRadius != 600 {
		t.Fatalf("expected pvp enemy core build radius=600, got %f", rules.EnemyCoreBuildRadius)
	}
	if rules.BuildCostMultiplier != 1 {
		t.Fatalf("expected pvp build cost multiplier=1, got %f", rules.BuildCostMultiplier)
	}
	if rules.BuildSpeedMultiplier != 1 {
		t.Fatalf("expected pvp build speed multiplier=1, got %f", rules.BuildSpeedMultiplier)
	}
	if rules.UnitBuildSpeedMultiplier != 2 {
		t.Fatalf("expected pvp unit build speed multiplier=2, got %f", rules.UnitBuildSpeedMultiplier)
	}
}

func TestRulesManagerEmptyTags(t *testing.T) {
	tags := map[string]string{
		"otherTag": "value",
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected no error for empty rules, got %v", err)
	}
}
