package world

import (
	"math"
	"testing"

	"mdt-server/internal/protocol"
)

func TestBlockSyncSnapshotsFallbackToInlineMapSyncDataForDrill(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		325: "mechanical-drill",
	}
	tile, err := model.TileAt(5, 6)
	if err != nil || tile == nil {
		t.Fatalf("drill tile lookup failed: %v", err)
	}
	tile.Block = 325
	tile.Team = 1

	raw := protocol.NewWriter()
	if err := raw.WriteFloat32(90); err != nil {
		t.Fatalf("write raw health failed: %v", err)
	}
	if err := raw.WriteByte(0x80); err != nil {
		t.Fatalf("write raw rotation failed: %v", err)
	}
	if err := raw.WriteByte(1); err != nil {
		t.Fatalf("write raw team failed: %v", err)
	}
	if err := raw.WriteByte(3); err != nil {
		t.Fatalf("write raw version failed: %v", err)
	}
	if err := raw.WriteByte(1); err != nil {
		t.Fatalf("write raw enabled failed: %v", err)
	}
	if err := raw.WriteByte(1 << 3); err != nil {
		t.Fatalf("write raw module bits failed: %v", err)
	}
	if err := raw.WriteByte(191); err != nil {
		t.Fatalf("write raw efficiency failed: %v", err)
	}
	if err := raw.WriteByte(191); err != nil {
		t.Fatalf("write raw optional efficiency failed: %v", err)
	}
	if err := raw.WriteFloat32(2.5); err != nil {
		t.Fatalf("write raw drill progress failed: %v", err)
	}
	if err := raw.WriteFloat32(0.75); err != nil {
		t.Fatalf("write raw drill warmup failed: %v", err)
	}

	tile.Build = &Building{
		Block:       325,
		Team:        1,
		X:           5,
		Y:           6,
		Health:      90,
		MaxHealth:   90,
		MapSyncData: append([]byte(nil), raw.Bytes()...),
	}
	w.SetModel(model)

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one drill snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if base.Efficiency != 191 || base.OptionalEfficiency != 191 {
		t.Fatalf("expected inline sync efficiency bytes 191/191, got %d/%d", base.Efficiency, base.OptionalEfficiency)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill warmup failed: %v", err)
	}
	if math.Abs(float64(progress-2.5)) > 0.0001 {
		t.Fatalf("expected inline drill progress 2.5, got %f", progress)
	}
	if math.Abs(float64(warmup-0.75)) > 0.0001 {
		t.Fatalf("expected inline drill warmup 0.75, got %f", warmup)
	}
}

func TestBlockSyncSnapshotsPreferLiveRuntimeOverInlineMapSyncData(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		325: "mechanical-drill",
	}
	tile, err := model.TileAt(5, 6)
	if err != nil || tile == nil {
		t.Fatalf("drill tile lookup failed: %v", err)
	}
	tile.Block = 325
	tile.Team = 1

	raw := protocol.NewWriter()
	_ = raw.WriteFloat32(90)
	_ = raw.WriteByte(0x80)
	_ = raw.WriteByte(1)
	_ = raw.WriteByte(3)
	_ = raw.WriteByte(1)
	_ = raw.WriteByte(1 << 3)
	_ = raw.WriteByte(191)
	_ = raw.WriteByte(191)
	_ = raw.WriteFloat32(2.5)
	_ = raw.WriteFloat32(0.75)

	tile.Build = &Building{
		Block:       325,
		Team:        1,
		X:           5,
		Y:           6,
		Health:      90,
		MaxHealth:   90,
		MapSyncData: append([]byte(nil), raw.Bytes()...),
	}
	w.SetModel(model)
	pos := int32(tile.Y*w.Model().Width + tile.X)
	w.drillStates[pos] = drillRuntimeState{Progress: 0.5, Warmup: 0.25}

	snaps := w.BlockSyncSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one drill snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if base.Efficiency == 191 {
		t.Fatalf("expected live runtime efficiency to override inline map sync bytes")
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill warmup failed: %v", err)
	}
	if math.Abs(float64(progress-0.5)) > 0.0001 {
		t.Fatalf("expected live drill progress 0.5, got %f", progress)
	}
	if math.Abs(float64(warmup-0.25)) > 0.0001 {
		t.Fatalf("expected live drill warmup 0.25, got %f", warmup)
	}
}

func TestBlockSyncSnapshotsLiveOnlyIgnoresInlineMapSyncData(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		325: "mechanical-drill",
	}
	tile, err := model.TileAt(5, 6)
	if err != nil || tile == nil {
		t.Fatalf("drill tile lookup failed: %v", err)
	}
	tile.Block = 325
	tile.Team = 1

	raw := protocol.NewWriter()
	_ = raw.WriteFloat32(90)
	_ = raw.WriteByte(0x80)
	_ = raw.WriteByte(1)
	_ = raw.WriteByte(3)
	_ = raw.WriteByte(1)
	_ = raw.WriteByte(1 << 3)
	_ = raw.WriteByte(191)
	_ = raw.WriteByte(191)
	_ = raw.WriteFloat32(2.5)
	_ = raw.WriteFloat32(0.75)

	tile.Build = &Building{
		Block:       325,
		Team:        1,
		X:           5,
		Y:           6,
		Health:      90,
		MaxHealth:   90,
		MapSyncData: append([]byte(nil), raw.Bytes()...),
	}
	w.SetModel(model)

	snaps := w.BlockSyncSnapshotsLiveOnly()
	if len(snaps) != 1 {
		t.Fatalf("expected one drill snapshot, got %d", len(snaps))
	}
	base, r := decodeBlockSyncBase(t, snaps[0].Data)
	if base.Efficiency == 191 || base.OptionalEfficiency == 191 {
		t.Fatalf("expected live-only snapshot to avoid inline map sync bytes, got %d/%d", base.Efficiency, base.OptionalEfficiency)
	}
	progress, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill progress failed: %v", err)
	}
	warmup, err := r.ReadFloat32()
	if err != nil {
		t.Fatalf("read drill warmup failed: %v", err)
	}
	if math.Abs(float64(progress)) > 0.0001 {
		t.Fatalf("expected zero live drill progress without runtime state, got %f", progress)
	}
	if math.Abs(float64(warmup)) > 0.0001 {
		t.Fatalf("expected zero live drill warmup without runtime state, got %f", warmup)
	}
}

func TestBlockSyncSnapshotsIgnoreItemTurretInlineMapTailWithoutAmmoChain(t *testing.T) {
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
		AmmoCapacity: 30,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	tile, err := model.TileAt(5, 6)
	if err != nil || tile == nil {
		t.Fatalf("duo tile lookup failed: %v", err)
	}
	tile.Block = 910
	tile.Team = 1
	tail := protocol.NewWriter()
	if err := tail.WriteFloat32(12.5); err != nil {
		t.Fatalf("write turret reload failed: %v", err)
	}
	if err := tail.WriteFloat32(90); err != nil {
		t.Fatalf("write turret rotation failed: %v", err)
	}
	if err := tail.WriteByte(1); err != nil {
		t.Fatalf("write turret ammo count failed: %v", err)
	}
	if err := tail.WriteInt16(int16(copperItemID)); err != nil {
		t.Fatalf("write turret ammo item failed: %v", err)
	}
	if err := tail.WriteInt16(6); err != nil {
		t.Fatalf("write turret ammo amount failed: %v", err)
	}
	tile.Build = &Building{
		Block:       910,
		Team:        1,
		X:           5,
		Y:           6,
		Health:      90,
		MaxHealth:   90,
		MapSyncTail: append([]byte(nil), tail.Bytes()...),
	}
	pos := int32(tile.Y*w.Model().Width + tile.X)

	snaps := w.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(5, 6)})
	if len(snaps) != 1 {
		t.Fatalf("expected one duo snapshot, got %d", len(snaps))
	}
	_, r := decodeBlockSyncBase(t, snaps[0].Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read turret reload counter failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read turret rotation failed: %v", err)
	}
	ammoKinds, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read turret ammo entry count failed: %v", err)
	}
	if ammoKinds != 0 {
		t.Fatalf("expected inline map tail ammo to stay disabled after chain removal, got %d", ammoKinds)
	}

	state := buildCombatState{}
	if w.consumeBuildingAmmoLocked(pos, tile, w.buildingProfilesByName["duo"], &state) {
		t.Fatal("expected disabled inline map tail ammo not to be consumable")
	}
	if got := w.totalBuildingAmmoLocked(tile, w.buildingProfilesByName["duo"]); got != 0 {
		t.Fatalf("expected disabled inline map tail ammo to stay at 0, got %d", got)
	}
	if got := totalBuildingItems(tile.Build); got != 0 {
		t.Fatalf("expected disabled inline map tail ammo not to populate internal items, got %d", got)
	}

	snaps = w.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(5, 6)})
	if len(snaps) != 1 {
		t.Fatalf("expected one duo snapshot after consume, got %d", len(snaps))
	}
	_, r = decodeBlockSyncBase(t, snaps[0].Data)
	_, _ = r.ReadFloat32()
	_, _ = r.ReadFloat32()
	ammoKinds, err = r.ReadByte()
	if err != nil {
		t.Fatalf("read turret ammo count after disabled inline path failed: %v", err)
	}
	if ammoKinds != 0 {
		t.Fatalf("expected disabled inline map tail ammo to remain 0 in later snapshot, got %d", ammoKinds)
	}
}
