package vanilla

import "testing"

func TestExtractBlocksParsesArmorWithoutLeakingNestedBulletFields(t *testing.T) {
	src := `
		copperWallLarge = new Wall("copper-wall-large"){{
			requirements(Category.defense, with(Items.copper, 24));
			armor = 8f;
			ammo(
				Items.graphite, new BasicBulletType(4f, 10f){{
					armorMultiplier = 4f;
				}}
			);
		}};
	`

	blocks := extractBlocks(src, map[string]itemMeta{
		"copper": {ID: 0, Name: "copper", Cost: 1},
	})
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block profile, got %d", len(blocks))
	}
	if blocks[0].Name != "copper-wall-large" {
		t.Fatalf("expected block name copper-wall-large, got=%q", blocks[0].Name)
	}
	if blocks[0].Armor != 8 {
		t.Fatalf("expected block armor=8, got=%f", blocks[0].Armor)
	}
}
