package world

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
)

type decodedBlockSyncBase struct {
	Health             float32
	Rotation           byte
	Team               byte
	Version            byte
	Enabled            byte
	ModuleBits         byte
	Items              map[ItemID]int32
	PowerLinks         []int32
	PowerStatus        float32
	Liquids            map[LiquidID]float32
	Efficiency         byte
	OptionalEfficiency byte
}

func decodeBlockSyncBase(t *testing.T, data []byte) (decodedBlockSyncBase, *protocol.Reader) {
	t.Helper()
	r := protocol.NewReader(data)
	var out decodedBlockSyncBase
	var err error
	if out.Health, err = r.ReadFloat32(); err != nil {
		t.Fatalf("read sync health failed: %v", err)
	}
	if out.Rotation, err = r.ReadByte(); err != nil {
		t.Fatalf("read sync rotation failed: %v", err)
	}
	if out.Team, err = r.ReadByte(); err != nil {
		t.Fatalf("read sync team failed: %v", err)
	}
	if out.Version, err = r.ReadByte(); err != nil {
		t.Fatalf("read sync version failed: %v", err)
	}
	if out.Enabled, err = r.ReadByte(); err != nil {
		t.Fatalf("read sync enabled failed: %v", err)
	}
	if out.ModuleBits, err = r.ReadByte(); err != nil {
		t.Fatalf("read sync module bits failed: %v", err)
	}
	if (out.ModuleBits & 1) != 0 {
		count, err := r.ReadInt16()
		if err != nil {
			t.Fatalf("read item module count failed: %v", err)
		}
		out.Items = make(map[ItemID]int32, count)
		for i := 0; i < int(count); i++ {
			id, err := r.ReadInt16()
			if err != nil {
				t.Fatalf("read item module id failed: %v", err)
			}
			amount, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read item module amount failed: %v", err)
			}
			out.Items[ItemID(id)] = amount
		}
	}
	if (out.ModuleBits & (1 << 1)) != 0 {
		count, err := r.ReadInt16()
		if err != nil {
			t.Fatalf("read power module link count failed: %v", err)
		}
		out.PowerLinks = make([]int32, 0, count)
		for i := 0; i < int(count); i++ {
			link, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read power module link failed: %v", err)
			}
			out.PowerLinks = append(out.PowerLinks, link)
		}
		if out.PowerStatus, err = r.ReadFloat32(); err != nil {
			t.Fatalf("read power module status failed: %v", err)
		}
	}
	if (out.ModuleBits & (1 << 2)) != 0 {
		count, err := r.ReadInt16()
		if err != nil {
			t.Fatalf("read liquid module count failed: %v", err)
		}
		out.Liquids = make(map[LiquidID]float32, count)
		for i := 0; i < int(count); i++ {
			id, err := r.ReadInt16()
			if err != nil {
				t.Fatalf("read liquid module id failed: %v", err)
			}
			amount, err := r.ReadFloat32()
			if err != nil {
				t.Fatalf("read liquid module amount failed: %v", err)
			}
			out.Liquids[LiquidID(id)] = amount
		}
	}
	if (out.ModuleBits & (1 << 4)) != 0 {
		if _, err := r.ReadFloat32(); err != nil {
			t.Fatalf("read timescale failed: %v", err)
		}
		if _, err := r.ReadFloat32(); err != nil {
			t.Fatalf("read timescale duration failed: %v", err)
		}
	}
	if (out.ModuleBits & (1 << 5)) != 0 {
		if _, err := r.ReadInt32(); err != nil {
			t.Fatalf("read last disabler failed: %v", err)
		}
	}
	if out.Version <= 2 {
		if _, err := r.ReadBool(); err != nil {
			t.Fatalf("read legacy consume flag failed: %v", err)
		}
	}
	if out.Efficiency, err = r.ReadByte(); err != nil {
		t.Fatalf("read efficiency failed: %v", err)
	}
	if out.OptionalEfficiency, err = r.ReadByte(); err != nil {
		t.Fatalf("read optional efficiency failed: %v", err)
	}
	return out, r
}

func placeTestBuilding(t *testing.T, w *World, x, y int, block int16, team TeamID, rotation int8) *Tile {
	t.Helper()
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = BlockID(block)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &Building{
		Block:     BlockID(block),
		Team:      team,
		Rotation:  rotation,
		X:         x,
		Y:         y,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.rebuildBlockOccupancyLocked()
	return tile
}

func mustEncodeConfig(t *testing.T, value any) []byte {
	t.Helper()
	w := protocol.NewWriter()
	if err := protocol.WriteObject(w, value, nil); err != nil {
		t.Fatalf("encode config failed: %v", err)
	}
	return append([]byte(nil), w.Bytes()...)
}

func linkPowerNode(t *testing.T, w *World, x, y int, links ...protocol.Point2) {
	t.Helper()
	pos := int32(y*w.Model().Width + x)
	w.applyBuildingConfigLocked(pos, links, true)
}

func stepForSeconds(w *World, seconds float32) {
	frames := int(seconds*60 + 0.5)
	if frames < 1 {
		frames = 1
	}
	for i := 0; i < frames; i++ {
		w.Step(time.Second / 60)
	}
}

func paintAreaOverlay(t *testing.T, w *World, cx, cy, size int, overlay int16) {
	t.Helper()
	low, high := blockFootprintRange(size)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			tile, err := w.Model().TileAt(cx+dx, cy+dy)
			if err != nil || tile == nil {
				t.Fatalf("overlay tile lookup failed at (%d,%d): %v", cx+dx, cy+dy, err)
			}
			tile.Overlay = OverlayID(overlay)
		}
	}
}

func paintAreaFloor(t *testing.T, w *World, cx, cy, size int, floor int16) {
	t.Helper()
	low, high := blockFootprintRange(size)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			tile, err := w.Model().TileAt(cx+dx, cy+dy)
			if err != nil || tile == nil {
				t.Fatalf("floor tile lookup failed at (%d,%d): %v", cx+dx, cy+dy, err)
			}
			tile.Floor = FloorID(floor)
		}
	}
}

func paintWallRect(t *testing.T, w *World, minX, minY, maxX, maxY int, block int16, skip map[int32]struct{}) {
	t.Helper()
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if skip != nil {
				if _, ok := skip[packTilePos(x, y)]; ok {
					continue
				}
			}
			tile, err := w.Model().TileAt(x, y)
			if err != nil || tile == nil {
				t.Fatalf("wall tile lookup failed at (%d,%d): %v", x, y, err)
			}
			tile.Block = BlockID(block)
			tile.Build = nil
			tile.Team = 0
		}
	}
}

func pickCopperBasePartSchematicForTest(t *testing.T) vanilla.BasePartSchematic {
	t.Helper()
	parts, err := vanilla.LoadEmbeddedBasePartSchematics()
	if err != nil {
		t.Fatalf("load embedded baseparts: %v", err)
	}
	for _, part := range parts {
		hasCopper := false
		hasCore := false
		usableTiles := 0
		for _, tile := range part.Tiles {
			name := normalizeBlockLookupName(tile.Block)
			switch name {
			case "itemsource":
				if ref, ok := tile.Config.(vanilla.BasePartContentRef); ok && ref.ContentType == vanilla.BasePartContentItem && ItemID(ref.ID) == copperItemID {
					hasCopper = true
				}
				continue
			case "liquidsource", "powersource", "powervoid", "payloadsource", "payloadvoid", "heatsource":
				continue
			}
			if strings.HasPrefix(name, "core") {
				hasCore = true
			}
			usableTiles++
		}
		if hasCopper && !hasCore && usableTiles >= 2 {
			return part
		}
	}
	t.Fatal("expected an official copper basepart candidate for buildAi tests")
	return vanilla.BasePartSchematic{}
}

func newPayloadBuildingWorld(t *testing.T, blockID int16, blockName string, rotation int8, unit RawEntity) (*World, int32, int32, int32) {
	t.Helper()
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		blockID: blockName,
	}
	model.UnitNames = map[int16]string{
		unit.TypeID: "dagger",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, blockID, 1, rotation)
	buildPos := int32(tile.Y*w.Model().Width + tile.X)
	buildPacked := protocol.PackPoint2(int32(tile.X), int32(tile.Y))
	added := w.Model().AddEntity(unit)
	return w, buildPacked, buildPos, added.ID
}

func newPayloadControlSelectWorld(t *testing.T, unit RawEntity) (*World, int32, int32, int32) {
	t.Helper()
	return newPayloadBuildingWorld(t, 700, "payload-conveyor", 0, unit)
}

func TestControlSelectPayloadUnitPackedMovesStandingUnitIntoPayload(t *testing.T) {
	unit := RawEntity{
		ID:        11,
		TypeID:    7,
		Team:      1,
		X:         5*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if !w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected standing unit to enter payload")
	}

	payload := w.payloadStateLocked(buildPos).Payload
	if payload == nil || payload.Kind != payloadKindUnit || payload.UnitTypeID != 7 {
		t.Fatalf("expected dagger unit payload on building, got %+v", payload)
	}
	if payload.UnitState == nil || payload.UnitState.ID != 0 {
		t.Fatalf("expected payload unit state to be serialized detached copy, got %+v", payload.UnitState)
	}
	for _, ent := range w.Model().Entities {
		if ent.ID == unitID {
			t.Fatalf("expected world unit %d to be removed after entering payload", unitID)
		}
	}
}

func TestControlSelectPayloadUnitPackedRejectsSpawnedByCoreUnit(t *testing.T) {
	unit := RawEntity{
		ID:            12,
		TypeID:        7,
		Team:          1,
		X:             6*8 + 4,
		Y:             6*8 + 4,
		Health:        90,
		MaxHealth:     90,
		SpawnedByCore: true,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected spawnedByCore unit to be rejected by payload control-select")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload != nil {
		t.Fatalf("expected building payload to stay empty, got %+v", payload)
	}
	found := false
	for _, ent := range w.Model().Entities {
		if ent.ID == unitID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected spawnedByCore unit %d to remain in world", unitID)
	}
}

func TestControlSelectPayloadUnitPackedRejectsUnitOutsideBuildingFootprint(t *testing.T) {
	unit := RawEntity{
		ID:        13,
		TypeID:    7,
		Team:      1,
		X:         8*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected unit outside payload building footprint to be rejected")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload != nil {
		t.Fatalf("expected building payload to stay empty, got %+v", payload)
	}
	found := false
	for _, ent := range w.Model().Entities {
		if ent.ID == unitID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rejected unit %d to remain in world", unitID)
	}
}

func TestControlSelectPayloadUnitPackedRejectsUnitDisallowedByRuntimeProfile(t *testing.T) {
	unit := RawEntity{
		ID:        15,
		TypeID:    8,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		700: "payload-conveyor",
	}
	model.UnitNames = map[int16]string{
		8: "custom-disallowed",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 700, 1, 0)
	buildPacked := protocol.PackPoint2(int32(tile.X), int32(tile.Y))
	buildPos := int32(tile.Y*w.Model().Width + tile.X)
	w.unitRuntimeProfilesByName["customdisallowed"] = unitRuntimeProfile{
		Name:              "customdisallowed",
		HitSize:           8,
		AllowedInPayloads: false,
	}
	unitID := w.Model().AddEntity(unit).ID

	if w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected runtime profile allowedInPayloads=false to reject control-select payload")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload != nil {
		t.Fatalf("expected building payload to stay empty, got %+v", payload)
	}
}

func TestControlSelectPayloadUnitPackedRejectsPayloadMassDriver(t *testing.T) {
	unit := RawEntity{
		ID:        16,
		TypeID:    7,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadBuildingWorld(t, 702, "payload-mass-driver", 0, unit)

	if w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected payload control-select to reject payload-mass-driver")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload != nil {
		t.Fatalf("expected payload-mass-driver to stay empty, got %+v", payload)
	}
}

func TestControlSelectPayloadUnitPackedSetsPayloadRouterRecDir(t *testing.T) {
	unit := RawEntity{
		ID:        17,
		TypeID:    7,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadBuildingWorld(t, 701, "payload-router", 3, unit)

	if !w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected payload-router control-select to accept standing unit")
	}
	state := w.payloadStateLocked(buildPos)
	if state.Payload == nil || state.Payload.Kind != payloadKindUnit {
		t.Fatalf("expected payload-router payload state to receive unit payload, got %+v", state.Payload)
	}
	if want := byte(tileRotationNorm(3)); state.RecDir != want {
		t.Fatalf("expected payload-router recDir=%d after direct insert, got %d", want, state.RecDir)
	}
}

func TestEnterUnitPayloadPackedUsesPackedBuildingPos(t *testing.T) {
	unit := RawEntity{
		ID:        14,
		TypeID:    7,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if !w.EnterUnitPayloadPacked(buildPacked, unitID) {
		t.Fatal("expected unitEnteredPayload path to accept packed building pos")
	}
	payload := w.payloadStateLocked(buildPos).Payload
	if payload == nil || payload.Kind != payloadKindUnit {
		t.Fatalf("expected payload state to receive unit payload, got %+v", payload)
	}
}

func TestEnterUnitPayloadPackedAcceptsSpawnedByCoreUnit(t *testing.T) {
	unit := RawEntity{
		ID:            18,
		TypeID:        7,
		Team:          1,
		X:             6*8 + 4,
		Y:             6*8 + 4,
		Health:        90,
		MaxHealth:     90,
		SpawnedByCore: true,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if !w.EnterUnitPayloadPacked(buildPacked, unitID) {
		t.Fatal("expected unitEnteredPayload path to allow spawnedByCore unit")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload == nil || payload.Kind != payloadKindUnit {
		t.Fatalf("expected building payload to receive spawnedByCore unit, got %+v", payload)
	}
}

func TestEnterUnitPayloadPackedDoesNotRequireStandingOnBuilding(t *testing.T) {
	unit := RawEntity{
		ID:        19,
		TypeID:    7,
		Team:      1,
		X:         8*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadControlSelectWorld(t, unit)

	if !w.EnterUnitPayloadPacked(buildPacked, unitID) {
		t.Fatal("expected unitEnteredPayload path to skip standing-on-building check")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload == nil || payload.Kind != payloadKindUnit {
		t.Fatalf("expected payload state to receive off-footprint unit payload, got %+v", payload)
	}
}

func TestEnterUnitPayloadPackedAcceptsPayloadMassDriver(t *testing.T) {
	unit := RawEntity{
		ID:        20,
		TypeID:    7,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}
	w, buildPacked, buildPos, unitID := newPayloadBuildingWorld(t, 702, "payload-mass-driver", 0, unit)

	if !w.EnterUnitPayloadPacked(buildPacked, unitID) {
		t.Fatal("expected unitEnteredPayload path to accept payload-mass-driver")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload == nil || payload.Kind != payloadKindUnit {
		t.Fatalf("expected payload-mass-driver to receive unit payload, got %+v", payload)
	}
}

func TestRequestUnitPayloadRejectsCarrierWithoutPickupUnits(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.UnitNames = map[int16]string{
		7:  "dagger",
		55: "incite",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["incite"] = unitRuntimeProfile{
		Name:              "incite",
		HitSize:           11,
		PickupUnits:       false,
		AllowedInPayloads: true,
	}
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{
		Name:              "dagger",
		HitSize:           8,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	carrier := w.Model().AddEntity(RawEntity{
		ID:              21,
		TypeID:          55,
		Team:            1,
		X:               80,
		Y:               80,
		Health:          200,
		MaxHealth:       200,
		PayloadCapacity: 256,
	})
	target := w.Model().AddEntity(RawEntity{
		ID:        22,
		TypeID:    7,
		Team:      1,
		X:         84,
		Y:         80,
		Health:    90,
		MaxHealth: 90,
	})

	if _, ok := w.RequestUnitPayload(carrier.ID, target.ID); ok {
		t.Fatal("expected pickupUnits=false carrier to reject unit payload pickup")
	}
	if len(w.Model().Entities) != 2 {
		t.Fatalf("expected both entities to remain after rejected pickup, got %d", len(w.Model().Entities))
	}
}

func TestRequestUnitPayloadUsesDynamicHitSizeRange(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		8:  "large-carrier",
		55: "large-target",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["largecarrier"] = unitRuntimeProfile{
		Name:              "largecarrier",
		HitSize:           24,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	w.unitRuntimeProfilesByName["largetarget"] = unitRuntimeProfile{
		Name:              "largetarget",
		HitSize:           20,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	carrier := w.Model().AddEntity(RawEntity{
		ID:              31,
		TypeID:          8,
		Team:            1,
		X:               80,
		Y:               80,
		Health:          300,
		MaxHealth:       300,
		PayloadCapacity: 1024,
	})
	target := w.Model().AddEntity(RawEntity{
		ID:        32,
		TypeID:    55,
		Team:      1,
		X:         140,
		Y:         80,
		Health:    160,
		MaxHealth: 160,
	})

	updated, ok := w.RequestUnitPayload(carrier.ID, target.ID)
	if !ok {
		t.Fatal("expected dynamic payload pickup range to accept larger unit farther than vanilla fixed 32")
	}
	if len(updated.Payloads) != 1 || updated.Payloads[0].Kind != payloadKindUnit || updated.Payloads[0].UnitTypeID != 55 {
		t.Fatalf("expected carrier to receive target as unit payload, got %+v", updated.Payloads)
	}
	if len(w.Model().Entities) != 1 {
		t.Fatalf("expected picked target to be removed from world, got %d entities", len(w.Model().Entities))
	}
}

func TestRequestUnitPayloadRejectsFlyingTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		8:  "large-carrier",
		55: "large-target",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["largecarrier"] = unitRuntimeProfile{
		Name:              "largecarrier",
		HitSize:           24,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	w.unitRuntimeProfilesByName["largetarget"] = unitRuntimeProfile{
		Name:              "largetarget",
		HitSize:           20,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	carrier := w.Model().AddEntity(RawEntity{
		ID:              33,
		TypeID:          8,
		Team:            1,
		X:               80,
		Y:               80,
		Health:          300,
		MaxHealth:       300,
		PayloadCapacity: 1024,
	})
	target := w.Model().AddEntity(RawEntity{
		ID:        34,
		TypeID:    55,
		Team:      1,
		X:         92,
		Y:         80,
		Health:    160,
		MaxHealth: 160,
		Flying:    true,
	})

	if _, ok := w.RequestUnitPayload(carrier.ID, target.ID); ok {
		t.Fatal("expected requestUnitPayload to reject flying target")
	}
	if len(w.Model().Entities) != 2 {
		t.Fatalf("expected flying target to remain in world, got %d entities", len(w.Model().Entities))
	}
}

func TestRequestDropPayloadWithoutUnitStateDoesNotInjectDefaultShieldOrArmor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		8:   "carrier",
		910: "plain-payload-unit",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["plainpayloadunit"] = unitRuntimeProfile{Name: "plain-payload-unit"}

	carrier := w.Model().AddEntity(RawEntity{
		ID:              41,
		TypeID:          8,
		Team:            1,
		X:               80,
		Y:               80,
		Health:          100,
		MaxHealth:       100,
		PayloadCapacity: 1024,
		Payloads: []payloadData{{
			Kind:       payloadKindUnit,
			UnitTypeID: 910,
			Health:     40,
			MaxHealth:  70,
		}},
		RuntimeInit: true,
		SlowMul:     1,
	})

	updated, ok := w.RequestDropPayload(carrier.ID, 96, 80)
	if !ok {
		t.Fatal("expected requestDropPayload to drop fallback unit payload")
	}
	if len(updated.Payloads) != 0 {
		t.Fatalf("expected carrier payloads to clear after drop, got %+v", updated.Payloads)
	}
	if len(w.Model().Entities) != 2 {
		t.Fatalf("expected carrier plus dropped unit, got %d entities", len(w.Model().Entities))
	}

	var dropped RawEntity
	found := false
	for _, ent := range w.Model().Entities {
		if ent.ID == carrier.ID {
			continue
		}
		dropped = ent
		found = true
		break
	}
	if !found {
		t.Fatal("expected dropped payload unit to be spawned")
	}
	if dropped.TypeID != 910 {
		t.Fatalf("expected dropped payload type 910, got %d", dropped.TypeID)
	}
	if dropped.Health != 40 || dropped.MaxHealth != 70 {
		t.Fatalf("expected dropped payload health 40/70, got %f/%f", dropped.Health, dropped.MaxHealth)
	}
	if dropped.Shield != 0 || dropped.ShieldMax != 0 || dropped.ShieldRegen != 0 {
		t.Fatalf("expected fallback dropped payload to avoid fake shield defaults, got shield=%f max=%f regen=%f", dropped.Shield, dropped.ShieldMax, dropped.ShieldRegen)
	}
	if dropped.Armor != 0 {
		t.Fatalf("expected fallback dropped payload to avoid fake armor default, got=%f", dropped.Armor)
	}
}

func TestRequestDropPayloadRestoresSerializedUnitPayloadState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		8:   "carrier",
		911: "serialized-payload-unit",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["serializedpayloadunit"] = unitRuntimeProfile{Name: "serialized-payload-unit"}

	source := RawEntity{
		TypeID:      911,
		Team:        1,
		Health:      35,
		MaxHealth:   80,
		Shield:      -2.5,
		Rotation:    45,
		RuntimeInit: true,
		SlowMul:     1,
		MineTilePos: invalidEntityTilePos,
	}
	payload := w.unitPayloadFromEntityLocked(source)
	if payload == nil {
		t.Fatal("expected serialized unit payload")
	}
	payload.UnitState = nil
	payload.UnitTypeID = -1

	carrier := w.Model().AddEntity(RawEntity{
		ID:              42,
		TypeID:          8,
		Team:            1,
		X:               80,
		Y:               80,
		Health:          100,
		MaxHealth:       100,
		PayloadCapacity: 1024,
		Payloads:        []payloadData{clonePayloadData(*payload)},
		RuntimeInit:     true,
		SlowMul:         1,
	})

	updated, ok := w.RequestDropPayload(carrier.ID, 96, 80)
	if !ok {
		t.Fatal("expected requestDropPayload to restore serialized unit payload state")
	}
	if len(updated.Payloads) != 0 {
		t.Fatalf("expected carrier payloads to clear after drop, got %+v", updated.Payloads)
	}

	var dropped RawEntity
	found := false
	for _, ent := range w.Model().Entities {
		if ent.ID == carrier.ID {
			continue
		}
		dropped = ent
		found = true
		break
	}
	if !found {
		t.Fatal("expected dropped serialized payload unit to be spawned")
	}
	if dropped.TypeID != 911 {
		t.Fatalf("expected dropped serialized payload type 911, got %d", dropped.TypeID)
	}
	if dropped.Health != 35 || dropped.MaxHealth != 80 {
		t.Fatalf("expected dropped serialized payload health 35/80, got %f/%f", dropped.Health, dropped.MaxHealth)
	}
	if dropped.Shield != -2.5 {
		t.Fatalf("expected dropped serialized payload shield -2.5, got %f", dropped.Shield)
	}
}

func TestRequestBuildPayloadPackedTakesCurrentPayloadBeforeBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		700: "payload-conveyor",
	}
	model.UnitNames = map[int16]string{
		8:  "dagger",
		55: "large-carrier",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{
		Name:              "dagger",
		HitSize:           8,
		AllowedInPayloads: true,
	}
	w.unitRuntimeProfilesByName["largecarrier"] = unitRuntimeProfile{
		Name:              "largecarrier",
		HitSize:           24,
		PickupUnits:       true,
		AllowedInPayloads: true,
	}
	tile := placeTestBuilding(t, w, 6, 6, 700, 1, 0)
	buildPos := int32(tile.Y*w.Model().Width + tile.X)
	buildPacked := protocol.PackPoint2(int32(tile.X), int32(tile.Y))
	setTestPayload(t, w, 6, 6, &payloadData{
		Kind:       payloadKindUnit,
		UnitTypeID: 8,
		Health:     90,
		MaxHealth:  90,
		UnitState: &RawEntity{
			TypeID:    8,
			Health:    90,
			MaxHealth: 90,
		},
	})
	carrier := w.Model().AddEntity(RawEntity{
		ID:              35,
		TypeID:          55,
		Team:            1,
		X:               6*8 + 4,
		Y:               6*8 + 4,
		Health:          300,
		MaxHealth:       300,
		PayloadCapacity: 1024,
	})

	updated, ok := w.RequestBuildPayloadPacked(carrier.ID, buildPacked)
	if !ok {
		t.Fatal("expected requestBuildPayload to take current building payload before detaching building")
	}
	if got := w.payloadStateLocked(buildPos).Payload; got != nil {
		t.Fatalf("expected building payload slot to be emptied after pickup, got %+v", got)
	}
	if tile.Block != 700 || tile.Build == nil {
		t.Fatalf("expected building to remain placed after taking internal payload, tile=%+v", tile)
	}
	if len(updated.Payloads) != 1 || updated.Payloads[0].Kind != payloadKindUnit || updated.Payloads[0].UnitTypeID != 8 {
		t.Fatalf("expected carrier to receive building's current payload, got %+v", updated.Payloads)
	}
}

func TestBlockSyncSnapshotsEncodeGenericCrafterRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		183: "silicon-smelter",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 10, 10, 183, 1, 2)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{
		{Item: coalItemID, Amount: 2},
		{Item: sandItemID, Amount: 4},
	}
	w.crafterStates[pos] = crafterRuntimeState{
		Progress: 0.625,
		Warmup:   0.5,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one block sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Pos != protocol.PackPoint2(10, 10) {
		t.Fatalf("unexpected snapshot pos=%d", snaps[0].Pos)
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if base.Version != 3 {
		t.Fatalf("expected sync base version 3, got %d", base.Version)
	}
	if (base.ModuleBits & 1) == 0 {
		t.Fatalf("expected item module bit to be present, bits=%08b", base.ModuleBits)
	}
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected power module bit to be present, bits=%08b", base.ModuleBits)
	}
	if got := base.Items[coalItemID]; got != 2 {
		t.Fatalf("expected coal amount 2, got %d", got)
	}
	if got := base.Items[sandItemID]; got != 4 {
		t.Fatalf("expected sand amount 4, got %d", got)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read crafter progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read crafter warmup failed: %v", err)
	}
	if math.Abs(float64(progress-0.625)) > 0.0001 {
		t.Fatalf("expected progress 0.625, got %f", progress)
	}
	if math.Abs(float64(warmup-0.5)) > 0.0001 {
		t.Fatalf("expected warmup 0.5, got %f", warmup)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected crafter sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodePowerNodeRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)
	nodeTile := placeTestBuilding(t, w, 12, 10, 425, 1, 0)
	placeTestBuilding(t, w, 6, 10, 430, 1, 0)
	nodePos := int32(nodeTile.Y*w.Model().Width + nodeTile.X)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: -6, Y: 0}}, true)
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) == 0 {
		t.Fatal("expected at least one power-node snapshot")
	}
	var nodeSnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(12, 10) {
			nodeSnap = &snaps[i]
			break
		}
	}
	if nodeSnap == nil {
		t.Fatalf("expected power-node snapshot at pos=%d, got %+v", protocol.PackPoint2(12, 10), snaps)
	}
	base, r := decodeBlockSyncBase(t, nodeSnap.Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected power module bit for power node, bits=%08b", base.ModuleBits)
	}
	targetPacked := protocol.PackPoint2(6, 10)
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != targetPacked {
		t.Fatalf("expected power-node runtime link to target packed=%d, got %v", targetPacked, base.PowerLinks)
	}
	if base.PowerStatus != 0 {
		t.Fatalf("expected power-node to sync vanilla non-consumer power status 0, got %f", base.PowerStatus)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected power-node sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeConductivePowerLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		430: "laser-drill",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 10, 421, 1, 0)
	placeTestBuilding(t, w, 7, 10, 430, 1, 0)
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 2 {
		t.Fatalf("expected two conductive block snapshots, got %d", len(snaps))
	}

	var batterySnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(6, 10) {
			batterySnap = &snaps[i]
			break
		}
	}
	if batterySnap == nil {
		t.Fatalf("expected battery snapshot at pos=%d", protocol.PackPoint2(6, 10))
	}
	base, r := decodeBlockSyncBase(t, batterySnap.Data)
	if len(base.PowerLinks) != 0 {
		t.Fatalf("expected conductive adjacency to stay out of PowerModule.links, got %v", base.PowerLinks)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected conductive power snapshot payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeReversePowerNodeLinksOnConsumers(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		421: "battery",
	}
	w.SetModel(model)
	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	battery := placeTestBuilding(t, w, 14, 10, 421, 1, 0)
	nodePacked := protocol.PackPoint2(int32(node.X), int32(node.Y))
	batteryPacked := protocol.PackPoint2(int32(battery.X), int32(battery.Y))
	nodePos := int32(node.Y*w.Model().Width + node.X)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 2 {
		t.Fatalf("expected node and drill snapshots, got %d", len(snaps))
	}

	var batterySnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(14, 10) {
			batterySnap = &snaps[i]
			break
		}
	}
	if batterySnap == nil {
		t.Fatalf("expected battery snapshot at pos=%d", protocol.PackPoint2(14, 10))
	}
	base, r := decodeBlockSyncBase(t, batterySnap.Data)
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != nodePacked {
		t.Fatalf("expected battery power module to keep reverse node packed link %d, got %v", nodePacked, base.PowerLinks)
	}
	if base.PowerStatus != 0 {
		t.Fatalf("expected idle battery power status 0 without stored charge, got %f", base.PowerStatus)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected reverse node-link snapshot payload to be fully consumed, remaining=%d", rem)
	}

	var nodeSnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(8, 10) {
			nodeSnap = &snaps[i]
			break
		}
	}
	if nodeSnap == nil {
		t.Fatalf("expected power-node snapshot at pos=%d", protocol.PackPoint2(8, 10))
	}
	nodeBase, rr := decodeBlockSyncBase(t, nodeSnap.Data)
	if len(nodeBase.PowerLinks) != 1 || nodeBase.PowerLinks[0] != batteryPacked {
		t.Fatalf("expected power-node to keep forward battery packed link %d, got %v", batteryPacked, nodeBase.PowerLinks)
	}
	if rem := rr.Remaining(); rem != 0 {
		t.Fatalf("expected node snapshot payload to be fully consumed, remaining=%d", rem)
	}
}

func TestPowerNodeConfigCreatesSymmetricRuntimeLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
	}
	w.SetModel(model)

	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	battery := placeTestBuilding(t, w, 14, 10, 421, 1, 0)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	batteryPos := int32(battery.Y*w.Model().Width + battery.X)

	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)

	if links := w.powerNodeLinks[nodePos]; len(links) != 1 || links[0] != batteryPos {
		t.Fatalf("expected node runtime links [%d], got %v", batteryPos, links)
	}
	if links := w.powerNodeLinks[batteryPos]; len(links) != 1 || links[0] != nodePos {
		t.Fatalf("expected battery reverse runtime links [%d], got %v", nodePos, links)
	}
	if node.Build == nil || len(node.Build.Config) == 0 {
		t.Fatal("expected power-node stored config to stay in sync with runtime links")
	}
	decoded, ok := decodeStoredBuildingConfig(node.Build.Config)
	if !ok {
		t.Fatal("expected power-node stored config to decode")
	}
	points, ok := decoded.([]protocol.Point2)
	if !ok || len(points) != 1 || points[0].X != 6 || points[0].Y != 0 {
		t.Fatalf("expected stored power-node config [{6 0}], got %T %#v", decoded, decoded)
	}
	if battery.Build != nil && len(battery.Build.Config) != 0 {
		t.Fatalf("expected battery to keep runtime power links only, got stored config bytes=%d", len(battery.Build.Config))
	}
}

func TestAutoLinkedPowerNodeCreatesSymmetricRuntimeLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)
	_ = w.DrainEntityEvents()

	consumerTile, err := model.TileAt(6, 10)
	if err != nil || consumerTile == nil {
		t.Fatalf("consumer tile lookup failed: %v", err)
	}
	w.placeTileLocked(consumerTile, 1, 430, 0, nil, 0)

	nodePos := int32(10*model.Width + 12)
	consumerPos := int32(10*model.Width + 6)
	if links := w.powerNodeLinks[nodePos]; len(links) != 1 || links[0] != consumerPos {
		t.Fatalf("expected autolinked node links [%d], got %v", consumerPos, links)
	}
	if links := w.powerNodeLinks[consumerPos]; len(links) != 1 || links[0] != nodePos {
		t.Fatalf("expected consumer reverse runtime links [%d], got %v", nodePos, links)
	}
	if nodeTile.Build == nil || len(nodeTile.Build.Config) == 0 {
		t.Fatal("expected autolinked power-node stored config to be written")
	}
}

func TestBuildingConfigPackedReturnsDetachedPointSlice(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		421: "battery",
	}
	w.SetModel(model)
	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	placeTestBuilding(t, w, 14, 10, 421, 1, 0)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	nodePacked := protocol.PackPoint2(8, 10)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)

	value, ok := w.BuildingConfigPacked(nodePacked)
	if !ok {
		t.Fatal("expected detached config value")
	}
	points, ok := value.([]protocol.Point2)
	if !ok || len(points) != 1 {
		t.Fatalf("expected detached []Point2 config, got %T %#v", value, value)
	}
	points[0] = protocol.Point2{X: 99, Y: 99}

	value, ok = w.BuildingConfigPacked(nodePacked)
	if !ok {
		t.Fatal("expected detached config value on second read")
	}
	points, ok = value.([]protocol.Point2)
	if !ok || len(points) != 1 {
		t.Fatalf("expected detached []Point2 config on second read, got %T %#v", value, value)
	}
	if points[0].X != 6 || points[0].Y != 0 {
		t.Fatalf("expected world config to stay unchanged after caller mutation, got %+v", points[0])
	}
}

func TestRelatedBlockSyncPackedPositionsIncludeLinkedPowerBuildings(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		101: "air-factory",
	}
	w.SetModel(model)
	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	placeTestBuilding(t, w, 14, 10, 101, 1, 1)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	factoryPacked := protocol.PackPoint2(14, 10)
	nodePacked := protocol.PackPoint2(8, 10)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)
	w.rebuildActiveTilesLocked()

	related := w.RelatedBlockSyncPackedPositions(nodePacked)
	if len(related) != 2 {
		t.Fatalf("expected node+factory related sync positions, got %v", related)
	}
	if related[0] != nodePacked || related[1] != factoryPacked {
		t.Fatalf("expected sorted related packed positions [%d %d], got %v", nodePacked, factoryPacked, related)
	}

	reverse := w.RelatedBlockSyncPackedPositions(factoryPacked)
	if len(reverse) != 2 {
		t.Fatalf("expected factory related sync positions to include node+factory, got %v", reverse)
	}
	if reverse[0] != nodePacked || reverse[1] != factoryPacked {
		t.Fatalf("expected sorted reverse related packed positions [%d %d], got %v", nodePacked, factoryPacked, reverse)
	}
}

func TestDestroyingLinkedPowerBuildingClearsBothSides(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
	}
	w.SetModel(model)

	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	battery := placeTestBuilding(t, w, 14, 10, 421, 1, 0)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	batteryPos := int32(battery.Y*w.Model().Width + battery.X)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)

	w.destroyTileLocked(battery, 1, 0)

	if links := w.powerNodeLinks[nodePos]; len(links) != 0 {
		t.Fatalf("expected destroying linked battery to clear node links, got %v", links)
	}
	if links := w.powerNodeLinks[batteryPos]; len(links) != 0 {
		t.Fatalf("expected destroying battery to clear reverse runtime links, got %v", links)
	}
	if _, ok := w.BuildingConfigPacked(protocol.PackPoint2(int32(node.X), int32(node.Y))); ok {
		t.Fatal("expected node config view to clear after linked battery is destroyed")
	}
	if node.Build != nil && len(node.Build.Config) != 0 {
		t.Fatalf("expected node stored config bytes to clear after destroy, got=%d", len(node.Build.Config))
	}
}

func TestBlockSyncSnapshotsEncodeBeamNodeBufferedPowerStatus(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		474: "beam-node",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 9, 9, 474, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.powerStorageState[pos] = 500
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one beam-node block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if math.Abs(float64(base.PowerStatus-0.5)) > 0.0001 {
		t.Fatalf("expected beam-node buffered power status 0.5, got %f", base.PowerStatus)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected beam-node sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeSolarPanelProductionEfficiency(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		404: "solar-panel",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 9, 9, 404, 1, 0)
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one solar-panel block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	productionEfficiency, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read solar-panel production efficiency failed: %v", err)
	}
	generateTime, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read solar-panel generateTime failed: %v", err)
	}
	if base.PowerStatus != 0 {
		t.Fatalf("expected solar-panel power status 0 for non-consumer producer, got %f", base.PowerStatus)
	}
	if math.Abs(float64(productionEfficiency-1)) > 0.0001 {
		t.Fatalf("expected solar-panel production efficiency 1, got %f", productionEfficiency)
	}
	if math.Abs(float64(generateTime)) > 0.0001 {
		t.Fatalf("expected solar-panel generateTime 0, got %f", generateTime)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected solar-panel sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeUnloaderConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		270: "unloader",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 7, 270, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.unloaderCfg[pos] = siliconItemID
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & 1) == 0 {
		t.Fatalf("expected unloader item module bit to be present, bits=%08b", base.ModuleBits)
	}
	itemID, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read unloader sort item failed: %v", err)
	}
	if itemID != int16(siliconItemID) {
		t.Fatalf("expected unloader sort item %d, got %d", siliconItemID, itemID)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected unloader sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeConveyorRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 257, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{
		{Item: copperItemID, Amount: 1},
		{Item: siliconItemID, Amount: 1},
	}
	w.conveyorStates[pos] = &conveyorRuntimeState{
		IDs:          [3]ItemID{copperItemID, siliconItemID},
		XS:           [3]float32{1, -1},
		YS:           [3]float32{0.25, 0.75},
		Len:          2,
		LastInserted: 1,
		Mid:          1,
		MinItem:      0.25,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one conveyor block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[copperItemID]; got != 1 {
		t.Fatalf("expected conveyor copper amount 1, got %d", got)
	}
	if got := base.Items[siliconItemID]; got != 1 {
		t.Fatalf("expected conveyor silicon amount 1, got %d", got)
	}
	amount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read conveyor item count failed: %v", err)
	}
	if amount != 2 {
		t.Fatalf("expected 2 conveyor runtime items, got %d", amount)
	}
	firstItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read first conveyor item failed: %v", err)
	}
	firstX, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read first conveyor x failed: %v", err)
	}
	firstY, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read first conveyor y failed: %v", err)
	}
	if firstItem != int16(copperItemID) || firstX != signedByteFromInt(127) || firstY != signedByteFromInt(-64) {
		t.Fatalf("unexpected first conveyor runtime entry item=%d x=%d y=%d", firstItem, firstX, firstY)
	}
	secondItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read second conveyor item failed: %v", err)
	}
	secondX, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read second conveyor x failed: %v", err)
	}
	secondY, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read second conveyor y failed: %v", err)
	}
	if secondItem != int16(siliconItemID) || secondX != signedByteFromInt(-127) || secondY != signedByteFromInt(63) {
		t.Fatalf("unexpected second conveyor runtime entry item=%d x=%d y=%d", secondItem, secondX, secondY)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected conveyor sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsLiveOnlyIncludeConveyorRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 257, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	w.conveyorStates[pos] = &conveyorRuntimeState{
		IDs: [3]ItemID{copperItemID},
		XS:  [3]float32{1},
		YS:  [3]float32{0.5},
		Len: 1,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshotsLiveOnly()
	if len(snaps) != 1 {
		t.Fatalf("expected one live-only conveyor block sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Pos != protocol.PackPoint2(6, 6) {
		t.Fatalf("expected live-only conveyor snapshot at %d, got %d", protocol.PackPoint2(6, 6), snaps[0].Pos)
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[copperItemID]; got != 1 {
		t.Fatalf("expected live-only conveyor copper amount 1, got %d", got)
	}
	amount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read live-only conveyor item count failed: %v", err)
	}
	if amount != 1 {
		t.Fatalf("expected 1 live-only conveyor runtime item, got %d", amount)
	}
}

func TestBlockSyncSnapshotsEncodeRouterRuntimeViaBaseOnly(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		418: "router",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 9, 6, 418, 1, 0)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one router block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[copperItemID]; got != 1 {
		t.Fatalf("expected router copper amount 1, got %d", got)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected router base-only sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeStackConveyorRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		447: "plastanium-conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 447, 1, 1)
	target := placeTestBuilding(t, w, 9, 8, 447, 1, 1)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	targetPos := int32(target.Y*w.Model().Width + target.X)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 3}}
	w.stackStates[pos] = &stackRuntimeState{
		Link:      targetPos,
		Cooldown:  0.35,
		LastItem:  copperItemID,
		HasItem:   true,
		Unloading: false,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	var snap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(8, 8) {
			snap = &snaps[i]
			break
		}
	}
	if snap == nil {
		t.Fatal("expected plastanium conveyor snapshot")
	}
	base, r := decodeBlockSyncBase(t, snap.Data)
	if got := base.Items[copperItemID]; got != 3 {
		t.Fatalf("expected plastanium conveyor copper amount 3, got %d", got)
	}
	link, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read plastanium conveyor link failed: %v", err)
	}
	cooldown, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read plastanium conveyor cooldown failed: %v", err)
	}
	if link != protocol.PackPoint2(9, 8) {
		t.Fatalf("expected plastanium conveyor link %d, got %d", protocol.PackPoint2(9, 8), link)
	}
	if math.Abs(float64(cooldown-0.35)) > 0.0001 {
		t.Fatalf("expected plastanium conveyor cooldown 0.35, got %f", cooldown)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected plastanium conveyor sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeMassDriverRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		432: "mass-driver",
	}
	w.SetModel(model)
	src := placeTestBuilding(t, w, 6, 10, 432, 1, 0)
	dst := placeTestBuilding(t, w, 12, 10, 432, 1, 0)
	pos := int32(src.Y*w.Model().Width + src.X)
	targetPos := int32(dst.Y*w.Model().Width + dst.X)
	src.Build.Items = []ItemStack{{Item: copperItemID, Amount: 20}}
	w.massDriverLinks[pos] = targetPos
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	var snap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(6, 10) {
			snap = &snaps[i]
			break
		}
	}
	if snap == nil {
		t.Fatal("expected mass-driver snapshot")
	}
	base, r := decodeBlockSyncBase(t, snap.Data)
	if got := base.Items[copperItemID]; got != 20 {
		t.Fatalf("expected mass-driver copper amount 20, got %d", got)
	}
	link, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read mass-driver link failed: %v", err)
	}
	rotation, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read mass-driver rotation failed: %v", err)
	}
	state, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read mass-driver state failed: %v", err)
	}
	if link != protocol.PackPoint2(12, 10) {
		t.Fatalf("expected mass-driver link %d, got %d", protocol.PackPoint2(12, 10), link)
	}
	wantRotation := lookAt(float32(src.X*8+4), float32(src.Y*8+4), float32(dst.X*8+4), float32(dst.Y*8+4))
	if math.Abs(float64(rotation-wantRotation)) > 0.0001 {
		t.Fatalf("expected mass-driver rotation %f, got %f", wantRotation, rotation)
	}
	if state != 2 {
		t.Fatalf("expected linked mass-driver to sync shooting state=2, got %d", state)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected mass-driver sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeLinkedStorageSharedInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		431: "vault",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	store := placeTestBuilding(t, w, 8, 5, 431, 1, 0)
	store.Build.AddItem(copperItemID, 4)
	w.rebuildBlockOccupancyLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one storage block sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Pos != protocol.PackPoint2(8, 5) {
		t.Fatalf("unexpected linked storage snapshot pos=%d", snaps[0].Pos)
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[copperItemID]; got != 4 {
		t.Fatalf("expected linked storage sync to expose shared core copper=4, got %d", got)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected linked storage sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeStandaloneStorageMultipleItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 7, 6, 500, 1, 0)
	tile.Build.Items = []ItemStack{
		{Item: copperItemID, Amount: 3},
		{Item: leadItemID, Amount: 5},
		{Item: coalItemID, Amount: 2},
		{Item: sandItemID, Amount: 4},
	}

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one storage snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[copperItemID]; got != 3 {
		t.Fatalf("expected copper=3, got %d", got)
	}
	if got := base.Items[leadItemID]; got != 5 {
		t.Fatalf("expected lead=5, got %d", got)
	}
	if got := base.Items[coalItemID]; got != 2 {
		t.Fatalf("expected coal=2, got %d", got)
	}
	if got := base.Items[sandItemID]; got != 4 {
		t.Fatalf("expected sand=4, got %d", got)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected standalone storage sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsSkipPendingBreakTurret(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	tile := placeTestBuilding(t, w, 5, 6, 910, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.pendingBreaks[pos] = pendingBreakState{
		Team:    1,
		BlockID: 910,
	}

	if snaps := w.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(tile.X, tile.Y)}); len(snaps) != 0 {
		t.Fatalf("expected pending-break item-turret snapshots to be suppressed, got %d", len(snaps))
	}
	if snaps := w.TurretBlockSyncSnapshotsLiveOnly(); len(snaps) != 0 {
		t.Fatalf("expected pending-break turret periodic snapshots to be suppressed, got %d", len(snaps))
	}
	if builds := w.BuildSyncSnapshot(); len(builds) != 0 {
		t.Fatalf("expected pending-break turret to be absent from build sync snapshot, got %d", len(builds))
	}
}

func TestBlockSyncSnapshotsEncodeUnitFactoryRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
		8: "crawler",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 10, 10, 100, 1, 1)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{
		{Item: siliconItemID, Amount: 8},
		{Item: coalItemID, Amount: 10},
	}
	w.factoryStates[pos] = factoryState{
		Progress:    123.5,
		UnitType:    8,
		CurrentPlan: 1,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one unit factory block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if got := base.Items[siliconItemID]; got != 8 {
		t.Fatalf("expected silicon amount 8, got %d", got)
	}
	if got := base.Items[coalItemID]; got != 10 {
		t.Fatalf("expected coal amount 10, got %d", got)
	}
	payX, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read unit factory payVector.x failed: %v", err)
	}
	payY, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read unit factory payVector.y failed: %v", err)
	}
	payRotation, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read unit factory payRotation failed: %v", err)
	}
	payloadExists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read unit factory payload exists flag failed: %v", err)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read unit factory progress failed: %v", err)
	}
	currentPlan, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read unit factory current plan failed: %v", err)
	}
	commandPos, err := protocol.ReadVecNullable(r)
	if err != nil {
		t.Fatalf("read unit factory commandPos failed: %v", err)
	}
	command, err := protocol.ReadCommand(r, nil)
	if err != nil {
		t.Fatalf("read unit factory command failed: %v", err)
	}
	if math.Abs(float64(payX)) > 0.0001 || math.Abs(float64(payY)) > 0.0001 {
		t.Fatalf("expected idle unit factory payload offset to stay at origin, got x=%f y=%f", payX, payY)
	}
	if math.Abs(float64(payRotation-90)) > 0.0001 {
		t.Fatalf("expected unit factory payload rotation 90, got %f", payRotation)
	}
	if payloadExists {
		t.Fatalf("expected empty unit factory payload")
	}
	if math.Abs(float64(progress-123.5)) > 0.0001 {
		t.Fatalf("expected progress 123.5, got %f", progress)
	}
	if currentPlan != 1 {
		t.Fatalf("expected current plan 1, got %d", currentPlan)
	}
	if commandPos != nil {
		t.Fatalf("expected nil commandPos, got %+v", commandPos)
	}
	if command != nil {
		t.Fatalf("expected nil command, got %+v", command)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected unit factory sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeUnitFactoryCommandState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 5, 5, 100, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	wantPos := &protocol.Vec2{X: 96, Y: 112}
	wantCommand := &protocol.UnitCommand{ID: 2, Name: "repair"}
	w.factoryStates[pos] = factoryState{
		Progress:    60,
		UnitType:    7,
		CurrentPlan: 0,
		CommandPos:  wantPos,
		Command:     wantCommand,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one unit factory block sync snapshot, got %d", len(snaps))
	}
	_, r := decodeBlockSyncBase(t, snaps[0].Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read unit factory payVector.x failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read unit factory payVector.y failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read unit factory payRotation failed: %v", err)
	}
	if _, err := r.ReadBool(); err != nil {
		t.Fatalf("read unit factory payload exists failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read unit factory progress failed: %v", err)
	}
	if _, err := r.ReadInt16(); err != nil {
		t.Fatalf("read unit factory current plan failed: %v", err)
	}
	commandPos, err := protocol.ReadVecNullable(r)
	if err != nil {
		t.Fatalf("read unit factory commandPos failed: %v", err)
	}
	command, err := protocol.ReadCommand(r, nil)
	if err != nil {
		t.Fatalf("read unit factory command failed: %v", err)
	}
	if commandPos == nil || math.Abs(float64(commandPos.X-wantPos.X)) > 0.0001 || math.Abs(float64(commandPos.Y-wantPos.Y)) > 0.0001 {
		t.Fatalf("expected unit factory commandPos %+v, got %+v", wantPos, commandPos)
	}
	if command == nil || command.ID != wantCommand.ID {
		t.Fatalf("expected unit factory command id %d, got %+v", wantCommand.ID, command)
	}
}

func TestBlockSyncSnapshotsEncodeDrillRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		430: "laser-drill",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 9, 9, 430, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.drillStates[pos] = drillRuntimeState{
		Progress: 91.5,
		Warmup:   0.75,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one drill block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected drill power module bit to be present, bits=%08b", base.ModuleBits)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill warmup failed: %v", err)
	}
	if math.Abs(float64(progress-91.5)) > 0.0001 {
		t.Fatalf("expected drill progress 91.5, got %f", progress)
	}
	if math.Abs(float64(warmup-0.75)) > 0.0001 {
		t.Fatalf("expected drill warmup 0.75, got %f", warmup)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected drill sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodePumpRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		441: "rotary-pump",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 11, 7, 441, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.AddLiquid(waterLiquidID, 12.5)
	w.pumpStates[pos] = pumpRuntimeState{
		Warmup:   0.65,
		Progress: 17,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one pump block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected pump power module bit to be present, bits=%08b", base.ModuleBits)
	}
	if (base.ModuleBits & (1 << 2)) == 0 {
		t.Fatalf("expected pump liquid module bit to be present, bits=%08b", base.ModuleBits)
	}
	if got := base.Liquids[waterLiquidID]; math.Abs(float64(got-12.5)) > 0.0001 {
		t.Fatalf("expected pump liquid amount 12.5, got %f", got)
	}
	if base.Efficiency == 0 {
		t.Fatalf("expected pump efficiency byte to reflect warmup, got 0")
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected pump sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeHeatProducerRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		461: "electric-heater",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 461, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.crafterStates[pos] = crafterRuntimeState{
		Progress: 33.25,
		Warmup:   0.5,
	}
	w.heatStates[pos] = 2.25
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one heat producer block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected heat producer power module bit to be present, bits=%08b", base.ModuleBits)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read heat producer progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read heat producer warmup failed: %v", err)
	}
	heat, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read heat producer heat failed: %v", err)
	}
	if math.Abs(float64(progress-33.25)) > 0.0001 {
		t.Fatalf("expected heat producer progress 33.25, got %f", progress)
	}
	if math.Abs(float64(warmup-0.5)) > 0.0001 {
		t.Fatalf("expected heat producer warmup 0.5, got %f", warmup)
	}
	if math.Abs(float64(heat-2.25)) > 0.0001 {
		t.Fatalf("expected heat producer heat 2.25, got %f", heat)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected heat producer sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodeVariableReactorRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		473: "flux-reactor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 9, 9, 473, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.AddLiquid(cyanogenLiquidID, 30)
	w.heatStates[pos] = 75
	w.powerGeneratorState[pos] = &powerGeneratorState{
		Warmup:      0.6,
		Instability: 0.2,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one variable reactor block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected variable reactor power module bit to be present, bits=%08b", base.ModuleBits)
	}
	if (base.ModuleBits & (1 << 2)) == 0 {
		t.Fatalf("expected variable reactor liquid module bit to be present, bits=%08b", base.ModuleBits)
	}
	productionEfficiency, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read variable reactor production efficiency failed: %v", err)
	}
	generateTime, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read variable reactor generateTime failed: %v", err)
	}
	heat, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read variable reactor heat failed: %v", err)
	}
	instability, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read variable reactor instability failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read variable reactor warmup failed: %v", err)
	}
	if math.Abs(float64(productionEfficiency-0.5)) > 0.0001 {
		t.Fatalf("expected variable reactor production efficiency 0.5, got %f", productionEfficiency)
	}
	if math.Abs(float64(generateTime)) > 0.0001 {
		t.Fatalf("expected variable reactor generateTime 0, got %f", generateTime)
	}
	if math.Abs(float64(heat-75)) > 0.0001 {
		t.Fatalf("expected variable reactor heat 75, got %f", heat)
	}
	if math.Abs(float64(instability-0.2)) > 0.0001 {
		t.Fatalf("expected variable reactor instability 0.2, got %f", instability)
	}
	if math.Abs(float64(warmup-0.6)) > 0.0001 {
		t.Fatalf("expected variable reactor warmup 0.6, got %f", warmup)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected variable reactor sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncPowerStatusTracksActualSupplyRatioPerBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		404: "solar-panel",
		422: "power-node",
		430: "laser-drill",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 4, 8, 404, 1, 0)
	placeTestBuilding(t, w, 8, 8, 430, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	w.Step(time.Second)

	snaps := w.BlockSyncSnapshots()
	var drillSnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(8, 8) {
			drillSnap = &snaps[i]
			break
		}
	}
	if drillSnap == nil {
		t.Fatal("expected laser drill block sync snapshot")
	}
	base, _ := decodeBlockSyncBase(t, drillSnap.Data)
	if base.PowerStatus != 0 {
		t.Fatalf("expected underpowered laser drill to sync power status 0, got %f", base.PowerStatus)
	}
}

func TestBlockSyncUnitFactoryEfficiencyTracksSyncedPowerCoverage(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 5, 5, 100, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.AddItem(siliconItemID, 10)
	tile.Build.AddItem(leadItemID, 10)
	w.factoryStates[pos] = factoryState{
		Progress:    60,
		CurrentPlan: 0,
		UnitType:    7,
	}
	w.powerRequested[pos] = 12
	w.powerSupplied[pos] = 6
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one unit factory block sync snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if math.Abs(float64(base.PowerStatus-0.5)) > 0.0001 {
		t.Fatalf("expected unit factory power status 0.5, got %f", base.PowerStatus)
	}
	if math.Abs(float64(float32(base.Efficiency)/255-0.5)) > 0.02 {
		t.Fatalf("expected unit factory efficiency byte to track power coverage ~=0.5, got raw=%d", base.Efficiency)
	}
	if math.Abs(float64(float32(base.OptionalEfficiency)/255-0.5)) > 0.02 {
		t.Fatalf("expected unit factory optional efficiency byte to track power coverage ~=0.5, got raw=%d", base.OptionalEfficiency)
	}
	_ = r
}

func TestBlockSyncAirFactoryKeepsReversePowerNodeLink(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		101: "air-factory",
		422: "power-node",
	}
	w.SetModel(model)
	factory := placeTestBuilding(t, w, 12, 10, 101, 1, 1)
	node := placeTestBuilding(t, w, 6, 10, 422, 1, 0)
	factoryPos := int32(factory.Y*w.Model().Width + factory.X)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	factory.Build.AddItem(siliconItemID, 15)
	w.factoryStates[factoryPos] = factoryState{
		Progress:    180,
		CurrentPlan: 0,
	}
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshots()
	var factorySnap *BlockSyncSnapshot
	for i := range snaps {
		if snaps[i].Pos == protocol.PackPoint2(12, 10) {
			factorySnap = &snaps[i]
			break
		}
	}
	if factorySnap == nil {
		t.Fatal("expected air-factory block sync snapshot")
	}
	base, r := decodeBlockSyncBase(t, factorySnap.Data)
	nodePacked := protocol.PackPoint2(6, 10)
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != nodePacked {
		t.Fatalf("expected air-factory power module to keep reverse node packed link %d, got %v", nodePacked, base.PowerLinks)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read air-factory payVector.x failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read air-factory payVector.y failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read air-factory payRotation failed: %v", err)
	}
	payloadExists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read air-factory payload flag failed: %v", err)
	}
	if payloadExists {
		t.Fatal("expected air-factory payload to stay empty in this snapshot")
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read air-factory progress failed: %v", err)
	}
	if math.Abs(float64(progress-180)) > 0.0001 {
		t.Fatalf("expected air-factory progress 180, got %f", progress)
	}
	currentPlan, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read air-factory current plan failed: %v", err)
	}
	if currentPlan != 0 {
		t.Fatalf("expected air-factory current plan 0, got %d", currentPlan)
	}
	commandPos, err := protocol.ReadVecNullable(r)
	if err != nil {
		t.Fatalf("read air-factory commandPos failed: %v", err)
	}
	if commandPos != nil {
		t.Fatalf("expected nil air-factory commandPos, got %+v", commandPos)
	}
	command, err := protocol.ReadCommand(r, nil)
	if err != nil {
		t.Fatalf("read air-factory command failed: %v", err)
	}
	if command != nil {
		t.Fatalf("expected nil air-factory command, got %+v", command)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected air-factory sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestUnitFactoryBlockSyncSnapshotsIncludeOnlyFactories(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		101: "air-factory",
		500: "container",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 12, 10, 101, 1, 1)
	placeTestBuilding(t, w, 4, 4, 500, 1, 0)
	w.rebuildActiveTilesLocked()

	snaps := w.UnitFactoryBlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one unit factory block sync snapshot, got %d", len(snaps))
	}
	if snaps[0].Pos != protocol.PackPoint2(12, 10) {
		t.Fatalf("expected air-factory snapshot at pos=%d, got %d", protocol.PackPoint2(12, 10), snaps[0].Pos)
	}
}

func TestUnitFactoryConfigAndAcceptedInputsMatchCurrentPlan(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		418: "router",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
		8: "crawler",
	}
	w.SetModel(model)
	src := placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	factory := placeTestBuilding(t, w, 3, 2, 100, 1, 0)
	srcPos := int32(src.Y*w.Model().Width + src.X)
	factoryPos := int32(factory.Y*w.Model().Width + factory.X)

	if got, ok := w.BuildingConfigPacked(protocol.PackPoint2(3, 2)); !ok {
		t.Fatalf("expected default unit factory config to be present")
	} else if got != int32(0) {
		t.Fatalf("expected default unit factory plan 0, got %#v", got)
	}
	if !w.canAcceptItemLocked(srcPos, factoryPos, siliconItemID, 0) {
		t.Fatalf("expected default dagger plan to accept silicon")
	}
	if !w.canAcceptItemLocked(srcPos, factoryPos, leadItemID, 0) {
		t.Fatalf("expected default dagger plan to accept lead")
	}
	if w.canAcceptItemLocked(srcPos, factoryPos, coalItemID, 0) {
		t.Fatalf("expected default dagger plan to reject coal")
	}

	w.applyBuildingConfigLocked(factoryPos, int32(1), true)

	if got, ok := w.BuildingConfigPacked(protocol.PackPoint2(3, 2)); !ok {
		t.Fatalf("expected configured unit factory config to be present")
	} else if got != int32(1) {
		t.Fatalf("expected configured unit factory plan 1, got %#v", got)
	}
	if !w.canAcceptItemLocked(srcPos, factoryPos, coalItemID, 0) {
		t.Fatalf("expected crawler plan to accept coal")
	}
	if !w.canAcceptItemLocked(srcPos, factoryPos, siliconItemID, 0) {
		t.Fatalf("expected crawler plan to accept silicon")
	}
	if w.canAcceptItemLocked(srcPos, factoryPos, leadItemID, 0) {
		t.Fatalf("expected crawler plan to reject lead")
	}
}

func TestUnitFactoryCommandConfigPreservesPlanSelection(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
		8: "crawler",
	}
	w.SetModel(model)
	factory := placeTestBuilding(t, w, 3, 2, 100, 1, 0)
	factoryPacked := protocol.PackPoint2(3, 2)
	factoryPos := int32(factory.Y*w.Model().Width + factory.X)

	w.applyBuildingConfigLocked(factoryPos, int32(1), true)
	w.applyBuildingConfigLocked(factoryPos, protocol.UnitCommand{ID: 2, Name: "repair"}, true)
	w.CommandBuildingsPacked([]int32{factoryPacked}, protocol.Vec2{X: 88, Y: 104})

	if got, ok := w.BuildingConfigPacked(factoryPacked); !ok {
		t.Fatalf("expected configured unit factory config to be present")
	} else if got != int32(1) {
		t.Fatalf("expected configured unit factory plan 1 to remain selected, got %#v", got)
	}
	if st := w.factoryStates[factoryPos]; st.Command == nil || st.Command.ID != 2 {
		t.Fatalf("expected unit factory command id 2, got %+v", st.Command)
	} else if st.CommandPos == nil || math.Abs(float64(st.CommandPos.X-88)) > 0.0001 || math.Abs(float64(st.CommandPos.Y-104)) > 0.0001 {
		t.Fatalf("expected unit factory commandPos (88,104), got %+v", st.CommandPos)
	}

	w.applyBuildingConfigLocked(factoryPos, nil, true)

	if got, ok := w.BuildingConfigPacked(factoryPacked); !ok {
		t.Fatalf("expected unit factory plan to stay configured after clearing command")
	} else if got != int32(1) {
		t.Fatalf("expected unit factory plan 1 after clearing command, got %#v", got)
	}
	if st := w.factoryStates[factoryPos]; st.Command != nil {
		t.Fatalf("expected unit factory command to clear, got %+v", st.Command)
	} else if st.CommandPos == nil || math.Abs(float64(st.CommandPos.X-88)) > 0.0001 || math.Abs(float64(st.CommandPos.Y-104)) > 0.0001 {
		t.Fatalf("expected unit factory commandPos to stay set, got %+v", st.CommandPos)
	}
}

func TestFactoryDumpedUnitCarriesCommandState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 2, 2, 339, 1, 0)
	tile := placeTestBuilding(t, w, 5, 5, 100, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	state := factoryState{
		UnitType:    7,
		CurrentPlan: 0,
		CommandPos:  &protocol.Vec2{X: 120, Y: 96},
		Command:     &protocol.UnitCommand{ID: 2, Name: "repair"},
	}
	w.factoryStates[pos] = state
	w.payloadStates[pos] = &payloadRuntimeState{Payload: w.newFactoryUnitPayloadLocked(tile, state)}
	if w.payloadStates[pos].Payload == nil {
		t.Fatal("expected factory payload to be created")
	}

	if !w.dumpUnitPayloadFromTileLocked(pos, tile) {
		t.Fatal("expected factory payload to dump into a world unit")
	}

	found := false
	for _, ent := range w.model.Entities {
		if ent.TypeID != 7 || ent.Team != 1 {
			continue
		}
		found = true
		if ent.CommandID != 2 {
			t.Fatalf("expected dumped unit command id 2, got %d", ent.CommandID)
		}
		if ent.Behavior != "move" {
			t.Fatalf("expected dumped unit behavior move, got %q", ent.Behavior)
		}
		if math.Abs(float64(ent.PatrolAX-120)) > 0.0001 || math.Abs(float64(ent.PatrolAY-96)) > 0.0001 {
			t.Fatalf("expected dumped unit target (120,96), got (%f,%f)", ent.PatrolAX, ent.PatrolAY)
		}
	}
	if !found {
		t.Fatal("expected dumped factory unit entity to exist")
	}
}

func TestTryInsertItemLockedRejectsUnrelatedCrafterInputs(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		183: "silicon-smelter",
		418: "router",
	}
	w.SetModel(model)
	src := placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	dst := placeTestBuilding(t, w, 3, 2, 183, 1, 0)
	srcPos := int32(src.Y*w.Model().Width + src.X)
	dstPos := int32(dst.Y*w.Model().Width + dst.X)

	if !w.canAcceptItemLocked(srcPos, dstPos, coalItemID, 0) {
		t.Fatalf("expected silicon smelter to accept required coal input")
	}
	if !w.canAcceptItemLocked(srcPos, dstPos, sandItemID, 0) {
		t.Fatalf("expected silicon smelter to accept required sand input")
	}
	if w.canAcceptItemLocked(srcPos, dstPos, leadItemID, 0) {
		t.Fatalf("expected silicon smelter to reject unrelated lead input")
	}
	if w.tryInsertItemLocked(srcPos, dstPos, leadItemID, 0) {
		t.Fatalf("expected tryInsertItemLocked to reject unrelated crafter input")
	}
	if got := dst.Build.ItemAmount(leadItemID); got != 0 {
		t.Fatalf("expected rejected lead input to leave crafter inventory unchanged, got %d", got)
	}
	if !w.tryInsertItemLocked(srcPos, dstPos, coalItemID, 0) {
		t.Fatalf("expected tryInsertItemLocked to insert required coal input")
	}
	if got := dst.Build.ItemAmount(coalItemID); got != 1 {
		t.Fatalf("expected inserted coal amount 1, got %d", got)
	}
}

func setTestPayload(t *testing.T, w *World, x, y int, payload *payloadData) int32 {
	t.Helper()
	pos := int32(y*w.Model().Width + x)
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("payload tile lookup failed at (%d,%d): %v", x, y, err)
	}
	st := w.payloadStateLocked(pos)
	st.Payload = payload
	st.Move = 0
	st.Work = 0
	st.Exporting = false
	w.syncPayloadTileLocked(tile, payload)
	return pos
}

func TestWorldSnapshot(t *testing.T) {
	w := New(Config{TPS: 60})
	before := w.Snapshot()
	w.Step(500 * time.Millisecond)
	after := w.Snapshot()
	if after.WaveTime <= before.WaveTime {
		t.Fatalf("expected wavetime to increase, before=%v after=%v", before.WaveTime, after.WaveTime)
	}
	if after.Tps != 60 {
		t.Fatalf("expected tps=60, got=%d", after.Tps)
	}
}

func TestApplyBuildPlansIsAsync(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 100)

	ops := []BuildPlanOp{{
		Breaking: false,
		X:        2,
		Y:        3,
		Rotation: 1,
		BlockID:  45,
	}}
	w.ApplyBuildPlans(TeamID(1), ops)

	tile, err := w.Model().TileAt(2, 3)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected no immediate placement, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	w.Step(500 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected still pending build at 0.5s, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	placed := false
	for i := 0; i < 16; i++ { // up to 3.2s
		w.Step(200 * time.Millisecond)
		tile, _ = w.Model().TileAt(2, 3)
		if tile.Block == 45 && tile.Build != nil {
			placed = true
			break
		}
	}
	if !placed {
		tile, _ = w.Model().TileAt(2, 3)
		t.Fatalf("expected placed block after progress, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestBuildPlanWaitsForMaterialsInsteadOfRejecting(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	pos := int32(2 + 2*model.Width)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	if _, ok := w.pendingBuilds[pos]; !ok {
		t.Fatalf("expected duo plan to stay queued without materials")
	}

	stepForSeconds(w, 1)
	st, ok := w.pendingBuilds[pos]
	if !ok {
		t.Fatalf("expected queued plan to remain pending while missing all copper")
	}
	if st.VisualPlaced {
		t.Fatalf("expected build to wait for first material before beginPlace semantics")
	}

	core.Build.AddItem(0, 1)
	stepForSeconds(w, 0.2)
	st, ok = w.pendingBuilds[pos]
	if !ok {
		t.Fatalf("expected pending build after only 1 copper")
	}
	if !st.VisualPlaced {
		t.Fatalf("expected build to begin once one copper became available")
	}

	core.Build.AddItem(0, 34)
	built := false
	for i := 0; i < 40; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(2, 2)
		if tile.Block == 45 && tile.Build != nil {
			built = true
			break
		}
	}
	if !built {
		tile, _ := w.Model().TileAt(2, 2)
		t.Fatalf("expected duo to finish after remaining copper arrived, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestCancelPendingBuildRefundsOnlyConsumedItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 10)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})

	spent := false
	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
		if core.Build.ItemAmount(0) < 10 {
			spent = true
			break
		}
	}
	if !spent {
		t.Fatalf("expected partial duo build to consume some copper before cancellation")
	}
	beforeCancel := core.Build.ItemAmount(0)
	if beforeCancel <= 0 || beforeCancel >= 10 {
		t.Fatalf("expected only a partial spend before cancel, got %d", beforeCancel)
	}

	w.CancelBuildAt(2, 2, false)

	if _, ok := w.pendingBuilds[int32(2+2*model.Width)]; ok {
		t.Fatalf("expected pending build removed after cancel")
	}
	if got := core.Build.ItemAmount(0); got != 10 {
		t.Fatalf("expected cancel to refund only consumed copper back to original 10, got %d", got)
	}
}

func TestLaterBuildPlansStayQueuedWhenItemsRunOut(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 35)

	posA := int32(2 + 2*model.Width)
	posB := int32(3 + 2*model.Width)
	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{
		{X: 2, Y: 2, BlockID: 45},
		{X: 3, Y: 2, BlockID: 45},
	})

	if len(w.pendingBuilds) != 2 {
		t.Fatalf("expected both build plans queued instead of rejecting the later one, pending=%d", len(w.pendingBuilds))
	}
	if _, ok := w.pendingBuilds[posA]; !ok {
		t.Fatalf("expected first pending build to exist")
	}
	if _, ok := w.pendingBuilds[posB]; !ok {
		t.Fatalf("expected second pending build to remain queued")
	}

	firstBuilt := false
	for i := 0; i < 40; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(2, 2)
		if tile.Block == 45 && tile.Build != nil {
			firstBuilt = true
			break
		}
	}
	if !firstBuilt {
		tile, _ := w.Model().TileAt(2, 2)
		t.Fatalf("expected first duo to finish, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if _, ok := w.pendingBuilds[posB]; !ok {
		t.Fatalf("expected second plan to still be queued after first consumed all copper")
	}

	core.Build.AddItem(0, 35)
	secondBuilt := false
	for i := 0; i < 40; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(3, 2)
		if tile.Block == 45 && tile.Build != nil {
			secondBuilt = true
			break
		}
	}
	if !secondBuilt {
		tile, _ := w.Model().TileAt(3, 2)
		t.Fatalf("expected second duo to finish after copper refill, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestSetModelResetsRulesBetweenMaps(t *testing.T) {
	w := New(Config{TPS: 60})

	first := NewWorldModel(8, 8)
	first.Tags = map[string]string{
		"rules": `{"attackMode":true,"enemyCoreBuildRadius":123,"defaultTeam":"blue"}`,
	}
	w.SetModel(first)

	rules := w.GetRulesManager().Get()
	if !rules.AttackMode || rules.EnemyCoreBuildRadius != 123 || rules.DefaultTeam != "blue" {
		t.Fatalf("expected first map rules to apply, got attack=%v radius=%v defaultTeam=%q", rules.AttackMode, rules.EnemyCoreBuildRadius, rules.DefaultTeam)
	}

	second := NewWorldModel(8, 8)
	w.SetModel(second)

	rules = w.GetRulesManager().Get()
	if rules.AttackMode {
		t.Fatalf("expected attack mode reset to default false")
	}
	if rules.EnemyCoreBuildRadius != 400 {
		t.Fatalf("expected enemy core build radius reset to default 400, got %v", rules.EnemyCoreBuildRadius)
	}
	if rules.DefaultTeam != "sharded" {
		t.Fatalf("expected default team reset to sharded, got %q", rules.DefaultTeam)
	}
}

func TestSetModelInfersAttackGamemodeFromMultipleCoreTeams(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	tileA, _ := model.TileAt(1, 1)
	tileA.Block = 339
	tileA.Team = 1
	tileB, _ := model.TileAt(6, 6)
	tileB.Block = 339
	tileB.Team = 2

	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	if !rules.AttackMode {
		t.Fatalf("expected attack mode inferred from multi-team cores")
	}
	if rules.WaveSpacing != 120.0 {
		t.Fatalf("expected attack mode wave spacing=120, got %v", rules.WaveSpacing)
	}
	if rules.InfiniteResources {
		t.Fatalf("expected attack mode not to enable global infinite resources")
	}
	if !rules.teamInfiniteResources(2) {
		t.Fatalf("expected attack mode to grant infinite resources to wave team")
	}
}

func TestSetModelAppliesSandboxDefaultsBeforeRuleOverlay(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.Tags = map[string]string{
		"rules": `{"infiniteResources":true}`,
	}

	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	if !rules.InfiniteResources {
		t.Fatalf("expected sandbox infinite resources")
	}
	if !rules.AllowEditRules {
		t.Fatalf("expected sandbox allowEditRules default to be applied")
	}
	if rules.WaveTimer {
		t.Fatalf("expected sandbox waveTimer=false")
	}
}

func TestSetModelAppliesEditorDefaultsBeforeRuleOverlay(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.Tags = map[string]string{
		"rules": `{"editor":true}`,
	}

	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	if !rules.Editor || !rules.InstantBuild || !rules.InfiniteResources {
		t.Fatalf("expected editor defaults, got editor=%v instant=%v infinite=%v", rules.Editor, rules.InstantBuild, rules.InfiniteResources)
	}
	if rules.Waves || rules.WaveTimer {
		t.Fatalf("expected editor to disable waves and timer, got waves=%v timer=%v", rules.Waves, rules.WaveTimer)
	}
}

func TestSetModelPrefersExplicitMapModeBeforeOverlayHeuristics(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.Tags = map[string]string{
		"mode":  "survival",
		"rules": `{"infiniteResources":true}`,
	}

	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	if rules.ModeName != "survival" {
		t.Fatalf("expected explicit map mode survival, got %q", rules.ModeName)
	}
	if !rules.InfiniteResources {
		t.Fatalf("expected overlay infiniteResources to remain enabled")
	}
	if !rules.Waves || !rules.WaveTimer {
		t.Fatalf("expected survival defaults to remain active, got waves=%v timer=%v", rules.Waves, rules.WaveTimer)
	}
}

func TestDeconstructRefund(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 100)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	w.Step(3 * time.Second)
	mid := w.TeamItems(TeamID(1))[0]
	if mid >= 3000 {
		t.Fatalf("expected build to consume copper from starter inventory, mid=%d", mid)
	}

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		Breaking: true, X: 2, Y: 2,
	}})
	tile, _ := w.Model().TileAt(2, 2)
	_, refundStacks := w.deconstructRefundStacks(tile, TeamID(1))
	expectedRefund := int32(0)
	for _, stack := range refundStacks {
		if stack.Item == 0 {
			expectedRefund = stack.Amount
			break
		}
	}
	if expectedRefund <= 0 {
		t.Fatalf("expected duo deconstruct to refund copper, refund=%v", refundStacks)
	}
	breakDuration := w.buildDurationSecondsForTeam(45, TeamID(1), w.GetRulesManager().Get())
	stepForSeconds(w, breakDuration*0.6)
	during := w.TeamItems(TeamID(1))[0]
	if during <= mid {
		t.Fatalf("expected deconstruct progress to refund during dismantle, mid=%d during=%d", mid, during)
	}
	if during >= mid+expectedRefund {
		t.Fatalf("expected partial refund before dismantle completion, mid=%d during=%d expected_final=%d", mid, during, mid+expectedRefund)
	}

	stepForSeconds(w, breakDuration)
	after := w.TeamItems(TeamID(1))[0]
	if after != mid+expectedRefund {
		t.Fatalf("expected exact final refund after deconstruct, mid=%d during=%d after=%d expected=%d", mid, during, after, mid+expectedRefund)
	}
}

func TestBuilderDurationUsesOwnerUnitSpeedAndIgnoresUnitBuildSpeedRule(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45: "duo",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
		37: "gamma",
	}
	w.SetModel(model)
	w.blockBuildTimesByName = map[string]float32{"duo": 2}

	rules := DefaultRules()
	rules.UnitBuildSpeedMultiplier = 4
	rules.BuildSpeedMultiplier = 1
	w.GetRulesManager().Set(rules)
	w.SetTeamBuilderSpeed(1, 0.5)

	if _, err := w.AddEntityWithID(37, 9001, 20, 20, 1); err != nil {
		t.Fatalf("add gamma entity: %v", err)
	}
	w.UpdateBuilderState(101, 1, 9001, 20, 20, true, 220)

	got := w.buildDurationSecondsForOwnerLocked(45, 101, 1, w.GetRulesManager().Get())
	want := w.blockBuildTimesByName["duo"]
	if want <= 0 {
		t.Fatalf("expected duo build time metadata")
	}
	want /= 1.0 // gamma buildSpeed
	if math.Abs(float64(got-want)) > 0.0001 {
		t.Fatalf("expected owner gamma build speed to decide duration and ignore unitBuildSpeedMultiplier, want=%f got=%f", want, got)
	}
}

func TestBuildAndDeconstructProgressHealthUseConstructBlockScale(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.blockBuildTimesByName = map[string]float32{"duo": 1.5}

	core := placeTestBuilding(t, w, 4, 4, 339, 1, 0)
	core.Build.AddItem(0, 100)
	owner := int32(101)
	team := TeamID(1)
	if _, err := w.AddEntityWithID(35, 9001, 20, 20, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}
	w.UpdateBuilderState(owner, team, 9001, float32(2*8+4), float32(2*8+4), true, 220)

	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})

	for i := 0; i < 200; i++ {
		w.Step(time.Second / 60)
		tile, _ := w.Model().TileAt(2, 2)
		if tile.Block != 0 || tile.Build != nil {
			break
		}
		for _, ev := range w.DrainEntityEvents() {
			if ev.Kind == EntityEventBuildHealth && ev.BuildHP > constructBlockHealthMax {
				t.Fatalf("expected in-progress build health to stay on construct scale <= %.0f, got=%f", constructBlockHealthMax, ev.BuildHP)
			}
		}
	}
	_ = w.DrainEntityEvents()

	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		Breaking: true, X: 2, Y: 2,
	}})

	for i := 0; i < 200; i++ {
		w.Step(time.Second / 60)
		tile, _ := w.Model().TileAt(2, 2)
		if tile.Block == 0 && tile.Build == nil {
			break
		}
		for _, ev := range w.DrainEntityEvents() {
			if ev.Kind == EntityEventBuildHealth && ev.BuildHP > constructBlockHealthMax {
				t.Fatalf("expected in-progress deconstruct health to stay on construct scale <= %.0f, got=%f", constructBlockHealthMax, ev.BuildHP)
			}
		}
	}
}

func TestFactoryProductionSpawnsUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
		421: "battery",
		422: "power-node",
	}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 10, 339, 1, 0)
	core.Build.AddItem(copperItemID, 200)
	core.Build.AddItem(leadItemID, 200)
	core.Build.AddItem(siliconItemID, 200)
	placeTestBuilding(t, w, 3, 8, 421, 1, 0)
	placeTestBuilding(t, w, 3, 6, 422, 1, 0)
	w.powerStorageState[int32(8*model.Width+3)] = 4000

	factory := placeTestBuilding(t, w, 3, 3, 100, 1, 0)
	factory.Build.AddItem(siliconItemID, 10)
	factory.Build.AddItem(leadItemID, 10)
	linkPowerNode(t, w, 3, 6, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})
	nodePos := int32(6*model.Width + 3)
	if got := len(w.powerNodeLinks[nodePos]); got != 2 {
		t.Fatalf("expected power node to keep 2 links, got=%d links=%v", got, w.powerNodeLinks[nodePos])
	}
	w.Step(time.Second / 60)
	if st := w.teamPowerStates[1]; st == nil || st.Capacity <= 0 || st.Stored <= 0 {
		t.Fatalf("expected linked battery power for factory, state=%+v", st)
	}
	stepForSeconds(w, 9)
	if len(w.Model().Entities) != 0 {
		t.Fatalf("expected no unit before factory cycle, got=%d", len(w.Model().Entities))
	}
	stepForSeconds(w, 7)
	if len(w.Model().Entities) == 0 {
		t.Fatalf("expected produced unit, got=%d", len(w.Model().Entities))
	}
	factoryPos := int32(3 + 3*model.Width)
	if got := w.payloadStateLocked(factoryPos).Payload; got != nil {
		t.Fatalf("expected factory payload to dump after spawning, got=%+v", got)
	}
	if got := core.Build.ItemAmount(siliconItemID); got != 200 {
		t.Fatalf("expected factory production to keep core silicon unchanged, got=%d", got)
	}
	if got := core.Build.ItemAmount(leadItemID); got != 200 {
		t.Fatalf("expected factory production to keep core lead unchanged, got=%d", got)
	}
}

func TestFactoryProductionStallsWithoutPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 10, 339, 1, 0)
	core.Build.AddItem(0, 200)
	core.Build.AddItem(1, 200)
	core.Build.AddItem(2, 200)

	factory := placeTestBuilding(t, w, 3, 3, 100, 1, 0)
	factory.Build.AddItem(siliconItemID, 10)
	factory.Build.AddItem(leadItemID, 10)
	stepForSeconds(w, 20)

	if got := len(w.Model().Entities); got != 0 {
		t.Fatalf("expected unpowered factory to stay idle, entities=%d", got)
	}
}

func TestFactoryProductionProgressStallsAndResumesWithPowerRestore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
		421: "battery",
		422: "power-node",
	}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 10, 339, 1, 0)
	core.Build.AddItem(copperItemID, 200)
	core.Build.AddItem(leadItemID, 200)
	core.Build.AddItem(siliconItemID, 200)
	placeTestBuilding(t, w, 3, 8, 421, 1, 0)
	placeTestBuilding(t, w, 3, 6, 422, 1, 0)
	batteryPos := int32(8*model.Width + 3)
	w.powerStorageState[batteryPos] = 4000

	factory := placeTestBuilding(t, w, 3, 3, 100, 1, 0)
	factory.Build.AddItem(siliconItemID, 10)
	factory.Build.AddItem(leadItemID, 10)
	linkPowerNode(t, w, 3, 6, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	factoryPos := int32(3 + 3*model.Width)
	nodePos := int32(6*model.Width + 3)
	stepForSeconds(w, 5)

	progressBeforeLoss := w.factoryStates[factoryPos].Progress
	if progressBeforeLoss <= 0 || progressBeforeLoss >= unitFactoryPlansByBlockName["ground-factory"][0].TimeFrames {
		t.Fatalf("expected in-flight factory progress before power loss, got %f", progressBeforeLoss)
	}

	w.applyBuildingConfigLocked(nodePos, nil, true)
	w.Step(time.Second / 60)
	progressAtLoss := w.factoryStates[factoryPos].Progress
	stepForSeconds(w, 3)
	progressDuringLoss := w.factoryStates[factoryPos].Progress
	if math.Abs(float64(progressDuringLoss-progressAtLoss)) > 0.0001 {
		t.Fatalf("expected power loss to stall progress instead of resetting, before=%f after=%f", progressAtLoss, progressDuringLoss)
	}

	w.powerStorageState[batteryPos] = 4000
	linkPowerNode(t, w, 3, 6, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})
	stepForSeconds(w, 2)
	progressAfterRestore := w.factoryStates[factoryPos].Progress
	if progressAfterRestore <= progressDuringLoss {
		t.Fatalf("expected restored power to resume progress, stalled=%f restored=%f", progressDuringLoss, progressAfterRestore)
	}

	stepForSeconds(w, 10)
	if len(w.Model().Entities) == 0 {
		t.Fatalf("expected restored factory to finish production, entities=%d progress=%f", len(w.Model().Entities), w.factoryStates[factoryPos].Progress)
	}
}

func TestFactoryProductionOutputsUnitPayloadToPayloadConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
		421: "battery",
		422: "power-node",
		700: "payload-conveyor",
	}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)
	rules := DefaultRules()
	rules.Waves = false
	w.GetRulesManager().Set(rules)
	core := placeTestBuilding(t, w, 1, 10, 339, 1, 0)
	core.Build.AddItem(0, 2000)
	core.Build.AddItem(1, 2000)
	core.Build.AddItem(2, 2000)
	placeTestBuilding(t, w, 3, 8, 421, 1, 0)
	placeTestBuilding(t, w, 3, 6, 422, 1, 0)
	w.powerStorageState[int32(8*model.Width+3)] = 4000
	placeTestBuilding(t, w, 5, 3, 700, 1, 0)

	factory := placeTestBuilding(t, w, 3, 3, 100, 1, 0)
	factory.Build.AddItem(siliconItemID, 10)
	factory.Build.AddItem(leadItemID, 10)
	linkPowerNode(t, w, 3, 6, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})
	stepForSeconds(w, 17)

	if got := len(w.Model().Entities); got != 0 {
		t.Fatalf("expected unit to stay as payload when conveyor is in front, got entities=%d", got)
	}
	conveyorPos := int32(5 + 3*model.Width)
	payload := w.payloadStateLocked(conveyorPos).Payload
	if payload == nil || payload.Kind != payloadKindUnit || payload.UnitTypeID != 7 {
		t.Fatalf("expected conveyor to receive dagger unit payload, got=%+v", payload)
	}
}

func TestFactoryUnitPayloadUsesOfficialEntityWriteHeader(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	factory := placeTestBuilding(t, w, 3, 3, 100, 1, 0)

	payload := w.newFactoryUnitPayloadLocked(factory, factoryState{UnitType: 7})
	if payload == nil {
		t.Fatal("expected factory unit payload")
	}
	r := protocol.NewReader(payload.Serialized)
	exists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read payload exists failed: %v", err)
	}
	if !exists {
		t.Fatal("expected payload exists flag to be true")
	}
	kind, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read payload kind failed: %v", err)
	}
	if kind != protocol.PayloadUnit {
		t.Fatalf("expected payload kind unit, got %d", kind)
	}
	classID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read unit payload class id failed: %v", err)
	}
	if classID != 4 {
		t.Fatalf("expected dagger payload class id 4, got %d", classID)
	}
	revision, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read unit payload revision failed: %v", err)
	}
	if revision != 7 {
		t.Fatalf("expected dagger payload revision 7, got %d", revision)
	}
}

func TestFactoryProductionHonorsCoreUnitCap(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		339: "core-shard",
		421: "battery",
		422: "power-node",
	}
	model.UnitNames = map[int16]string{7: "dagger"}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 2000)
	core.Build.AddItem(1, 2000)
	core.Build.AddItem(2, 2000)
	placeTestBuilding(t, w, 3, 8, 421, 1, 0)
	placeTestBuilding(t, w, 3, 6, 422, 1, 0)
	w.powerStorageState[int32(8*model.Width+3)] = 4000

	placeTestBuilding(t, w, 3, 3, 100, 1, 0)
	factoryPos := int32(3 + 3*model.Width)
	linkPowerNode(t, w, 3, 6, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	countUnits := func() int {
		total := 0
		for _, e := range w.Model().Entities {
			if e.Team == 1 && e.TypeID == 7 {
				total++
			}
		}
		return total
	}

	for i := 0; i < 8; i++ {
		w.Model().AddEntity(w.newProducedUnitEntityLocked(7, 1, 60+float32(i*4), 80, 0))
	}
	if got := countUnits(); got != 8 {
		t.Fatalf("expected preseeded shard cap count 8, got=%d", got)
	}

	factoryTile := &w.Model().Tiles[factoryPos]
	if factoryTile.Build == nil || factoryTile.Block == 0 {
		t.Fatalf("expected factory to remain placed before capped cycle, block=%d build=%v entities=%d", factoryTile.Block, factoryTile.Build != nil, len(w.Model().Entities))
	}
	factoryTile.Build.AddItem(siliconItemID, 10)
	factoryTile.Build.AddItem(leadItemID, 10)
	stepForSeconds(w, 17)
	if got := countUnits(); got != 8 {
		t.Fatalf("expected 9th unit to remain blocked by cap, got=%d", got)
	}
	if payload := w.payloadStateLocked(factoryPos).Payload; payload == nil || payload.Kind != payloadKindUnit {
		t.Fatalf("expected capped factory to hold a unit payload, got=%+v", payload)
	}

	removedID := w.Model().Entities[0].ID
	if _, ok := w.model.RemoveEntity(removedID); !ok {
		t.Fatalf("expected to remove one capped unit id=%d", removedID)
	}
	w.Step(time.Second / 60)
	if got := countUnits(); got != 8 {
		t.Fatalf("expected held payload to dump after cap freed, got=%d", got)
	}
	if payload := w.payloadStateLocked(factoryPos).Payload; payload != nil {
		t.Fatalf("expected held payload to clear after dump, got=%+v", payload)
	}
}

func TestBuildPlanSnapshotClearsOnlyCurrentOwner(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		46:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)

	ownerA := int32(101)
	ownerB := int32(202)
	team := TeamID(1)
	w.UpdateBuilderState(ownerA, team, 9001, float32(1*8+4), float32(1*8+4), true, 220)
	w.UpdateBuilderState(ownerB, team, 9002, float32(2*8+4), float32(2*8+4), true, 220)

	w.ApplyBuildPlanSnapshotForOwner(ownerA, team, []BuildPlanOp{{
		X: 1, Y: 1, BlockID: 45,
	}})
	w.ApplyBuildPlanSnapshotForOwner(ownerB, team, []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 46,
	}})

	// Owner A sends an empty authoritative snapshot, equivalent to Q-clearing plans.
	w.ApplyBuildPlanSnapshotForOwner(ownerA, team, nil)

	// Owner B's plan must remain and continue progressing.
	placed := false
	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
		tileA, _ := w.Model().TileAt(1, 1)
		if tileA.Block != 0 || tileA.Build != nil {
			t.Fatalf("owner A plan should have been cleared, got block=%d build=%v", tileA.Block, tileA.Build != nil)
		}
		tileB, _ := w.Model().TileAt(2, 2)
		if tileB.Block == 46 && tileB.Build != nil {
			placed = true
			break
		}
	}
	if !placed {
		tileB, _ := w.Model().TileAt(2, 2)
		t.Fatalf("owner B plan should remain active, got block=%d build=%v", tileB.Block, tileB.Build != nil)
	}
}

func TestCancelBuildAtForOwnerDoesNotTouchOtherOwner(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		46:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)

	ownerA := int32(101)
	ownerB := int32(202)
	team := TeamID(1)
	w.UpdateBuilderState(ownerA, team, 9001, float32(1*8+4), float32(1*8+4), true, 220)
	w.UpdateBuilderState(ownerB, team, 9002, float32(2*8+4), float32(2*8+4), true, 220)

	w.ApplyBuildPlansForOwner(ownerA, team, []BuildPlanOp{{
		X: 1, Y: 1, BlockID: 45,
	}})
	w.ApplyBuildPlansForOwner(ownerB, team, []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 46,
	}})

	w.CancelBuildAtForOwner(ownerA, 1, 1, false)

	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
	}

	tileA, _ := w.Model().TileAt(1, 1)
	if tileA.Block != 0 || tileA.Build != nil {
		t.Fatalf("owner A tile should remain empty after cancel, got block=%d build=%v", tileA.Block, tileA.Build != nil)
	}
	tileB, _ := w.Model().TileAt(2, 2)
	if tileB.Block != 46 || tileB.Build == nil {
		t.Fatalf("owner B tile should still build successfully, got block=%d build=%v", tileB.Block, tileB.Build != nil)
	}
}

func TestBuildSnapshotWaitsForActiveBuilderInRange(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)

	owner := int32(101)
	team := TeamID(1)
	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 4, Y: 4, BlockID: 45,
	}})

	// Queue alone must not start build visuals or progress until the builder is
	// both active and inside Vars.buildingRange, mirroring BuilderComp.
	w.UpdateBuilderState(owner, team, 9001, 0, 0, false, 220)
	for i := 0; i < 10; i++ {
		w.Step(200 * time.Millisecond)
	}
	for _, ev := range w.DrainEntityEvents() {
		if ev.Kind == EntityEventBuildPlaced || ev.Kind == EntityEventBuildConstructed {
			t.Fatalf("unexpected build progress while builder inactive: %+v", ev)
		}
	}
	tile, _ := w.Model().TileAt(4, 4)
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected queued plan to stay unbuilt while inactive, got block=%d build=%v", tile.Block, tile.Build != nil)
	}

	w.UpdateBuilderState(owner, team, 9001, float32(4*8+4), float32(4*8+4), true, 220)
	built := false
	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ = w.Model().TileAt(4, 4)
		if tile.Block == 45 && tile.Build != nil {
			built = true
			break
		}
	}
	if !built {
		tile, _ = w.Model().TileAt(4, 4)
		t.Fatalf("expected build to finish once builder became active and in range, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestBuilderVisualPlaceDoesNotRequireStartItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	_ = placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	owner := int32(101)
	team := TeamID(1)
	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 4, Y: 4, BlockID: 45,
	}})
	w.UpdateBuilderState(owner, team, 9001, float32(4*8+4), float32(4*8+4), true, 220)

	w.Step(200 * time.Millisecond)

	evs := w.DrainEntityEvents()
	placed := false
	constructed := false
	for _, ev := range evs {
		if ev.Kind == EntityEventBuildPlaced {
			placed = true
		}
		if ev.Kind == EntityEventBuildConstructed {
			constructed = true
		}
	}
	if !placed {
		t.Fatal("expected active builder to emit build_placed even without starting items")
	}
	if constructed {
		t.Fatal("expected missing items to prevent immediate construction completion")
	}
}

func TestStaleBuilderStateStillProgressesBuild(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)

	owner := int32(101)
	team := TeamID(1)
	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 4, Y: 4, BlockID: 45,
	}})
	w.UpdateBuilderState(owner, team, 9001, float32(4*8+4), float32(4*8+4), true, 220)
	state := w.builderStates[owner]
	state.UpdatedAt = time.Now().Add(-5 * time.Second)
	w.builderStates[owner] = state

	for i := 0; i < 20; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(4, 4)
		if tile.Block == 45 && tile.Build != nil {
			return
		}
	}
	tile, _ := w.Model().TileAt(4, 4)
	t.Fatalf("expected stale builder state to still allow progress like Java, got block=%d build=%v", tile.Block, tile.Build != nil)
}

func TestPlaceTileLockedResetsStaleCrafterRuntimeState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		188: "melter",
		189: "cryofluid-mixer",
	}
	w.SetModel(model)

	tile, err := model.TileAt(4, 4)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	pos := int32(4*model.Width + 4)
	tile.Block = 188
	tile.Team = 1
	tile.Rotation = 0
	tile.Build = &Building{
		Block:     188,
		Team:      1,
		Rotation:  0,
		X:         4,
		Y:         4,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.crafterStates[pos] = crafterRuntimeState{Progress: 0.75, Warmup: 0.5, Seed: 7}
	w.rebuildBlockOccupancyLocked()

	w.placeTileLocked(tile, 1, 189, 0, nil, 0)

	state, ok := w.crafterStates[pos]
	if !ok {
		t.Fatal("expected new crafter runtime state to exist after placement")
	}
	if state.Progress != 0 || state.Warmup != 0 || state.Seed != 0 {
		t.Fatalf("expected stale crafter runtime state to reset, got %+v", state)
	}
}

func TestSnapshotCancelEmitsBuildCancelledNotDestroyed(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)

	owner := int32(101)
	team := TeamID(1)
	w.UpdateBuilderState(owner, team, 9001, float32(1*8+4), float32(1*8+4), true, 220)
	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 1, Y: 1, BlockID: 45,
	}})
	w.Step(200 * time.Millisecond)
	_ = w.DrainEntityEvents()

	w.ApplyBuildPlanSnapshotForOwner(owner, team, nil)
	evs := w.DrainEntityEvents()
	cancelled := false
	for _, ev := range evs {
		if ev.Kind == EntityEventBuildDestroyed {
			t.Fatalf("expected queue cancel to avoid build_destroyed, got %+v", ev)
		}
		if ev.Kind == EntityEventBuildCancelled {
			cancelled = true
		}
	}
	if !cancelled {
		t.Fatalf("expected build_cancelled event after authoritative queue clear")
	}
	tile, _ := w.Model().TileAt(1, 1)
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected cancelled tile to remain empty, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestPlacementSnapshotPreservesPendingBreaks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)
	placeTestBuilding(t, w, 1, 1, 45, 1, 0)

	owner := int32(101)
	team := TeamID(1)
	w.UpdateBuilderState(owner, team, 9001, float32(1*8+4), float32(1*8+4), true, 220)
	w.ApplyBuildPlansForOwner(owner, team, []BuildPlanOp{{
		Breaking: true,
		X:        1,
		Y:        1,
	}})

	pos := int32(1 + 1*model.Width)
	if _, ok := w.pendingBreaks[pos]; !ok {
		t.Fatalf("expected pending break at pos=%d before placement snapshot reconcile", pos)
	}

	w.ApplyPlacementPlanSnapshotForOwner(owner, team, nil)

	if _, ok := w.pendingBreaks[pos]; !ok {
		t.Fatalf("expected placement snapshot reconcile to preserve pending break at pos=%d", pos)
	}
}

func TestPlayerEntityOutOfBoundsIsClampedNotRemoved(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	w.SetModel(model)

	if _, err := w.AddEntityWithID(35, 1234, -16, 12, 1); err != nil {
		t.Fatalf("add entity: %v", err)
	}
	if _, ok := w.SetEntityPlayerController(1234, 77); !ok {
		t.Fatalf("expected player controller to be set")
	}

	w.Step(time.Second / 60)

	ent, ok := w.GetEntity(1234)
	if !ok {
		t.Fatalf("expected player-controlled entity to survive out-of-bounds correction")
	}
	if ent.X < 0 || ent.Y < 0 {
		t.Fatalf("expected clamped position, got (%f,%f)", ent.X, ent.Y)
	}
}

func TestReserveEntityIDPreventsWorldAllocationCollision(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	w.SetModel(model)

	reserved := w.ReserveEntityID()
	if reserved == 0 {
		t.Fatalf("expected reserved entity id")
	}
	if _, err := w.AddEntityWithID(35, reserved, 8, 8, 1); err != nil {
		t.Fatalf("add reserved entity: %v", err)
	}
	ent, err := w.AddEntity(35, 16, 16, 2)
	if err != nil {
		t.Fatalf("add next entity: %v", err)
	}
	if ent.ID == reserved {
		t.Fatalf("expected next entity id to differ from reserved id %d", reserved)
	}
}

func TestAddEntityWithIDRejectsDuplicateID(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	w.SetModel(model)

	if _, err := w.AddEntityWithID(35, 4321, 8, 8, 1); err != nil {
		t.Fatalf("add entity: %v", err)
	}
	if _, err := w.AddEntityWithID(35, 4321, 16, 16, 1); !errors.Is(err, ErrEntityExists) {
		t.Fatalf("expected ErrEntityExists, got %v", err)
	}
}

func TestAddEntityWithIDDoesNotInjectDefaultShieldOrArmor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.UnitNames = map[int16]string{
		910: "plain-unit",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["plain-unit"] = unitRuntimeProfile{Name: "plain-unit"}

	ent, err := w.AddEntityWithID(910, 5001, 8, 8, 1)
	if err != nil {
		t.Fatalf("add entity: %v", err)
	}
	if ent.Shield != 0 || ent.ShieldMax != 0 || ent.ShieldRegen != 0 {
		t.Fatalf("expected plain unit to start without default shield, got shield=%f max=%f regen=%f", ent.Shield, ent.ShieldMax, ent.ShieldRegen)
	}
	if ent.Armor != 0 {
		t.Fatalf("expected plain unit to start without default armor, got=%f", ent.Armor)
	}
}

func TestNewProducedUnitEntityDoesNotInjectDefaultShieldOrArmor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.UnitNames = map[int16]string{
		911: "produced-plain-unit",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["produced-plain-unit"] = unitRuntimeProfile{Name: "produced-plain-unit"}

	ent := w.newProducedUnitEntityLocked(911, 1, 16, 16, 0)
	if ent.Shield != 0 || ent.ShieldMax != 0 || ent.ShieldRegen != 0 {
		t.Fatalf("expected produced plain unit to start without default shield, got shield=%f max=%f regen=%f", ent.Shield, ent.ShieldMax, ent.ShieldRegen)
	}
	if ent.Armor != 0 {
		t.Fatalf("expected produced plain unit to start without default armor, got=%f", ent.Armor)
	}
}

func TestCustomCombatDamagesPlayerControlledUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	w.SetModel(model)

	enemy := RawEntity{
		ID:                 1,
		TypeID:             56,
		Team:               2,
		X:                  100,
		Y:                  100,
		Health:             100,
		MaxHealth:          100,
		AttackDamage:       40,
		AttackInterval:     0.05,
		AttackRange:        160,
		AttackFireMode:     "beam",
		AttackTargetAir:    true,
		AttackTargetGround: true,
		SlowMul:            1,
		RuntimeInit:        true,
	}
	player := RawEntity{
		ID:          2,
		PlayerID:    77,
		TypeID:      37,
		Team:        1,
		X:           120,
		Y:           100,
		Health:      220,
		MaxHealth:   220,
		Shield:      0,
		ShieldMax:   0,
		SlowMul:     1,
		RuntimeInit: true,
	}
	model.Entities = append(model.Entities, enemy, player)

	for i := 0; i < 20; i++ {
		w.Step(time.Second / 60)
	}

	ent, ok := w.GetEntity(2)
	if !ok {
		t.Fatalf("expected player-controlled entity to remain present")
	}
	if ent.Health >= 220 {
		t.Fatalf("expected custom combat to damage player-controlled unit health, got=%f", ent.Health)
	}
}

func TestUnitBeamUsesBuildingDamageMultiplier(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	model.Entities = append(model.Entities, RawEntity{
		ID:                   1,
		TypeID:               35,
		Team:                 1,
		X:                    float32(12*8 - 32),
		Y:                    float32(12*8 + 4),
		Health:               100,
		MaxHealth:            100,
		AttackDamage:         20,
		AttackBuildingDamage: 0.25,
		AttackInterval:       0.05,
		AttackRange:          120,
		AttackFireMode:       "beam",
		AttackBuildings:      true,
		AttackTargetGround:   true,
		SlowMul:              1,
		StatusDamageMul:      1,
		StatusHealthMul:      1,
		StatusSpeedMul:       1,
		StatusReloadMul:      1,
		StatusBuildSpeedMul:  1,
		StatusDragMul:        1,
		StatusArmorOverride:  -1,
		RuntimeInit:          true,
	})

	w.Step(time.Second / 60)

	if got := tile.Build.Health; math.Abs(float64(got-95)) > 0.01 {
		t.Fatalf("expected beam to deal 5 building damage, got health=%f", got)
	}
}

func TestProjectileUsesBuildingDamageMultiplierBelowOne(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	w.bullets = append(w.bullets, simBullet{
		ID:             1,
		Team:           1,
		X:              float32(12*8 + 4),
		Y:              float32(12*8 + 4),
		Damage:         20,
		Radius:         8,
		HitBuilds:      true,
		TargetGround:   true,
		BuildingDamage: 0.25,
	})
	w.stepBullets(0, map[int32]int{}, nil, nil)

	if got := tile.Build.Health; math.Abs(float64(got-95)) > 0.01 {
		t.Fatalf("expected projectile to deal 5 building damage, got health=%f", got)
	}
}

func TestProjectileCanDealZeroBuildingDamage(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	w.bullets = append(w.bullets, simBullet{
		ID:             1,
		Team:           1,
		X:              float32(12*8 + 4),
		Y:              float32(12*8 + 4),
		Damage:         20,
		Radius:         8,
		HitBuilds:      true,
		TargetGround:   true,
		BuildingDamage: 0,
	})
	w.stepBullets(0, map[int32]int{}, nil, nil)

	if got := tile.Build.Health; math.Abs(float64(got-100)) > 0.01 {
		t.Fatalf("expected zero building multiplier to deal no building damage, got health=%f", got)
	}
}

func TestBurningStatusDamagesAndExpires(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	w.SetModel(model)
	w.statusProfilesByID[1] = statusEffectProfile{
		ID:                   1,
		Name:                 "burning",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      1,
		ReloadMultiplier:     1,
		BuildSpeedMultiplier: 1,
		DragMultiplier:       1,
		Damage:               0.167 * 60,
	}
	w.statusProfilesByName["burning"] = w.statusProfilesByID[1]

	model.Entities = append(model.Entities, RawEntity{
		ID:                  1,
		TypeID:              35,
		Team:                1,
		X:                   16,
		Y:                   16,
		Health:              100,
		MaxHealth:           100,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
	})
	w.applyStatusToEntity(&model.Entities[0], 1, "burning", 1)

	stepForSeconds(w, 1.2)

	if got := model.Entities[0].Health; got >= 95 {
		t.Fatalf("expected burning DOT to reduce health, got=%f", got)
	}
	if len(model.Entities[0].Statuses) != 0 {
		t.Fatalf("expected burning to expire, statuses=%v", model.Entities[0].Statuses)
	}
}

func TestWetAndShockedUseOfficialReactiveDamage(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	w.SetModel(model)
	w.statusProfilesByID[2] = statusEffectProfile{
		ID:                   2,
		Name:                 "wet",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      0.94,
		ReloadMultiplier:     1,
		BuildSpeedMultiplier: 1,
		DragMultiplier:       1,
		TransitionDamage:     14,
	}
	w.statusProfilesByName["wet"] = w.statusProfilesByID[2]
	w.statusProfilesByID[3] = statusEffectProfile{
		ID:                   3,
		Name:                 "shocked",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      1,
		ReloadMultiplier:     1,
		BuildSpeedMultiplier: 1,
		DragMultiplier:       1,
		Reactive:             true,
	}
	w.statusProfilesByName["shocked"] = w.statusProfilesByID[3]

	entity := RawEntity{
		ID:                  1,
		TypeID:              35,
		Team:                1,
		Health:              100,
		MaxHealth:           100,
		Shield:              0,
		ShieldMax:           0,
		Armor:               0,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
	}
	w.applyStatusToEntity(&entity, 2, "wet", 2)
	w.applyStatusToEntity(&entity, 3, "shocked", 1)

	if math.Abs(float64(entity.Health-86)) > 0.01 {
		t.Fatalf("expected wet+shocked transition damage=14, got health=%f", entity.Health)
	}
	if len(entity.Statuses) != 1 || entity.Statuses[0].Name != "wet" {
		t.Fatalf("expected reactive shocked to not persist, statuses=%v", entity.Statuses)
	}
}

func TestDisarmedStatusPreventsAttacking(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	w.SetModel(model)
	w.statusProfilesByID[4] = statusEffectProfile{
		ID:                   4,
		Name:                 "disarmed",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      1,
		ReloadMultiplier:     1,
		BuildSpeedMultiplier: 1,
		DragMultiplier:       1,
		Disarm:               true,
	}
	w.statusProfilesByName["disarmed"] = w.statusProfilesByID[4]

	model.Entities = append(model.Entities,
		RawEntity{
			ID:                  1,
			TypeID:              35,
			Team:                1,
			X:                   40,
			Y:                   40,
			Health:              100,
			MaxHealth:           100,
			AttackDamage:        20,
			AttackInterval:      0.05,
			AttackRange:         80,
			AttackFireMode:      "beam",
			AttackTargetAir:     true,
			AttackTargetGround:  true,
			SlowMul:             1,
			StatusDamageMul:     1,
			StatusHealthMul:     1,
			StatusSpeedMul:      1,
			StatusReloadMul:     1,
			StatusBuildSpeedMul: 1,
			StatusDragMul:       1,
			StatusArmorOverride: -1,
			RuntimeInit:         true,
		},
		RawEntity{
			ID:                  2,
			TypeID:              35,
			Team:                2,
			X:                   72,
			Y:                   40,
			Health:              100,
			MaxHealth:           100,
			SlowMul:             1,
			StatusDamageMul:     1,
			StatusHealthMul:     1,
			StatusSpeedMul:      1,
			StatusReloadMul:     1,
			StatusBuildSpeedMul: 1,
			StatusDragMul:       1,
			StatusArmorOverride: -1,
			RuntimeInit:         true,
		},
	)
	w.applyStatusToEntity(&model.Entities[0], 4, "disarmed", 2)

	stepForSeconds(w, 0.5)

	if got := model.Entities[1].Health; got != 100 {
		t.Fatalf("expected disarmed attacker to deal no damage, target health=%f", got)
	}
}

func TestNuclearReactorOverheatsFromItemSource(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	reactor, _ := w.Model().TileAt(3, 3)
	reactor.Block = 315
	reactor.Team = 1
	reactor.Build = &Building{
		Block:     315,
		Team:      1,
		X:         3,
		Y:         3,
		Health:    1000,
		MaxHealth: 1000,
	}

	source, _ := w.Model().TileAt(2, 3)
	source.Block = 412
	source.Team = 1
	source.Build = &Building{
		Block:     412,
		Team:      1,
		X:         2,
		Y:         3,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.rebuildBlockOccupancyLocked()
	w.ConfigureItemSource(int32(3*model.Width+2), 5)

	for i := 0; i < 300; i++ {
		w.Step(time.Second / 60)
	}

	reactor, _ = w.Model().TileAt(3, 3)
	if reactor.Block != 0 || reactor.Build != nil {
		t.Fatalf("expected thorium reactor to explode and be destroyed, got block=%d build=%v", reactor.Block, reactor.Build != nil)
	}
}

func TestPoweredTurretDoesNotRechargeWithoutTeamPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		410: "arc",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 8, 8, 410, 1, 0)
	arcPos := int32(8*model.Width + 8)
	w.buildStates[arcPos] = buildCombatState{Power: 0}
	model.Entities = append(model.Entities, RawEntity{
		ID:          1,
		TypeID:      35,
		Team:        2,
		X:           float32(8*8 + 4 + 32),
		Y:           float32(8*8 + 4),
		Health:      100,
		MaxHealth:   100,
		SlowMul:     1,
		RuntimeInit: true,
	})

	for i := 0; i < 240; i++ {
		w.Step(time.Second / 60)
	}

	if got := model.Entities[0].Health; got != 100 {
		t.Fatalf("expected unpowered arc to not fire, health=%f", got)
	}
	if st := w.buildStates[arcPos]; st.Power != 0 {
		t.Fatalf("expected unpowered arc to stay empty, power=%f", st.Power)
	}
}

func TestThoriumReactorPowersTurretRecharge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		316: "power-node",
		410: "arc",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 5, 8, 315, 1, 0)
	reactor.Build.AddItem(7, 30)
	reactor.Build.AddLiquid(3, 30)
	node := placeTestBuilding(t, w, 8, 8, 316, 1, 0)
	placeTestBuilding(t, w, 11, 8, 410, 1, 0)
	nodePos := int32(8*model.Width + 8)
	arcPos := int32(8*model.Width + 11)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: -3, Y: 0}, {X: 3, Y: 0}}, true)
	w.buildStates[arcPos] = buildCombatState{Power: 0}
	model.Entities = append(model.Entities, RawEntity{
		ID:          1,
		TypeID:      35,
		Team:        2,
		X:           float32(11*8 + 4 + 32),
		Y:           float32(8*8 + 4),
		Health:      100,
		MaxHealth:   100,
		SlowMul:     1,
		RuntimeInit: true,
	})

	for i := 0; i < 240; i++ {
		w.Step(time.Second / 60)
	}

	if got := model.Entities[0].Health; got >= 100 {
		t.Fatalf("expected powered arc to fire after reactor recharge, health=%f power=%f produced=%f consumed=%f", got, w.buildStates[arcPos].Power, w.teamPowerStates[1].Produced, w.teamPowerStates[1].Consumed)
	}
	if st := w.teamPowerStates[1]; st == nil || st.Produced <= 0 {
		t.Fatalf("expected team power production from thorium reactor")
	}
	_ = reactor
	_ = node
}

func TestThoriumReactorBuildsHeatProgress(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 8, 8, 315, 1, 0)
	reactor.Build.AddItem(thoriumItemID, 30)

	w.Step(time.Second / 60)

	reactorPos := int32(8*model.Width + 8)
	st, ok := w.reactorStates[reactorPos]
	if !ok {
		t.Fatal("expected thorium reactor runtime state to exist")
	}
	if st.Heat <= 0 {
		t.Fatalf("expected thorium reactor heat to rise, heat=%f", st.Heat)
	}
	if st.HeatProgress <= 0 {
		t.Fatalf("expected thorium reactor heat progress to rise like vanilla, heatProgress=%f", st.HeatProgress)
	}
	if got := w.heatStates[reactorPos]; got <= 0 {
		t.Fatalf("expected thorium reactor to publish heat state, heat=%f", got)
	}
}

func TestSolarPowerStoresIntoBattery(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		420: "solar-panel-large",
		421: "battery",
		422: "power-node",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 4, 420, 1, 0)
	placeTestBuilding(t, w, 8, 4, 421, 1, 0)
	placeTestBuilding(t, w, 6, 4, 422, 1, 0)
	nodePos := int32(4*model.Width + 6)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: -2, Y: 0}, {X: 2, Y: 0}}, true)

	for i := 0; i < 600; i++ {
		w.Step(time.Second / 60)
	}

	st := w.teamPowerStates[1]
	if st == nil {
		t.Fatalf("expected team power state to exist")
	}
	if st.Stored <= 0 {
		t.Fatalf("expected battery to store solar power, stored=%f", st.Stored)
	}
	if st.Stored > st.Capacity {
		t.Fatalf("expected stored power <= capacity, stored=%f capacity=%f", st.Stored, st.Capacity)
	}
}

func TestLaserDrillRequiresPowerToMine(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		430: "laser-drill",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 6, 6, 3, 2)
	drill := placeTestBuilding(t, w, 6, 6, 430, 1, 0)

	stepForSeconds(w, 20)

	if got := totalBuildingItems(drill.Build); got != 0 {
		t.Fatalf("expected unpowered laser drill to stay idle, items=%d", got)
	}
}

func TestLaserDrillMinesOreWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		421: "battery",
		422: "power-node",
		430: "laser-drill",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 6, 6, 3, 2)
	drill := placeTestBuilding(t, w, 6, 6, 430, 1, 0)
	placeTestBuilding(t, w, 6, 10, 421, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	w.powerStorageState[int32(10*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 8, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 12)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected powered laser drill to mine ore, items=%d", got)
	}
}

func TestMechanicalDrillMinesWithoutPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		429: "mechanical-drill",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 5, 5, 2, 2)
	drill := placeTestBuilding(t, w, 5, 5, 429, 1, 0)

	stepForSeconds(w, 26)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected mechanical drill to mine without power, items=%d", got)
	}
}

func TestImpactDrillOffloadsEntireBurstIntoAdjacentCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		16:  "ore-beryllium",
		478: "power-source",
		904: "impact-drill",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 8, 8, 4, 16)
	core := placeTestBuilding(t, w, 5, 8, 339, 1, 0)
	drill := placeTestBuilding(t, w, 8, 8, 904, 1, 0)
	drill.Build.AddLiquid(waterLiquidID, 200)
	placeTestBuilding(t, w, 8, 12, 478, 1, 0)
	linkPowerNode(t, w, 8, 12, protocol.Point2{X: 0, Y: -4})

	for i := 0; i < 600; i++ {
		w.Step(time.Second / 60)
		if core.Build.ItemAmount(berylliumItemID) > 0 || drill.Build.ItemAmount(berylliumItemID) > 0 {
			break
		}
	}

	if got := core.Build.ItemAmount(berylliumItemID); got != 16 {
		t.Fatalf("expected adjacent core to receive full impact-drill burst 16, got %d", got)
	}
	if got := drill.Build.ItemAmount(berylliumItemID); got != 0 {
		t.Fatalf("expected impact-drill buffer to stay empty when offload path is open, got %d", got)
	}
}

func TestMechanicalPumpPumpsFloorLiquid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		1:   "water",
		440: "mechanical-pump",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 4, 4, 1, 1)
	pump := placeTestBuilding(t, w, 4, 4, 440, 1, 0)

	stepForSeconds(w, 2)

	if got := pump.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected mechanical pump to extract floor water, amount=%f", got)
	}
}

func TestRotaryPumpRequiresPowerToPump(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		1:   "water",
		421: "battery",
		422: "power-node",
		441: "rotary-pump",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 6, 6, 2, 1)
	pump := placeTestBuilding(t, w, 6, 6, 441, 1, 0)

	stepForSeconds(w, 2)
	if got := pump.Build.LiquidAmount(waterLiquidID); got != 0 {
		t.Fatalf("expected unpowered rotary pump to stay idle, amount=%f", got)
	}

	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if got := pump.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected powered rotary pump to extract floor water, amount=%f", got)
	}
}

func TestWaterExtractorProducesWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		10:  "stone",
		421: "battery",
		422: "power-node",
		442: "water-extractor",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 6, 6, 2, 10)
	extractor := placeTestBuilding(t, w, 6, 6, 442, 1, 0)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if got := extractor.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected powered water extractor to produce water, amount=%f", got)
	}
}

func TestOilExtractorConsumesResourcesWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		11:  "shale",
		421: "battery",
		422: "power-node",
		443: "oil-extractor",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 8, 8, 3, 11)
	extractor := placeTestBuilding(t, w, 8, 8, 443, 1, 0)
	extractor.Build.AddItem(sandItemID, 2)
	extractor.Build.AddLiquid(waterLiquidID, 40)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 5)

	if got := extractor.Build.LiquidAmount(oilLiquidID); got <= 0 {
		t.Fatalf("expected oil extractor to produce oil, amount=%f", got)
	}
	if got := extractor.Build.LiquidAmount(waterLiquidID); got >= 40 {
		t.Fatalf("expected oil extractor to consume water, remaining=%f", got)
	}
	if got := extractor.Build.ItemAmount(sandItemID); got >= 2 {
		t.Fatalf("expected oil extractor to consume sand over time, remaining=%d", got)
	}
}

func TestGraphitePressCraftsWithoutPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		450: "graphite-press",
	}
	w.SetModel(model)
	press := placeTestBuilding(t, w, 4, 4, 450, 1, 0)
	press.Build.AddItem(coalItemID, 2)

	stepForSeconds(w, 2)

	if got := press.Build.ItemAmount(graphiteItemID); got <= 0 {
		t.Fatalf("expected graphite press to craft graphite without power, amount=%d", got)
	}
}

func TestSiliconSmelterRequiresPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		451: "silicon-smelter",
	}
	w.SetModel(model)
	smelter := placeTestBuilding(t, w, 6, 6, 451, 1, 0)
	smelter.Build.AddItem(coalItemID, 2)
	smelter.Build.AddItem(sandItemID, 4)

	stepForSeconds(w, 2)
	if got := smelter.Build.ItemAmount(siliconItemID); got != 0 {
		t.Fatalf("expected unpowered silicon smelter to stay idle, amount=%d", got)
	}

	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if got := smelter.Build.ItemAmount(siliconItemID); got <= 0 {
		t.Fatalf("expected powered silicon smelter to craft silicon, amount=%d", got)
	}
}

func TestSiliconArcFurnaceMatchesVanillaPowerCraft(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		454: "silicon-arc-furnace",
	}
	w.SetModel(model)
	furnace := placeTestBuilding(t, w, 6, 6, 454, 1, 0)
	furnace.Build.AddItem(graphiteItemID, 1)
	furnace.Build.AddItem(sandItemID, 4)

	stepForSeconds(w, 1)
	if got := furnace.Build.ItemAmount(siliconItemID); got != 0 {
		t.Fatalf("expected unpowered silicon arc furnace to stay idle, amount=%d", got)
	}

	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	if got := furnace.Build.ItemAmount(siliconItemID); got != 4 {
		t.Fatalf("expected powered silicon arc furnace to craft 4 silicon, amount=%d", got)
	}
	if got := furnace.Build.ItemAmount(graphiteItemID); got != 0 {
		t.Fatalf("expected powered silicon arc furnace to consume graphite, remaining=%d", got)
	}
	if got := furnace.Build.ItemAmount(sandItemID); got != 0 {
		t.Fatalf("expected powered silicon arc furnace to consume sand, remaining=%d", got)
	}
}

func TestCryofluidMixerProducesCryofluid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		452: "cryofluid-mixer",
	}
	w.SetModel(model)
	mixer := placeTestBuilding(t, w, 6, 6, 452, 1, 0)
	mixer.Build.AddItem(titaniumItemID, 2)
	mixer.Build.AddLiquid(waterLiquidID, 36)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	if got := mixer.Build.LiquidAmount(cryofluidLiquidID); got <= 0 {
		t.Fatalf("expected cryofluid mixer to produce cryofluid, amount=%f", got)
	}
	if got := mixer.Build.LiquidAmount(waterLiquidID); got >= 36 {
		t.Fatalf("expected cryofluid mixer to consume water, remaining=%f", got)
	}
}

func TestSeparatorProducesItemsFromSlag(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		453: "separator",
	}
	w.SetModel(model)
	separator := placeTestBuilding(t, w, 6, 6, 453, 1, 0)
	separator.Build.AddLiquid(slagLiquidID, 20)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	total := totalBuildingItems(separator.Build)
	if total <= 0 {
		t.Fatalf("expected separator to produce at least one item, total=%d", total)
	}
	produced := separator.Build.ItemAmount(copperItemID) + separator.Build.ItemAmount(leadItemID) + separator.Build.ItemAmount(graphiteItemID) + separator.Build.ItemAmount(titaniumItemID)
	if produced != total {
		t.Fatalf("expected separator outputs to match vanilla result pool, total=%d produced=%d", total, produced)
	}
}

func TestDisassemblerConsumesScrapAndSlag(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		458: "disassembler",
	}
	w.SetModel(model)
	disassembler := placeTestBuilding(t, w, 8, 8, 458, 1, 0)
	disassembler.Build.AddItem(scrapItemID, 2)
	disassembler.Build.AddLiquid(slagLiquidID, 20)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	total := totalBuildingItems(disassembler.Build)
	if total <= 0 {
		t.Fatalf("expected disassembler to produce at least one item, total=%d", total)
	}
	if got := disassembler.Build.ItemAmount(scrapItemID); got >= 2 {
		t.Fatalf("expected disassembler to consume scrap, remaining=%d", got)
	}
	if got := disassembler.Build.LiquidAmount(slagLiquidID); got >= 20 {
		t.Fatalf("expected disassembler to consume slag, remaining=%f", got)
	}
	produced := disassembler.Build.ItemAmount(sandItemID) + disassembler.Build.ItemAmount(graphiteItemID) + disassembler.Build.ItemAmount(titaniumItemID) + disassembler.Build.ItemAmount(thoriumItemID)
	if produced != total {
		t.Fatalf("expected disassembler outputs to match vanilla result pool, total=%d produced=%d", total, produced)
	}
}

func TestSlagCentrifugeConsumesSandAndSlag(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		459: "slag-centrifuge",
	}
	w.SetModel(model)
	centrifuge := placeTestBuilding(t, w, 8, 8, 459, 1, 0)
	centrifuge.Build.AddItem(sandItemID, 1)
	centrifuge.Build.AddLiquid(slagLiquidID, 80)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	if got := centrifuge.Build.LiquidAmount(galliumLiquidID); got <= 0 {
		t.Fatalf("expected slag centrifuge to produce gallium, amount=%f", got)
	}
	if got := centrifuge.Build.LiquidAmount(slagLiquidID); got >= 80 {
		t.Fatalf("expected slag centrifuge to consume slag, remaining=%f", got)
	}
	if got := centrifuge.Build.ItemAmount(sandItemID); got != 0 {
		t.Fatalf("expected slag centrifuge to consume sand after one craft, remaining=%d", got)
	}
}

func TestPlastaniumCompressorConsumesOilAndTitanium(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		454: "plastanium-compressor",
	}
	w.SetModel(model)
	compressor := placeTestBuilding(t, w, 6, 6, 454, 1, 0)
	compressor.Build.AddItem(titaniumItemID, 4)
	compressor.Build.AddLiquid(oilLiquidID, 60)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	if got := compressor.Build.ItemAmount(plastaniumItemID); got <= 0 {
		t.Fatalf("expected plastanium compressor to craft plastanium, amount=%d", got)
	}
	if got := compressor.Build.LiquidAmount(oilLiquidID); got >= 60 {
		t.Fatalf("expected plastanium compressor to consume oil, remaining=%f", got)
	}
}

func TestSiliconCrucibleGetsHeatBoost(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		20:  "hotrock",
		421: "battery",
		422: "power-node",
		455: "silicon-crucible",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 8, 8, 3, 20)
	crucible := placeTestBuilding(t, w, 8, 8, 455, 1, 0)
	crucible.Build.AddItem(coalItemID, 4)
	crucible.Build.AddItem(sandItemID, 6)
	crucible.Build.AddItem(pyratiteItemID, 1)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	if got := crucible.Build.ItemAmount(siliconItemID); got < 8 {
		t.Fatalf("expected heated silicon crucible to finish one craft within 1s, amount=%d", got)
	}
}

func TestSporePressProducesOilWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		456: "spore-press",
	}
	w.SetModel(model)
	press := placeTestBuilding(t, w, 6, 6, 456, 1, 0)
	press.Build.AddItem(sporePodItemID, 2)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	if got := press.Build.LiquidAmount(oilLiquidID); got <= 0 {
		t.Fatalf("expected spore press to produce oil, amount=%f", got)
	}
}

func TestCultivatorGetsSporeBoost(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		21:  "spore-moss",
		22:  "stone",
		421: "battery",
		422: "power-node",
		457: "cultivator",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 6, 6, 2, 21)
	paintAreaFloor(t, w, 12, 6, 2, 22)
	boosted := placeTestBuilding(t, w, 6, 6, 457, 1, 0)
	plain := placeTestBuilding(t, w, 12, 6, 457, 1, 0)
	boosted.Build.AddLiquid(waterLiquidID, 80)
	plain.Build.AddLiquid(waterLiquidID, 80)
	placeTestBuilding(t, w, 9, 12, 421, 1, 0)
	placeTestBuilding(t, w, 9, 9, 422, 1, 0)
	w.powerStorageState[int32(12*model.Width+9)] = 4000
	linkPowerNode(t, w, 9, 9, protocol.Point2{X: -3, Y: -3}, protocol.Point2{X: 3, Y: -3}, protocol.Point2{X: 0, Y: 3})

	stepForSeconds(w, 1)

	if got := boosted.Build.ItemAmount(sporePodItemID); got <= 0 {
		t.Fatalf("expected spore-boosted cultivator to finish within 1s, amount=%d", got)
	}
	if got := plain.Build.ItemAmount(sporePodItemID); got != 0 {
		t.Fatalf("expected plain cultivator to still be in progress after 1s, amount=%d", got)
	}
}

func TestVentCondenserRequiresFullSteamFootprint(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		30:  "rhyolite",
		31:  "rhyolite-vent",
		421: "battery",
		422: "power-node",
		458: "vent-condenser",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 8, 8, 3, 31)
	tile, err := w.Model().TileAt(7, 7)
	if err != nil || tile == nil {
		t.Fatalf("floor tile lookup failed: %v", err)
	}
	tile.Floor = 30

	condenser := placeTestBuilding(t, w, 8, 8, 458, 1, 0)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)
	if got := condenser.Build.LiquidAmount(waterLiquidID); got != 0 {
		t.Fatalf("expected vent condenser to stay idle below vanilla min efficiency, amount=%f", got)
	}

	paintAreaFloor(t, w, 8, 8, 3, 31)
	stepForSeconds(w, 3)

	if got := condenser.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected vent condenser to produce water on full steam footprint, amount=%f", got)
	}
}

func TestElectricHeaterBuildsHeatWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		461: "electric-heater",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 6, 461, 1, 0)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	if heat := w.heatStates[int32(6*model.Width+6)]; heat <= 0 {
		t.Fatalf("expected powered electric heater to build heat, heat=%f", heat)
	}
}

func TestHeatSourceProducesMaxHeatImmediately(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		490: "heat-source",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 4, 4, 490, 1, 0)

	w.Step(time.Second / 60)

	if heat := w.heatStates[int32(4*model.Width+4)]; heat < 999 {
		t.Fatalf("expected heat-source to publish vanilla max heat immediately, heat=%f", heat)
	}
}

func TestItemVoidAcceptsAndDeletesItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		430: "router",
		491: "item-void",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 3, 3, 430, 1, 0)
	placeTestBuilding(t, w, 4, 3, 491, 1, 0)
	src.Build.AddItem(coalItemID, 1)
	item := coalItemID

	moved := w.dumpSingleItemLocked(int32(3*model.Width+3), src, &item, nil)
	if !moved {
		t.Fatal("expected item-void to accept dumped item")
	}
	if got := src.Build.ItemAmount(coalItemID); got != 0 {
		t.Fatalf("expected item to be deleted by item-void, remaining=%d", got)
	}
}

func TestLiquidVoidAcceptsAndDeletesLiquids(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		431: "liquid-router",
		492: "liquid-void",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 3, 3, 431, 1, 0)
	placeTestBuilding(t, w, 4, 3, 492, 1, 0)
	src.Build.AddLiquid(waterLiquidID, 10)

	moved := w.dumpLiquidLocked(int32(3*model.Width+3), src, waterLiquidID, 10)
	if !moved {
		t.Fatal("expected liquid-void to accept dumped liquid")
	}
	if got := src.Build.LiquidAmount(waterLiquidID); got != 0 {
		t.Fatalf("expected liquid to be deleted by liquid-void, remaining=%f", got)
	}
}

func TestAtmosphericConcentratorRequiresHeat(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		463: "atmospheric-concentrator",
	}
	w.SetModel(model)
	concentrator := placeTestBuilding(t, w, 8, 8, 463, 1, 0)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)
	if got := concentrator.Build.LiquidAmount(nitrogenLiquidID); got != 0 {
		t.Fatalf("expected unheated atmospheric concentrator to stay idle, amount=%f", got)
	}

	west := placeTestBuilding(t, w, 5, 8, 462, 1, 0)
	east := placeTestBuilding(t, w, 11, 8, 462, 1, 2)
	north := placeTestBuilding(t, w, 8, 5, 462, 1, 1)
	west.Build.AddLiquid(slagLiquidID, 120)
	east.Build.AddLiquid(slagLiquidID, 120)
	north.Build.AddLiquid(slagLiquidID, 120)

	stepForSeconds(w, 3)

	if got := concentrator.Build.LiquidAmount(nitrogenLiquidID); got <= 0 {
		t.Fatalf("expected heated atmospheric concentrator to produce nitrogen, amount=%f", got)
	}
	if heat := w.crafterReceivedHeatLocked(int32(8*model.Width+8), concentrator); heat < 24 {
		t.Fatalf("expected atmospheric concentrator to receive vanilla heat requirement, heat=%f", heat)
	}
}

func TestOxidationChamberProducesOxideAndHeat(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		464: "oxidation-chamber",
	}
	w.SetModel(model)
	chamber := placeTestBuilding(t, w, 8, 8, 464, 1, 0)
	chamber.Build.AddItem(berylliumItemID, 2)
	chamber.Build.AddLiquid(ozoneLiquidID, 10)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	if got := chamber.Build.ItemAmount(oxideItemID); got <= 0 {
		t.Fatalf("expected oxidation chamber to craft oxide, amount=%d", got)
	}
	if heat := w.heatStates[int32(8*model.Width+8)]; heat <= 0 {
		t.Fatalf("expected oxidation chamber to output heat while active, heat=%f", heat)
	}
}

func TestHeatRedirectorRelaysHeatToCrafter(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		463: "atmospheric-concentrator",
		465: "heat-redirector",
	}
	w.SetModel(model)
	concentrator := placeTestBuilding(t, w, 12, 12, 463, 1, 0)
	placeTestBuilding(t, w, 12, 17, 421, 1, 0)
	placeTestBuilding(t, w, 12, 15, 422, 1, 0)
	w.powerStorageState[int32(17*model.Width+12)] = 4000
	linkPowerNode(t, w, 12, 15, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	placeTestBuilding(t, w, 9, 12, 465, 1, 0)
	placeTestBuilding(t, w, 15, 12, 465, 1, 2)
	placeTestBuilding(t, w, 12, 9, 465, 1, 1)
	westHeater := placeTestBuilding(t, w, 6, 12, 462, 1, 0)
	eastHeater := placeTestBuilding(t, w, 18, 12, 462, 1, 2)
	northHeater := placeTestBuilding(t, w, 12, 6, 462, 1, 1)
	westHeater.Build.AddLiquid(slagLiquidID, 240)
	eastHeater.Build.AddLiquid(slagLiquidID, 240)
	northHeater.Build.AddLiquid(slagLiquidID, 240)

	stepForSeconds(w, 4)

	if got := concentrator.Build.LiquidAmount(nitrogenLiquidID); got <= 0 {
		t.Fatalf("expected redirected heat to drive atmospheric concentrator, amount=%f", got)
	}
	if heat := w.crafterReceivedHeatLocked(int32(12*model.Width+12), concentrator); heat < 24 {
		t.Fatalf("expected redirected heat to satisfy vanilla requirement, heat=%f", heat)
	}
}

func TestHeatRouterDoesNotOutputToFront(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		463: "atmospheric-concentrator",
		466: "heat-router",
	}
	w.SetModel(model)
	router := placeTestBuilding(t, w, 10, 10, 466, 1, 0)
	w.heatStates[int32(10*model.Width+10)] = 24
	blocked := placeTestBuilding(t, w, 13, 10, 463, 1, 0)
	allowed := placeTestBuilding(t, w, 10, 7, 463, 1, 0)

	if heat := w.crafterReceivedHeatLocked(int32(10*model.Width+13), blocked); heat != 0 {
		t.Fatalf("expected heat router front side to block heat, heat=%f", heat)
	}
	if heat := w.crafterReceivedHeatLocked(int32(7*model.Width+10), allowed); heat <= 0 {
		t.Fatalf("expected heat router side to output split heat, heat=%f", heat)
	}
	if heat := w.crafterReceivedHeatLocked(int32(7*model.Width+10), allowed); heat >= 24 {
		t.Fatalf("expected heat router side to split heat across surfaces, heat=%f", heat)
	}
	if router == nil {
		t.Fatalf("expected heat router placement to succeed")
	}
}

func TestElectrolyzerSplitsLiquidOutputsByDirection(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		423: "liquid-router",
		460: "electrolyzer",
	}
	w.SetModel(model)
	electrolyzer := placeTestBuilding(t, w, 8, 8, 460, 1, 0)
	electrolyzer.Build.AddLiquid(waterLiquidID, 20)
	north := placeTestBuilding(t, w, 8, 6, 423, 1, 0)
	south := placeTestBuilding(t, w, 8, 10, 423, 1, 0)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 4000
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if got := south.Build.LiquidAmount(ozoneLiquidID); got <= 0 {
		t.Fatalf("expected south side to receive ozone, amount=%f", got)
	}
	if got := south.Build.LiquidAmount(hydrogenLiquidID); got != 0 {
		t.Fatalf("expected south side to reject hydrogen, amount=%f", got)
	}
	if got := north.Build.LiquidAmount(hydrogenLiquidID); got <= 0 {
		t.Fatalf("expected north side to receive hydrogen, amount=%f", got)
	}
	if got := north.Build.LiquidAmount(ozoneLiquidID); got != 0 {
		t.Fatalf("expected north side to reject ozone, amount=%f", got)
	}
}

func TestCarbideCrucibleRequiresHeatAndCraftsCarbide(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		465: "heat-redirector",
		467: "carbide-crucible",
	}
	w.SetModel(model)
	crucible := placeTestBuilding(t, w, 10, 10, 467, 1, 0)
	crucible.Build.AddItem(tungstenItemID, 6)
	crucible.Build.AddItem(graphiteItemID, 9)
	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 16, 422, 1, 0)
	w.powerStorageState[int32(18*model.Width+10)] = 4000
	linkPowerNode(t, w, 10, 16, protocol.Point2{X: 0, Y: -6}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)
	if got := crucible.Build.ItemAmount(carbideItemID); got != 0 {
		t.Fatalf("expected unheated carbide crucible to stay idle, amount=%d", got)
	}

	placeTestBuilding(t, w, 7, 10, 465, 1, 0)
	west := placeTestBuilding(t, w, 4, 10, 462, 1, 0)
	redirectorNorth := placeTestBuilding(t, w, 7, 7, 462, 1, 1)
	east := placeTestBuilding(t, w, 13, 10, 462, 1, 2)
	north := placeTestBuilding(t, w, 10, 7, 462, 1, 1)
	south := placeTestBuilding(t, w, 10, 13, 462, 1, 3)
	west.Build.AddLiquid(slagLiquidID, 120)
	redirectorNorth.Build.AddLiquid(slagLiquidID, 120)
	east.Build.AddLiquid(slagLiquidID, 120)
	north.Build.AddLiquid(slagLiquidID, 120)
	south.Build.AddLiquid(slagLiquidID, 120)

	stepForSeconds(w, 3)

	if got := crucible.Build.ItemAmount(carbideItemID); got <= 0 {
		t.Fatalf("expected heated carbide crucible to craft carbide, amount=%d heat=%f", got, w.crafterReceivedHeatLocked(int32(10*model.Width+10), crucible))
	}
	if heat := w.crafterReceivedHeatLocked(int32(10*model.Width+10), crucible); heat < 40 {
		t.Fatalf("expected carbide crucible to receive vanilla heat requirement, heat=%f", heat)
	}
}

func TestSurgeCrucibleRequiresHeatAndCraftsSurgeAlloy(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		465: "heat-redirector",
		467: "surge-crucible",
	}
	w.SetModel(model)
	crucible := placeTestBuilding(t, w, 10, 10, 467, 1, 0)
	crucible.Build.AddItem(siliconItemID, 9)
	crucible.Build.AddLiquid(slagLiquidID, 200)
	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 16, 422, 1, 0)
	w.powerStorageState[int32(18*model.Width+10)] = 4000
	linkPowerNode(t, w, 10, 16, protocol.Point2{X: 0, Y: -6}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)
	if got := crucible.Build.ItemAmount(surgeAlloyItemID); got != 0 {
		t.Fatalf("expected unheated surge crucible to stay idle, amount=%d", got)
	}

	placeTestBuilding(t, w, 7, 10, 465, 1, 0)
	west := placeTestBuilding(t, w, 4, 10, 462, 1, 0)
	redirectorNorth := placeTestBuilding(t, w, 7, 7, 462, 1, 1)
	east := placeTestBuilding(t, w, 13, 10, 462, 1, 2)
	north := placeTestBuilding(t, w, 10, 7, 462, 1, 1)
	south := placeTestBuilding(t, w, 10, 13, 462, 1, 3)
	west.Build.AddLiquid(slagLiquidID, 120)
	redirectorNorth.Build.AddLiquid(slagLiquidID, 120)
	east.Build.AddLiquid(slagLiquidID, 120)
	north.Build.AddLiquid(slagLiquidID, 120)
	south.Build.AddLiquid(slagLiquidID, 120)

	stepForSeconds(w, 3)

	if got := crucible.Build.ItemAmount(surgeAlloyItemID); got <= 0 {
		t.Fatalf("expected heated surge crucible to craft surge alloy, amount=%d heat=%f", got, w.crafterReceivedHeatLocked(int32(10*model.Width+10), crucible))
	}
	if heat := w.crafterReceivedHeatLocked(int32(10*model.Width+10), crucible); heat < 40 {
		t.Fatalf("expected surge crucible to receive vanilla heat requirement, heat=%f", heat)
	}
}

func TestCyanogenSynthesizerRequiresHeatAndCraftsCyanogen(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		467: "cyanogen-synthesizer",
	}
	w.SetModel(model)
	synth := placeTestBuilding(t, w, 10, 10, 467, 1, 0)
	synth.Build.AddItem(graphiteItemID, 6)
	synth.Build.AddLiquid(arkyciteLiquidID, 80)
	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 16, 422, 1, 0)
	w.powerStorageState[int32(18*model.Width+10)] = 4000
	linkPowerNode(t, w, 10, 16, protocol.Point2{X: 0, Y: -6}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)
	if got := synth.Build.LiquidAmount(cyanogenLiquidID); got != 0 {
		t.Fatalf("expected unheated cyanogen synthesizer to stay idle, amount=%f", got)
	}

	west := placeTestBuilding(t, w, 7, 10, 462, 1, 0)
	east := placeTestBuilding(t, w, 13, 10, 462, 1, 2)
	north := placeTestBuilding(t, w, 10, 7, 462, 1, 1)
	west.Build.AddLiquid(slagLiquidID, 120)
	east.Build.AddLiquid(slagLiquidID, 120)
	north.Build.AddLiquid(slagLiquidID, 120)

	stepForSeconds(w, 3)

	if got := synth.Build.LiquidAmount(cyanogenLiquidID); got <= 0 {
		t.Fatalf("expected heated cyanogen synthesizer to craft cyanogen, amount=%f heat=%f", got, w.crafterReceivedHeatLocked(int32(10*model.Width+10), synth))
	}
	if heat := w.crafterReceivedHeatLocked(int32(10*model.Width+10), synth); heat < 20 {
		t.Fatalf("expected cyanogen synthesizer to receive vanilla heat requirement, heat=%f", heat)
	}
}

func TestPhaseSynthesizerRequiresHeatAndCraftsPhaseFabric(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		462: "slag-heater",
		467: "phase-synthesizer",
	}
	w.SetModel(model)
	synth := placeTestBuilding(t, w, 10, 10, 467, 1, 0)
	synth.Build.AddItem(thoriumItemID, 6)
	synth.Build.AddItem(sandItemID, 18)
	synth.Build.AddLiquid(ozoneLiquidID, 20)
	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 16, 422, 1, 0)
	w.powerStorageState[int32(18*model.Width+10)] = 4000
	linkPowerNode(t, w, 10, 16, protocol.Point2{X: 0, Y: -6}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)
	if got := synth.Build.ItemAmount(phaseFabricItemID); got != 0 {
		t.Fatalf("expected unheated phase synthesizer to stay idle, amount=%d", got)
	}

	west := placeTestBuilding(t, w, 7, 10, 462, 1, 0)
	east := placeTestBuilding(t, w, 13, 10, 462, 1, 2)
	north := placeTestBuilding(t, w, 10, 7, 462, 1, 1)
	south := placeTestBuilding(t, w, 10, 13, 462, 1, 3)
	west.Build.AddLiquid(slagLiquidID, 120)
	east.Build.AddLiquid(slagLiquidID, 120)
	north.Build.AddLiquid(slagLiquidID, 120)
	south.Build.AddLiquid(slagLiquidID, 120)

	stepForSeconds(w, 3)

	if got := synth.Build.ItemAmount(phaseFabricItemID); got <= 0 {
		t.Fatalf("expected heated phase synthesizer to craft phase fabric, amount=%d heat=%f", got, w.crafterReceivedHeatLocked(int32(10*model.Width+10), synth))
	}
	if heat := w.crafterReceivedHeatLocked(int32(10*model.Width+10), synth); heat < 32 {
		t.Fatalf("expected phase synthesizer to receive vanilla heat requirement, heat=%f", heat)
	}
}

func TestHeatReactorCraftsFissileMatterAndHeat(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		469: "heat-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 8, 8, 469, 1, 0)
	reactor.Build.AddItem(thoriumItemID, 6)
	reactor.Build.AddLiquid(nitrogenLiquidID, 10)

	stepForSeconds(w, 10.2)

	if got := reactor.Build.ItemAmount(fissileMatterItemID); got <= 0 {
		t.Fatalf("expected heat reactor to craft fissile matter, amount=%d", got)
	}
	if heat := w.heatStates[int32(8*model.Width+8)]; heat <= 0 {
		t.Fatalf("expected heat reactor to output heat while active, heat=%f", heat)
	}
}

func TestTurbineCondenserProducesPowerAndWaterOnSteam(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		31:  "rhyolite-vent",
		421: "battery",
		422: "power-node",
		470: "turbine-condenser",
	}
	w.SetModel(model)
	paintAreaFloor(t, w, 8, 8, 3, 31)

	condenser := placeTestBuilding(t, w, 8, 8, 470, 1, 0)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 0
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 3)

	if got := condenser.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected turbine condenser to output water on steam, amount=%f", got)
	}
	if st := w.teamPowerStates[1]; st == nil || st.Stored <= 0 || st.Produced <= 0 {
		t.Fatalf("expected turbine condenser to produce and store power, state=%+v", st)
	}
}

func TestChemicalCombustionChamberConsumesLiquidsForPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		471: "chemical-combustion-chamber",
	}
	w.SetModel(model)

	chamber := placeTestBuilding(t, w, 8, 8, 471, 1, 0)
	chamber.Build.AddLiquid(ozoneLiquidID, 20)
	chamber.Build.AddLiquid(arkyciteLiquidID, 80)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 0
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if st := w.teamPowerStates[1]; st == nil || st.Stored <= 0 || st.Produced <= 0 {
		t.Fatalf("expected chemical combustion chamber to produce power, state=%+v", st)
	}
	if got := chamber.Build.LiquidAmount(ozoneLiquidID); got >= 20 {
		t.Fatalf("expected chemical combustion chamber to consume ozone, amount=%f", got)
	}
	if got := chamber.Build.LiquidAmount(arkyciteLiquidID); got >= 80 {
		t.Fatalf("expected chemical combustion chamber to consume arkycite, amount=%f", got)
	}
}

func TestPyrolysisGeneratorProducesPowerAndWater(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(18, 18)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		472: "pyrolysis-generator",
	}
	w.SetModel(model)

	gen := placeTestBuilding(t, w, 8, 8, 472, 1, 0)
	gen.Build.AddLiquid(slagLiquidID, 60)
	gen.Build.AddLiquid(arkyciteLiquidID, 100)
	placeTestBuilding(t, w, 8, 13, 421, 1, 0)
	placeTestBuilding(t, w, 8, 11, 422, 1, 0)
	w.powerStorageState[int32(13*model.Width+8)] = 0
	linkPowerNode(t, w, 8, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 2)

	if st := w.teamPowerStates[1]; st == nil || st.Stored <= 0 || st.Produced <= 0 {
		t.Fatalf("expected pyrolysis generator to produce power, state=%+v", st)
	}
	if got := gen.Build.LiquidAmount(waterLiquidID); got <= 0 {
		t.Fatalf("expected pyrolysis generator to output water, amount=%f", got)
	}
}

func TestFluxReactorRequiresHeatToProducePower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		473: "flux-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 473, 1, 0)
	reactor.Build.AddLiquid(cyanogenLiquidID, 30)
	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 15, 422, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5}, protocol.Point2{X: 0, Y: 3})

	w.Step(time.Second / 60)

	if st := w.teamPowerStates[1]; st != nil && st.Stored > 0 {
		t.Fatalf("expected flux reactor without heat to stay idle, state=%+v", st)
	}
	if got := reactor.Build.LiquidAmount(cyanogenLiquidID); got != 30 {
		t.Fatalf("expected flux reactor without heat to preserve coolant, amount=%f", got)
	}
}

func TestFluxReactorConsumesCyanogenAndProducesPowerWithHeat(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		465: "heat-redirector",
		473: "flux-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 473, 1, 0)
	reactor.Build.AddLiquid(cyanogenLiquidID, 30)
	placeTestBuilding(t, w, 6, 10, 465, 1, 0)
	redirectorPos := int32(10*model.Width + 6)
	w.heatStates[redirectorPos] = 150

	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 15, 422, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5}, protocol.Point2{X: 0, Y: 3})

	w.Step(time.Second / 60)

	if st := w.teamPowerStates[1]; st == nil || st.Stored <= 0 || st.Produced <= 0 {
		t.Fatalf("expected heated flux reactor to produce power, state=%+v", st)
	}
	if got := reactor.Build.LiquidAmount(cyanogenLiquidID); got >= 30 {
		t.Fatalf("expected heated flux reactor to consume cyanogen, amount=%f", got)
	}
}

func TestImpactReactorWarmupDoesNotJumpToFullImmediately(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		481: "impact-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 481, 1, 0)
	reactor.Build.AddItem(blastCompoundItemID, 2)
	reactor.Build.AddLiquid(cryofluidLiquidID, 80)

	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 15, 422, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5}, protocol.Point2{X: 0, Y: 3})

	batteryPos := int32(18*model.Width + 10)
	w.powerStorageState[batteryPos] = 4000

	w.Step(time.Second / 60)

	reactorPos := int32(10*model.Width + 10)
	st := w.powerGeneratorState[reactorPos]
	if st == nil {
		t.Fatal("expected impact reactor runtime state to exist")
	}
	if st.Warmup <= 0 || st.Warmup >= 0.01 {
		t.Fatalf("expected impact reactor warmup to start near zero like vanilla, warmup=%f", st.Warmup)
	}
	if st.FuelFrames <= 0 || st.FuelFrames >= 140 {
		t.Fatalf("expected impact reactor fuel timer to tick down after startup, fuel=%f", st.FuelFrames)
	}
}

func TestImpactReactorWarmupDecaysWithoutStartupPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		481: "impact-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 481, 1, 0)
	reactor.Build.AddItem(blastCompoundItemID, 1)
	reactor.Build.AddLiquid(cryofluidLiquidID, 80)

	reactorPos := int32(10*model.Width + 10)
	w.powerGeneratorState[reactorPos] = &powerGeneratorState{
		FuelFrames: 60,
		Warmup:     1,
	}

	w.Step(time.Second / 60)

	st := w.powerGeneratorState[reactorPos]
	if st == nil {
		t.Fatal("expected impact reactor runtime state to persist")
	}
	if st.Warmup >= 1 {
		t.Fatalf("expected impact reactor warmup to decay without startup power, warmup=%f", st.Warmup)
	}
}

func TestNeoplasiaReactorProducesPowerHeatAndNeoplasm(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		480: "neoplasia-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 480, 1, 0)
	reactor.Build.AddItem(phaseFabricItemID, 1)
	reactor.Build.AddLiquid(arkyciteLiquidID, 80)
	reactor.Build.AddLiquid(waterLiquidID, 10)

	placeTestBuilding(t, w, 10, 18, 421, 1, 0)
	placeTestBuilding(t, w, 10, 15, 422, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5}, protocol.Point2{X: 0, Y: 3})

	w.Step(time.Second / 60)

	if st := w.teamPowerStates[1]; st == nil || st.Produced <= 0 || st.Stored <= 0 {
		t.Fatalf("expected neoplasia reactor to produce power, state=%+v", st)
	}
	if heat := w.heatStates[int32(10*model.Width+10)]; heat <= 0 {
		t.Fatalf("expected neoplasia reactor to produce heat, heat=%f", heat)
	}
	if got := reactor.Build.LiquidAmount(neoplasmLiquidID); got <= 0 {
		t.Fatalf("expected neoplasia reactor to output neoplasm, amount=%f", got)
	}
}

func TestNeoplasiaReactorExplodesWhenNeoplasmFills(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		480: "neoplasia-reactor",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 10, 10, 480, 1, 0)
	reactor.Build.AddItem(phaseFabricItemID, 1)
	reactor.Build.AddLiquid(arkyciteLiquidID, 80)
	reactor.Build.AddLiquid(waterLiquidID, 10)
	reactor.Build.AddLiquid(neoplasmLiquidID, 79.9)

	w.Step(time.Second / 60)

	if reactor.Block != 0 || reactor.Build != nil {
		t.Fatalf("expected neoplasia reactor to be destroyed on full neoplasm, block=%d build=%v", reactor.Block, reactor.Build)
	}
}

func TestIncineratorBurnsItemsWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		421: "battery",
		422: "power-node",
		459: "incinerator",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 6, 257, 1, 0)
	placeTestBuilding(t, w, 6, 10, 421, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	placeTestBuilding(t, w, 6, 6, 459, 1, 0)
	w.powerStorageState[int32(10*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 8, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	incPos := int32(6*model.Width + 6)
	srcPos := int32(6*model.Width + 4)
	if heat := w.incineratorStates[incPos]; heat <= 0.5 {
		t.Fatalf("expected powered incinerator to heat up, heat=%f", heat)
	}
	if !w.tryInsertItemLocked(srcPos, incPos, copperItemID, 0) {
		t.Fatalf("expected hot incinerator to accept and burn item")
	}
	if got := totalBuildingItems(w.Model().Tiles[incPos].Build); got != 0 {
		t.Fatalf("expected incinerator to keep no item inventory, total=%d", got)
	}
}

func TestIncineratorBurnsLiquidsWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		500: "conduit",
		421: "battery",
		422: "power-node",
		459: "incinerator",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 6, 500, 1, 0)
	placeTestBuilding(t, w, 6, 10, 421, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	placeTestBuilding(t, w, 6, 6, 459, 1, 0)
	w.powerStorageState[int32(10*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 8, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 2})

	stepForSeconds(w, 1)

	incPos := int32(6*model.Width + 6)
	srcPos := int32(6*model.Width + 4)
	moved := w.tryMoveLiquidLocked(srcPos, incPos, waterLiquidID, 5, 0)
	if moved <= 0 {
		t.Fatalf("expected hot incinerator to accept and burn liquid, moved=%f", moved)
	}
	if got := totalBuildingLiquids(w.Model().Tiles[incPos].Build); got != 0 {
		t.Fatalf("expected incinerator to keep no liquid inventory, total=%f", got)
	}
}

func TestPowerDiodeTransfersBatteryChargeOneWay(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "diode",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 6, 421, 1, 0)
	placeTestBuilding(t, w, 5, 6, 422, 1, 0)
	placeTestBuilding(t, w, 6, 6, 421, 1, 0)

	backPos := int32(6*model.Width + 4)
	frontPos := int32(6*model.Width + 6)
	w.powerStorageState[backPos] = 3000
	w.powerStorageState[frontPos] = 0

	w.Step(time.Second / 60)

	if got := w.powerStorageState[frontPos]; got <= 0 {
		t.Fatalf("expected diode to move power into front graph, stored=%f", got)
	}
	if got := w.powerStorageState[backPos]; got >= 3000 {
		t.Fatalf("expected diode to drain some back-graph power, stored=%f", got)
	}
}

func TestBeamNodeConnectsBatteryToPoweredConsumer(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		430: "laser-drill",
		474: "beam-node",
		2:   "ore-copper",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 14, 10, 3, 2)

	placeTestBuilding(t, w, 4, 10, 421, 1, 0)
	drill := placeTestBuilding(t, w, 14, 10, 430, 1, 0)
	placeTestBuilding(t, w, 10, 10, 474, 1, 0)
	w.powerStorageState[int32(10*model.Width+4)] = 3000

	stepForSeconds(w, 3)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected beam node to power laser drill from battery, items=%d", got)
	}
}

func TestBeamNodeBlockedByPlastaniumWall(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		430: "laser-drill",
		474: "beam-node",
		475: "plastanium-wall",
		2:   "ore-copper",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 14, 10, 3, 2)

	placeTestBuilding(t, w, 4, 10, 421, 1, 0)
	drill := placeTestBuilding(t, w, 14, 10, 430, 1, 0)
	placeTestBuilding(t, w, 10, 10, 474, 1, 0)
	placeTestBuilding(t, w, 12, 10, 475, 1, 0)
	w.powerStorageState[int32(10*model.Width+4)] = 3000

	stepForSeconds(w, 3)

	if got := totalBuildingItems(drill.Build); got != 0 {
		t.Fatalf("expected plastanium wall to block beam node power transfer, items=%d", got)
	}
}

func TestBeamTowerProvidesLargeBufferedStorage(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		420: "solar-panel-large",
		476: "beam-tower",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 6, 10, 420, 1, 0)
	placeTestBuilding(t, w, 12, 10, 476, 1, 0)

	stepForSeconds(w, 10)

	if st := w.teamPowerStates[1]; st == nil || st.Capacity < 40000 || st.Stored <= 0 {
		t.Fatalf("expected beam tower to contribute large power storage, state=%+v", st)
	}
}

func TestPowerNodeLargeAutoLinksOnPlacement(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		425: "power-node-large",
	}
	w.SetModel(model)

	batteryTile, err := model.TileAt(5, 10)
	if err != nil || batteryTile == nil {
		t.Fatalf("battery tile lookup failed: %v", err)
	}
	w.placeTileLocked(batteryTile, 1, 421, 0, nil, 0)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)

	nodePos := int32(10*model.Width + 12)
	batteryPos := int32(10*model.Width + 5)
	links := w.powerNodeLinks[nodePos]
	if len(links) == 0 {
		t.Fatal("expected power-node-large to autolink on placement")
	}
	found := false
	for _, link := range links {
		if link == batteryPos {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected power-node-large to autolink to nearby battery, links=%v", links)
	}
}

func TestPowerNodeLargeAutoLinksNearbyConsumerOnPlacement(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)
	_ = w.DrainEntityEvents()

	consumerTile, err := model.TileAt(6, 10)
	if err != nil || consumerTile == nil {
		t.Fatalf("consumer tile lookup failed: %v", err)
	}
	w.placeTileLocked(consumerTile, 1, 430, 0, nil, 0)

	nodePos := int32(10*model.Width + 12)
	consumerPos := int32(10*model.Width + 6)
	links := w.powerNodeLinks[nodePos]
	if len(links) == 0 {
		t.Fatal("expected power-node-large to autolink nearby consumer on placement")
	}
	found := false
	for _, link := range links {
		if link == consumerPos {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected power-node-large to link nearby consumer, links=%v", links)
	}

	cfg, ok := w.BuildingConfigPacked(protocol.PackPoint2(12, 10))
	if !ok {
		t.Fatal("expected autolinked power node config to be readable")
	}
	points, ok := cfg.([]protocol.Point2)
	if !ok {
		t.Fatalf("expected power node config as []Point2, got %T", cfg)
	}
	found = false
	for _, point := range points {
		if point.X == -6 && point.Y == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected power node config to include relative consumer link (-6,0), got %#v", points)
	}
}

func TestPowerNodeLargeAutoLinkAvoidsDuplicateConductingGraph(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		421: "battery",
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)

	batteryTile, err := model.TileAt(5, 10)
	if err != nil || batteryTile == nil {
		t.Fatalf("battery tile lookup failed: %v", err)
	}
	w.placeTileLocked(batteryTile, 1, 421, 0, nil, 0)

	drillTile, err := model.TileAt(7, 10)
	if err != nil || drillTile == nil {
		t.Fatalf("drill tile lookup failed: %v", err)
	}
	w.placeTileLocked(drillTile, 1, 430, 0, nil, 0)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)

	nodePos := int32(10*model.Width + 12)
	drillPos := int32(10*model.Width + 7)
	links := w.powerNodeLinks[nodePos]
	if len(links) != 1 {
		t.Fatalf("expected one autolink target for the shared conducting graph, got %v", links)
	}
	if links[0] != drillPos {
		t.Fatalf("expected nearer drill %d to represent the shared conducting graph, got %v", drillPos, links)
	}
}

func TestPowerNodeLargeAutoLinkBlockedByPlastaniumWall(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
		475: "plastanium-wall",
	}
	w.SetModel(model)

	consumerTile, err := model.TileAt(6, 10)
	if err != nil || consumerTile == nil {
		t.Fatalf("consumer tile lookup failed: %v", err)
	}
	w.placeTileLocked(consumerTile, 1, 430, 0, nil, 0)

	wallTile, err := model.TileAt(9, 10)
	if err != nil || wallTile == nil {
		t.Fatalf("wall tile lookup failed: %v", err)
	}
	w.placeTileLocked(wallTile, 1, 475, 0, nil, 0)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)

	nodePos := int32(10*model.Width + 12)
	if links := w.powerNodeLinks[nodePos]; len(links) != 0 {
		t.Fatalf("expected plastanium wall to block power-node autolink, links=%v", links)
	}
}

func TestAutoLinkedPowerNodeEmitsConfigEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)
	_ = w.DrainEntityEvents()

	consumerTile, err := model.TileAt(6, 10)
	if err != nil || consumerTile == nil {
		t.Fatalf("consumer tile lookup failed: %v", err)
	}
	w.placeTileLocked(consumerTile, 1, 430, 0, nil, 0)

	nodePacked := protocol.PackPoint2(12, 10)
	targetPacked := protocol.PackPoint2(6, 10)
	for _, ev := range w.DrainEntityEvents() {
		if ev.Kind != EntityEventBuildConfig || ev.BuildPos != nodePacked {
			continue
		}
		target, ok := ev.BuildConfig.(int32)
		if !ok {
			t.Fatalf("expected power node config event payload as packed int32, got %T", ev.BuildConfig)
		}
		if target != targetPacked {
			t.Fatalf("expected packed target=%d, got %d", targetPacked, target)
		}
		return
	}
	t.Fatal("expected autolinked power node to emit build_config event")
}

func TestAutoLinkedPowerNodeConfigEventComesAfterConstructed(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	w.SetModel(model)

	consumerTile, err := model.TileAt(6, 10)
	if err != nil || consumerTile == nil {
		t.Fatalf("consumer tile lookup failed: %v", err)
	}
	w.placeTileLocked(consumerTile, 1, 430, 0, nil, 0)
	_ = w.DrainEntityEvents()

	nodeTile, err := model.TileAt(12, 10)
	if err != nil || nodeTile == nil {
		t.Fatalf("node tile lookup failed: %v", err)
	}
	w.placeTileLocked(nodeTile, 1, 425, 0, nil, 0)

	nodePacked := protocol.PackPoint2(12, 10)
	constructIndex := -1
	configIndex := -1
	for i, ev := range w.DrainEntityEvents() {
		if ev.BuildPos != nodePacked {
			continue
		}
		if ev.Kind == EntityEventBuildConstructed && constructIndex < 0 {
			constructIndex = i
		}
		if ev.Kind == EntityEventBuildConfig && configIndex < 0 {
			configIndex = i
		}
	}
	if constructIndex < 0 {
		t.Fatal("expected power node constructed event")
	}
	if configIndex < 0 {
		t.Fatal("expected power node build_config event")
	}
	if configIndex <= constructIndex {
		t.Fatalf("expected build_config after build_constructed, got constructed=%d config=%d", constructIndex, configIndex)
	}
}

func TestBuildStepDoesNotDeadlockWhenPowerNodeAutoLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		421: "battery",
		425: "power-node-large",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)
	placeTestBuilding(t, w, 6, 10, 421, 1, 0)

	owner := int32(101)
	team := TeamID(1)
	w.UpdateBuilderState(owner, team, 9001, float32(12*8+4), float32(10*8+4), true, 220)
	w.ApplyBuildPlanSnapshotForOwner(owner, team, []BuildPlanOp{{
		X: 12, Y: 10, BlockID: 425,
	}})

	done := make(chan struct{})
	go func() {
		w.Step(200 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("step deadlocked while constructing autolinked power node")
	}
}

func TestBeamLinkTransfersPowerAcrossLongDistance(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(80, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		430: "laser-drill",
		477: "beam-link",
		2:   "ore-copper",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 70, 10, 3, 2)

	placeTestBuilding(t, w, 6, 10, 421, 1, 0)
	placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	placeTestBuilding(t, w, 10, 10, 477, 1, 0)
	placeTestBuilding(t, w, 60, 10, 477, 1, 0)
	placeTestBuilding(t, w, 66, 10, 422, 1, 0)
	drill := placeTestBuilding(t, w, 70, 10, 430, 1, 0)
	w.powerStorageState[int32(10*model.Width+6)] = 3000
	w.applyBuildingConfigLocked(int32(10*model.Width+10), []protocol.Point2{{X: 50, Y: 0}}, true)
	linkPowerNode(t, w, 8, 10, protocol.Point2{X: -2, Y: 0}, protocol.Point2{X: 2, Y: 0})
	linkPowerNode(t, w, 66, 10, protocol.Point2{X: -6, Y: 0}, protocol.Point2{X: 4, Y: 0})

	stepForSeconds(w, 3)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected beam link to transfer power across long range, items=%d", got)
	}
}

func TestPowerSourcePowersLaserDrill(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		430: "laser-drill",
		478: "power-source",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 10, 8, 3, 2)

	drill := placeTestBuilding(t, w, 10, 8, 430, 1, 0)
	placeTestBuilding(t, w, 10, 12, 478, 1, 0)
	linkPowerNode(t, w, 10, 12, protocol.Point2{X: 0, Y: -4})

	stepForSeconds(w, 3)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected power source to power laser drill, items=%d", got)
	}
}

func TestPowerVoidDrainsNetworkStorage(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		420: "solar-panel-large",
		421: "battery",
		422: "power-node",
		479: "power-void",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 8, 420, 1, 0)
	placeTestBuilding(t, w, 8, 8, 421, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	placeTestBuilding(t, w, 6, 11, 479, 1, 0)
	linkPowerNode(t, w, 6, 8, protocol.Point2{X: -2, Y: 0}, protocol.Point2{X: 2, Y: 0}, protocol.Point2{X: 0, Y: 3})

	stepForSeconds(w, 10)

	if st := w.teamPowerStates[1]; st == nil || st.Produced <= 0 {
		t.Fatalf("expected power void network to still produce power, state=%+v", st)
	} else {
		if st.Stored != 0 {
			t.Fatalf("expected power void to drain all stored power, state=%+v", st)
		}
		if st.Consumed <= 0 {
			t.Fatalf("expected power void to consume network power, state=%+v", st)
		}
	}
}

func TestItemLogisticsMovesThroughConveyorChainToReactor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 257, 1, 0)
	placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	placeTestBuilding(t, w, 4, 3, 315, 1, 0)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)

	for i := 0; i < 420; i++ {
		w.Step(time.Second / 60)
	}

	reactor, _ := w.Model().TileAt(4, 3)
	if reactor.Block != 0 || reactor.Build != nil {
		t.Fatalf("expected thorium reactor to explode after conveyor-fed thorium, got block=%d build=%v", reactor.Block, reactor.Build != nil)
	}
}

func TestSorterRoutesMatchingItemForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	east := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	north := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureSorter(int32(3*model.Width+2), 5)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected matching item to go forward through sorter")
	}
	if north.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected matching item not to route sideways, got north=%d", north.Build.ItemAmount(5))
	}
}

func TestJunctionPassesCrossedFlows(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		501: "junction",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 2, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 501, 1, 0)
	east := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	south := placeTestBuilding(t, w, 2, 4, 257, 1, 1)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureItemSource(int32(2*model.Width+2), 0)

	for i := 0; i < 240; i++ {
		w.Step(time.Second / 60)
	}

	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected west->east flow to pass through junction")
	}
	if south.Build.ItemAmount(0) == 0 {
		t.Fatalf("expected north->south flow to pass through junction")
	}
}

func TestPendingBuildAppliesConfigOnCompletion(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
	}
	w.SetModel(model)

	pos := int32(3*model.Width + 2)
	w.UpdateBuilderState(1, 1, 9001, float32(2*8+4), float32(3*8+4), true, 220)
	w.ApplyBuildPlansForOwner(1, 1, []BuildPlanOp{{
		X:       2,
		Y:       3,
		BlockID: 412,
		Config:  protocol.ItemRef{ItmID: 5},
	}})

	for i := 0; i < 30; i++ {
		w.Step(200 * time.Millisecond)
		tile, _ := w.Model().TileAt(2, 3)
		if tile.Build != nil {
			break
		}
	}

	if got := w.itemSourceCfg[pos]; got != 5 {
		t.Fatalf("expected pending build config to apply item-source item=5, got=%d", got)
	}
}

func TestRestoreSavedBridgeAndItemSourceConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		420: "bridge-conveyor",
	}

	w.SetModel(model)
	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	bridgeA := placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 4, 3, 420, 1, 0)
	out := placeTestBuilding(t, w, 5, 3, 257, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridgeA.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 2, Y: 0})

	w.SetModel(model)

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	if out.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected saved item-source + bridge config to restore and move items across bridge")
	}
}

func TestRestoreSavedSorterConfig(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}

	w.SetModel(model)
	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	sorter := placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	forward := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	side := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	sorter.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})

	w.SetModel(model)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if forward.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected saved sorter config to restore matching forward route")
	}
	if side.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected saved sorter config not to route matching item sideways, got=%d", side.Build.ItemAmount(5))
	}
}

func TestSorterIntConfigFallbackSyncPath(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		500: "sorter",
	}

	w.SetModel(model)
	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	sorter := placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	forward := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	side := placeTestBuilding(t, w, 2, 2, 257, 1, 3)

	w.ConfigureItemSource(int32(3*model.Width+1), 5)
	w.ConfigureBuilding(int32(3*model.Width+2), int32(5))

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if got, ok := w.BuildingConfigPacked(protocol.PackPoint2(2, 3)); !ok {
		t.Fatalf("expected sorter config to persist")
	} else if item, ok := got.(protocol.ItemRef); !ok || item.ItmID != 5 {
		t.Fatalf("expected sorter normalized config item=5, got=%T %#v", got, got)
	}
	if forward.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected int-based sorter config to route matching item forward")
	}
	if side.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected int-based sorter config not to route matching item sideways, got=%d", side.Build.ItemAmount(5))
	}
	if sorter.Build == nil || len(sorter.Build.Config) == 0 {
		t.Fatalf("expected sorter config bytes to be stored")
	}
}

func TestUnlinkedBridgeDoesNotAcceptItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	bridge := placeTestBuilding(t, w, 2, 3, 420, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	w.SetModel(model)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	if bridge.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected unlinked bridge not to accept items, got=%d", bridge.Build.ItemAmount(5))
	}
}

func TestLinkedBridgeRejectsInputFromLinkedSide(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 3, 3, 412, 1, 0)
	bridge := placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 5, 3, 420, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridge.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 3, Y: 0})
	w.SetModel(model)

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	bridge, _ = w.Model().TileAt(2, 3)
	if totalBuildingItems(bridge.Build) != 0 {
		t.Fatalf("expected linked bridge to reject input from its link-facing side, got=%d", totalBuildingItems(bridge.Build))
	}
}

func TestUnlinkedBridgeDoesNotDumpBackTowardIncomingBridge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	source := placeTestBuilding(t, w, 0, 3, 412, 1, 0)
	bridgeA := placeTestBuilding(t, w, 1, 3, 420, 1, 0)
	placeTestBuilding(t, w, 4, 3, 420, 1, 2)
	west := placeTestBuilding(t, w, 3, 3, 418, 1, 0)
	east := placeTestBuilding(t, w, 5, 3, 418, 1, 0)

	source.Build.Config = mustEncodeConfig(t, protocol.ItemRef{ItmID: 5})
	bridgeA.Build.Config = mustEncodeConfig(t, protocol.Point2{X: 3, Y: 0})
	w.SetModel(model)

	for i := 0; i < 420; i++ {
		w.Step(time.Second / 60)
	}

	if west.Build.ItemAmount(5) != 0 {
		t.Fatalf("expected unlinked bridge not to dump back toward incoming side, got west=%d", west.Build.ItemAmount(5))
	}
	if east.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected unlinked bridge to dump to a non-incoming side")
	}
}

func TestConveyorRuntimeTracksPerItemPositions(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 1, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 1, 418, 1, 0)

	conveyorPos := int32(2*model.Width + 2)
	behindPos := int32(2*model.Width + 1)
	northPos := int32(1*model.Width + 2)

	if !w.tryInsertItemLocked(behindPos, conveyorPos, 5, 0) {
		t.Fatalf("expected rear insert into conveyor to succeed")
	}
	state := w.conveyorStates[conveyorPos]
	if state == nil || state.Len != 1 || state.YS[0] != 0 {
		t.Fatalf("expected first item at conveyor origin, got state=%+v", state)
	}

	w.Step(500 * time.Millisecond)
	state = w.conveyorStates[conveyorPos]
	if state == nil || state.MinItem <= 0.7 {
		t.Fatalf("expected conveyor item to advance enough for side insert, got minitem=%v", state.MinItem)
	}

	if !w.tryInsertItemLocked(northPos, conveyorPos, 6, 0) {
		t.Fatalf("expected side insert into conveyor to succeed after spacing opens")
	}
	state = w.conveyorStates[conveyorPos]
	if state.Len != 2 {
		t.Fatalf("expected 2 runtime items, got=%d", state.Len)
	}
	if state.YS[state.LastInserted] != 0.5 {
		t.Fatalf("expected side inserted item at y=0.5, got=%v", state.YS[state.LastInserted])
	}
	if totalBuildingItems(w.Model().Tiles[conveyorPos].Build) != 2 {
		t.Fatalf("expected mirrored building inventory total=2, got=%d", totalBuildingItems(w.Model().Tiles[conveyorPos].Build))
	}
}

func TestConveyorRuntimePassesItemsForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 3, 2, 257, 1, 0)
	placeTestBuilding(t, w, 1, 2, 418, 1, 0)

	firstPos := int32(2*model.Width + 2)
	secondPos := int32(2*model.Width + 3)
	sourcePos := int32(2*model.Width + 1)

	if !w.tryInsertItemLocked(sourcePos, firstPos, 5, 0) {
		t.Fatalf("expected rear insert into first conveyor to succeed")
	}

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
	}

	first := w.conveyorStates[firstPos]
	second := w.conveyorStates[secondPos]
	if first != nil && first.Len != 0 {
		t.Fatalf("expected first conveyor to pass item onward, len=%d", first.Len)
	}
	if second == nil || second.Len == 0 {
		t.Fatalf("expected second conveyor runtime to receive item")
	}
	if totalBuildingItems(w.Model().Tiles[secondPos].Build) == 0 {
		t.Fatalf("expected mirrored inventory on second conveyor to contain item")
	}
}

func TestRouterImmediatelyPassesToConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 418, 1, 0)
	placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	router.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	conveyorPos := int32(3*model.Width + 3)
	state := w.conveyorStates[conveyorPos]
	if state == nil || state.Len == 0 {
		t.Fatalf("expected router to immediately pass item into conveyor on first frame")
	}
	if totalBuildingItems(router.Build) != 0 {
		t.Fatalf("expected router inventory to be empty after immediate pass, got=%d", totalBuildingItems(router.Build))
	}
}

func TestSorterRejectsInstantTransferThreeChain(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		500: "sorter",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 502, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	placeTestBuilding(t, w, 3, 3, 502, 1, 0)

	if w.tryInsertItemLocked(int32(3*model.Width+1), int32(3*model.Width+2), 5, 0) {
		t.Fatalf("expected sorter to reject instantTransfer three-chain forward path")
	}
}

func TestOverflowRejectsInstantTransferForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		500: "sorter",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 500, 1, 0)
	placeTestBuilding(t, w, 2, 3, 502, 1, 0)
	placeTestBuilding(t, w, 3, 3, 500, 1, 0)

	if w.tryInsertItemLocked(int32(3*model.Width+1), int32(3*model.Width+2), 5, 0) {
		t.Fatalf("expected overflow gate to reject instantTransfer forward chain")
	}
}

func TestSorterCanAcceptDoesNotFlipRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		500: "sorter",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 500, 1, 0)
	placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 4, 418, 1, 0)

	sorterPos := int32(3*model.Width + 2)
	sourcePos := int32(3*model.Width + 1)
	w.routerRotation[sorterPos] = 0

	if !w.canAcceptItemLocked(sourcePos, sorterPos, 5, 0) {
		t.Fatalf("expected sorter accept probe to succeed")
	}
	if w.routerRotation[sorterPos] != 0 {
		t.Fatalf("expected sorter accept probe not to flip rotation, got=%d", w.routerRotation[sorterPos])
	}
}

func TestOverflowCanAcceptDoesNotFlipRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		502: "overflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 502, 1, 0)
	placeTestBuilding(t, w, 2, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 4, 418, 1, 0)

	gatePos := int32(3*model.Width + 2)
	sourcePos := int32(3*model.Width + 1)
	w.routerRotation[gatePos] = 0

	if !w.canAcceptItemLocked(sourcePos, gatePos, 5, 0) {
		t.Fatalf("expected overflow accept probe to succeed")
	}
	if w.routerRotation[gatePos] != 0 {
		t.Fatalf("expected overflow accept probe not to flip rotation, got=%d", w.routerRotation[gatePos])
	}
}

func TestConsumeGeneratorItemCapacitiesMatchVanilla(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	model.BlockNames = map[int16]string{
		308: "combustion-generator",
		309: "steam-generator",
		310: "differential-generator",
		311: "rtg-generator",
	}
	w.SetModel(model)

	tests := []struct {
		name  string
		block int16
		want  int32
	}{
		{name: "combustion-generator", block: 308, want: 10},
		{name: "steam-generator", block: 309, want: 10},
		{name: "differential-generator", block: 310, want: 10},
		{name: "rtg-generator", block: 311, want: 10},
	}

	for _, tc := range tests {
		if got := w.itemCapacityForBlockLocked(&Tile{Block: BlockID(tc.block)}); got != tc.want {
			t.Fatalf("expected %s capacity=%d, got=%d", tc.name, tc.want, got)
		}
	}
}

func TestUnderflowRoutesIntoConsumeGeneratorBeforeCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		308: "combustion-generator",
		339: "core-shard",
		412: "item-source",
		503: "underflow-gate",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 5, 412, 1, 0)
	placeTestBuilding(t, w, 3, 5, 503, 1, 0)
	gen := placeTestBuilding(t, w, 3, 4, 308, 1, 0)
	core := placeTestBuilding(t, w, 5, 5, 339, 1, 0)

	sourcePos := int32(5*model.Width + 2)
	gatePos := int32(5*model.Width + 3)
	item := coalItemID

	if !w.tryInsertItemLocked(sourcePos, gatePos, item, 0) {
		t.Fatalf("expected underflow gate to route item into combustion generator")
	}
	if got := gen.Build.ItemAmount(item); got != 1 {
		t.Fatalf("expected combustion generator to receive item, got=%d", got)
	}
	if got := core.Build.ItemAmount(item); got != 0 {
		t.Fatalf("expected core inventory to remain untouched, got=%d", got)
	}
}

func TestArmoredConveyorRejectsSideInputFromNonConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		418: "router",
		259: "armored-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 259, 1, 0)
	placeTestBuilding(t, w, 2, 1, 418, 1, 0)

	armoredPos := int32(2*model.Width + 2)
	sourcePos := int32(1*model.Width + 2)
	if w.tryInsertItemLocked(sourcePos, armoredPos, 5, 0) {
		t.Fatalf("expected armored conveyor to reject side input from non-conveyor")
	}
}

func TestArmoredConveyorAcceptsSideInputFromConveyor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		259: "armored-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 259, 1, 0)
	placeTestBuilding(t, w, 2, 1, 257, 1, 1)

	armoredPos := int32(2*model.Width + 2)
	sourcePos := int32(1*model.Width + 2)
	if !w.tryInsertItemLocked(sourcePos, armoredPos, 5, 0) {
		t.Fatalf("expected armored conveyor to accept side input from conveyor")
	}
}

func TestUnlinkedBridgeDumpRotatesTargets(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 2, 420, 1, 0)
	placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	placeTestBuilding(t, w, 2, 3, 418, 1, 0)

	bridgePos := int32(2*model.Width + 2)
	first, ok := w.bridgeDumpTargetLocked(bridgePos, bridge, 5)
	if !ok {
		t.Fatalf("expected first dump target for unlinked bridge")
	}
	second, ok := w.bridgeDumpTargetLocked(bridgePos, bridge, 5)
	if !ok {
		t.Fatalf("expected second dump target for unlinked bridge")
	}
	if first == second {
		t.Fatalf("expected dump rotation to advance to a different target, got same=%d", first)
	}
}

func TestItemSourceDumpsOnePathPerUpdate(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		412: "item-source",
		418: "router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 412, 1, 0)
	east := placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	south := placeTestBuilding(t, w, 2, 3, 418, 1, 0)
	w.ConfigureItemSource(int32(2*model.Width+2), 5)

	w.Step(time.Second / 60)

	total := totalBuildingItems(east.Build) + totalBuildingItems(south.Build)
	if total != 1 {
		t.Fatalf("expected item source to dump through exactly one path on first update, got total=%d", total)
	}
}

func TestUnlinkedBridgeDumpsOnFirstFrame(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 2, 420, 1, 0)
	out := placeTestBuilding(t, w, 3, 2, 418, 1, 0)
	bridge.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	if totalBuildingItems(out.Build) != 1 {
		t.Fatalf("expected unlinked bridge to dump on first frame, got=%d", totalBuildingItems(out.Build))
	}
	if totalBuildingItems(bridge.Build) != 0 {
		t.Fatalf("expected bridge inventory empty after first-frame dump, got=%d", totalBuildingItems(bridge.Build))
	}
}

func TestDistributorUsesMultiBlockProximity(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		600: "distributor",
	}
	w.SetModel(model)

	distributor := placeTestBuilding(t, w, 3, 3, 600, 1, 0)
	placeTestBuilding(t, w, 2, 3, 257, 1, 0)
	placeTestBuilding(t, w, 2, 4, 257, 1, 0)
	placeTestBuilding(t, w, 5, 3, 257, 1, 0)
	placeTestBuilding(t, w, 5, 4, 257, 1, 0)

	distributor.Build.AddItem(5, 1)

	w.Step(time.Second / 60)

	moved := 0
	for _, pos := range []int32{
		int32(3*model.Width + 2),
		int32(4*model.Width + 2),
		int32(3*model.Width + 5),
		int32(4*model.Width + 5),
	} {
		if st := w.conveyorStates[pos]; st != nil && st.Len > 0 {
			moved++
		}
	}
	if moved != 1 {
		t.Fatalf("expected distributor to route exactly one item into adjacent edge conveyor, moved=%d", moved)
	}
	if totalBuildingItems(distributor.Build) != 0 {
		t.Fatalf("expected distributor inventory empty after routing, got=%d", totalBuildingItems(distributor.Build))
	}
}

func TestItemSourceFeedsThoriumReactorAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		412: "item-source",
	}
	w.SetModel(model)

	reactor := placeTestBuilding(t, w, 4, 4, 315, 1, 0)
	placeTestBuilding(t, w, 6, 4, 412, 1, 0)
	w.ConfigureItemSource(int32(4*model.Width+6), 5)

	w.Step(time.Second / 60)

	if reactor.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected item source to feed thorium reactor through multiblock edge")
	}
}

func TestItemSourceFeedsCoreAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 1)

	w.Step(time.Second / 60)

	if core.Build.ItemAmount(1) == 0 {
		t.Fatalf("expected item source to feed core through multiblock edge")
	}
}

func TestConveyorFeedsCoreAcrossMultiBlockEdge(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(14, 14)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	conveyor := placeTestBuilding(t, w, 8, 5, 257, 1, 2)
	conveyor.Build.AddItem(0, 1)

	for i := 0; i < 120; i++ {
		w.Step(time.Second / 60)
		if core.Build.ItemAmount(0) > 0 {
			break
		}
	}

	if core.Build.ItemAmount(0) == 0 {
		t.Fatalf("expected conveyor to feed core through multiblock edge")
	}
	if conveyor.Build.ItemAmount(0) != 0 {
		t.Fatalf("expected conveyor inventory drained into core, got=%d", conveyor.Build.ItemAmount(0))
	}
}

func TestTeamCoreItemSnapshotsUseRealCoreInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	core.Build.AddItem(0, 7)
	core.Build.AddItem(5, 3)
	w.teamItems[1] = map[ItemID]int32{
		0: 2900,
		1: 1900,
	}

	snapshots := w.TeamCoreItemSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 core snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Team != 1 {
		t.Fatalf("expected team 1 snapshot, got team %d", snapshots[0].Team)
	}
	if len(snapshots[0].Items) != 2 {
		t.Fatalf("expected 2 real core items, got %d", len(snapshots[0].Items))
	}
	if snapshots[0].Items[0].Item != 0 || snapshots[0].Items[0].Amount != 7 {
		t.Fatalf("expected copper amount 7, got item=%d amount=%d", snapshots[0].Items[0].Item, snapshots[0].Items[0].Amount)
	}
	if snapshots[0].Items[1].Item != 5 || snapshots[0].Items[1].Amount != 3 {
		t.Fatalf("expected sand amount 3, got item=%d amount=%d", snapshots[0].Items[1].Item, snapshots[0].Items[1].Amount)
	}
}

func TestTeamCoreItemSnapshotsDoNotFallbackToTeamInventoryWhenCoreEmpty(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	w.teamItems[1] = map[ItemID]int32{
		0: 120,
		4: 45,
	}

	snapshots := w.TeamCoreItemSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 core snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Team != 1 {
		t.Fatalf("expected team 1 snapshot, got team %d", snapshots[0].Team)
	}
	if len(snapshots[0].Items) != 0 {
		t.Fatalf("expected empty real core inventory, got %d items", len(snapshots[0].Items))
	}
}

func TestFillItemsTeamFillsRealCoreInventoryOnStep(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	rules := w.GetRulesManager().Get()
	rules.setTeamRule(1, TeamRule{FillItems: true})

	pos := int32(core.Y*model.Width + core.X)
	capacity := w.itemCapacityAtLocked(pos)
	if capacity <= 0 {
		t.Fatalf("expected positive shared core capacity, got %d", capacity)
	}
	if got := core.Build.ItemAmount(copperItemID); got != 0 {
		t.Fatalf("expected empty core before fill step, got copper=%d", got)
	}

	w.Step(time.Second / 60)

	snapshots := w.TeamCoreItemSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 core snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Team != 1 {
		t.Fatalf("expected team 1 snapshot, got team %d", snapshots[0].Team)
	}
	if got := core.Build.ItemAmount(copperItemID); got != capacity {
		t.Fatalf("expected fillItems step to fill copper to %d, got %d", capacity, got)
	}
	if got := core.Build.ItemAmount(titaniumItemID); got != capacity {
		t.Fatalf("expected fillItems step to fill titanium to %d, got %d", capacity, got)
	}
	itemAmounts := make(map[ItemID]int32, len(snapshots[0].Items))
	for _, stack := range snapshots[0].Items {
		itemAmounts[stack.Item] = stack.Amount
	}
	if got := itemAmounts[copperItemID]; got != capacity {
		t.Fatalf("expected core snapshot copper=%d after fill step, got %d", capacity, got)
	}
	if got := itemAmounts[titaniumItemID]; got != capacity {
		t.Fatalf("expected core snapshot titanium=%d after fill step, got %d", capacity, got)
	}
}

func TestTeamItemsReflectRealCoreInput(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 0)

	w.Step(time.Second / 60)

	items := w.TeamItems(1)
	if items[0] == 0 {
		t.Fatalf("expected team item view to reflect real core input, got copper=%d", items[0])
	}
}

func TestCoreFeedEmitsTeamItemEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		412: "item-source",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	placeTestBuilding(t, w, 8, 5, 412, 1, 0)
	w.ConfigureItemSource(int32(5*model.Width+8), 0)

	w.Step(time.Second / 60)

	evs := w.DrainEntityEvents()
	for _, ev := range evs {
		if ev.Kind == EntityEventTeamItems && ev.BuildTeam == 1 && ev.ItemID == 0 && ev.ItemAmount > 0 {
			return
		}
	}
	t.Fatalf("expected feeding a core to emit a team item sync event")
}

func TestSiliconSmelterDumpsOutputIntoCoreAndEmitsTeamItemEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
		421: "battery",
		422: "power-node",
		451: "silicon-smelter",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 10, 6, 343, 1, 0)
	smelter := placeTestBuilding(t, w, 6, 6, 451, 1, 0)
	placeTestBuilding(t, w, 6, 11, 421, 1, 0)
	placeTestBuilding(t, w, 6, 9, 422, 1, 0)
	w.powerStorageState[int32(11*model.Width+6)] = 4000
	linkPowerNode(t, w, 6, 9, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 2})

	smelter.Build.AddItem(coalItemID, 2)
	smelter.Build.AddItem(sandItemID, 4)

	foundTeamItemEvent := false
	foundBlockItemSync := false
	for i := 0; i < 240; i++ {
		w.Step(time.Second / 60)
		for _, ev := range w.DrainEntityEvents() {
			if ev.Kind == EntityEventTeamItems && ev.BuildTeam == 1 && ev.ItemID == siliconItemID && ev.ItemAmount > 0 {
				foundTeamItemEvent = true
			}
			if ev.Kind == EntityEventBlockItemSync && ev.BuildPos == protocol.PackPoint2(6, 6) {
				foundBlockItemSync = true
			}
		}
		if core.Build.ItemAmount(siliconItemID) > 0 {
			break
		}
	}

	if got := core.Build.ItemAmount(siliconItemID); got <= 0 {
		t.Fatalf("expected silicon smelter output to reach adjacent core, got=%d", got)
	}
	if got := smelter.Build.ItemAmount(siliconItemID); got != 0 {
		t.Fatalf("expected smelter output buffer to dump into the core, got=%d", got)
	}
	if got := w.TeamItems(1)[siliconItemID]; got <= 0 {
		t.Fatalf("expected team item view to reflect dumped silicon, got=%d", got)
	}
	if !foundTeamItemEvent {
		t.Fatalf("expected crafter dumping into core to emit a team item sync event")
	}
	if !foundBlockItemSync {
		t.Fatalf("expected crafter inventory changes to emit a block item sync event")
	}
}

func TestFindNearestFriendlyCoreLockedChoosesNearestCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 4, 339, 1, 0)
	placeTestBuilding(t, w, 18, 4, 339, 1, 0)

	src := RawEntity{Team: 1, X: float32(18*8 + 4), Y: float32(6*8 + 4)}
	target, ok := w.findNearestFriendlyCoreLocked(src)
	if !ok {
		t.Fatalf("expected nearest friendly core")
	}
	if target.BuildPos != int32(4*model.Width+18) {
		t.Fatalf("expected right core to be chosen, got pos=%d", target.BuildPos)
	}
	if !w.entityNearCoreLocked(RawEntity{Team: 1, X: float32(18*8 + 4), Y: float32(4*8 + 4)}, 80) {
		t.Fatalf("expected entityNearCoreLocked to detect nearby core via cached core positions")
	}
}

func TestFindNearestEnemyCoreLockedChoosesNearestEnemyCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 4, 339, 2, 0)
	placeTestBuilding(t, w, 18, 4, 339, 3, 0)

	src := RawEntity{Team: 1, X: float32(17*8 + 4), Y: float32(5*8 + 4)}
	target, ok := w.findNearestEnemyCoreLocked(src)
	if !ok {
		t.Fatalf("expected nearest enemy core")
	}
	if target.BuildPos != int32(4*model.Width+18) {
		t.Fatalf("expected nearest enemy core at right side, got pos=%d", target.BuildPos)
	}
}

func TestActiveTileIndexesTrackPowerAndLogisticsCategories(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		339: "core-shard",
		422: "power-node",
		480: "diode",
		481: "power-void",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 257, 1, 0)
	placeTestBuilding(t, w, 4, 4, 422, 1, 0)
	placeTestBuilding(t, w, 6, 4, 480, 1, 0)
	placeTestBuilding(t, w, 8, 4, 481, 1, 0)
	placeTestBuilding(t, w, 10, 10, 339, 1, 0)

	if got := len(w.itemLogisticsTilePositions); got != 1 {
		t.Fatalf("expected 1 item logistics tile, got %d", got)
	}
	if got := len(w.powerTilePositions); got != 3 {
		t.Fatalf("expected 3 power tiles, got %d", got)
	}
	if got := len(w.powerDiodeTilePositions); got != 1 {
		t.Fatalf("expected 1 power diode tile, got %d", got)
	}
	if got := len(w.powerVoidTilePositions); got != 1 {
		t.Fatalf("expected 1 power void tile, got %d", got)
	}
	if got := len(w.teamPowerTiles[1]); got != 3 {
		t.Fatalf("expected team power tile list size 3, got %d", got)
	}
	if got := len(w.teamPowerNodeTiles[1]); got != 1 {
		t.Fatalf("expected team power node tile list size 1, got %d", got)
	}
	if got := len(w.teamCoreTiles[1]); got != 1 {
		t.Fatalf("expected team core tile list size 1, got %d", got)
	}
}

func TestActiveTileIndexesTrackProductionCategories(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		100: "ground-factory",
		429: "mechanical-drill",
		440: "mechanical-pump",
		442: "water-extractor",
		451: "silicon-smelter",
		453: "separator",
		459: "incinerator",
		465: "heat-redirector",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 2, 451, 1, 0)
	placeTestBuilding(t, w, 4, 2, 453, 1, 0)
	placeTestBuilding(t, w, 6, 2, 429, 1, 0)
	placeTestBuilding(t, w, 8, 2, 440, 1, 0)
	placeTestBuilding(t, w, 10, 2, 442, 1, 0)
	placeTestBuilding(t, w, 12, 2, 459, 1, 0)
	placeTestBuilding(t, w, 14, 2, 100, 1, 0)
	placeTestBuilding(t, w, 16, 2, 465, 1, 0)

	if got := len(w.crafterTilePositions); got != 2 {
		t.Fatalf("expected 2 crafter tiles, got %d", got)
	}
	if got := len(w.drillTilePositions); got != 1 {
		t.Fatalf("expected 1 drill tile, got %d", got)
	}
	if got := len(w.pumpTilePositions); got != 2 {
		t.Fatalf("expected 2 pump tiles, got %d", got)
	}
	if got := len(w.incineratorTilePositions); got != 1 {
		t.Fatalf("expected 1 incinerator tile, got %d", got)
	}
	if got := len(w.factoryTilePositions); got != 1 {
		t.Fatalf("expected 1 factory tile, got %d", got)
	}
	if got := len(w.heatConductorTilePositions); got != 1 {
		t.Fatalf("expected 1 heat conductor tile, got %d", got)
	}
}

func TestLinkedStorageMergesIntoCoreInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 12)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		431: "vault",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	store := placeTestBuilding(t, w, 8, 5, 431, 1, 0)
	store.Build.AddItem(0, 4)
	w.rebuildBlockOccupancyLocked()

	if got := w.TeamItems(1)[0]; got != 4 {
		t.Fatalf("expected linked vault items to merge into core inventory view, got %d", got)
	}
	if core.Build.ItemAmount(0) != 4 {
		t.Fatalf("expected primary core to hold merged linked inventory, got %d", core.Build.ItemAmount(0))
	}
	if store.Build.ItemAmount(0) != 0 {
		t.Fatalf("expected linked vault local inventory cleared after merge, got %d", store.Build.ItemAmount(0))
	}
	positions := w.TeamItemSyncPositions(1)
	if len(positions) != 2 {
		t.Fatalf("expected core and linked vault sync positions, got %d", len(positions))
	}
}

func TestBuildCostConsumesCoreInventoryWhenCorePresent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	core.Build.AddItem(0, 40)

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	w.Step(3 * time.Second)

	if core.Build.ItemAmount(0) != 5 {
		t.Fatalf("expected duo build to consume real core copper, got %d", core.Build.ItemAmount(0))
	}
}

func TestFillItemsTeamBuildUsesRealCoreInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		343: "core-citadel",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 343, 1, 0)
	rules := w.GetRulesManager().Get()
	rules.setTeamRule(1, TeamRule{FillItems: true})
	pos := int32(core.Y*model.Width + core.X)
	capacity := w.itemCapacityAtLocked(pos)
	if capacity <= 0 {
		t.Fatalf("expected positive shared core capacity, got %d", capacity)
	}

	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	stepForSeconds(w, 3)

	tile, err := w.Model().TileAt(2, 2)
	if err != nil || tile == nil {
		t.Fatalf("built tile lookup failed: %v", err)
	}
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected duo to be fully constructed from fillItems core, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if got := core.Build.ItemAmount(copperItemID); got != capacity {
		t.Fatalf("expected fillItems core copper to refill back to %d after build, got %d", capacity, got)
	}
}

func TestSandboxModeBuildIgnoresResourceCost(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.Tags = map[string]string{
		"mode": "sandbox",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	stepForSeconds(w, 3)

	tile, err := w.Model().TileAt(2, 2)
	if err != nil || tile == nil {
		t.Fatalf("built tile lookup failed: %v", err)
	}
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected sandbox build to finish without materials, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if got := core.Build.ItemAmount(copperItemID); got != 0 {
		t.Fatalf("expected sandbox build to consume no core copper, got %d", got)
	}
}

func TestAttackModeWaveTeamBuildIgnoresResourceCost(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.Tags = map[string]string{
		"mode": "attack",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 5, 5, 339, 2, 0)
	w.ApplyBuildPlans(TeamID(2), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	stepForSeconds(w, 3)

	tile, err := w.Model().TileAt(2, 2)
	if err != nil || tile == nil {
		t.Fatalf("built tile lookup failed: %v", err)
	}
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected attack wave team build to finish without materials, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if got := core.Build.ItemAmount(copperItemID); got != 0 {
		t.Fatalf("expected attack wave team build to consume no core copper, got %d", got)
	}
}

func TestSurvivalModeBuildStillWaitsForResources(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.Tags = map[string]string{
		"mode": "survival",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	pos := int32(2 + 2*model.Width)
	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	stepForSeconds(w, 3)

	tile, err := w.Model().TileAt(2, 2)
	if err != nil || tile == nil {
		t.Fatalf("built tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected survival build without copper to stay pending, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if _, ok := w.pendingBuilds[pos]; !ok {
		t.Fatalf("expected survival build to remain queued while missing materials")
	}
}

func TestPvpModeBuildStillWaitsForResources(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.Tags = map[string]string{
		"mode": "pvp",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 339, 1, 0)
	pos := int32(2 + 2*model.Width)
	w.ApplyBuildPlans(TeamID(1), []BuildPlanOp{{
		X: 2, Y: 2, BlockID: 45,
	}})
	stepForSeconds(w, 3)

	tile, err := w.Model().TileAt(2, 2)
	if err != nil || tile == nil {
		t.Fatalf("built tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected pvp build without copper to stay pending, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if _, ok := w.pendingBuilds[pos]; !ok {
		t.Fatalf("expected pvp build to remain queued while missing materials")
	}
}

func TestPhaseConveyorTransfersLinkedItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 8)
	model.BlockNames = map[int16]string{
		418: "router",
		421: "phase-conveyor",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 2, 3, 421, 1, 0)
	dst := placeTestBuilding(t, w, 6, 3, 421, 1, 0)
	out := placeTestBuilding(t, w, 7, 3, 418, 1, 0)
	w.ConfigureBuilding(int32(3*model.Width+2), protocol.Point2{X: 4, Y: 0})

	src.Build.AddItem(5, 1)

	for i := 0; i < 6; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) != 1 {
		t.Fatalf("expected linked phase conveyor to transfer item, got=%d", totalBuildingItems(out.Build))
	}
	if totalBuildingItems(dst.Build) != 0 {
		t.Fatalf("expected target phase conveyor inventory drained after transfer, got=%d", totalBuildingItems(dst.Build))
	}
}

func TestConfigureBuildingPackedBridgeUsesOfficialAbsolutePos(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 8)
	model.BlockNames = map[int16]string{
		420: "bridge-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 2, 3, 420, 1, 0)
	placeTestBuilding(t, w, 6, 3, 420, 1, 0)

	srcPacked := protocol.PackPoint2(2, 3)
	dstPacked := protocol.PackPoint2(6, 3)

	w.ConfigureBuildingPacked(srcPacked, dstPacked)

	cfg, ok := w.BuildingConfigPacked(srcPacked)
	if !ok {
		t.Fatalf("expected packed building config to be readable")
	}
	point, ok := cfg.(protocol.Point2)
	if !ok {
		t.Fatalf("expected normalized bridge config to be Point2, got %T", cfg)
	}
	if point.X != 4 || point.Y != 0 {
		t.Fatalf("expected relative bridge config (4,0), got (%d,%d)", point.X, point.Y)
	}
}

func TestRotateBuildingPackedUpdatesTileRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 3, 4, 425, 1, 0)
	pos := protocol.PackPoint2(3, 4)

	res, ok := w.RotateBuildingPacked(pos, true)
	if !ok {
		t.Fatal("expected rotate call to succeed")
	}
	if res.BlockID != 425 || res.Rotation != 1 || res.Team != 1 {
		t.Fatalf("unexpected rotate result: %+v", res)
	}
	if res.EffectX != 28 || res.EffectY != 36 {
		t.Fatalf("expected effect position (28,36), got (%f,%f)", res.EffectX, res.EffectY)
	}
	if res.EffectRot != 2 {
		t.Fatalf("expected rotate effect payload size 2, got %f", res.EffectRot)
	}

	tile, err := model.TileAt(3, 4)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("tile lookup failed after rotate: %v", err)
	}
	if tile.Rotation != 1 || tile.Build.Rotation != 1 {
		t.Fatalf("expected tile/build rotation=1, got tile=%d build=%d", tile.Rotation, tile.Build.Rotation)
	}
}

func TestUnloaderMovesConfiguredItemBetweenAdjacentBlocks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		430: "unloader",
		431: "vault",
	}
	w.SetModel(model)

	store := placeTestBuilding(t, w, 2, 3, 431, 1, 0)
	placeTestBuilding(t, w, 3, 3, 430, 1, 0)
	placeTestBuilding(t, w, 4, 3, 257, 1, 0)
	store.Build.AddItem(5, 3)
	w.ConfigureUnloader(int32(3*model.Width+3), 5)

	for i := 0; i < 6; i++ {
		w.Step(time.Second / 60)
	}

	if st := w.conveyorStates[int32(3*model.Width+4)]; st == nil || st.Len == 0 {
		t.Fatalf("expected unloader to move configured item into conveyor")
	}
	if store.Build.ItemAmount(5) >= 3 {
		t.Fatalf("expected unloader to remove item from source storage")
	}
}

func TestUnloaderPrefersOnlyGiveSourceOverFactoryInputBuffer(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		419: "item-void",
		430: "unloader",
		451: "silicon-smelter",
		100: "ground-factory",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 3, 4, 419, 1, 0)
	placeTestBuilding(t, w, 3, 3, 430, 1, 0)
	smelter := placeTestBuilding(t, w, 4, 3, 451, 1, 0)
	factory := placeTestBuilding(t, w, 3, 1, 100, 1, 0)

	smelter.Build.AddItem(siliconItemID, 1)
	factory.Build.AddItem(siliconItemID, 5)
	unloaderPos := int32(3*model.Width + 3)
	w.ConfigureUnloader(unloaderPos, siliconItemID)

	neighbors := w.dumpProximityLocked(unloaderPos)
	if len(neighbors) != 3 {
		t.Fatalf("expected unloader to see 3 neighbors, got %d", len(neighbors))
	}
	item, ok := w.unloaderTargetItemLocked(unloaderPos, neighbors)
	if !ok || item != siliconItemID {
		t.Fatalf("expected unloader target item %d, got item=%d ok=%v", siliconItemID, item, ok)
	}
	fromPos, toPos, ok := w.unloaderTransferPairPreviewLocked(unloaderPos, neighbors, siliconItemID)
	if !ok {
		t.Fatalf("expected unloader transfer pair for silicon, neighbors=%v", neighbors)
	}
	if fromPos != int32(3*model.Width+4) {
		t.Fatalf("expected silicon-smelter to be chosen as source, got fromPos=%d", fromPos)
	}
	if toPos != int32(4*model.Width+3) {
		t.Fatalf("expected north item-void to be chosen as target, got toPos=%d", toPos)
	}

	w.transportAccum[unloaderPos] = float32(60.0 / 11.0)
	unloaderTile, err := w.Model().TileAt(3, 3)
	if err != nil || unloaderTile == nil {
		t.Fatalf("unloader tile lookup failed: %v", err)
	}
	w.stepUnloaderLocked(unloaderPos, unloaderTile, 0)
	foundBlockItemSync := false
	for _, ev := range w.DrainEntityEvents() {
		if ev.Kind == EntityEventBlockItemSync && ev.BuildPos == protocol.PackPoint2(4, 3) {
			foundBlockItemSync = true
		}
	}

	if got := smelter.Build.ItemAmount(siliconItemID); got != 0 {
		t.Fatalf("expected unloader to prefer the only-give silicon-smelter output first, smelter silicon=%d", got)
	}
	if got := factory.Build.ItemAmount(siliconItemID); got != 5 {
		t.Fatalf("expected unit factory silicon buffer to stay intact, got=%d", got)
	}
	if !foundBlockItemSync {
		t.Fatalf("expected unloader source inventory change to emit a block item sync event")
	}
}

func TestMassDriverTransfersBatchToLinkedTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 12)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		432: "mass-driver",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 4, 4, 432, 1, 0)
	dst := placeTestBuilding(t, w, 14, 4, 432, 1, 0)
	placeTestBuilding(t, w, 4, 8, 421, 1, 0)
	placeTestBuilding(t, w, 14, 8, 421, 1, 0)
	placeTestBuilding(t, w, 4, 6, 422, 1, 0)
	placeTestBuilding(t, w, 14, 6, 422, 1, 0)
	w.powerStorageState[int32(8*model.Width+4)] = 4000
	w.powerStorageState[int32(8*model.Width+14)] = 4000
	linkPowerNode(t, w, 4, 6, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 2})
	linkPowerNode(t, w, 14, 6, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 2})
	w.ConfigureBuilding(int32(4*model.Width+4), protocol.Point2{X: 10, Y: 0})
	src.Build.AddItem(5, 20)

	for i := 0; i < 260; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(dst.Build) == 0 {
		t.Fatalf("expected mass driver to transfer batch to linked target")
	}
	if totalBuildingItems(src.Build) >= 20 {
		t.Fatalf("expected mass driver source inventory to decrease")
	}
}

func TestDuctMovesItemForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		412: "item-source",
		440: "duct",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 1, 3, 412, 1, 0)
	placeTestBuilding(t, w, 2, 3, 440, 1, 0)
	placeTestBuilding(t, w, 3, 3, 440, 1, 0)
	out := placeTestBuilding(t, w, 4, 3, 257, 1, 0)
	w.ConfigureItemSource(int32(3*model.Width+1), 5)

	for i := 0; i < 40; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct chain to move item into output")
	}
}

func TestDuctBridgeTransfersBetweenLinks(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		440: "duct",
		445: "duct-bridge",
	}
	w.SetModel(model)

	in := placeTestBuilding(t, w, 1, 3, 440, 1, 0)
	placeTestBuilding(t, w, 2, 3, 445, 1, 0)
	placeTestBuilding(t, w, 5, 3, 445, 1, 0)
	out := placeTestBuilding(t, w, 6, 3, 257, 1, 0)
	in.Build.AddItem(5, 1)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct bridge pair to deliver item forward")
	}
}

func TestDirectionalDuctUnloaderMovesConfiguredItem(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		431: "vault",
		446: "duct-unloader",
	}
	w.SetModel(model)

	store := placeTestBuilding(t, w, 1, 3, 431, 1, 0)
	placeTestBuilding(t, w, 2, 3, 446, 1, 0)
	out := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	store.Build.AddItem(5, 3)
	w.ConfigureUnloader(int32(3*model.Width+2), 5)

	for i := 0; i < 20; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected duct-unloader to push configured item forward")
	}
	if store.Build.ItemAmount(5) >= 3 {
		t.Fatalf("expected duct-unloader to remove item from rear storage")
	}
}

func TestPlastaniumConveyorTransfersWholeStack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		447: "plastanium-conveyor",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 2, 3, 447, 1, 0)
	dst := placeTestBuilding(t, w, 3, 3, 447, 1, 0)
	src.Build.AddItem(5, 10)

	for i := 0; i < 80; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(dst.Build) == 0 {
		t.Fatalf("expected plastanium conveyor to transfer stacked items")
	}
	if totalBuildingItems(src.Build) != 0 {
		t.Fatalf("expected source plastanium conveyor to hand off full stack")
	}
}

func TestSurgeRouterUnloadsBatchToForwardSide(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		421: "battery",
		422: "power-node",
		448: "surge-router",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 448, 1, 0)
	out := placeTestBuilding(t, w, 3, 3, 257, 1, 0)
	placeTestBuilding(t, w, 2, 6, 421, 1, 0)
	placeTestBuilding(t, w, 2, 5, 422, 1, 0)
	w.powerStorageState[int32(6*model.Width+2)] = 4000
	linkPowerNode(t, w, 2, 5, protocol.Point2{X: 0, Y: -2}, protocol.Point2{X: 0, Y: 1})
	router.Build.AddItem(5, 10)
	w.ConfigureSorter(int32(3*model.Width+2), 5)

	for i := 0; i < 80; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingItems(out.Build) == 0 {
		t.Fatalf("expected surge router to unload batch to forward output")
	}
}

func TestLiquidRouterMovesLiquidForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		450: "liquid-router",
		315: "thorium-reactor",
	}
	w.SetModel(model)

	router := placeTestBuilding(t, w, 2, 3, 450, 1, 0)
	store := placeTestBuilding(t, w, 3, 3, 315, 1, 0)
	router.Build.AddLiquid(1, 10)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(store.Build) <= 0 {
		t.Fatalf("expected liquid router to move liquid into container")
	}
}

func TestLiquidBridgeTransfersLinkedLiquid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 8)
	model.BlockNames = map[int16]string{
		452: "bridge-conduit",
		315: "thorium-reactor",
	}
	w.SetModel(model)

	bridge := placeTestBuilding(t, w, 2, 3, 452, 1, 0)
	placeTestBuilding(t, w, 5, 3, 452, 1, 0)
	tank := placeTestBuilding(t, w, 6, 3, 315, 1, 0)
	bridge.Build.AddLiquid(1, 20)
	w.ConfigureBuilding(int32(3*model.Width+2), protocol.Point2{X: 3, Y: 0})

	for i := 0; i < 40; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(tank.Build) <= 0 {
		t.Fatalf("expected linked liquid bridge to move liquid into tank")
	}
}

func TestConduitMovesLiquidForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		454: "conduit",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 1, 3, 454, 1, 0)
	placeTestBuilding(t, w, 2, 3, 454, 1, 0)
	dst := placeTestBuilding(t, w, 3, 3, 315, 1, 0)
	src.Build.AddLiquid(1, 20)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(dst.Build) <= 0 {
		t.Fatalf("expected conduit chain to move liquid into destination")
	}
}

func TestPlatedConduitAcceptsRearLiquid(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		455: "plated-conduit",
	}
	w.SetModel(model)

	src := placeTestBuilding(t, w, 1, 3, 455, 1, 0)
	dst := placeTestBuilding(t, w, 2, 3, 315, 1, 0)
	src.Build.AddLiquid(1, 30)

	for i := 0; i < 30; i++ {
		w.Step(time.Second / 60)
	}

	if totalBuildingLiquids(dst.Build) <= 0 {
		t.Fatalf("expected plated conduit to move liquid forward")
	}
}

func TestPayloadConveyorTransfersBlockPayloadForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 12)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		700: "payload-conveyor",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 4, 4, 700, 1, 0)
	placeTestBuilding(t, w, 7, 4, 700, 1, 0)

	sourcePos := setTestPayload(t, w, 4, 4, &payloadData{Kind: payloadKindBlock, BlockID: 257})
	targetPos := int32(4*model.Width + 7)

	for i := 0; i < 60; i++ {
		w.Step(time.Second / 60)
	}

	if w.payloadStateLocked(sourcePos).Payload != nil {
		t.Fatalf("expected source payload conveyor to hand off payload")
	}
	if got := w.payloadStateLocked(targetPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected target payload conveyor to receive block payload, got=%+v", got)
	}
}

func TestPayloadRouterRoutesMatchingBlockForward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		700: "payload-conveyor",
		701: "payload-router",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 8, 8, 701, 1, 1)
	placeTestBuilding(t, w, 11, 8, 700, 1, 0)
	placeTestBuilding(t, w, 8, 11, 700, 1, 1)

	routerPos := int32(8*model.Width + 8)
	w.ConfigureBuildingPacked(protocol.PackPoint2(8, 8), protocol.BlockRef{BlkID: 257})
	cfg, ok := w.BuildingConfigPacked(protocol.PackPoint2(8, 8))
	if !ok {
		t.Fatalf("expected payload router config to persist")
	}
	filter, ok := cfg.(protocol.Content)
	if !ok || filter.ContentType() != protocol.ContentBlock || filter.ID() != 257 {
		t.Fatalf("expected payload router block filter config, got=%T %+v", cfg, cfg)
	}

	st := w.payloadStateLocked(routerPos)
	st.Payload = &payloadData{Kind: payloadKindBlock, BlockID: 257}
	st.RecDir = 0

	for i := 0; i < 60; i++ {
		w.Step(time.Second / 60)
	}

	forwardPos := int32(8*model.Width + 11)
	sidePos := int32(11*model.Width + 8)
	if got := w.payloadStateLocked(forwardPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected matching payload to route forward, got=%+v", got)
	}
	if got := w.payloadStateLocked(sidePos).Payload; got != nil {
		t.Fatalf("expected side target to stay empty, got=%+v", got)
	}
}

func TestPayloadMassDriverTransfersPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		421: "battery",
		422: "power-node",
		702: "payload-mass-driver",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 6, 8, 702, 1, 0)
	placeTestBuilding(t, w, 16, 8, 702, 1, 0)
	placeTestBuilding(t, w, 6, 14, 421, 1, 0)
	placeTestBuilding(t, w, 16, 14, 421, 1, 0)
	placeTestBuilding(t, w, 6, 11, 422, 1, 0)
	placeTestBuilding(t, w, 16, 11, 422, 1, 0)
	w.powerStorageState[int32(14*model.Width+6)] = 4000
	w.powerStorageState[int32(14*model.Width+16)] = 4000
	linkPowerNode(t, w, 6, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 3})
	linkPowerNode(t, w, 16, 11, protocol.Point2{X: 0, Y: -3}, protocol.Point2{X: 0, Y: 3})

	sourcePos := int32(8*model.Width + 6)
	targetPos := int32(8*model.Width + 16)
	w.ConfigureBuilding(sourcePos, protocol.Point2{X: 10, Y: 0})
	setTestPayload(t, w, 6, 8, &payloadData{Kind: payloadKindBlock, BlockID: 257})

	for i := 0; i < 260; i++ {
		w.Step(time.Second / 60)
	}

	if w.payloadStateLocked(sourcePos).Payload != nil {
		t.Fatalf("expected source payload mass driver to launch payload")
	}
	if got := w.payloadStateLocked(targetPos).Payload; got == nil || got.BlockID != 257 {
		t.Fatalf("expected target payload mass driver to receive payload, got=%+v", got)
	}
}

func TestPayloadLoaderAndUnloaderTransferItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 16)
	model.BlockNames = map[int16]string{
		315: "thorium-reactor",
		703: "payload-loader",
		704: "payload-unloader",
	}
	w.SetModel(model)

	loader := placeTestBuilding(t, w, 8, 8, 703, 1, 0)
	unloader := placeTestBuilding(t, w, 11, 8, 704, 1, 0)
	loader.Build.AddItem(5, 6)
	setTestPayload(t, w, 8, 8, &payloadData{Kind: payloadKindBlock, BlockID: 315})

	for i := 0; i < 180; i++ {
		w.Step(time.Second / 60)
	}

	if unloader.Build.ItemAmount(5) == 0 {
		t.Fatalf("expected payload loader/unloader pair to transfer items into unloader inventory")
	}
}

func TestControlSelectPayloadUnitPackedAcceptsReconstructor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		706: "additive-reconstructor",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
		8: "mace",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 706, 1, 0)
	buildPacked := protocol.PackPoint2(int32(tile.X), int32(tile.Y))
	buildPos := int32(tile.Y*w.Model().Width + tile.X)
	unitID := w.Model().AddEntity(RawEntity{
		ID:        81,
		TypeID:    7,
		Team:      1,
		X:         6*8 + 4,
		Y:         6*8 + 4,
		Health:    90,
		MaxHealth: 90,
	}).ID

	if !w.ControlSelectPayloadUnitPacked(buildPacked, unitID) {
		t.Fatal("expected reconstructor control-select to accept upgradeable unit payload")
	}

	payload := w.payloadStateLocked(buildPos).Payload
	if payload == nil || payload.Kind != payloadKindUnit || payload.UnitTypeID != 7 {
		t.Fatalf("expected reconstructor to receive dagger payload, got %+v", payload)
	}
}

func TestEnterUnitPayloadPackedRejectsSpawnedByCoreOnPayloadDeconstructor(t *testing.T) {
	unit := RawEntity{
		ID:            82,
		TypeID:        7,
		Team:          1,
		X:             8*8 + 4,
		Y:             8*8 + 4,
		Health:        90,
		MaxHealth:     90,
		SpawnedByCore: true,
	}
	w, buildPacked, buildPos, unitID := newPayloadBuildingWorld(t, 705, "small-deconstructor", 0, unit)

	if w.EnterUnitPayloadPacked(buildPacked, unitID) {
		t.Fatal("expected unitEnteredPayload path to reject spawnedByCore units on payload deconstructor")
	}
	if payload := w.payloadStateLocked(buildPos).Payload; payload != nil {
		t.Fatalf("expected small deconstructor payload to stay empty, got %+v", payload)
	}
	found := false
	for _, ent := range w.Model().Entities {
		if ent.ID == unitID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rejected unit %d to remain in world", unitID)
	}
}

func TestPayloadVoidConsumesPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		705: "payload-void",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 705, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	payload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(7, 1, 0, 0, 0))
	if payload == nil {
		t.Fatal("expected dagger payload for payload-void test")
	}
	setTestPayload(t, w, 8, 8, payload)

	stepForSeconds(w, 1)

	if got := w.payloadStateLocked(pos).Payload; got != nil {
		t.Fatalf("expected payload-void to incinerate payload, got %+v", got)
	}
	if len(tile.Build.Payload) != 0 {
		t.Fatalf("expected payload-void build payload bytes to clear, got len=%d", len(tile.Build.Payload))
	}
}

func TestPayloadDeconstructorProcessesUnitPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		421: "battery",
		422: "power-node",
		705: "small-deconstructor",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 705, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	placeTestBuilding(t, w, 5, 8, 421, 1, 0)
	placeTestBuilding(t, w, 6, 8, 422, 1, 0)
	w.powerStorageState[int32(8*model.Width+5)] = 4000
	linkPowerNode(t, w, 6, 8, protocol.Point2{X: -1, Y: 0}, protocol.Point2{X: 2, Y: 0})

	payload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(7, 1, 0, 0, 0))
	if payload == nil {
		t.Fatal("expected dagger payload for deconstructor test")
	}
	setTestPayload(t, w, 8, 8, payload)

	stepForSeconds(w, 1)

	if got := tile.Build.ItemAmount(siliconItemID); got != 10 {
		t.Fatalf("expected deconstructor to recover 10 silicon, got %d", got)
	}
	if got := tile.Build.ItemAmount(leadItemID); got != 10 {
		t.Fatalf("expected deconstructor to recover 10 lead, got %d", got)
	}
	if got := w.payloadStateLocked(pos).Payload; got != nil {
		t.Fatalf("expected deconstructor input payload to clear, got %+v", got)
	}
	if st, ok := w.payloadDeconstructorStates[pos]; ok && st != nil && st.Deconstructing != nil {
		t.Fatalf("expected deconstructor runtime payload to finish, got %+v", st.Deconstructing)
	}
}

func TestReconstructorUpgradesUnitPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		418: "router",
		421: "battery",
		422: "power-node",
		706: "additive-reconstructor",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
		8: "mace",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 10, 10, 706, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	placeTestBuilding(t, w, 12, 10, 418, 1, 0)
	placeTestBuilding(t, w, 7, 10, 421, 1, 0)
	placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	w.powerStorageState[int32(10*model.Width+7)] = 4000
	linkPowerNode(t, w, 8, 10, protocol.Point2{X: -1, Y: 0}, protocol.Point2{X: 2, Y: 0})
	tile.Build.AddItem(siliconItemID, 40)
	tile.Build.AddItem(graphiteItemID, 40)

	payload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(7, 1, 0, 0, 0))
	if payload == nil {
		t.Fatal("expected dagger payload for reconstructor test")
	}
	setTestPayload(t, w, 10, 10, payload)

	stepForSeconds(w, 11)

	current := w.payloadStateLocked(pos).Payload
	if current == nil || current.Kind != payloadKindUnit || current.UnitTypeID != 8 {
		t.Fatalf("expected additive reconstructor to upgrade dagger into mace payload, got %+v", current)
	}
	if got := tile.Build.ItemAmount(siliconItemID); got != 0 {
		t.Fatalf("expected reconstructor to consume silicon on completion, got %d", got)
	}
	if got := tile.Build.ItemAmount(graphiteItemID); got != 0 {
		t.Fatalf("expected reconstructor to consume graphite on completion, got %d", got)
	}
}

func TestReconstructorDumpedUnitCarriesCommandState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		706: "additive-reconstructor",
	}
	model.UnitNames = map[int16]string{
		8: "mace",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 2, 2, 339, 1, 0)
	tile := placeTestBuilding(t, w, 8, 8, 706, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.reconstructorStates[pos] = reconstructorState{
		CommandPos: &protocol.Vec2{X: 144, Y: 80},
		Command:    &protocol.UnitCommand{ID: 2, Name: "repair"},
	}
	payload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(8, 1, 0, 0, 0))
	if payload == nil {
		t.Fatal("expected upgraded unit payload for reconstructor dump test")
	}
	w.payloadStates[pos] = &payloadRuntimeState{Payload: payload}
	w.syncPayloadTileLocked(tile, payload)

	if !w.dumpUnitPayloadFromTileLocked(pos, tile) {
		t.Fatal("expected reconstructor payload to dump into a world unit")
	}

	found := false
	for _, ent := range w.model.Entities {
		if ent.TypeID != 8 || ent.Team != 1 {
			continue
		}
		found = true
		if ent.CommandID != 2 {
			t.Fatalf("expected dumped reconstructor unit command id 2, got %d", ent.CommandID)
		}
		if ent.Behavior != "move" {
			t.Fatalf("expected dumped reconstructor unit behavior move, got %q", ent.Behavior)
		}
		if math.Abs(float64(ent.PatrolAX-144)) > 0.0001 || math.Abs(float64(ent.PatrolAY-80)) > 0.0001 {
			t.Fatalf("expected dumped reconstructor unit target (144,80), got (%f,%f)", ent.PatrolAX, ent.PatrolAY)
		}
	}
	if !found {
		t.Fatal("expected dumped reconstructor unit entity to exist")
	}
}

func TestBlockSyncSnapshotsEncodePayloadProcessorsRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(28, 20)
	model.BlockNames = map[int16]string{
		705: "payload-void",
		706: "small-deconstructor",
		707: "additive-reconstructor",
	}
	model.UnitNames = map[int16]string{
		7: "dagger",
	}
	w.SetModel(model)
	voidTile := placeTestBuilding(t, w, 6, 8, 705, 1, 0)
	deconTile := placeTestBuilding(t, w, 14, 8, 706, 1, 0)
	reconTile := placeTestBuilding(t, w, 22, 8, 707, 1, 0)
	voidPos := int32(voidTile.Y*w.Model().Width + voidTile.X)
	deconPos := int32(deconTile.Y*w.Model().Width + deconTile.X)
	reconPos := int32(reconTile.Y*w.Model().Width + reconTile.X)

	voidPayload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(7, 1, 0, 0, 0))
	deconstructingPayload := w.unitPayloadFromEntityLocked(w.newProducedUnitEntityLocked(7, 1, 0, 0, 0))
	if voidPayload == nil || deconstructingPayload == nil {
		t.Fatal("expected payload processor test payloads")
	}
	setTestPayload(t, w, 6, 8, voidPayload)
	w.payloadDeconstructorStates[deconPos] = &payloadDeconstructorState{
		Deconstructing: deconstructingPayload,
		Accum:          []float32{1.25, 0.5},
		Progress:       0.75,
		PayRotation:    45,
	}
	w.reconstructorStates[reconPos] = reconstructorState{
		Progress:   321,
		CommandPos: &protocol.Vec2{X: 160, Y: 96},
		Command:    &protocol.UnitCommand{ID: 2, Name: "repair"},
	}
	w.rebuildActiveTilesLocked()

	fastSnaps := w.PayloadProcessorBlockSyncSnapshots()
	if len(fastSnaps) != 3 {
		t.Fatalf("expected three fast payload processor snapshots, got %d", len(fastSnaps))
	}

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 3 {
		t.Fatalf("expected three payload processor snapshots, got %d", len(snaps))
	}
	byPos := make(map[int32]BlockSyncSnapshot, len(snaps))
	for _, snap := range snaps {
		byPos[snap.Pos] = snap
	}

	voidSnap, ok := byPos[protocol.PackPoint2(int32(voidTile.X), int32(voidTile.Y))]
	if !ok {
		t.Fatalf("missing payload-void snapshot at %d", protocol.PackPoint2(int32(voidTile.X), int32(voidTile.Y)))
	}
	_, r := decodeBlockSyncBase(t, voidSnap.Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read payload-void payVector.x failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read payload-void payVector.y failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read payload-void payRotation failed: %v", err)
	}
	payloadExists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read payload-void payload exists failed: %v", err)
	}
	if !payloadExists {
		t.Fatal("expected payload-void snapshot to include payload bytes")
	}

	deconSnap, ok := byPos[protocol.PackPoint2(int32(deconTile.X), int32(deconTile.Y))]
	if !ok {
		t.Fatalf("missing payload-deconstructor snapshot at %d", protocol.PackPoint2(int32(deconTile.X), int32(deconTile.Y)))
	}
	_, r = decodeBlockSyncBase(t, deconSnap.Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read deconstructor payVector.x failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read deconstructor payVector.y failed: %v", err)
	}
	payRotation, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read deconstructor payRotation failed: %v", err)
	}
	if math.Abs(float64(payRotation-45)) > 0.0001 {
		t.Fatalf("expected deconstructor payRotation 45, got %f", payRotation)
	}
	currentExists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read deconstructor current payload exists failed: %v", err)
	}
	if currentExists {
		t.Fatal("expected deconstructor current payload slot to be empty during deconstruction")
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read deconstructor progress failed: %v", err)
	}
	if math.Abs(float64(progress-0.75)) > 0.0001 {
		t.Fatalf("expected deconstructor progress 0.75, got %f", progress)
	}
	accumLen, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read deconstructor accum length failed: %v", err)
	}
	if accumLen != 2 {
		t.Fatalf("expected deconstructor accum length 2, got %d", accumLen)
	}
	acc0, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read deconstructor accum[0] failed: %v", err)
	}
	acc1, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read deconstructor accum[1] failed: %v", err)
	}
	if math.Abs(float64(acc0-1.25)) > 0.0001 || math.Abs(float64(acc1-0.5)) > 0.0001 {
		t.Fatalf("expected deconstructor accum [1.25 0.5], got [%f %f]", acc0, acc1)
	}
	deconstructingExists, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read deconstructor deconstructing payload exists failed: %v", err)
	}
	if !deconstructingExists {
		t.Fatal("expected deconstructor snapshot to include deconstructing payload bytes")
	}

	reconSnap, ok := byPos[protocol.PackPoint2(int32(reconTile.X), int32(reconTile.Y))]
	if !ok {
		t.Fatalf("missing reconstructor snapshot at %d", protocol.PackPoint2(int32(reconTile.X), int32(reconTile.Y)))
	}
	_, r = decodeBlockSyncBase(t, reconSnap.Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read reconstructor payVector.x failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read reconstructor payVector.y failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read reconstructor payRotation failed: %v", err)
	}
	payloadExists, err = r.ReadBool()
	if err != nil {
		t.Fatalf("read reconstructor payload exists failed: %v", err)
	}
	if payloadExists {
		t.Fatal("expected reconstructor payload slot to be empty in snapshot test")
	}
	progress, err = r.ReadFloat32()
	if err != nil {
		t.Fatalf("read reconstructor progress failed: %v", err)
	}
	if math.Abs(float64(progress-321)) > 0.0001 {
		t.Fatalf("expected reconstructor progress 321, got %f", progress)
	}
	commandPos, err := protocol.ReadVecNullable(r)
	if err != nil {
		t.Fatalf("read reconstructor commandPos failed: %v", err)
	}
	command, err := protocol.ReadCommand(r, nil)
	if err != nil {
		t.Fatalf("read reconstructor command failed: %v", err)
	}
	if commandPos == nil || math.Abs(float64(commandPos.X-160)) > 0.0001 || math.Abs(float64(commandPos.Y-96)) > 0.0001 {
		t.Fatalf("expected reconstructor commandPos (160,96), got %+v", commandPos)
	}
	if command == nil || command.ID != 2 {
		t.Fatalf("expected reconstructor command id 2, got %+v", command)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected reconstructor sync payload to be fully consumed, remaining=%d", rem)
	}

	_ = voidPos
}

func findTestEntity(t *testing.T, w *World, id int32) RawEntity {
	t.Helper()
	for _, ent := range w.Model().Entities {
		if ent.ID == id {
			return ent
		}
	}
	t.Fatalf("entity %d not found", id)
	return RawEntity{}
}

func TestGroundAIAdvancesTowardEnemyCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		1: "dagger",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{Name: "dagger", Speed: 24}

	core := placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:      1,
		X:           float32(20*8 + 4),
		Y:           float32(12*8 + 4),
		Team:        2,
		MineTilePos: invalidEntityTilePos,
	})

	coreX := float32(core.X*8 + 4)
	coreY := float32(core.Y*8 + 4)
	before := float32(math.Hypot(float64(ent.X-coreX), float64(ent.Y-coreY)))
	stepForSeconds(w, 3)
	got := findTestEntity(t, w, ent.ID)
	after := float32(math.Hypot(float64(got.X-coreX), float64(got.Y-coreY)))
	if after >= before-16 {
		t.Fatalf("expected ground AI to advance toward enemy core, before=%f after=%f", before, after)
	}
}

func TestGroundAIPathsAroundBlockingBuildings(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		600: "test-wall",
	}
	model.UnitNames = map[int16]string{
		1: "dagger",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{Name: "dagger", Speed: 24}

	placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	for y := 0; y < 24; y++ {
		if y == 4 {
			continue
		}
		placeTestBuilding(t, w, 10, y, 600, 1, 0)
	}

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      1,
		X:           float32(20*8 + 4),
		Y:           float32(12*8 + 4),
		Team:        2,
		MineTilePos: invalidEntityTilePos,
	})

	stepForSeconds(w, 9)
	got := findTestEntity(t, w, ent.ID)
	if got.X >= float32(10*8+4) {
		t.Fatalf("expected ground AI to route around wall line, got x=%f y=%f", got.X, got.Y)
	}
}

func TestFlyingAIIgnoresGroundWallPathing(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		600: "test-wall",
	}
	model.UnitNames = map[int16]string{
		2: "flare",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["flare"] = unitRuntimeProfile{Name: "flare", Speed: 28, Flying: true}

	placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	for y := 0; y < 24; y++ {
		placeTestBuilding(t, w, 10, y, 600, 1, 0)
	}

	startX := float32(20*8 + 4)
	startY := float32(12*8 + 4)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:      2,
		X:           startX,
		Y:           startY,
		Team:        2,
		MineTilePos: invalidEntityTilePos,
	})

	stepForSeconds(w, 3)
	got := findTestEntity(t, w, ent.ID)
	if got.X >= startX-24 {
		t.Fatalf("expected flying AI to keep advancing through wall line, startX=%f gotX=%f", startX, got.X)
	}
	if math.Abs(float64(got.Y-startY)) > 12 {
		t.Fatalf("expected flying AI to keep a mostly direct line, startY=%f gotY=%f", startY, got.Y)
	}
}

func TestFlyingFollowAITracksNearbyAlly(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		10: "quell",
		11: "mace",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["quell"] = unitRuntimeProfile{Name: "quell", Speed: 28, Flying: true}
	w.unitRuntimeProfilesByName["mace"] = unitRuntimeProfile{Name: "mace", Speed: 6}

	placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	ally := w.Model().AddEntity(RawEntity{
		TypeID:      11,
		X:           float32(20*8 + 4),
		Y:           float32(18*8 + 4),
		Team:        2,
		Health:      5000,
		MaxHealth:   5000,
		MineTilePos: invalidEntityTilePos,
	})
	startX := float32(20*8 + 4)
	startY := float32(8*8 + 4)
	follower := w.Model().AddEntity(RawEntity{
		TypeID:      10,
		X:           startX,
		Y:           startY,
		Team:        2,
		MineTilePos: invalidEntityTilePos,
	})

	before := float32(math.Hypot(float64(startX-ally.X), float64(startY-ally.Y)))
	stepForSeconds(w, 2)
	got := findTestEntity(t, w, follower.ID)
	after := float32(math.Hypot(float64(got.X-ally.X), float64(got.Y-ally.Y)))
	if after >= before-20 {
		t.Fatalf("expected FlyingFollowAI to close on ally before free-flying, before=%f after=%f", before, after)
	}
}

func TestBuilderAIExecutesEntityPlansWithoutExternalBuilderState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.blockBuildTimesByName = map[string]float32{"duo": 1.5}

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 100)
	if _, err := w.AddEntityWithID(35, 9101, 20, 20, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}
	for i := range w.Model().Entities {
		if w.Model().Entities[i].ID != 9101 {
			continue
		}
		w.Model().Entities[i].UpdateBuilding = true
		w.Model().Entities[i].Plans = []entityBuildPlan{{
			Pos:     packTilePos(2, 2),
			BlockID: 45,
		}}
		break
	}

	built := false
	for i := 0; i < 200; i++ {
		w.Step(time.Second / 60)
		tile, _ := w.Model().TileAt(2, 2)
		if tile.Block == 45 && tile.Build != nil {
			built = true
			break
		}
	}
	if !built {
		tile, _ := w.Model().TileAt(2, 2)
		t.Fatalf("expected builder AI plans to construct duo, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestBuilderAIStaysIdleWithoutThreat(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 3, 3, 339, 1, 0)
	if _, err := w.AddEntityWithID(35, 9102, 20*8+4, 20*8+4, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}
	start := findTestEntity(t, w, 9102)
	stepForSeconds(w, 3)
	got := findTestEntity(t, w, 9102)
	if moved := float32(math.Hypot(float64(got.X-start.X), float64(got.Y-start.Y))); moved > 0.001 {
		t.Fatalf("expected idle builder AI to stay put without threats, moved=%f start=(%f,%f) got=(%f,%f)", moved, start.X, start.Y, got.X, got.Y)
	}
	coreX := float32(core.X*8 + 4)
	coreY := float32(core.Y*8 + 4)
	before := float32(math.Hypot(float64(start.X-coreX), float64(start.Y-coreY)))
	after := float32(math.Hypot(float64(got.X-coreX), float64(got.Y-coreY)))
	if math.Abs(float64(after-before)) > 0.001 {
		t.Fatalf("expected idle builder AI to keep its core distance without threats, before=%f after=%f", before, after)
	}
}

func TestBuilderAIAlwaysFleeRetreatsToFriendlyCoreWhenThreatened(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		1:  "dagger",
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{Name: "dagger", Speed: 24}

	core := placeTestBuilding(t, w, 3, 3, 339, 1, 0)
	if _, err := w.AddEntityWithID(35, 9103, 20*8+4, 20*8+4, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}
	for i := range w.Model().Entities {
		if w.Model().Entities[i].ID != 9103 {
			continue
		}
		w.Model().Entities[i].UpdateBuilding = true
		w.Model().Entities[i].Plans = []entityBuildPlan{{
			Pos:     packTilePos(20, 20),
			BlockID: 45,
		}}
		break
	}
	w.Model().AddEntity(RawEntity{
		TypeID:      1,
		X:           18*8 + 4,
		Y:           20*8 + 4,
		Team:        2,
		Health:      100,
		MaxHealth:   100,
		MoveSpeed:   24,
		MineTilePos: invalidEntityTilePos,
	})

	start := findTestEntity(t, w, 9103)
	coreX := float32(core.X*8 + 4)
	coreY := float32(core.Y*8 + 4)
	before := float32(math.Hypot(float64(start.X-coreX), float64(start.Y-coreY)))
	stepForSeconds(w, 1)
	got := findTestEntity(t, w, 9103)
	after := float32(math.Hypot(float64(got.X-coreX), float64(got.Y-coreY)))
	if after >= before {
		t.Fatalf("expected threatened builder AI to retreat toward friendly core, before=%f after=%f", before, after)
	}
	if len(got.Plans) != 0 {
		t.Fatalf("expected threatened builder AI to clear build plans while fleeing, got %v", got.Plans)
	}
}

func TestBuilderAIFollowsNearbyConstructBuilderBeforeRebuildQueue(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	leader := w.Model().AddEntity(RawEntity{
		ID:           9201,
		TypeID:       35,
		PlayerID:     1,
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
	op := BuildPlanOp{X: 18, Y: 18, Rotation: 0, BlockID: 45}
	primeAssistConstructBuilder(t, w, leader, op)
	w.teamRebuildPlans[1] = []rebuildBlockPlan{{
		X:       30,
		Y:       30,
		BlockID: 45,
	}}
	if _, err := w.AddEntityWithID(35, 9202, 18*8+4, 16*8+4, 1); err != nil {
		t.Fatalf("add follower alpha entity: %v", err)
	}

	w.Step(time.Second / 60)
	w.Step(time.Second / 60)

	got := findTestEntity(t, w, 9202)
	if len(got.Plans) == 0 {
		t.Fatal("expected nearby builder AI to copy active construct leader plan")
	}
	if got.Plans[0].Breaking {
		t.Fatalf("expected copied construct plan to remain a build plan, got %+v", got.Plans[0])
	}
	if got.Plans[0].Pos != packTilePos(18, 18) {
		t.Fatalf("expected nearby builder AI to follow construct leader plan at (18,18), got %+v", got.Plans[0])
	}
}

func TestBuilderAIFollowsNearbyDeconstructBuilder(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
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
		ID:           9203,
		TypeID:       35,
		PlayerID:     1,
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
	if _, err := w.AddEntityWithID(35, 9204, 18*8+4, 16*8+4, 1); err != nil {
		t.Fatalf("add follower alpha entity: %v", err)
	}

	w.Step(time.Second / 60)
	w.Step(time.Second / 60)

	got := findTestEntity(t, w, 9204)
	if len(got.Plans) == 0 {
		t.Fatal("expected nearby builder AI to copy active deconstruct leader plan")
	}
	if !got.Plans[0].Breaking || got.Plans[0].Pos != packTilePos(18, 18) {
		t.Fatalf("expected nearby builder AI to follow deconstruct leader plan at (18,18), got %+v", got.Plans[0])
	}
}

func TestBuilderAIRemovedQueuedRebuildPlanClearsEntityPlan(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.blockBuildTimesByName = map[string]float32{"duo": 1.5}
	rules := w.GetRulesManager().Get()
	rules.GhostBlocks = true

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 1000)
	placeTestBuilding(t, w, 10, 10, 45, 1, 0)
	if !w.DamageBuildingPacked(packTilePos(10, 10), 2000) {
		t.Fatal("expected destroyed building to enter broken-block rebuild queue")
	}
	if len(w.teamRebuildPlans[1]) != 1 {
		t.Fatalf("expected exactly one queued rebuild plan, got %d", len(w.teamRebuildPlans[1]))
	}
	if _, err := w.AddEntityWithID(35, 9205, 20, 20, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}

	w.Step(time.Second / 60)
	got := findTestEntity(t, w, 9205)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected builder AI to pick queued rebuild plan at (10,10), got %+v", got.Plans)
	}

	delete(w.teamRebuildPlans, TeamID(1))
	stepForSeconds(w, 5)

	got = findTestEntity(t, w, 9205)
	if len(got.Plans) != 0 {
		t.Fatalf("expected builder AI to drop removed queued rebuild plan, got %+v", got.Plans)
	}
	tile, err := w.Model().TileAt(10, 10)
	if err != nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected removed queued rebuild plan to stop any hidden rebuild, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
}

func TestBuilderAIQueuedRebuildPlanYieldsToPlayerBreakingSameTile(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	rules := w.GetRulesManager().Get()
	rules.GhostBlocks = true

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 1000)
	placeTestBuilding(t, w, 10, 10, 45, 1, 0)
	if !w.DamageBuildingPacked(packTilePos(10, 10), 2000) {
		t.Fatal("expected destroyed building to enter broken-block rebuild queue")
	}
	if _, err := w.AddEntityWithID(35, 9208, 20, 20, 1); err != nil {
		t.Fatalf("add autonomous alpha entity: %v", err)
	}

	w.Step(time.Second / 60)
	got := findTestEntity(t, w, 9208)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected autonomous builder to pick queued rebuild plan, got %+v", got.Plans)
	}

	playerBuilder := w.Model().AddEntity(RawEntity{
		ID:           9209,
		TypeID:       35,
		PlayerID:     7,
		X:            10*8 + 4,
		Y:            10*8 + 4,
		Team:         1,
		Health:       100,
		MaxHealth:    100,
		MoveSpeed:    24,
		BuildSpeed:   0.5,
		ItemCapacity: 30,
		Flying:       true,
	})
	w.UpdateBuilderState(playerBuilder.ID, playerBuilder.Team, playerBuilder.ID, playerBuilder.X, playerBuilder.Y, true, 220)
	if _, ok := w.SetEntityBuildState(playerBuilder.ID, true, []*protocol.BuildPlan{{
		Breaking: true,
		X:        10,
		Y:        10,
	}}); !ok {
		t.Fatal("expected player break plan to apply")
	}

	w.Step(time.Second / 60)

	got = findTestEntity(t, w, 9208)
	if len(got.Plans) != 0 {
		t.Fatalf("expected autonomous builder to yield rebuild plan to player breaking same tile, got %+v", got.Plans)
	}
	if _, ok := w.teamRebuildPlans[1]; ok {
		t.Fatalf("expected matching broken-block rebuild queue item to be cleared, got %+v", w.teamRebuildPlans[1])
	}
}

func TestBuilderAINearEnemyPlanUsesOfficialRectUnitCheck(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(96, 96)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
		400: "lancer",
	}
	model.UnitNames = map[int16]string{
		1:  "dagger",
		35: "alpha",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 1000)

	w.teamRebuildPlans[1] = []rebuildBlockPlan{{
		X:       10,
		Y:       10,
		BlockID: 45,
	}}
	if _, err := w.AddEntityWithID(35, 9210, 20, 20, 1); err != nil {
		t.Fatalf("add autonomous alpha entity: %v", err)
	}
	planX := float32(10*8 + 4)
	planY := float32(10*8 + 4)
	w.Model().AddEntity(RawEntity{
		ID:        9211,
		TypeID:    1,
		X:         planX + 300,
		Y:         planY,
		Team:      2,
		Health:    100,
		MaxHealth: 100,
	})

	w.Step(time.Second / 60)

	got := findTestEntity(t, w, 9210)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected builder AI to keep rebuild plan when enemy unit is outside official nearEnemy square, got %+v", got.Plans)
	}

	placeTestBuilding(t, w, 30, 10, 400, 2, 0)
	w.teamRebuildPlans[1] = []rebuildBlockPlan{{
		X:       10,
		Y:       10,
		BlockID: 45,
	}}
	for i := range w.Model().Entities {
		if w.Model().Entities[i].ID != 9210 {
			continue
		}
		w.Model().Entities[i].Plans = nil
		w.Model().Entities[i].UpdateBuilding = true
		break
	}

	w.Step(time.Second / 60)

	got = findTestEntity(t, w, 9210)
	if len(got.Plans) != 0 {
		t.Fatalf("expected enemy turret range to count as nearEnemy and block rebuild pickup, got %+v", got.Plans)
	}
}

func TestBuilderAIMultiBuilderUsesDistinctRebuildPlans(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	core.Build.AddItem(0, 1000)
	w.teamRebuildPlans[1] = []rebuildBlockPlan{
		{X: 5, Y: 5, BlockID: 45},
		{X: 9, Y: 9, BlockID: 45},
	}
	if _, err := w.AddEntityWithID(35, 9206, 20, 20, 1); err != nil {
		t.Fatalf("add first alpha entity: %v", err)
	}
	if _, err := w.AddEntityWithID(35, 9207, 24, 24, 1); err != nil {
		t.Fatalf("add second alpha entity: %v", err)
	}

	w.Step(time.Second / 60)

	first := findTestEntity(t, w, 9206)
	second := findTestEntity(t, w, 9207)
	if len(first.Plans) == 0 || len(second.Plans) == 0 {
		t.Fatalf("expected both builder AI units to acquire rebuild plans, first=%+v second=%+v", first.Plans, second.Plans)
	}
	if first.Plans[0].Pos == second.Plans[0].Pos {
		t.Fatalf("expected multi-builder rebuild queue to distribute distinct plans, both got %+v", first.Plans[0])
	}
}

func TestWaveTeamBuilderFallsBackToAdvanceLikeOfficial(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{Name: "alpha", Speed: 24, Flying: true}

	core := placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	if _, err := w.AddEntityWithID(35, 9212, 20*8+4, 12*8+4, 2); err != nil {
		t.Fatalf("add wave-team alpha entity: %v", err)
	}

	start := findTestEntity(t, w, 9212)
	coreX := float32(core.X*8 + 4)
	coreY := float32(core.Y*8 + 4)
	before := float32(math.Hypot(float64(start.X-coreX), float64(start.Y-coreY)))
	stepForSeconds(w, 3)
	got := findTestEntity(t, w, 9212)
	after := float32(math.Hypot(float64(got.X-coreX), float64(got.Y-coreY)))
	if after >= before {
		t.Fatalf("expected wave-team builder to use fallback AI and advance toward enemy core, before=%f after=%f", before, after)
	}
}

func TestWaveTeamBuilderDoesNotFallbackWhenRtsAiEnabled(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{Name: "alpha", Speed: 24, Flying: true}

	rules := w.GetRulesManager().Get()
	rules.RtsAi = true

	placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	if _, err := w.AddEntityWithID(35, 9213, 20*8+4, 12*8+4, 2); err != nil {
		t.Fatalf("add wave-team alpha entity: %v", err)
	}

	start := findTestEntity(t, w, 9213)
	stepForSeconds(w, 3)
	got := findTestEntity(t, w, 9213)
	if moved := float32(math.Hypot(float64(got.X-start.X), float64(got.Y-start.Y))); moved > 0.001 {
		t.Fatalf("expected wave-team builder to keep builder AI when rtsAi is enabled, moved=%f start=(%f,%f) got=(%f,%f)", moved, start.X, start.Y, got.X, got.Y)
	}
}

func TestPrebuildAIFallbackRequiresOfficialAITeam(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        24,
		Flying:       true,
		BuildSpeed:   0.5,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.PrebuildAi = true

	placeTestBuilding(t, w, 4, 4, 339, 1, 0)
	core := placeTestBuilding(t, w, 24, 16, 339, 2, 0)
	core.Build.AddItem(0, 100)
	w.queueTeamBuildPlanFrontLocked(2, BuildPlanOp{X: 20, Y: 16, BlockID: 45})

	if _, err := w.AddEntityWithID(35, 9301, 24*8+4, 16*8+4, 2); err != nil {
		t.Fatalf("add ai alpha entity: %v", err)
	}
	if _, ok := w.SetEntityFlag(9301, float64(packTilePos(core.X, core.Y))); !ok {
		t.Fatal("bind ai alpha to core")
	}
	if _, err := w.AddEntityWithID(35, 9302, 8*8+4, 8*8+4, 1); err != nil {
		t.Fatalf("add default-team alpha entity: %v", err)
	}
	if _, ok := w.SetEntityFlag(9302, float64(packTilePos(4, 4))); !ok {
		t.Fatal("bind default-team alpha to core")
	}

	w.Step(time.Second / 60)

	gotAI := findTestEntity(t, w, 9301)
	if len(gotAI.Plans) == 0 || gotAI.Plans[0].Pos != packTilePos(20, 16) {
		t.Fatalf("expected official AI team to use PrebuildAI queue plan, got %+v", gotAI.Plans)
	}
	gotDefault := findTestEntity(t, w, 9302)
	if len(gotDefault.Plans) != 0 {
		t.Fatalf("expected default team builder to keep BuilderAI instead of PrebuildAI, got %+v", gotDefault.Plans)
	}
}

func TestPrebuildAIMinesMissingItemsBeforeBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		100: "router",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[16*model.Width+10].Floor = 1
	model.Tiles[16*model.Width+10].Overlay = 2
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        24,
		Flying:       true,
		BuildSpeed:   0.5,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	w.blockBuildTimesByName["router"] = 0.35

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.PrebuildAi = true

	placeTestBuilding(t, w, 6, 16, 339, 2, 0)
	w.queueTeamBuildPlanFrontLocked(2, BuildPlanOp{X: 12, Y: 16, BlockID: 100})

	if _, err := w.AddEntityWithID(35, 9303, 8*8+4, 16*8+4, 2); err != nil {
		t.Fatalf("add ai alpha entity: %v", err)
	}
	if _, ok := w.SetEntityFlag(9303, float64(packTilePos(6, 16))); !ok {
		t.Fatal("bind ai alpha to core")
	}

	w.Step(time.Second / 60)
	got := findTestEntity(t, w, 9303)
	if got.UpdateBuilding || len(got.Plans) == 0 {
		t.Fatalf("expected PrebuildAI to queue the team plan immediately, updateBuilding=%v plans=%+v", got.UpdateBuilding, got.Plans)
	}

	stepForSeconds(w, 6)

	tile, err := w.Model().TileAt(12, 16)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 100 || tile.Build == nil || tile.Team != 2 {
		t.Fatalf("expected PrebuildAI to mine copper and construct router, block=%d build=%v team=%d", tile.Block, tile.Build != nil, tile.Team)
	}
	if len(w.teamAIBuildPlans[2]) != 0 {
		t.Fatalf("expected finished prebuild plan to be cleared from team queue, got %+v", w.teamAIBuildPlans[2])
	}
	got = findTestEntity(t, w, 9303)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected builder stack to be deposited after prebuild, got %+v", got.Stack)
	}
}

func TestPrebuildAIRemovedTeamPlanClearsEntityPlan(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        24,
		Flying:       true,
		BuildSpeed:   0.5,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.PrebuildAi = true

	core := placeTestBuilding(t, w, 3, 12, 339, 2, 0)
	core.Build.AddItem(0, 100)
	w.queueTeamBuildPlanFrontLocked(2, BuildPlanOp{X: 8, Y: 12, BlockID: 45})

	if _, err := w.AddEntityWithID(35, 9304, 6*8+4, 12*8+4, 2); err != nil {
		t.Fatalf("add ai alpha entity: %v", err)
	}
	if _, ok := w.SetEntityFlag(9304, float64(packTilePos(core.X, core.Y))); !ok {
		t.Fatal("bind ai alpha to core")
	}

	w.Step(time.Second / 60)
	got := findTestEntity(t, w, 9304)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(8, 12) {
		t.Fatalf("expected prebuild builder to pick queued team plan, got %+v", got.Plans)
	}

	delete(w.teamAIBuildPlans, TeamID(2))
	stepForSeconds(w, 1)

	got = findTestEntity(t, w, 9304)
	if len(got.Plans) != 0 {
		t.Fatalf("expected removed prebuild team plan to clear builder plan, got %+v", got.Plans)
	}
}

func TestPrebuildAICoreSpawnCreatesDedicatedBuilderPerCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		340: "core-foundation",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
		36: "beta",
	}
	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	rules.PrebuildAi = true

	placeTestBuilding(t, w, 5, 5, 339, 2, 0)
	placeTestBuilding(t, w, 10, 10, 339, 2, 0)
	placeTestBuilding(t, w, 16, 16, 340, 2, 0)

	w.Step(time.Second / 60)

	model = w.Model()
	if got := len(model.Entities); got != 3 {
		t.Fatalf("expected one dedicated core builder per core, got %d entities", got)
	}

	want := map[int16]map[float64]struct{}{
		35: {
			float64(packTilePos(5, 5)):   {},
			float64(packTilePos(10, 10)): {},
		},
		36: {
			float64(packTilePos(16, 16)): {},
		},
	}
	for _, ent := range model.Entities {
		flags, ok := want[ent.TypeID]
		if !ok {
			t.Fatalf("unexpected spawned core builder type=%d flag=%f", ent.TypeID, ent.Flag)
		}
		if !ent.SpawnedByCore {
			t.Fatalf("expected spawned core builder to be marked spawnedByCore, entity=%+v", ent)
		}
		if _, ok := flags[ent.Flag]; !ok {
			t.Fatalf("unexpected core binding for type=%d flag=%f", ent.TypeID, ent.Flag)
		}
		delete(flags, ent.Flag)
	}
	for typeID, flags := range want {
		if len(flags) != 0 {
			t.Fatalf("missing core builders for type=%d flags=%v", typeID, flags)
		}
	}
}

func TestPrebuildAICoreSpawnDoesNotShareBoundBuilderAcrossCores(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)

	rules := w.GetRulesManager().Get()
	rules.PrebuildAi = true

	placeTestBuilding(t, w, 4, 4, 339, 2, 0)
	placeTestBuilding(t, w, 12, 12, 339, 2, 0)

	if _, err := w.AddEntityWithID(35, 9401, 4*8+4, 4*8+4, 2); err != nil {
		t.Fatalf("add prebound alpha entity: %v", err)
	}
	if _, ok := w.SetEntityFlag(9401, float64(packTilePos(4, 4))); !ok {
		t.Fatal("bind alpha to first core")
	}

	w.Step(time.Second / 60)

	model = w.Model()
	if got := len(model.Entities); got != 2 {
		t.Fatalf("expected existing bound builder plus one missing builder, got %d entities", got)
	}

	seen := map[float64]struct{}{}
	for _, ent := range model.Entities {
		if ent.TypeID != 35 {
			t.Fatalf("expected only alpha builders, got type=%d", ent.TypeID)
		}
		seen[ent.Flag] = struct{}{}
	}
	if _, ok := seen[float64(packTilePos(4, 4))]; !ok {
		t.Fatal("expected first core builder to remain bound")
	}
	if _, ok := seen[float64(packTilePos(12, 12))]; !ok {
		t.Fatal("expected second core to receive its own bound builder")
	}
}

func TestBuildAIQueuesMechanicalDrillPlanNearOre(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		339: "core-shard",
		429: "mechanical-drill",
	}
	w.SetModel(model)
	paintAreaOverlay(t, w, 10, 8, 2, 2)
	placeTestBuilding(t, w, 8, 8, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.BuildAi = true
	rules.BuildAiTier = 1

	w.Step(time.Second / 60)

	plans := w.teamAIBuildPlans[2]
	if len(plans) == 0 {
		t.Fatal("expected buildAi to queue at least one team build plan")
	}
	if plans[0].BlockID != 429 {
		t.Fatalf("expected buildAi to queue mechanical drill first, got %+v", plans[0])
	}
	if x, y := int(plans[0].X), int(plans[0].Y); abs(x-8) > buildAISeedRangeTiles || abs(y-8) > buildAISeedRangeTiles {
		t.Fatalf("expected queued drill to stay near core seed, plan=%+v", plans[0])
	}
}

func TestBuildAIFillCoresRefillsPrimaryCoreWithoutFillItemsRule(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		343: "core-citadel",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 6, 6, 343, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true
	if rules.teamFillItems(2) {
		t.Fatal("expected fillItems rule to stay disabled for this buildAi fill-cores test")
	}

	pos := int32(core.Y*model.Width + core.X)
	capacity := w.itemCapacityAtLocked(pos)
	if capacity <= 0 {
		t.Fatalf("expected positive core capacity, got %d", capacity)
	}
	if got := core.Build.ItemAmount(copperItemID); got != 0 {
		t.Fatalf("expected empty core before buildAi fill, got copper=%d", got)
	}

	w.Step(time.Second / 60)

	if got := core.Build.ItemAmount(copperItemID); got != capacity {
		t.Fatalf("expected buildAi fill-cores to refill copper to %d, got %d", capacity, got)
	}
	if got := core.Build.ItemAmount(titaniumItemID); got != capacity {
		t.Fatalf("expected buildAi fill-cores to refill titanium to %d, got %d", capacity, got)
	}
}

func TestBuildAICoreSpawnSpawnsCoreUnitAfterOfficialInterval(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 8, 8, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true
	rules.AiCoreSpawn = true

	stepForSeconds(w, 6.2)

	if len(w.Model().Entities) != 1 {
		t.Fatalf("expected one core-spawned alpha after aiCoreSpawn interval, got %d", len(w.Model().Entities))
	}
	got := w.Model().Entities[0]
	if got.TypeID != 35 || got.Team != 2 {
		t.Fatalf("expected spawned alpha for team 2, got %+v", got)
	}
	if !got.SpawnedByCore {
		t.Fatalf("expected aiCoreSpawn unit to be marked spawnedByCore, got %+v", got)
	}
}

func TestBuildAIRefreshPathBuildsCorridorTowardEnemyCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 14)
	model.BlockNames = map[int16]string{
		1:   "spawn",
		339: "core-shard",
	}
	w.SetModel(model)
	model.Tiles[0].Overlay = 1
	model.Tiles[7*model.Width+23].Overlay = 1
	placeTestBuilding(t, w, 3, 7, 339, 1, 0)
	placeTestBuilding(t, w, 20, 7, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true

	w.Step(time.Second / 60)

	state := w.teamBuildAIStates[2]
	if len(state.PathCells) == 0 {
		t.Fatal("expected buildAi refresh path to generate a non-empty corridor")
	}
	_, roots, ok := w.buildAIEnemyCoreCellsLocked(2)
	if !ok || len(roots) == 0 {
		t.Fatal("expected enemy core roots for buildAi path test")
	}
	spawnX, spawnY, ok := w.buildAIPathSpawnCellLocked(roots)
	if !ok {
		t.Fatal("expected buildAi path spawn cell")
	}
	if _, ok := state.PathCells[packTilePos(spawnX, spawnY)]; !ok {
		t.Fatalf("expected buildAi corridor to include chosen spawn cell (%d,%d), path=%v", spawnX, spawnY, state.PathCells)
	}
	if _, ok := state.PathCells[packTilePos(3, 7)]; !ok {
		t.Fatalf("expected buildAi corridor to reach enemy core footprint, path=%v", state.PathCells)
	}
}

func TestBuildAITryPlaceAvoidsRefreshedPathCorridor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 14)
	model.BlockNames = map[int16]string{
		1:   "spawn",
		45:  "duo",
		339: "core-shard",
	}
	w.SetModel(model)
	model.Tiles[7*model.Width+23].Overlay = 1
	placeTestBuilding(t, w, 3, 7, 339, 1, 0)
	placeTestBuilding(t, w, 20, 7, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true

	w.Step(time.Second / 60)
	_, roots, ok := w.buildAIEnemyCoreCellsLocked(2)
	if !ok || len(roots) == 0 {
		t.Fatal("expected enemy core roots for path rejection test")
	}
	spawnX, spawnY, ok := w.buildAIPathSpawnCellLocked(roots)
	if !ok {
		t.Fatal("expected buildAi path spawn cell for path rejection test")
	}

	part := buildAIBasePart{
		Name:   "manual-solid-path-test",
		Width:  1,
		Height: 1,
		Tiles: []buildAIBasePartTile{{
			BlockName: "duo",
			BlockID:   45,
		}},
	}
	if w.queueBuildAIPartAtSeedLocked(2, part, spawnX, spawnY, 0) {
		t.Fatal("expected buildAi tryPlace to reject solid block placement intersecting refreshed path corridor")
	}
	if len(w.teamAIBuildPlans[2]) != 0 {
		t.Fatalf("expected path-rejected tryPlace to avoid queueing plans, got %+v", w.teamAIBuildPlans[2])
	}
}

func TestBuildAIPathSpawnUsesLastOfficialSpawnOverlay(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 14)
	model.BlockNames = map[int16]string{
		1: "spawn",
	}
	w.SetModel(model)
	model.Tiles[1*model.Width+1].Overlay = 1
	model.Tiles[7*model.Width+23].Overlay = 1

	spawnX, spawnY, ok := w.buildAIPathSpawnCellLocked(nil)
	if !ok {
		t.Fatal("expected buildAi path spawn cell from official spawn overlay")
	}
	if spawnX != 23 || spawnY != 7 {
		t.Fatalf("expected buildAi path to use the last official spawn overlay, got (%d,%d)", spawnX, spawnY)
	}
}

func TestBuildAIPathSpawnAppendsAttackWaveCoreSpawnsAfterOverlaySpawns(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 16)
	model.BlockNames = map[int16]string{
		1:   "spawn",
		339: "core-shard",
	}
	w.SetModel(model)
	model.Tiles[7*model.Width+31].Overlay = 1
	placeTestBuilding(t, w, 4, 7, 339, 1, 0)
	placeTestBuilding(t, w, 20, 7, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.WavesSpawnAtCores = true

	spawnX, spawnY, ok := w.buildAIPathSpawnCellLocked(nil)
	if !ok {
		t.Fatal("expected buildAi path spawn cell to include official attack-mode wave core spawns")
	}
	if spawnX != 15 || spawnY != 7 {
		t.Fatalf("expected buildAi path to append wave-core ground spawn after overlays and choose (15,7), got (%d,%d)", spawnX, spawnY)
	}
}

func TestBuildAIOverlaySpawnRadiusCheckDoesNotUseWaveCoreSpawns(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 16)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 4, 7, 339, 1, 0)
	placeTestBuilding(t, w, 20, 7, 339, 2, 0)

	rules := w.GetRulesManager().Get()
	rules.AttackMode = true
	rules.WavesSpawnAtCores = true

	if w.buildAITileNearGroundSpawnLocked(15, 7, buildAISpawnProtectRadiusTiles) {
		t.Fatal("expected buildAi 40-tile spawn protection to stay tied to official overlay spawns, not attack-mode wave-core spawn points")
	}
}

func TestBuildAIPathBlockingUsesOfficialSolidSemantics(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(16, 16))
	w.teamBuildAIStates = map[TeamID]buildAIPlannerState{
		2: {
			PathCells: map[int32]struct{}{
				packTilePos(5, 5): {},
			},
		},
	}

	if !w.buildAIPlanIntersectsPathLocked(2, 5, 5, "bridge-conveyor") {
		t.Fatal("expected official solid bridge-conveyor to block buildAi path corridor")
	}
	if w.buildAIPlanIntersectsPathLocked(2, 5, 5, "router") {
		t.Fatal("expected official non-solid router to be ignored by buildAi path corridor checks")
	}
	if w.buildAIPlanIntersectsPathLocked(2, 5, 5, "payload-conveyor") {
		t.Fatal("expected official non-solid payload-conveyor to be ignored by buildAi path corridor checks")
	}
}

func TestBuildAIDrillFallbackAvoidsOfficialSpawnRadius(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(80, 20)
	model.BlockNames = map[int16]string{
		1:   "spawn",
		2:   "ore-copper",
		339: "core-shard",
		429: "mechanical-drill",
	}
	w.SetModel(model)
	model.Tiles[10*model.Width+5].Overlay = 1
	paintAreaOverlay(t, w, 35, 10, 2, 2)
	paintAreaOverlay(t, w, 55, 10, 2, 2)
	placeTestBuilding(t, w, 45, 10, 339, 2, 0)

	op, ok := w.findBuildAIDrillPlanLocked(2, 45, 10)
	if !ok {
		t.Fatal("expected buildAi drill fallback to find a valid ore tile outside the official spawn radius")
	}
	if op.X != 54 || op.Y != 10 {
		t.Fatalf("expected buildAi drill fallback to skip spawn-adjacent ore and choose (54,10), got (%d,%d)", op.X, op.Y)
	}
}

func TestBuildAIQueuesOfficialBasePartIntoIndependentTeamPlans(t *testing.T) {
	raw := pickCopperBasePartSchematicForTest(t)

	w := New(Config{TPS: 60})
	model := NewWorldModel(96, 96)
	model.BlockNames = map[int16]string{
		2:   "ore-copper",
		339: "core-shard",
	}
	nextID := int16(1000)
	for _, tile := range raw.Tiles {
		name := normalizeBlockLookupName(tile.Block)
		switch name {
		case "itemsource", "liquidsource", "powersource", "powervoid", "payloadsource", "payloadvoid", "heatsource":
			continue
		}
		known := false
		for _, existing := range model.BlockNames {
			if existing == tile.Block {
				known = true
				break
			}
		}
		if known {
			continue
		}
		model.BlockNames[nextID] = tile.Block
		nextID++
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 12, 12, 339, 2, 0)

	part, ok := w.convertBuildAIBasePartLocked(raw)
	if !ok {
		t.Fatalf("expected official copper basepart %q to convert into buildAi part", raw.Name)
	}
	if len(part.Tiles) < 2 {
		t.Fatalf("expected converted official basepart to stay multi-tile, got %d", len(part.Tiles))
	}

	seedX, seedY := 48, 48
	cx := seedX - part.CenterX
	cy := seedY - part.CenterY
	for _, tile := range part.Tiles {
		if !buildAIBasePartCountsForCenter(tile.BlockName) {
			continue
		}
		paintAreaOverlay(t, w, tile.X+cx, tile.Y+cy, blockSizeByName(tile.BlockName), 2)
	}

	if !w.queueBuildAIPartAtSeedLocked(2, part, seedX, seedY, 0) {
		t.Fatalf("expected official basepart %q to queue successfully", raw.Name)
	}

	plans := w.teamAIBuildPlans[2]
	if len(plans) != len(part.Tiles) {
		t.Fatalf("expected one independent team plan per basepart tile, want=%d got=%d", len(part.Tiles), len(plans))
	}
	seen := map[int32]struct{}{}
	for _, plan := range plans {
		pos := packTilePos(int(plan.X), int(plan.Y))
		if _, ok := seen[pos]; ok {
			t.Fatalf("expected basepart queue to keep distinct tile plans, got duplicate pos=%d plans=%+v", pos, plans)
		}
		seen[pos] = struct{}{}
	}
}

func TestBuilderAIConsumesTeamBuildPlansWhenBuildAIEnabled(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 5, 5, 339, 1, 0).Build.AddItem(0, 100)
	w.queueTeamBuildPlanBackLocked(1, BuildPlanOp{X: 9, Y: 5, BlockID: 45})

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true
	rules.BuildAiTier = 1

	if _, err := w.AddEntityWithID(35, 9501, 5*8+4, 5*8+4, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}

	w.Step(time.Second / 60)

	got := findTestEntity(t, w, 9501)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(9, 5) {
		t.Fatalf("expected buildAi-enabled builder to consume team build plan, got %+v", got.Plans)
	}
}

func TestBuilderAIBuildAiModeUsesTeamPlanQueueInsteadOfRebuildQueue(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		45:  "duo",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	core := placeTestBuilding(t, w, 4, 4, 339, 1, 0)
	core.Build.AddItem(0, 1000)
	placeTestBuilding(t, w, 10, 10, 45, 1, 0)
	if !w.DamageBuildingPacked(packTilePos(10, 10), 2000) {
		t.Fatal("expected destroyed building to enter rebuild queues")
	}

	rules := w.GetRulesManager().Get()
	rules.BuildAi = true
	rules.BuildAiTier = 1

	if _, err := w.AddEntityWithID(35, 9502, 6*8+4, 4*8+4, 1); err != nil {
		t.Fatalf("add alpha entity: %v", err)
	}

	w.Step(time.Second / 60)
	got := findTestEntity(t, w, 9502)
	if len(got.Plans) == 0 || got.Plans[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected buildAi-mode builder to pick mirrored team plan, got %+v", got.Plans)
	}

	delete(w.teamAIBuildPlans, TeamID(1))
	stepForSeconds(w, 1)

	got = findTestEntity(t, w, 9502)
	if len(got.Plans) != 0 {
		t.Fatalf("expected removing team build queue to clear buildAi-mode builder plan even if rebuild queue remains, got %+v", got.Plans)
	}
	if len(w.teamRebuildPlans[1]) == 0 {
		t.Fatal("expected dedicated rebuild queue to remain untouched by buildAi-mode validation test")
	}
}

func TestTriggerWaveSpawnsAtEdgeAndAdvances(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		0: "dagger",
		1: "dagger",
		2: "dagger",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["dagger"] = unitRuntimeProfile{Name: "dagger", Speed: 24}

	core := placeTestBuilding(t, w, 3, 12, 339, 1, 0)
	w.wavesMgr = NewWaveManager(&WaveConfig{
		InitialSpacingSec:  1,
		BaseSpacingSec:     1,
		EnemyBaseCount:     1,
		EnemyGrowthFactor:  0,
		MaxEnemiesPerGroup: 1,
		EnemyTypes:         []int16{1},
	})

	w.triggerWave(w.wavesMgr)
	if len(w.Model().Entities) != 1 {
		t.Fatalf("expected one wave unit to spawn, got %d", len(w.Model().Entities))
	}
	spawned := w.Model().Entities[0]
	if spawned.X < float32(w.Model().Width*8)*0.5 {
		t.Fatalf("expected wave spawn to use the far edge from the player core, got x=%f", spawned.X)
	}

	coreX := float32(core.X*8 + 4)
	coreY := float32(core.Y*8 + 4)
	before := float32(math.Hypot(float64(spawned.X-coreX), float64(spawned.Y-coreY)))
	stepForSeconds(w, 3)
	got := findTestEntity(t, w, spawned.ID)
	after := float32(math.Hypot(float64(got.X-coreX), float64(got.Y-coreY)))
	if after >= before-12 {
		t.Fatalf("expected wave unit to advance after spawning, before=%f after=%f", before, after)
	}
}

func TestRepairPointHealsDamagedFriendlyUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		478: "power-source",
		900: "repair-point",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 8, 8, 900, 1, 0)
	placeTestBuilding(t, w, 8, 10, 478, 1, 0)
	linkPowerNode(t, w, 8, 10, protocol.Point2{X: 0, Y: -2})

	unit := w.Model().AddEntity(RawEntity{
		ID:        5001,
		TypeID:    35,
		Team:      1,
		X:         float32(10*8 + 4),
		Y:         float32(8*8 + 4),
		Health:    40,
		MaxHealth: 100,
	})

	stepForSeconds(w, 4)

	got := findTestEntity(t, w, unit.ID)
	if got.Health <= 40 {
		pos := int32(8*model.Width + 8)
		t.Fatalf("expected repair-point to heal damaged unit, got=%f state=%+v power=%f targets=%v", got.Health, w.repairTurretStates[pos], w.blockSyncPowerStatusLocked(pos, &w.Model().Tiles[pos], "repair-point"), w.repairTurretTilePositions)
	}
}

func TestRepairTurretBlockSyncIncludesHeadRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		901: "repair-turret",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 5, 901, 1, 0)
	pos := int32(5*model.Width + 5)
	w.repairTurretStates[pos] = repairTurretRuntimeState{Rotation: 37.5}

	snaps := w.BlockSyncSnapshotsForPacked([]int32{packTilePos(5, 5)})
	if len(snaps) != 1 {
		t.Fatalf("expected one repair-turret snapshot, got %d", len(snaps))
	}
	_, r := decodeBlockSyncBase(t, snaps[0].Data)
	rot, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read repair turret rotation failed: %v", err)
	}
	if math.Abs(float64(rot-37.5)) > 0.001 {
		t.Fatalf("expected repair turret sync rotation=37.5, got=%f", rot)
	}
}

func TestBlockSyncSnapshotsEncodeItemTurretRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	tile := placeTestBuilding(t, w, 5, 5, 910, 2, 1)
	pos := int32(5*model.Width + 5)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 12}}
	w.buildStates[pos] = buildCombatState{Cooldown: 0.25}
	w.rebuildActiveTilesLocked()

	snaps := w.ItemTurretBlockSyncSnapshotsForPacked([]int32{packTilePos(5, 5)})
	if len(snaps) != 1 {
		t.Fatalf("expected one turret block sync snapshot, got %d", len(snaps))
	}

	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & 1) == 0 {
		t.Fatalf("expected item turret to keep the vanilla base item module bit, bits=%08b", base.ModuleBits)
	}
	if len(base.Items) != 0 {
		t.Fatalf("expected item turret base item module to stay empty, got %v", base.Items)
	}
	if (base.ModuleBits & (1 << 2)) == 0 {
		t.Fatalf("expected item turret to keep the vanilla base liquid module bit, bits=%08b", base.ModuleBits)
	}
	if len(base.Liquids) != 0 {
		t.Fatalf("expected item turret base liquid module to stay empty, got %v", base.Liquids)
	}
	reloadCounter, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read turret reload counter failed: %v", err)
	}
	rot, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read turret rotation failed: %v", err)
	}
	ammoKinds, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read turret ammo count failed: %v", err)
	}
	if math.Abs(float64(reloadCounter-0.35)) > 0.001 {
		t.Fatalf("expected turret reload counter 0.35, got %f", reloadCounter)
	}
	if math.Abs(float64(rot-90)) > 0.001 {
		t.Fatalf("expected turret rotation 90, got %f", rot)
	}
	if ammoKinds != 1 {
		t.Fatalf("expected one ammo entry, got %d", ammoKinds)
	}
	ammoItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read turret ammo item failed: %v", err)
	}
	ammoAmount, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read turret ammo amount failed: %v", err)
	}
	if ammoItem != int16(copperItemID) || ammoAmount != 12 {
		t.Fatalf("expected copper ammo x12, got item=%d amount=%d", ammoItem, ammoAmount)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected item turret sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestBlockSyncSnapshotsEncodePowerTurretRuntime(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		478: "power-source",
		911: "arc",
	}
	w.SetModel(model)
	w.buildingProfilesByName["arc"] = buildingWeaponProfile{
		ClassName:     "PowerTurret",
		Range:         88,
		Damage:        24,
		Interval:      0.42,
		PowerCapacity: 140,
		PowerPerShot:  30,
		TargetAir:     true,
		TargetGround:  true,
		HitBuildings:  true,
		ChainCount:    2,
		ChainRange:    32,
	}

	placeTestBuilding(t, w, 8, 9, 478, 2, 0)
	placeTestBuilding(t, w, 8, 8, 911, 2, 0)
	linkPowerNode(t, w, 8, 9, protocol.Point2{X: 0, Y: -1})
	pos := int32(8*model.Width + 8)
	w.buildStates[pos] = buildCombatState{Cooldown: 0.12, Power: 75}
	w.rebuildActiveTilesLocked()

	snaps := w.BlockSyncSnapshotsForPacked([]int32{packTilePos(8, 8)})
	if len(snaps) != 1 {
		t.Fatalf("expected one power turret block sync snapshot, got %d", len(snaps))
	}

	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if (base.ModuleBits & (1 << 1)) == 0 {
		t.Fatalf("expected power turret to include power module, bits=%08b", base.ModuleBits)
	}
	reloadCounter, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read power turret reload counter failed: %v", err)
	}
	rot, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read power turret rotation failed: %v", err)
	}
	if math.Abs(float64(reloadCounter-0.30)) > 0.001 {
		t.Fatalf("expected power turret reload counter 0.30, got %f", reloadCounter)
	}
	if math.Abs(float64(rot-0)) > 0.001 {
		t.Fatalf("expected power turret rotation 0, got %f", rot)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected power turret sync payload to be fully consumed, remaining=%d", rem)
	}
}

func TestPeriodicBlockSyncSnapshotsSkipAllTurrets(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		910: "duo",
		911: "arc",
		912: "container",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	w.buildingProfilesByName["arc"] = buildingWeaponProfile{
		ClassName:     "PowerTurret",
		Range:         88,
		Damage:        24,
		Interval:      0.42,
		PowerCapacity: 140,
		PowerPerShot:  30,
		TargetAir:     true,
		TargetGround:  true,
		HitBuildings:  true,
	}

	duo := placeTestBuilding(t, w, 5, 5, 910, 2, 1)
	duo.Build.Items = []ItemStack{{Item: copperItemID, Amount: 12}}
	arc := placeTestBuilding(t, w, 8, 8, 911, 2, 0)
	_ = arc
	container := placeTestBuilding(t, w, 10, 10, 912, 2, 0)
	container.Build.Items = []ItemStack{{Item: copperItemID, Amount: 5}}
	w.rebuildActiveTilesLocked()

	snaps := w.PeriodicBlockSyncSnapshotsLiveOnly()
	if len(snaps) != 1 {
		t.Fatalf("expected only the non-turret building in periodic snapshots, got %d", len(snaps))
	}
	if snaps[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected periodic snapshot to keep container only, got pos=%d", snaps[0].Pos)
	}

	targeted := w.BlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(5, 5)})
	if len(targeted) != 0 {
		t.Fatalf("expected generic targeted snapshots to skip item turrets, got %d", len(targeted))
	}

	targeted = w.BlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(8, 8)})
	if len(targeted) != 1 {
		t.Fatalf("expected generic targeted snapshots to keep power turrets, got %d", len(targeted))
	}
	if targeted[0].Pos != packTilePos(8, 8) {
		t.Fatalf("expected generic targeted snapshot to keep arc, got pos=%d", targeted[0].Pos)
	}

	targeted = w.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(5, 5)})
	if len(targeted) != 1 {
		t.Fatalf("expected targeted duo snapshot to remain available, got %d", len(targeted))
	}
	if targeted[0].Pos != packTilePos(5, 5) {
		t.Fatalf("expected targeted snapshot to keep duo, got pos=%d", targeted[0].Pos)
	}
}

func TestPeriodicBlockSyncSnapshotsSkipConveyorsButTargetedKeepsThem(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		257: "conveyor",
		912: "container",
	}
	w.SetModel(model)
	conveyor := placeTestBuilding(t, w, 5, 5, 257, 2, 1)
	conveyor.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	conveyorPos := int32(conveyor.Y*w.Model().Width + conveyor.X)
	w.conveyorStates[conveyorPos] = &conveyorRuntimeState{
		IDs: [3]ItemID{copperItemID},
		XS:  [3]float32{1},
		YS:  [3]float32{0.25},
		Len: 1,
	}
	container := placeTestBuilding(t, w, 10, 10, 912, 2, 0)
	container.Build.Items = []ItemStack{{Item: copperItemID, Amount: 5}}
	w.rebuildActiveTilesLocked()

	snaps := w.PeriodicBlockSyncSnapshotsLiveOnly()
	if len(snaps) != 1 {
		t.Fatalf("expected periodic snapshots to skip conveyors and keep container only, got %d", len(snaps))
	}
	if snaps[0].Pos != packTilePos(10, 10) {
		t.Fatalf("expected periodic snapshot to keep container only, got pos=%d", snaps[0].Pos)
	}

	targeted := w.BlockSyncSnapshotsForPackedLiveOnly([]int32{protocol.PackPoint2(5, 5)})
	if len(targeted) != 1 {
		t.Fatalf("expected targeted live-only snapshots to keep conveyor runtime, got %d", len(targeted))
	}
	if targeted[0].Pos != protocol.PackPoint2(5, 5) {
		t.Fatalf("expected targeted live-only conveyor snapshot at %d, got %d", protocol.PackPoint2(5, 5), targeted[0].Pos)
	}
}

func TestBlockSyncSnapshotsEncodeTurretHeadRotation(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	tile := placeTestBuilding(t, w, 5, 5, 910, 2, 1)
	pos := int32(5*model.Width + 5)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 12}}
	w.buildStates[pos] = buildCombatState{
		Cooldown:       0.25,
		TurretRotation: 37.5,
		HasRotation:    true,
	}
	w.rebuildActiveTilesLocked()

	snaps := w.ItemTurretBlockSyncSnapshotsForPacked([]int32{packTilePos(5, 5)})
	if len(snaps) != 1 {
		t.Fatalf("expected one turret block sync snapshot, got %d", len(snaps))
	}

	_, r := decodeBlockSyncBase(t, snaps[0].Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read turret reload counter failed: %v", err)
	}
	rot, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read turret head rotation failed: %v", err)
	}
	if math.Abs(float64(rot-37.5)) > 0.001 {
		t.Fatalf("expected turret head rotation 37.5, got %f", rot)
	}
}

func TestMergeBuildingProfileIncludesTurretAimFields(t *testing.T) {
	p := buildingWeaponProfile{}
	mergeBuildingProfile(&p, vanillaTurretProfile{
		Rotate:               true,
		RotateSpeed:          7,
		BaseRotation:         15,
		PredictTarget:        true,
		TargetInterval:       0.4,
		TargetSwitchInterval: 0.7,
		ShootCone:            12,
		RotationLimit:        180,
	})
	if !p.Rotate || p.RotateSpeed != 7 || p.BaseRotation != 15 || !p.PredictTarget {
		t.Fatalf("expected turret aim fields to merge, got %+v", p)
	}
	if p.TargetInterval != 0.4 || p.TargetSwitchInterval != 0.7 || p.ShootCone != 12 || p.RotationLimit != 180 {
		t.Fatalf("expected turret targeting timings to merge, got %+v", p)
	}
}

func TestBuildingTurretRotationTurnsGraduallyBeforeFiring(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:      "ItemTurret",
		FireMode:       "projectile",
		Range:          136,
		Damage:         9,
		Interval:       0.1,
		BulletType:     94,
		BulletSpeed:    60,
		HitBuildings:   true,
		TargetAir:      true,
		TargetGround:   true,
		Rotate:         true,
		RotateSpeed:    5,
		PredictTarget:  false,
		ShootCone:      5,
		AmmoCapacity:   80,
		AmmoPerShot:    1,
		TargetInterval: 0.2,
	}

	tile := placeTestBuilding(t, w, 10, 10, 910, 1, 0)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 12}}
	w.rebuildActiveTilesLocked()
	pos := int32(10*model.Width + 10)

	w.Model().AddEntity(RawEntity{
		ID:                  1,
		TypeID:              35,
		Team:                2,
		X:                   float32(10*8 + 4),
		Y:                   float32(15*8 + 4),
		Health:              100,
		MaxHealth:           100,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
	})

	stepWorldFrames(w, 1)

	state := w.buildStates[pos]
	if math.Abs(float64(state.TurretRotation-5)) > 0.001 {
		t.Fatalf("expected turret to rotate by 5 degrees on first frame, got %f", state.TurretRotation)
	}
	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected turret not to fire before lining up with target, bullets=%d", got)
	}

	stepWorldFrames(w, 17)

	state = w.buildStates[pos]
	if state.TurretRotation < 89.9 {
		t.Fatalf("expected turret to finish turning toward 90 degrees, got %f", state.TurretRotation)
	}
	if got := len(w.bullets); got == 0 {
		t.Fatalf("expected turret to fire after finishing rotation")
	}
}

func TestCloneModelForWorldStreamBuildsConveyorPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 257, 1, 0)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	w.conveyorStates[pos] = &conveyorRuntimeState{
		IDs: [3]ItemID{copperItemID},
		XS:  [3]float32{1},
		YS:  [3]float32{0.5},
		Len: 1,
	}
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected conveyor world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected conveyor world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsItemTurretPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	tile := placeTestBuilding(t, w, 5, 5, 910, 2, 1)
	pos := int32(5*model.Width + 5)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 12}}
	w.buildStates[pos] = buildCombatState{
		Cooldown:       0.25,
		TurretRotation: 37.5,
		HasRotation:    true,
	}
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(5, 5)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 2 {
		t.Fatalf("expected item turret world-stream revision 2, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected item turret world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsPayloadRouterPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		701: "payload-router",
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 701, 1, 0)
	pos := int32(8*model.Width + 8)
	payload := &payloadData{Kind: payloadKindBlock, BlockID: 257}
	w.payloadStateLocked(pos).Payload = payload
	w.payloadStateLocked(pos).RecDir = 2
	w.syncPayloadTileLocked(tile, payload)
	w.ConfigureBuildingPacked(protocol.PackPoint2(8, 8), protocol.BlockRef{BlkID: 257})
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(8, 8)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected payload router world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected payload router world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsPayloadLoaderPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		703: "payload-loader",
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 703, 1, 0)
	pos := int32(8*model.Width + 8)
	payload := &payloadData{Kind: payloadKindBlock, BlockID: 257}
	w.payloadStateLocked(pos).Payload = payload
	w.payloadStateLocked(pos).Exporting = true
	w.syncPayloadTileLocked(tile, payload)
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(8, 8)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected payload loader world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected payload loader world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsPowerNodePayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		421: "battery",
	}
	w.SetModel(model)
	node := placeTestBuilding(t, w, 8, 10, 422, 1, 0)
	placeTestBuilding(t, w, 14, 10, 421, 1, 0)
	nodePos := int32(node.Y*w.Model().Width + node.X)
	w.applyBuildingConfigLocked(nodePos, []protocol.Point2{{X: 6, Y: 0}}, true)
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(8, 10)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 0 {
		t.Fatalf("expected power node world-stream revision 0, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected power node world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsDuctPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		440: "duct",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 440, 1, 0)
	pos := int32(6*model.Width + 6)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	st := w.ductStateLocked(pos, tile)
	st.Current = copperItemID
	st.HasItem = true
	st.RecDir = 2
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected duct world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected duct world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsDuctRouterPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		446: "duct-router",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 6, 6, 446, 1, 0)
	pos := int32(6*model.Width + 6)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 1}}
	w.sorterCfg[pos] = copperItemID
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected duct-router world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected duct-router world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsSorterPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		262: "sorter",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 6, 262, 1, 0)
	pos := int32(6*model.Width + 6)
	w.sorterCfg[pos] = copperItemID
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 2 {
		t.Fatalf("expected sorter world-stream revision 2, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected sorter world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsOverflowGatePayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		265: "overflow-gate",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 6, 265, 1, 0)
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 4 {
		t.Fatalf("expected overflow-gate world-stream revision 4, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected overflow-gate world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsPhaseConveyorPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		262: "phase-conveyor",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 6, 262, 1, 0)
	placeTestBuilding(t, w, 10, 6, 262, 1, 0)
	pos := int32(6*model.Width + 6)
	target := int32(6*model.Width + 10)
	w.bridgeLinks[pos] = target
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected phase conveyor world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected phase conveyor world-stream payload bytes")
	}
	_, r := decodeBlockSyncBase(t, tileClone.Build.MapSyncData)
	link, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read phase conveyor link failed: %v", err)
	}
	if want := packTilePos(10, 6); link != want {
		t.Fatalf("expected phase conveyor link %d, got %d", want, link)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read phase conveyor warmup failed: %v", err)
	}
	if math.Abs(float64(warmup-1)) > 0.0001 {
		t.Fatalf("expected linked phase conveyor warmup 1, got %f", warmup)
	}
	incoming, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read phase conveyor incoming count failed: %v", err)
	}
	if incoming != 0 {
		t.Fatalf("expected no incoming phase conveyors, got %d", incoming)
	}
	moved, err := r.ReadBool()
	if err != nil {
		t.Fatalf("read phase conveyor moved flag failed: %v", err)
	}
	if moved {
		t.Fatal("expected idle phase conveyor payload to report unmoved state")
	}
	if remaining := r.Remaining(); remaining != 0 {
		t.Fatalf("expected phase conveyor payload to be fully consumed, got %d trailing bytes", remaining)
	}
}

func TestCloneModelForWorldStreamBuildsItemSourcePayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		412: "item-source",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 4, 4, 412, 1, 0)
	pos := int32(4*model.Width + 4)
	w.ConfigureItemSource(pos, thoriumItemID)

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(4, 4)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 0 {
		t.Fatalf("expected item source world-stream revision 0, got %d", tileClone.Build.MapSyncRevision)
	}
	_, r := decodeBlockSyncBase(t, tileClone.Build.MapSyncData)
	itemID, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read item source item id failed: %v", err)
	}
	if itemID != int16(thoriumItemID) {
		t.Fatalf("expected item source item %d, got %d", thoriumItemID, itemID)
	}
}

func TestCloneModelForWorldStreamBuildsDrillPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		429: "mechanical-drill",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 5, 5, 429, 1, 0)
	pos := int32(5*model.Width + 5)
	w.drillStates[pos] = drillRuntimeState{
		Progress: 33.5,
		Warmup:   0.75,
	}

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(5, 5)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected drill world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	_, r := decodeBlockSyncBase(t, tileClone.Build.MapSyncData)
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill warmup failed: %v", err)
	}
	if math.Abs(float64(progress-33.5)) > 0.0001 {
		t.Fatalf("expected drill progress 33.5, got %f", progress)
	}
	if math.Abs(float64(warmup-0.75)) > 0.0001 {
		t.Fatalf("expected drill warmup 0.75, got %f", warmup)
	}
}

func TestCloneModelForWorldStreamBuildsGeneratorPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		308: "combustion-generator",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 6, 6, 308, 1, 0)
	pos := int32(6*model.Width + 6)
	w.powerGeneratorState[pos] = &powerGeneratorState{
		FuelFrames: 90,
	}

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(6, 6)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected generator world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	_, r := decodeBlockSyncBase(t, tileClone.Build.MapSyncData)
	productionEfficiency, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read generator productionEfficiency failed: %v", err)
	}
	generateTime, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read generator generateTime failed: %v", err)
	}
	if math.Abs(float64(productionEfficiency-1)) > 0.0001 {
		t.Fatalf("expected generator productionEfficiency 1, got %f", productionEfficiency)
	}
	if math.Abs(float64(generateTime-90)) > 0.0001 {
		t.Fatalf("expected generator generateTime 90, got %f", generateTime)
	}
}

func TestCloneModelForWorldStreamMarksMultiblockEdges(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		703: "payload-loader",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 8, 8, 703, 1, 0)
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	centerTile, err := clone.TileAt(8, 8)
	if err != nil || centerTile == nil || centerTile.Build == nil {
		t.Fatalf("center tile lookup failed: %v", err)
	}
	edgeTile, err := clone.TileAt(7, 8)
	if err != nil || edgeTile == nil {
		t.Fatalf("edge tile lookup failed: %v", err)
	}
	if edgeTile.Block != 703 {
		t.Fatalf("expected multiblock edge to mirror center block 703, got %d", edgeTile.Block)
	}
	if edgeTile.Build == nil {
		t.Fatal("expected multiblock edge to share center build for world stream encoding")
	}
	if edgeTile.Build != centerTile.Build {
		t.Fatal("expected multiblock edge to reference center build")
	}
}

func TestCloneModelForWorldStreamBuildsPayloadMassDriverPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		702: "payload-mass-driver",
		257: "conveyor",
	}
	w.SetModel(model)
	tile := placeTestBuilding(t, w, 8, 8, 702, 1, 0)
	pos := int32(8*model.Width + 8)
	target := placeTestBuilding(t, w, 14, 8, 702, 1, 0)
	targetPos := int32(target.Y*model.Width + target.X)
	payload := &payloadData{Kind: payloadKindBlock, BlockID: 257}
	w.payloadStateLocked(pos).Payload = payload
	w.payloadDriverLinks[pos] = targetPos
	w.payloadDriverStateLocked(pos).Charge = 12
	w.syncPayloadTileLocked(tile, payload)
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(8, 8)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 1 {
		t.Fatalf("expected payload mass driver world-stream revision 1, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected payload mass driver world-stream payload bytes")
	}
}

func TestCloneModelForWorldStreamBuildsPayloadDeconstructorPayload(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		705: "payload-deconstructor",
		257: "conveyor",
	}
	w.SetModel(model)
	placeTestBuilding(t, w, 8, 8, 705, 1, 0)
	pos := int32(8*model.Width + 8)
	state := w.payloadDeconstructorStateLocked(pos)
	state.Progress = 0.5
	state.Accum = []float32{1.25, 0.5}
	state.Deconstructing = &payloadData{Kind: payloadKindBlock, BlockID: 257}
	w.rebuildActiveTilesLocked()

	clone := w.CloneModelForWorldStream()
	if clone == nil {
		t.Fatal("expected world stream model clone")
	}
	tileClone, err := clone.TileAt(8, 8)
	if err != nil || tileClone == nil || tileClone.Build == nil {
		t.Fatalf("clone tile lookup failed: %v", err)
	}
	if tileClone.Build.MapSyncRevision != 0 {
		t.Fatalf("expected payload deconstructor world-stream revision 0, got %d", tileClone.Build.MapSyncRevision)
	}
	if len(tileClone.Build.MapSyncData) == 0 {
		t.Fatal("expected payload deconstructor world-stream payload bytes")
	}
}

func TestDerelictTurretDoesNotAttack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       12,
		Interval:     0.1,
		BulletSpeed:  60,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	placeTestBuilding(t, w, 10, 10, 910, 0, 0)
	w.rebuildActiveTilesLocked()
	w.Model().AddEntity(RawEntity{
		ID:                  1,
		TypeID:              35,
		Team:                1,
		X:                   float32(13*8 + 4),
		Y:                   float32(10*8 + 4),
		Health:              100,
		MaxHealth:           100,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
	})

	stepForSeconds(w, 1)

	if got := findTestEntity(t, w, 1).Health; math.Abs(float64(got-100)) > 0.001 {
		t.Fatalf("expected derelict turret to stay idle, target health=%f", got)
	}
}

func TestTargetBuildsTurretPrioritizesBuilding(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(32, 32)
	model.BlockNames = map[int16]string{
		950: "scathe",
		600: "copper-wall",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.buildingProfilesByName["scathe"] = buildingWeaponProfile{
		ClassName:    "PowerTurret",
		Range:        240,
		Damage:       25,
		Interval:     1.0,
		BulletSpeed:  80,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
		TargetBuilds: true,
	}

	placeTestBuilding(t, w, 10, 10, 950, 2, 0)
	target := placeTestBuilding(t, w, 14, 10, 600, 1, 0)
	target.Build.Health = 100
	target.Build.MaxHealth = 100
	w.rebuildActiveTilesLocked()
	w.Model().AddEntity(RawEntity{
		ID:                  1,
		TypeID:              35,
		Team:                1,
		X:                   float32(10*8 + 4),
		Y:                   float32(14*8 + 4),
		Health:              100,
		MaxHealth:           100,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
	})

	stepForSeconds(w, 0.5)

	if target.Build != nil && target.Build.Health >= 100 {
		t.Fatalf("expected target-builds turret to damage building first, health=%f", target.Build.Health)
	}
	if got := findTestEntity(t, w, 1).Health; math.Abs(float64(got-100)) > 0.001 {
		t.Fatalf("expected off-axis unit to be ignored while turret targets building, health=%f", got)
	}
}

func TestUnitRepairTowerHealsMultipleUnits(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		478: "power-source",
		902: "unit-repair-tower",
	}
	w.SetModel(model)

	tower := placeTestBuilding(t, w, 12, 12, 902, 1, 0)
	placeTestBuilding(t, w, 12, 15, 478, 1, 0)
	tower.Build.AddLiquid(ozoneLiquidID, 30)
	linkPowerNode(t, w, 12, 15, protocol.Point2{X: 0, Y: -3})

	first := w.Model().AddEntity(RawEntity{ID: 5002, TypeID: 35, Team: 1, X: float32(10*8 + 4), Y: float32(12*8 + 4), Health: 30, MaxHealth: 100})
	second := w.Model().AddEntity(RawEntity{ID: 5003, TypeID: 35, Team: 1, X: float32(14*8 + 4), Y: float32(12*8 + 4), Health: 45, MaxHealth: 100})

	stepForSeconds(w, 3)

	gotFirst := findTestEntity(t, w, first.ID)
	gotSecond := findTestEntity(t, w, second.ID)
	if gotFirst.Health <= 30 || gotSecond.Health <= 45 {
		t.Fatalf("expected unit-repair-tower to heal both units, got=(%f,%f)", gotFirst.Health, gotSecond.Health)
	}
}

func TestSuppressionFieldAbilitySuppresssEnemyMendProjectorAndExpires(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		910: "mend-projector",
		911: "container",
	}
	model.UnitNames = map[int16]string{
		950: "suppressor",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["suppressor"] = unitRuntimeProfile{
		Name: "suppressor",
		Abilities: []unitAbilityProfile{{
			Kind:     unitAbilitySuppressionField,
			Active:   true,
			Reload:   1.5,
			Cooldown: 1.5,
			Range:    200,
		}},
	}

	projector := placeTestBuilding(t, w, 12, 12, 910, 2, 0)
	target := placeTestBuilding(t, w, 14, 12, 911, 2, 0)
	target.Build.Health = 400
	target.Build.MaxHealth = 1000

	w.Model().AddEntity(RawEntity{
		ID:          7001,
		TypeID:      950,
		Team:        1,
		X:           float32(12*8 + 4),
		Y:           float32(8*8 + 4),
		Health:      100,
		MaxHealth:   100,
		RuntimeInit: true,
		SlowMul:     1,
	})

	stepForSeconds(w, 1.6)

	if !w.isBuildingHealSuppressedLocked(projector.Build) {
		t.Fatal("expected suppression ability to suppress enemy mend-projector")
	}

	projectorPos := int32(projector.Y*model.Width + projector.X)
	w.mendProjectorStateLocked(projectorPos).Charge = mendProjectorProfiles["mend-projector"].Reload
	beforeSuppressed := target.Build.Health
	w.stepSupportBuildingsLocked(time.Second / 60)
	if got := target.Build.Health; got != beforeSuppressed {
		t.Fatalf("expected suppressed mend-projector to stop healing, before=%f after=%f", beforeSuppressed, got)
	}

	w.Model().Entities = nil
	w.mendProjectorStateLocked(projectorPos).Charge = 0
	stepForSeconds(w, 2)

	if w.isBuildingHealSuppressedLocked(projector.Build) {
		t.Fatal("expected mend-projector suppression to expire after suppressor is gone")
	}

	beforeExpired := target.Build.Health
	w.mendProjectorStateLocked(projectorPos).Charge = mendProjectorProfiles["mend-projector"].Reload
	w.stepSupportBuildingsLocked(time.Second / 60)
	if got := target.Build.Health; got <= beforeExpired {
		t.Fatalf("expected mend-projector healing to resume after suppression expires, before=%f after=%f", beforeExpired, got)
	}
}

func TestSuppressionFieldAbilitySuppressesEnemyUnitRepairTower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		478: "power-source",
		902: "unit-repair-tower",
	}
	model.UnitNames = map[int16]string{
		950: "suppressor",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["suppressor"] = unitRuntimeProfile{
		Name: "suppressor",
		Abilities: []unitAbilityProfile{{
			Kind:     unitAbilitySuppressionField,
			Active:   true,
			Reload:   1.5,
			Cooldown: 1.5,
			Range:    200,
		}},
	}

	tower := placeTestBuilding(t, w, 12, 12, 902, 2, 0)
	placeTestBuilding(t, w, 12, 15, 478, 2, 0)
	tower.Build.AddLiquid(ozoneLiquidID, 30)
	linkPowerNode(t, w, 12, 15, protocol.Point2{X: 0, Y: -3})

	w.Model().AddEntity(RawEntity{
		ID:          7002,
		TypeID:      950,
		Team:        1,
		X:           float32(12*8 + 4),
		Y:           float32(8*8 + 4),
		Health:      100,
		MaxHealth:   100,
		RuntimeInit: true,
		SlowMul:     1,
	})

	stepForSeconds(w, 1.6)

	if !w.isBuildingHealSuppressedLocked(tower.Build) {
		t.Fatal("expected suppression ability to suppress enemy unit-repair-tower")
	}

	unit := w.Model().AddEntity(RawEntity{
		ID:        7003,
		TypeID:    35,
		Team:      2,
		X:         float32(10*8 + 4),
		Y:         float32(12*8 + 4),
		Health:    20,
		MaxHealth: 100,
	})

	towerPos := int32(tower.Y*model.Width + tower.X)
	w.repairTowerStates[towerPos] = repairTowerRuntimeState{Targets: []int32{unit.ID}}
	beforeSuppressed := findTestEntity(t, w, unit.ID).Health
	w.stepRepairBlocks(time.Second / 60)
	if got := findTestEntity(t, w, unit.ID).Health; got != beforeSuppressed {
		t.Fatalf("expected suppressed unit-repair-tower to stop healing, before=%f after=%f", beforeSuppressed, got)
	}

	w.Model().Entities = w.Model().Entities[1:]
	stepForSeconds(w, 2)

	if w.isBuildingHealSuppressedLocked(tower.Build) {
		t.Fatal("expected unit-repair-tower suppression to expire after suppressor is gone")
	}

	w.repairTowerStates[towerPos] = repairTowerRuntimeState{Targets: []int32{unit.ID}}
	beforeExpired := findTestEntity(t, w, unit.ID).Health
	w.Step(time.Second / 60)
	if got := findTestEntity(t, w, unit.ID).Health; got <= beforeExpired {
		t.Fatalf("expected unit-repair-tower healing to resume after suppression expires, before=%f after=%f", beforeExpired, got)
	}
}

func TestSlagIncineratorAcceptsSlagAndBurnsItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		500: "conduit",
		903: "slag-incinerator",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 5, 6, 500, 1, 0)
	inc := placeTestBuilding(t, w, 6, 6, 903, 1, 0)

	srcPos := int32(6*model.Width + 5)
	incPos := int32(6*model.Width + 6)
	if moved := w.tryMoveLiquidLocked(srcPos, incPos, slagLiquidID, 5, 0); moved <= 0 {
		t.Fatalf("expected slag-incinerator to accept slag input, moved=%f", moved)
	}

	stepForSeconds(w, 1)

	if !w.tryInsertItemLocked(srcPos, incPos, copperItemID, 0) {
		t.Fatal("expected slag-incinerator to burn items after slag gate is present")
	}
	if got := totalBuildingItems(inc.Build); got != 0 {
		t.Fatalf("expected slag-incinerator to keep no item inventory, got=%d", got)
	}
	if got := inc.Build.LiquidAmount(slagLiquidID); got <= 0 {
		t.Fatalf("expected slag gate liquid to stay stored, got=%f", got)
	}
}

func TestImpactDrillRequiresWater(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		2:   "ore-beryllium",
		478: "power-source",
		904: "impact-drill",
	}
	w.SetModel(model)

	paintAreaOverlay(t, w, 8, 8, 4, 2)
	drill := placeTestBuilding(t, w, 8, 8, 904, 1, 0)
	placeTestBuilding(t, w, 8, 12, 478, 1, 0)
	linkPowerNode(t, w, 8, 12, protocol.Point2{X: 0, Y: -4})

	stepForSeconds(w, 7)

	if got := totalBuildingItems(drill.Build); got != 0 {
		t.Fatalf("expected impact-drill without water to stay idle, items=%d", got)
	}
}

func TestImpactDrillProducesBurstWithWaterAndPower(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		2:   "ore-beryllium",
		478: "power-source",
		904: "impact-drill",
	}
	w.SetModel(model)

	paintAreaOverlay(t, w, 8, 8, 4, 2)
	drill := placeTestBuilding(t, w, 8, 8, 904, 1, 0)
	drill.Build.AddLiquid(waterLiquidID, 100)
	placeTestBuilding(t, w, 8, 12, 478, 1, 0)
	linkPowerNode(t, w, 8, 12, protocol.Point2{X: 0, Y: -4})

	stepForSeconds(w, 7)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		pos := int32(8*model.Width + 8)
		t.Fatalf("expected impact-drill burst to produce items, got=%d state=%+v power=%f liquids=%v", got, w.burstDrillStates[pos], w.blockSyncPowerStatusLocked(pos, &w.Model().Tiles[pos], "impact-drill"), drill.Build.Liquids)
	}
}

func TestEruptionDrillProducesWithHydrogen(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(28, 28)
	model.BlockNames = map[int16]string{
		2:   "ore-tungsten",
		478: "power-source",
		905: "eruption-drill",
	}
	w.SetModel(model)

	paintAreaOverlay(t, w, 10, 10, 5, 2)
	drill := placeTestBuilding(t, w, 10, 10, 905, 1, 0)
	drill.Build.AddLiquid(hydrogenLiquidID, 40)
	placeTestBuilding(t, w, 10, 15, 478, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5})

	stepForSeconds(w, 6)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		t.Fatalf("expected eruption-drill to produce items with hydrogen, got=%d", got)
	}
}

func TestPlasmaBoreMinesWallOreWhenPowered(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		2:   "ore-beryllium",
		478: "power-source",
		906: "plasma-bore",
	}
	w.SetModel(model)

	drill := placeTestBuilding(t, w, 8, 8, 906, 1, 0)
	placeTestBuilding(t, w, 8, 12, 478, 1, 0)
	linkPowerNode(t, w, 8, 12, protocol.Point2{X: 0, Y: -4})

	skip := map[int32]struct{}{}
	low, high := blockFootprintRange(blockSizeByName("plasma-bore"))
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			skip[packTilePos(8+dx, 8+dy)] = struct{}{}
		}
	}
	skip[packTilePos(8, 12)] = struct{}{}
	paintWallRect(t, w, 4, 4, 16, 16, 2, skip)

	stepForSeconds(w, 4)

	if got := totalBuildingItems(drill.Build); got <= 0 {
		pos := int32(8*model.Width + 8)
		t.Fatalf("expected plasma-bore to mine surrounding wall ore, got=%d state=%+v power=%f", got, w.beamDrillStates[pos], w.blockSyncPowerStatusLocked(pos, &w.Model().Tiles[pos], "plasma-bore"))
	}
}

func TestLargePlasmaBoreRequiresHydrogen(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(28, 28)
	model.BlockNames = map[int16]string{
		2:   "ore-tungsten",
		478: "power-source",
		907: "large-plasma-bore",
	}
	w.SetModel(model)

	drill := placeTestBuilding(t, w, 10, 10, 907, 1, 0)
	placeTestBuilding(t, w, 10, 15, 478, 1, 0)
	linkPowerNode(t, w, 10, 15, protocol.Point2{X: 0, Y: -5})

	skip := map[int32]struct{}{}
	low, high := blockFootprintRange(blockSizeByName("large-plasma-bore"))
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			skip[packTilePos(10+dx, 10+dy)] = struct{}{}
		}
	}
	skip[packTilePos(10, 15)] = struct{}{}
	paintWallRect(t, w, 4, 4, 18, 18, 2, skip)

	stepForSeconds(w, 4)
	if got := totalBuildingItems(drill.Build); got != 0 {
		t.Fatalf("expected large-plasma-bore without hydrogen to stay idle, got=%d", got)
	}

	drill.Build.AddLiquid(hydrogenLiquidID, 30)
	stepForSeconds(w, 3)
	if got := totalBuildingItems(drill.Build); got <= 0 {
		pos := int32(10*model.Width + 10)
		t.Fatalf("expected large-plasma-bore with hydrogen to mine, got=%d state=%+v power=%f liquids=%v", got, w.beamDrillStates[pos], w.blockSyncPowerStatusLocked(pos, &w.Model().Tiles[pos], "large-plasma-bore"), drill.Build.Liquids)
	}
}
