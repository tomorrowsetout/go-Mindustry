package vanilla

import "testing"

func TestParseUnitMetadataTracksPayloadFlagsAndMissileDefaults(t *testing.T) {
	meta := parseUnitMetadata(`
		hitSize = 11f;
		allowedInPayloads = false;
		pickupUnits = false;
	`, nil, "ErekirUnitType")
	if meta.allowedInPayloads {
		t.Fatal("expected explicit allowedInPayloads=false to be preserved")
	}
	if meta.pickupUnits {
		t.Fatal("expected explicit pickupUnits=false to be preserved")
	}

	defaults := parseUnitMetadata(`hitSize = 8f;`, nil, "LegsUnitType")
	if !defaults.allowedInPayloads {
		t.Fatal("expected regular unit types to default allowedInPayloads=true")
	}
	if !defaults.pickupUnits {
		t.Fatal("expected regular unit types to default pickupUnits=true")
	}

	missile := parseUnitMetadata(`hitSize = 6f;`, nil, "MissileUnitType")
	if missile.allowedInPayloads {
		t.Fatal("expected MissileUnitType default allowedInPayloads=false")
	}
}

func TestExtractUnitsMatchesUnitTypeSubclasses(t *testing.T) {
	src := `
		public static @EntityDef({Unitc.class, BuildingTetherc.class, Payloadc.class}) UnitType manifold;
		public static @EntityDef({Unitc.class, TimedKillc.class}) UnitType scatheMissile;

		manifold = new ErekirUnitType("manifold"){{
			hitSize = 11f;
			allowedInPayloads = false;
		}};

		scatheMissile = new MissileUnitType("scathe-missile"){{
			hitSize = 6f;
		}};
	`

	units, err := extractUnits(src, nil)
	if err != nil {
		t.Fatalf("extract units failed: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("expected 2 unit profiles, got %d", len(units))
	}

	got := map[string]UnitProfile{}
	for _, unit := range units {
		got[unit.Name] = unit
	}
	if manifold, ok := got["manifold"]; !ok {
		t.Fatal("expected manifold profile to be extracted from ErekirUnitType")
	} else if manifold.AllowedInPayloads {
		t.Fatal("expected manifold allowed_in_payloads=false")
	} else if manifold.MovementClass != "tether" || !manifold.BuildingTether || !manifold.PayloadUnit {
		t.Fatalf("expected manifold entity definition to be preserved, got movement=%q tether=%v payload=%v", manifold.MovementClass, manifold.BuildingTether, manifold.PayloadUnit)
	}
	if missile, ok := got["scathe-missile"]; !ok {
		t.Fatal("expected scathe-missile profile to be extracted from MissileUnitType")
	} else if missile.AllowedInPayloads {
		t.Fatal("expected MissileUnitType profile default allowed_in_payloads=false")
	} else if missile.MovementClass != "missile" || !missile.TimedKill {
		t.Fatalf("expected missile entity definition to be preserved, got movement=%q timedKill=%v", missile.MovementClass, missile.TimedKill)
	}
}

func TestExtractUnitEntityDefsClassifiesMovementComponents(t *testing.T) {
	src := `
		public static @EntityDef({Unitc.class, WaterMovec.class}) UnitType risso, minke;
		public static @EntityDef({Unitc.class, Legsc.class}) UnitType merui;
		public static @EntityDef({Unitc.class, ElevationMovec.class}) UnitType elude;
		public static @EntityDef({Unitc.class, Tankc.class}) UnitType stell;
	`
	defs := extractUnitEntityDefs(src)
	if got := defs["risso"]; got.movementClass != "naval" || !got.naval {
		t.Fatalf("expected risso naval definition, got %+v", got)
	}
	if got := defs["merui"]; got.movementClass != "legs" || !got.legged {
		t.Fatalf("expected merui legs definition, got %+v", got)
	}
	if got := defs["elude"]; got.movementClass != "hover" || !got.hover {
		t.Fatalf("expected elude hover definition, got %+v", got)
	}
	if got := defs["stell"]; got.movementClass != "tank" || !got.tank {
		t.Fatalf("expected stell tank definition, got %+v", got)
	}
	if got := defs["minke"]; got.movementClass != "naval" || !got.naval {
		t.Fatalf("expected comma-separated minke naval definition, got %+v", got)
	}
}
