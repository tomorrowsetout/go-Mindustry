package world

import (
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestControlledTurretFiresOnlyWhileShooting(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 10, 10, 45, 1, 0)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 30}}
	enemy := RawEntity{
		ID:          1,
		TypeID:      35,
		Team:        2,
		X:           float32(10*8 + 4 + 36),
		Y:           float32(10*8 + 4),
		Health:      100,
		MaxHealth:   100,
		SlowMul:     1,
		RuntimeInit: true,
	}
	model.Entities = append(model.Entities, enemy)

	buildPos := protocol.PackPoint2(10, 10)
	if _, ok := w.ClaimControlledBuildingPacked(7, buildPos); !ok {
		t.Fatal("expected duo turret to be claimable as a control block")
	}

	aimX := float32(10*8 + 4 + 36)
	aimY := float32(10*8 + 4)
	if ok := w.SetControlledBuildingInputPacked(7, buildPos, aimX, aimY, false); !ok {
		t.Fatal("expected controlled turret input update to succeed")
	}
	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}
	if got := model.Entities[0].Health; got != 100 {
		t.Fatalf("expected controlled turret to stay idle while not shooting, health=%f", got)
	}

	if ok := w.SetControlledBuildingInputPacked(7, buildPos, aimX, aimY, true); !ok {
		t.Fatal("expected controlled turret shooting update to succeed")
	}
	for i := 0; i < 90; i++ {
		w.Step(time.Second / 60)
		if model.Entities[0].Health < 100 {
			return
		}
	}
	t.Fatalf("expected controlled turret to damage target after shooting, health=%f", model.Entities[0].Health)
}

func TestControlledTurretRotatesTowardAimWithoutFiring(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 10, 10, 45, 1, 0)
	buildPos := protocol.PackPoint2(10, 10)
	if _, ok := w.ClaimControlledBuildingPacked(7, buildPos); !ok {
		t.Fatal("expected duo turret to be claimable as a control block")
	}

	if ok := w.SetControlledBuildingInputPacked(7, buildPos, float32(10*8-40), float32(10*8+4), false); !ok {
		t.Fatal("expected controlled turret input update to succeed")
	}
	for i := 0; i < 5; i++ {
		w.Step(time.Second / 60)
	}

	if tile.Rotation == 0 {
		t.Fatalf("expected controlled turret rotation to follow aim while idle, got=%d", tile.Rotation)
	}
}

func TestCanControlSelectBuildingPackedOnlyAllowsCoreAndPayloadBuildings(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
		418: "router",
		700: "payload-conveyor",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	payload := placeTestBuilding(t, w, 7, 5, 700, 1, 0)
	router := placeTestBuilding(t, w, 9, 5, 418, 1, 0)
	turret := placeTestBuilding(t, w, 11, 5, 45, 1, 0)

	if !w.CanControlSelectBuildingPacked(protocol.PackPoint2(int32(core.X), int32(core.Y))) {
		t.Fatal("expected core to remain control-selectable")
	}
	if !w.CanControlSelectBuildingPacked(protocol.PackPoint2(int32(payload.X), int32(payload.Y))) {
		t.Fatal("expected payload conveyor to remain control-selectable")
	}
	if w.CanControlSelectBuildingPacked(protocol.PackPoint2(int32(router.X), int32(router.Y))) {
		t.Fatal("expected router to stay non-control-selectable")
	}
	if w.CanControlSelectBuildingPacked(protocol.PackPoint2(int32(turret.X), int32(turret.Y))) {
		t.Fatal("expected turret to stay non-control-selectable")
	}
}
