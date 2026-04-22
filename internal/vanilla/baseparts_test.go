package vanilla

import "testing"

func TestLoadEmbeddedBasePartSchematicsIncludesOfficialData(t *testing.T) {
	parts, err := LoadEmbeddedBasePartSchematics()
	if err != nil {
		t.Fatalf("load embedded baseparts: %v", err)
	}
	if len(parts) < 200 {
		t.Fatalf("expected official basepart set to be loaded, got %d", len(parts))
	}
	foundCopper := false
	for _, part := range parts {
		if part.Width <= 0 || part.Height <= 0 || len(part.Tiles) == 0 {
			t.Fatalf("expected non-empty embedded basepart, got %+v", part)
		}
		for _, tile := range part.Tiles {
			if tile.Block != "item-source" {
				continue
			}
			ref, ok := tile.Config.(BasePartContentRef)
			if ok && ref.ContentType == BasePartContentItem && ref.ID == 0 {
				foundCopper = true
				break
			}
		}
		if foundCopper {
			break
		}
	}
	if !foundCopper {
		t.Fatal("expected at least one embedded copper-required official basepart")
	}
}
