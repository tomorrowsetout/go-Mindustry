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
	if !rules.AiCoreSpawn {
		t.Fatalf("expected default aiCoreSpawn=true to match official team rule defaults")
	}
	if !rules.WavesSpawnAtCores {
		t.Fatalf("expected default wavesSpawnAtCores=true to match official rules")
	}
	if rules.BuildAiTier != 1 {
		t.Fatalf("expected default buildAiTier=1 to match official team rule defaults, got %d", rules.BuildAiTier)
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

func TestRulesManagerFromTagsSupportsMindustryJsonIOStyleRules(t *testing.T) {
	tags := map[string]string{
		"rules": `{attackMode:true,infiniteResources:true,waveTimer:false,staticFog:false,teams:{2:{fillItems:true,infiniteResources:true}},waveTeam:crux}`,
	}

	rm := NewRulesManager(nil)
	if err := rm.FromTags(tags); err != nil {
		t.Fatalf("expected Java-style rules tag to parse, got %v", err)
	}

	rules := rm.Get()
	if !rules.AttackMode {
		t.Fatal("expected attackMode=true from Java-style rules tag")
	}
	if !rules.InfiniteResources {
		t.Fatal("expected infiniteResources=true from Java-style rules tag")
	}
	if rules.WaveTimer {
		t.Fatal("expected waveTimer=false from Java-style rules tag")
	}
	if rules.StaticFog {
		t.Fatal("expected staticFog=false from Java-style rules tag")
	}
	if rules.WaveTeam != "crux" {
		t.Fatalf("expected waveTeam=crux, got %q", rules.WaveTeam)
	}
	team2, ok := rules.teamRule(2)
	if !ok {
		t.Fatal("expected team 2 rules to be present")
	}
	if !team2.FillItems {
		t.Fatal("expected teams[2].fillItems=true")
	}
	if !team2.InfiniteResources {
		t.Fatal("expected teams[2].infiniteResources=true")
	}
}

func TestDescribeRuleModePrefersInferredModeAndFlags(t *testing.T) {
	model := NewWorldModel(8, 8)
	model.Tags = map[string]string{"mode": "sandbox"}
	rules := DefaultRules()
	rules.ModeName = "custom-sandbox"
	rules.Waves = true
	rules.WaveTimer = false
	rules.InfiniteResources = true
	rules.InfiniteAmmo = true

	summary := DescribeRuleMode(model, rules)
	if summary.Mode != "sandbox" {
		t.Fatalf("expected inferred mode sandbox, got %q", summary.Mode)
	}
	if summary.ModeName != "custom-sandbox" {
		t.Fatalf("expected modeName custom-sandbox, got %q", summary.ModeName)
	}
	if !summary.InfiniteResources || !summary.InfiniteAmmo {
		t.Fatal("expected summary to expose infinite resource/ammo flags")
	}
	if summary.WaveTimer {
		t.Fatal("expected summary waveTimer=false")
	}
}
