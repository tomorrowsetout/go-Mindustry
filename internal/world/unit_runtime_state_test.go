package world

import (
	"testing"

	"mdt-server/internal/protocol"
)

func TestSetEntityMotionAndPositionUpdateAuthoritativeFields(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	if _, err := w.AddEntityWithID(35, 1234, 16, 24, 1); err != nil {
		t.Fatalf("add entity: %v", err)
	}
	if _, ok := w.SetEntityMotion(1234, 3.5, -2.25, 1.75); !ok {
		t.Fatal("expected motion update to succeed")
	}
	if _, ok := w.SetEntityPosition(1234, 44, 88, 135); !ok {
		t.Fatal("expected position update to succeed")
	}

	ent, ok := w.GetEntity(1234)
	if !ok {
		t.Fatal("expected entity after updates")
	}
	if ent.VelX != 3.5 || ent.VelY != -2.25 || ent.RotVel != 1.75 {
		t.Fatalf("unexpected motion state: vx=%v vy=%v rotVel=%v", ent.VelX, ent.VelY, ent.RotVel)
	}
	if ent.X != 44 || ent.Y != 88 || ent.Rotation != 135 {
		t.Fatalf("unexpected position state: x=%v y=%v rotation=%v", ent.X, ent.Y, ent.Rotation)
	}
}

func TestSetEntityRuntimeStateClonesPlansAndMineTile(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	if _, err := w.AddEntityWithID(35, 2234, 16, 24, 1); err != nil {
		t.Fatalf("add entity: %v", err)
	}

	configBytes := []byte{9, 8, 7}
	plans := []*protocol.BuildPlan{{
		X:        5,
		Y:        6,
		Rotation: 3,
		Block:    protocol.BlockRef{BlkID: 45, BlkName: "router"},
		Config:   configBytes,
	}}
	minePos := protocol.PackPoint2(3, 4)
	if _, ok := w.SetEntityRuntimeState(2234, true, true, true, minePos, plans); !ok {
		t.Fatal("expected runtime state update to succeed")
	}

	configBytes[0] = 42
	ent, ok := w.GetEntity(2234)
	if !ok {
		t.Fatal("expected entity after runtime state update")
	}
	if !ent.Shooting || !ent.UpdateBuilding || ent.MineTilePos != minePos || ent.Elevation != 1 {
		t.Fatalf("unexpected runtime flags: shooting=%v updateBuilding=%v mine=%d elevation=%v", ent.Shooting, ent.UpdateBuilding, ent.MineTilePos, ent.Elevation)
	}
	if len(ent.Plans) != 1 {
		t.Fatalf("expected one stored build plan, got %d", len(ent.Plans))
	}
	if ent.Plans[0].BlockID != 45 || ent.Plans[0].Pos != protocol.PackPoint2(5, 6) {
		t.Fatalf("unexpected stored build plan: %+v", ent.Plans[0])
	}
	storedConfig, ok := ent.Plans[0].Config.([]byte)
	if !ok {
		t.Fatalf("expected stored config bytes, got %T", ent.Plans[0].Config)
	}
	if len(storedConfig) != 3 || storedConfig[0] != 9 {
		t.Fatalf("expected cloned config [9 8 7], got %v", storedConfig)
	}

	if _, ok := w.SetEntityRuntimeState(2234, false, false, false, -1, nil); !ok {
		t.Fatal("expected runtime reset to succeed")
	}
	ent, ok = w.GetEntity(2234)
	if !ok {
		t.Fatal("expected entity after runtime reset")
	}
	if ent.MineTilePos != invalidEntityTilePos {
		t.Fatalf("expected invalid mine tile sentinel, got %d", ent.MineTilePos)
	}
	if len(ent.Plans) != 0 {
		t.Fatalf("expected runtime reset to clear plans, got %#v", ent.Plans)
	}
	if ent.Elevation != 0 {
		t.Fatalf("expected non-flying non-boosting runtime reset to drop elevation, got %v", ent.Elevation)
	}
}

func TestSetEntityPlayerControllerUpdatesAuthoritativeOwner(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	if _, err := w.AddEntityWithID(35, 3234, 16, 24, 1); err != nil {
		t.Fatalf("add entity: %v", err)
	}
	if _, ok := w.SetEntityPlayerController(3234, 77); !ok {
		t.Fatal("expected player controller update to succeed")
	}

	ent, ok := w.GetEntity(3234)
	if !ok {
		t.Fatal("expected entity after player controller update")
	}
	if ent.PlayerID != 77 {
		t.Fatalf("expected player id 77, got %d", ent.PlayerID)
	}
}
