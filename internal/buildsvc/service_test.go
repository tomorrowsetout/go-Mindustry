package buildsvc

import (
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func newBuildSvcTestWorld(t *testing.T) *world.World {
	t.Helper()
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	coreTile, err := model.TileAt(4, 4)
	if err != nil || coreTile == nil {
		t.Fatalf("core tile lookup failed: %v", err)
	}
	coreTile.Block = 339
	coreTile.Team = 1
	coreTile.Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         4,
		Y:         4,
		Health:    1000,
		MaxHealth: 1000,
		Items:     []world.ItemStack{{Item: 0, Amount: 500}},
	}
	if _, err := w.AddEntityWithID(35, 9001, float32(4*8+4), float32(4*8+4), 1); err != nil {
		t.Fatalf("add builder entity failed: %v", err)
	}
	w.UpdateBuilderState(101, 1, 9001, float32(4*8+4), float32(4*8+4), true, 220)
	return w
}

func TestSyncPlansEmptyAfterNonEmptyClearsOwnerQueue(t *testing.T) {
	w := newBuildSvcTestWorld(t)
	svc := New(w, Options{})

	svc.SyncPlans(101, 1, []*protocol.BuildPlan{{
		X: 2, Y: 2, Rotation: 0,
		Block: protocol.BlockRef{BlkID: 45, BlkName: "duo"},
	}})
	if !w.HasPendingPlansForOwner(101) {
		t.Fatal("expected non-empty sync to create pending owner plans")
	}

	// Vanilla clearBuilding + empty plan packet: stop the queue.
	svc.SyncPlans(101, 1, nil)
	if w.HasPendingPlansForOwner(101) {
		t.Fatal("expected empty sync after plans to clear owner queue")
	}
}

func TestSyncPlansReplacesQueueOnNonEmptySnapshot(t *testing.T) {
	w := newBuildSvcTestWorld(t)
	svc := New(w, Options{})

	svc.SyncPlans(101, 1, []*protocol.BuildPlan{{
		X: 2, Y: 2, Rotation: 0,
		Block: protocol.BlockRef{BlkID: 45, BlkName: "duo"},
	}})
	svc.SyncPlans(101, 1, []*protocol.BuildPlan{{
		X: 3, Y: 2, Rotation: 0,
		Block: protocol.BlockRef{BlkID: 45, BlkName: "duo"},
	}})

	// Non-empty snapshots are authoritative: cancelled (2,2) must be gone.
	if next, ok := w.FindNextPendingBuildPlan(1, 101); !ok || next.X != 3 || next.Y != 2 {
		t.Fatalf("expected only remaining plan (3,2), got %+v ok=%v", next, ok)
	}
}
