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

func TestEntitySyncSnapshotsSkipReservedAndInvalidTypes(t *testing.T) {
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
	content.RegisterUnitType(entitySyncTestUnitType{id: 36, name: "beta"})

	snaps := w.EntitySyncSnapshots(content, map[int32]struct{}{
		102: {},
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

func TestEntitySyncSnapshotsStripVolatileStateFromMapLoadedEntities(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		22: "mega",
	}
	model.Entities = append(model.Entities,
		RawEntity{
			ID:          201,
			TypeID:      22,
			Team:        1,
			Health:      80,
			X:           32,
			Y:           40,
			RuntimeInit: false,
			Payload:     []byte{1, 2, 3},
			Plans: []entityBuildPlan{{
				Pos:      protocol.PackPoint2(5, 6),
				BlockID:  12,
				Rotation: 2,
			}},
		},
	)
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 22, name: "mega"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one synced map entity, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	if unit.ID() != 201 {
		t.Fatalf("expected map entity id 201, got %d", unit.ID())
	}
	if len(unit.Payloads) != 0 {
		t.Fatalf("expected map-loaded entity payloads to be stripped, got %d", len(unit.Payloads))
	}
	if len(unit.Plans) != 0 {
		t.Fatalf("expected map-loaded entity plans to be stripped, got %d", len(unit.Plans))
	}
}

func TestUnitSyncSnapshotIncludesRuntimeState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	entityID := int32(401)
	model.AddEntity(RawEntity{
		ID:             entityID,
		TypeID:         35,
		PlayerID:       77,
		Team:           1,
		Health:         125,
		Shield:         14,
		Ammo:           9,
		Elevation:      1,
		Shooting:       true,
		SpawnedByCore:  true,
		UpdateBuilding: true,
		MineTilePos:    packTilePos(3, 4),
		Stack:          ItemStack{Item: 5, Amount: 7},
		Plans: []entityBuildPlan{{
			Pos:      protocol.PackPoint2(5, 6),
			BlockID:  12,
			Rotation: 2,
			Config:   protocol.Point2{X: 1, Y: 0},
		}},
		Abilities: []entityAbilityState{{Data: 2.5}},
		X:         96,
		Y:         128,
		Rotation:  45,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})
	content.RegisterItem(protocol.ItemRef{ItmID: 5, ItmName: "copper"})
	content.RegisterBlock(protocol.BlockRef{BlkID: 12, BlkName: "conveyor"})

	snapshot, ok := w.UnitSyncSnapshot(content, entityID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 77})
	if !ok || snapshot == nil {
		t.Fatal("expected unit sync snapshot")
	}
	if snapshot.Controller == nil {
		t.Fatal("expected controller to be present")
	}
	controller, ok := snapshot.Controller.(*protocol.ControllerState)
	if !ok || controller.Type != protocol.ControllerPlayer || controller.PlayerID != 77 {
		t.Fatalf("unexpected controller %+v", snapshot.Controller)
	}
	if snapshot.Ammo != 9 {
		t.Fatalf("expected ammo 9, got %f", snapshot.Ammo)
	}
	if !snapshot.Shooting {
		t.Fatal("expected shooting flag from runtime state")
	}
	if !snapshot.SpawnedByCore {
		t.Fatal("expected spawnedByCore to be preserved")
	}
	if !snapshot.UpdateBuilding {
		t.Fatal("expected updateBuilding to be preserved")
	}
	if snapshot.MineTile == nil || snapshot.MineTile.Pos() != packTilePos(3, 4) {
		t.Fatalf("unexpected mine tile %+v", snapshot.MineTile)
	}
	if snapshot.Stack.Item == nil || snapshot.Stack.Item.ID() != 5 || snapshot.Stack.Amount != 7 {
		t.Fatalf("unexpected stack %+v", snapshot.Stack)
	}
	if len(snapshot.Plans) != 1 {
		t.Fatalf("expected one build plan, got %d", len(snapshot.Plans))
	}
	if snapshot.Plans[0].Block == nil || snapshot.Plans[0].Block.ID() != 12 {
		t.Fatalf("unexpected plan block %+v", snapshot.Plans[0].Block)
	}
	if snapshot.Plans[0].X != 5 || snapshot.Plans[0].Y != 6 {
		t.Fatalf("unexpected plan position (%d,%d)", snapshot.Plans[0].X, snapshot.Plans[0].Y)
	}
	if len(snapshot.Abilities) != 1 || math.Abs(float64(snapshot.Abilities[0].Data()-2.5)) > 0.0001 {
		t.Fatalf("unexpected abilities %+v", snapshot.Abilities)
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

func TestEntitySyncSnapshotsDoNotMarkIdleRotatingMountAsTurningWithoutTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.AddEntity(RawEntity{
		ID:        202,
		TypeID:    35,
		Team:      1,
		Health:    100,
		MaxHealth: 100,
		X:         64,
		Y:         64,
	})
	w.SetModel(model)
	w.unitMountProfilesByName = map[string][]unitWeaponMountProfile{
		"alpha": {
			{Rotate: true},
		},
	}
	w.unitMountStates[202] = []unitMountState{
		{
			AimX:           72,
			AimY:           64,
			Rotation:       30,
			TargetRotation: 30,
			TargetID:       0,
			TargetBuildPos: -1,
		},
	}

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	if len(unit.Mounts) != 1 {
		t.Fatalf("expected one weapon mount, got %d", len(unit.Mounts))
	}
	if unit.Mounts[0].Rotate() {
		t.Fatalf("expected idle rotating mount without target to report Rotate=false")
	}
}

func TestUnitSyncSnapshotPreservesNegativeShield(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	entityID := int32(203)
	model.AddEntity(RawEntity{
		ID:        entityID,
		TypeID:    35,
		Team:      1,
		Health:    100,
		MaxHealth: 100,
		Shield:    -2.5,
		X:         64,
		Y:         64,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})

	snapshot, ok := w.UnitSyncSnapshot(content, entityID, nil)
	if !ok || snapshot == nil {
		t.Fatal("expected unit sync snapshot")
	}
	if math.Abs(float64(snapshot.Shield-(-2.5))) > 0.0001 {
		t.Fatalf("expected negative shield to be preserved, got=%f", snapshot.Shield)
	}
}

func TestEntitySyncSnapshotsExposePlayerTeamUnitsAsCommandableController(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.AddEntity(RawEntity{
		ID:        204,
		TypeID:    35,
		Team:      1,
		Health:    100,
		MaxHealth: 100,
		X:         64,
		Y:         64,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state, got %+v", unit.Controller)
	}
	if state.Type != protocol.ControllerCommand9 {
		t.Fatalf("expected player-team unit to use command controller, got %v", state.Type)
	}
	if state.Command.CommandID != 0 {
		t.Fatalf("expected idle move command id 0, got %d", state.Command.CommandID)
	}
}

func TestEntitySyncSnapshotsKeepWaveTeamUnitsOnGenericAI(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.AddEntity(RawEntity{
		ID:        205,
		TypeID:    35,
		Team:      2,
		Health:    100,
		MaxHealth: 100,
		X:         64,
		Y:         64,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 35, name: "alpha"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state, got %+v", unit.Controller)
	}
	if state.Type != protocol.ControllerGenericAI {
		t.Fatalf("expected wave-team unit to remain generic AI, got %v", state.Type)
	}
}

func TestEntitySyncSnapshotsKeepNonCommandableUtilityUnitsOnGenericAI(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.UnitNames = map[int16]string{
		80: "assembly-drone",
	}
	model.AddEntity(RawEntity{
		ID:        206,
		TypeID:    80,
		Team:      1,
		Health:    100,
		MaxHealth: 100,
		X:         64,
		Y:         64,
	})
	w.SetModel(model)

	content := protocol.NewContentRegistry()
	content.RegisterUnitType(entitySyncTestUnitType{id: 80, name: "assembly-drone"})

	snaps := w.EntitySyncSnapshots(content, nil)
	if len(snaps) != 1 {
		t.Fatalf("expected one entity snapshot, got %d", len(snaps))
	}
	unit, ok := snaps[0].(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit snapshot type, got %T", snaps[0])
	}
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil {
		t.Fatalf("expected controller state, got %+v", unit.Controller)
	}
	if state.Type != protocol.ControllerGenericAI {
		t.Fatalf("expected assembly drone to remain generic AI, got %v", state.Type)
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
