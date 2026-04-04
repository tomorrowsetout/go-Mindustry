package world

import (
	"errors"
	"math"
	"testing"
	"time"

	"mdt-server/internal/protocol"
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
	targetPos := int32(10*w.Model().Width + 6)
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
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != targetPos {
		t.Fatalf("expected power-node runtime link to target %d, got %v", targetPos, base.PowerLinks)
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
	nodePos := int32(node.Y*w.Model().Width + node.X)
	batteryPos := int32(battery.Y*w.Model().Width + battery.X)
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
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != nodePos {
		t.Fatalf("expected battery power module to keep reverse node link %d, got %v", nodePos, base.PowerLinks)
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
	if len(nodeBase.PowerLinks) != 1 || nodeBase.PowerLinks[0] != batteryPos {
		t.Fatalf("expected power-node to keep forward battery link %d, got %v", batteryPos, nodeBase.PowerLinks)
	}
	if rem := rr.Remaining(); rem != 0 {
		t.Fatalf("expected node snapshot payload to be fully consumed, remaining=%d", rem)
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
	if len(base.PowerLinks) != 1 || base.PowerLinks[0] != nodePos {
		t.Fatalf("expected air-factory power module to keep reverse node link %d, got %v", nodePos, base.PowerLinks)
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

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
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

	payload := w.newFactoryUnitPayloadLocked(factory, 7)
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

func TestCustomCombatDoesNotDamagePlayerControlledUnit(t *testing.T) {
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
	if ent.Health != 220 {
		t.Fatalf("expected custom combat to ignore player-controlled unit health, got=%f", ent.Health)
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

	batteryTile, err := model.TileAt(6, 10)
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
	batteryPos := int32(10*model.Width + 6)
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

	batteryTile, err := model.TileAt(6, 10)
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

	for i := 0; i < 8; i++ {
		w.Step(time.Second / 60)
	}

	if st := w.conveyorStates[int32(3*model.Width+4)]; st == nil || st.Len == 0 {
		t.Fatalf("expected unloader to move configured item into conveyor")
	}
	if store.Build.ItemAmount(5) >= 3 {
		t.Fatalf("expected unloader to remove item from source storage")
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
