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

func TestRulesManagerEmptyTags(t *testing.T) {
	tags := map[string]string{
		"otherTag": "value",
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected no error for empty rules, got %v", err)
	}
}
