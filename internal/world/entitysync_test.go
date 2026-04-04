package world

import (
	"math"
	"testing"

	"mdt-server/internal/protocol"
)

type entitySyncTestUnitType struct {
	id   int16
	name string
}

func (t entitySyncTestUnitType) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (t entitySyncTestUnitType) ID() int16                         { return t.id }
func (t entitySyncTestUnitType) Name() string                      { return t.name }

type entitySyncTestStatusEffect struct {
	id   int16
	name string
}

func (t entitySyncTestStatusEffect) ContentType() protocol.ContentType { return protocol.ContentStatus }
func (t entitySyncTestStatusEffect) ID() int16                         { return t.id }
func (t entitySyncTestStatusEffect) Name() string                      { return t.name }
func (t entitySyncTestStatusEffect) Dynamic() bool                     { return false }

func TestEntitySyncSnapshotsSkipPlayerControlledAndInvalidTypes(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
		36: "beta",
	}
	model.AddEntity(RawEntity{ID: 101, TypeID: 35, Team: 1, Health: 80, X: 32, Y: 40})
	model.AddEntity(RawEntity{ID: 102, TypeID: 36, Team: 1, PlayerID: 77, Health: 90, X: 48, Y: 40})
	model.AddEntity(RawEntity{ID: 103, TypeID: 0, Team: 1, Health: 90, X: 56, Y: 40})
	model.AddEntity(RawEntity{ID: 104, TypeID: 36, Team: 1, Health: 90, X: 64, Y: 40})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})

	snaps := w.EntitySyncSnapshots(content, map[int32]struct{}{
		104: {},
	})
	if len(snaps) != 1 {
		t.Fatalf("expected exactly one synced world unit, got %d", len(snaps))
	}

	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	if unit.ID() != 101 {
		t.Fatalf("expected synced unit id 101, got %d", unit.ID())
	}
	if unit.TypeID != 35 {
		t.Fatalf("expected synced unit type 35, got %d", unit.TypeID)
	}
}

func TestEntitySyncSnapshotsIncludeMountsAndStatuses(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.AddEntity(RawEntity{
		ID:       201,
		TypeID:   35,
		Team:     2,
		Health:   135,
		Shield:   18,
		Rotation: 90,
		VelX:     1.5,
		VelY:     -2,
		X:        80,
		Y:        96,
		Statuses: []entityStatusState{
			{ID: 5, Name: "burning", Time: 3.5},
		},
	})
	w.SetModel(model)
	w.unitMountProfilesByName = map[string][]unitWeaponMountProfile{
		"alpha": {
			{Rotate: true},
			{},
		},
	}
	w.unitMountStates[201] = []unitMountState{
		{AimX: 120, AimY: 140, Warmup: 0.8, Rotation: 45, TargetRotation: 90, TargetID: 999},
		{AimX: 82, AimY: 88},
	}

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})
	content.RegisterStatusEffect(entitySyncTestStatusEffect{id: 5, name: "burning"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}

	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	if !unit.Shooting {
		t.Fatalf("expected unit shooting flag to reflect active mount warmup")
	}
	if len(unit.Mounts) != 2 {
		t.Fatalf("expected two weapon mounts, got %d", len(unit.Mounts))
	}
	if math.Abs(float64(unit.Mounts[0].AimX()-120)) > 0.0001 || math.Abs(float64(unit.Mounts[0].AimY()-140)) > 0.0001 {
		t.Fatalf("unexpected first mount aim=(%f,%f)", unit.Mounts[0].AimX(), unit.Mounts[0].AimY())
	}
	if !unit.Mounts[0].Shoot() {
		t.Fatalf("expected first mount shoot flag to be true")
	}
	if !unit.Mounts[0].Rotate() {
		t.Fatalf("expected first mount rotate flag to be true")
	}
	if len(unit.Statuses) != 1 {
		t.Fatalf("expected one synced status effect, got %d", len(unit.Statuses))
	}
	if unit.Statuses[0].Effect == nil || unit.Statuses[0].Effect.ID() != 5 {
		t.Fatalf("expected synced burning status effect id 5, got %+v", unit.Statuses[0].Effect)
	}
	if math.Abs(float64(unit.Statuses[0].Time-3.5)) > 0.0001 {
		t.Fatalf("expected status time 3.5, got %f", unit.Statuses[0].Time)
	}
}

func TestEntitySyncSnapshotsApplyOfficialPayloadUnitClass(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		55: "emanate",
	}
	model.AddEntity(RawEntity{
		ID:       301,
		TypeID:   55,
		Team:     1,
		Health:   700,
		Rotation: 90,
		X:        72,
		Y:        88,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 55, name: "emanate"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	if got := unit.ClassID(); got != 5 {
		t.Fatalf("expected official payload unit class id 5, got %d", got)
	}
}

func TestBlockSyncSnapshotsEncodeSeparatorRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		453: "separator",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 9, 9, 453, 1, 1)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{{Item: scrapItemID, Amount: 1}}
	tile.Build.Liquids = []LiquidStack{{Liquid: slagLiquidID, Amount: 18}}
	w.crafterStates[pos] = crafterRuntimeState{
		Progress: 0.75,
		Warmup:   0.5,
		Seed:     123456,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one block sync snapshot, got %d", len(snaps))
	}

	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[scrapItemID]; got != 1 {
		t.Fatalf("expected separator input scrap=1, got %d", got)
	}
	if got := base.Liquids[slagLiquidID]; math.Abs(float64(got-18)) > 0.0001 {
		t.Fatalf("expected separator slag=18, got %f", got)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read separator progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read separator warmup failed: %v", err)
	}
	seed, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read separator seed failed: %v", err)
	}
	if math.Abs(float64(progress-0.75)) > 0.0001 {
		t.Fatalf("expected separator progress 0.75, got %f", progress)
	}
	if math.Abs(float64(warmup-0.5)) > 0.0001 {
		t.Fatalf("expected separator warmup 0.5, got %f", warmup)
	}
	if seed != 123456 {
		t.Fatalf("expected separator seed 123456, got %d", seed)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected separator sync payload to be fully consumed, remaining=%d", rem)
	}
}
