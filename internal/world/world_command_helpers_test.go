package world

import (
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func primeAssistConstructBuilder(t *testing.T, w *World, leader RawEntity, op BuildPlanOp) {
	t.Helper()
	if w == nil {
		t.Fatalf("expected world")
	}
	rules := w.GetRulesManager().Get()
	rules.InfiniteResources = true
	w.ApplyBuildPlanSnapshotForOwner(leader.ID, leader.Team, []BuildPlanOp{op})
	w.UpdateBuilderState(leader.ID, leader.Team, leader.ID, leader.X, leader.Y, true, 220)
	if _, ok := w.SetEntityBuildState(leader.ID, true, []*protocol.BuildPlan{{
		Breaking: op.Breaking,
		X:        op.X,
		Y:        op.Y,
		Rotation: byte(op.Rotation),
		Block:    protocol.BlockRef{BlkID: op.BlockID},
		Config:   op.Config,
	}}); !ok {
		t.Fatalf("expected leader build state to apply")
	}
}

func TestFindNearestAssistConstructBuilderRequiresVisualConstructStateAndRadius(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(256, 256)
	model.BlockNames = map[int16]string{
		0:   "air",
		257: "duo",
	}
	model.UnitNames = map[int16]string{
		21: "poly",
		35: "alpha",
	}
	w.SetModel(model)

	leader := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		X:            140,
		Y:            140,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	op := BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 257}
	primeAssistConstructBuilder(t, w, leader, op)

	if _, ok := w.FindNearestAssistConstructBuilder(1, 999, 8, 8, 220, 24, 1500); ok {
		t.Fatalf("expected assist builder to stay hidden until construct visual state begins")
	}

	w.Step(time.Second / 60)

	got, ok := w.FindNearestAssistConstructBuilder(1, 999, 8, 8, 220, 24, 1500)
	if !ok || got.ID != leader.ID {
		gotID := int32(0)
		if ok {
			gotID = got.ID
		}
		t.Fatalf("expected visualized construct builder %d to be found, got %d ok=%v", leader.ID, gotID, ok)
	}

	if _, ok := w.FindNearestAssistConstructBuilder(1, 999, 8, 8, 220, 24, 100); ok {
		t.Fatalf("expected assist builder outside buildRadius slice to be ignored")
	}
}

func TestPendingBuildCountsAssistConstructFollowers(t *testing.T) {
	runProgress := func(t *testing.T, withAssistant bool) float32 {
		t.Helper()
		w := New(Config{TPS: 60})
		model := NewWorldModel(64, 64)
		model.BlockNames = map[int16]string{
			0:   "air",
			257: "duo",
		}
		model.UnitNames = map[int16]string{
			35: "alpha",
		}
		w.SetModel(model)

		leader := w.Model().AddEntity(RawEntity{
			TypeID:       35,
			X:            140,
			Y:            140,
			Team:         1,
			Health:       100,
			MaxHealth:    100,
			MoveSpeed:    24,
			BuildSpeed:   0.5,
			ItemCapacity: 30,
			Flying:       true,
		})
		op := BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 257}
		primeAssistConstructBuilder(t, w, leader, op)

		if withAssistant {
			assistant := w.Model().AddEntity(RawEntity{
				TypeID:       35,
				X:            148,
				Y:            148,
				Team:         1,
				Health:       100,
				MaxHealth:    100,
				MoveSpeed:    24,
				BuildSpeed:   0.5,
				ItemCapacity: 30,
				Flying:       true,
			})
			w.UpdateBuilderState(assistant.ID, assistant.Team, assistant.ID, assistant.X, assistant.Y, true, 220)
			if _, ok := w.SetEntityBuildState(assistant.ID, true, []*protocol.BuildPlan{{
				X:        op.X,
				Y:        op.Y,
				Rotation: byte(op.Rotation),
				Block:    protocol.BlockRef{BlkID: op.BlockID},
			}}); !ok {
				t.Fatalf("expected assistant build state to apply")
			}
		}

		pos := int32(op.Y*int32(model.Width) + op.X)
		before := w.pendingBuilds[pos].Progress
		w.Step(time.Second / 60)
		after := w.pendingBuilds[pos].Progress
		return after - before
	}

	soloProgress := runProgress(t, false)
	assistProgress := runProgress(t, true)
	if assistProgress <= soloProgress+0.0001 {
		t.Fatalf("expected mirrored assist builder to add construct progress, solo=%f assist=%f", soloProgress, assistProgress)
	}
}

func TestPendingBreakCountsAssistConstructFollowers(t *testing.T) {
	runProgress := func(t *testing.T, withAssistant bool) float32 {
		t.Helper()
		w := New(Config{TPS: 60})
		model := NewWorldModel(64, 64)
		model.BlockNames = map[int16]string{
			0:   "air",
			45:  "duo",
			339: "core-shard",
		}
		model.UnitNames = map[int16]string{
			35: "alpha",
		}
		w.SetModel(model)
		placeTestBuilding(t, w, 0, 0, 339, 1, 0)
		placeTestBuilding(t, w, 18, 18, 45, 1, 0)

		leader := w.Model().AddEntity(RawEntity{
			TypeID:       35,
			X:            140,
			Y:            140,
			Team:         1,
			Health:       100,
			MaxHealth:    100,
			MoveSpeed:    24,
			BuildSpeed:   0.5,
			ItemCapacity: 30,
			Flying:       true,
		})
		op := BuildPlanOp{Breaking: true, X: 18, Y: 18}
		primeAssistConstructBuilder(t, w, leader, op)

		if withAssistant {
			assistant := w.Model().AddEntity(RawEntity{
				TypeID:       35,
				X:            148,
				Y:            148,
				Team:         1,
				Health:       100,
				MaxHealth:    100,
				MoveSpeed:    24,
				BuildSpeed:   0.5,
				ItemCapacity: 30,
				Flying:       true,
			})
			w.UpdateBuilderState(assistant.ID, assistant.Team, assistant.ID, assistant.X, assistant.Y, true, 220)
			if _, ok := w.SetEntityBuildState(assistant.ID, true, []*protocol.BuildPlan{{
				Breaking: true,
				X:        op.X,
				Y:        op.Y,
			}}); !ok {
				t.Fatalf("expected assistant break state to apply")
			}
		}

		w.Step(time.Second / 60)

		pos := int32(op.Y*int32(model.Width) + op.X)
		before := w.pendingBreaks[pos].Progress
		w.Step(time.Second / 60)
		after := w.pendingBreaks[pos].Progress
		return after - before
	}

	soloProgress := runProgress(t, false)
	assistProgress := runProgress(t, true)
	if assistProgress <= soloProgress+0.0001 {
		t.Fatalf("expected mirrored assist builder to add deconstruct progress, solo=%f assist=%f", soloProgress, assistProgress)
	}
}
