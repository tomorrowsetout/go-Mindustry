package vanilla

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdt-server/internal/protocol"
)

func TestLoadContentIDsIncludesOfficialBuilderCommandAndStanceIDs(t *testing.T) {
	ids, err := LoadContentIDs(filepath.Join("..", "..", "data", "vanilla", "content_ids.json"))
	if err != nil {
		t.Fatalf("load content ids: %v", err)
	}

	assertEntry := func(entries []ContentIDEntry, id int16, want string) {
		t.Helper()
		for _, entry := range entries {
			if entry.ID != id {
				continue
			}
			if entry.Name != want {
				t.Fatalf("expected id=%d to be %q, got %q", id, want, entry.Name)
			}
			return
		}
		t.Fatalf("expected id=%d entry %q", id, want)
	}

	assertEntry(ids.Commands, 5, "enterpayload")
	assertEntry(ids.Commands, 9, "looppayload")
	assertEntry(ids.Stances, 6, "holdposition")
	assertEntry(ids.Stances, 7, "mineauto")
	assertEntry(ids.Stances, 8, "item-copper")
	assertEntry(ids.Stances, 9, "item-lead")
}

func TestApplyContentIDsRegistersUnitCommandAndStanceNames(t *testing.T) {
	reg := protocol.NewContentRegistry()
	ApplyContentIDs(reg, &ContentIDsFile{
		Commands: []ContentIDEntry{
			{ID: 5, Name: "enterpayload"},
		},
		Stances: []ContentIDEntry{
			{ID: 6, Name: "holdposition"},
			{ID: 7, Name: "mineauto"},
		},
	})

	if got := reg.UnitCommand(5); got.Name != "enterpayload" {
		t.Fatalf("expected registered command name enterpayload, got %+v", got)
	}
	if got := reg.UnitStance(6); got.Name != "holdposition" {
		t.Fatalf("expected registered stance name holdposition, got %+v", got)
	}
	if got := reg.UnitStance(7); got.Name != "mineauto" {
		t.Fatalf("expected registered stance name mineauto, got %+v", got)
	}
}

func TestParseNamedNewEntriesMatchesUnitTypeSubclasses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "UnitTypes.java")
	src := []byte(`
		evoke = new ErekirUnitType("evoke"){{
		}};
		emanate = new MissileUnitType("emanate"){{
		}};
		alpha = new UnitType("alpha"){{
		}};
	`)
	if err := os.WriteFile(path, src, 0644); err != nil {
		t.Fatalf("write temp UnitTypes.java: %v", err)
	}

	entries, err := parseNamedNewEntries(path, `=\s*new\s+[A-Za-z0-9_$.]*UnitType\s*\(\s*"([^"]+)"`)
	if err != nil {
		t.Fatalf("parse unit subclasses: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 parsed unit entries, got %d", len(entries))
	}
	if entries[0].Name != "evoke" || entries[1].Name != "emanate" || entries[2].Name != "alpha" {
		t.Fatalf("unexpected parsed entries: %+v", entries)
	}
}

func TestParseBulletEntriesIncludesInlineAndImplicitWeaponBulletsInLoadOrder(t *testing.T) {
	dir := t.TempDir()
	bulletsPath := filepath.Join(dir, "Bullets.java")
	unitsPath := filepath.Join(dir, "UnitTypes.java")
	blocksPath := filepath.Join(dir, "Blocks.java")

	if err := os.WriteFile(bulletsPath, []byte(`
		placeholder = new BasicBulletType(2.5f, 9){{
		}};
		damageLightning = new BulletType(0.0001f, 0f){{
		}};
	`), 0644); err != nil {
		t.Fatalf("write Bullets.java: %v", err)
	}
	if err := os.WriteFile(unitsPath, []byte(`
		alpha.weapons.add(new Weapon(){{
			bullet = new BasicBulletType(3f, 10){{
			}};
		}});
		alpha.weapons.add(new BuildWeapon("build-weapon"){{
		}});
	`), 0644); err != nil {
		t.Fatalf("write UnitTypes.java: %v", err)
	}
	if err := os.WriteFile(blocksPath, []byte(`
		duo = new ItemTurret("duo"){{
			ammo(
				Items.copper, new BasicBulletType(2.5f, 9){{
				}}
			);
		}};
	`), 0644); err != nil {
		t.Fatalf("write Blocks.java: %v", err)
	}

	entries, err := parseBulletEntries(bulletsPath, unitsPath, blocksPath)
	if err != nil {
		t.Fatalf("parse bullet entries: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 bullet entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].ID != 0 || entries[1].ID != 1 || entries[2].ID != 2 || entries[3].ID != 3 || entries[4].ID != 4 {
		t.Fatalf("expected sequential bullet ids, got %+v", entries)
	}
	if !strings.Contains(entries[3].Name, "buildweapon") {
		t.Fatalf("expected build weapon default bullet entry at index 3, got %+v", entries[3])
	}
}
