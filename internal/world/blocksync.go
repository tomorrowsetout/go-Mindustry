package world

import (
	"log"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

// BlockSyncSnapshot mirrors the payload written by vanilla NetServer.writeBlockSnapshots():
// packed tile position, block ID, then build.writeSync(...) bytes.
type BlockSyncSnapshot struct {
	Pos     int32
	BlockID int16
	Data    []byte
}

type blockSyncKind byte

const (
	blockSyncNone blockSyncKind = iota
	blockSyncBaseOnly
	blockSyncConveyor
	blockSyncStackConveyor
	blockSyncMassDriver
	blockSyncTurret
	blockSyncItemTurret
	blockSyncContinuousTurret
	blockSyncPointDefenseTurret
	blockSyncTractorBeamTurret
	blockSyncPayloadTurret
	blockSyncStorage
	blockSyncUnloader
	blockSyncUnitFactory
	blockSyncPayloadVoid
	blockSyncPayloadDeconstructor
	blockSyncReconstructor
	blockSyncGenericCrafter
	blockSyncHeatProducer
	blockSyncSeparator
	blockSyncPowerGenerator
	blockSyncNuclearReactor
	blockSyncImpactReactor
	blockSyncHeaterGenerator
	blockSyncVariableReactor
	blockSyncRepairTurret
	blockSyncDrill
	blockSyncPump
	blockSyncIncinerator
)

func (w *World) BlockSyncSnapshots() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}

	// CRITICAL: Only sync buildings that have BlockFlag.synced set in Java
	// This matches Java's: indexer.getFlagged(team.team, BlockFlag.synced)
	syncedPositions := w.filterSyncedBuildingsLocked(w.activeTilePositions)
	return w.blockSyncSnapshotsForTilePositionsLocked(syncedPositions, true)
}

// filterSyncedBuildingsLocked filters building positions to only include buildings
// that should be synchronized according to Java's BlockFlag.synced logic.
// Based on Java source code analysis, only these building types need periodic sync:
// - Storage buildings (container, vault)
// - Factories (GenericCrafter, Separator)
// - Power generators (PowerGenerator, NuclearReactor, etc.)
// - Conveyors (including PayloadConveyor)
// - Mass drivers (MassDriver, PayloadMassDriver)
// - Unit assemblers (UnitAssembler)
// - Turrets (all turret types)
// Buildings that should NOT be synced here are only the routes that already have
// a dedicated sender elsewhere in the Go server. Base-only runtime blocks such as
// power nodes, batteries and routers still need snapshots for connect-time
// correction, matching the expectations covered by world tests.
func (w *World) filterSyncedBuildingsLocked(positions []int32) []int32 {
	return w.filterSyncedBuildingsByRouteLocked(positions, false)
}

func (w *World) filterItemTurretBuildingsLocked(positions []int32) []int32 {
	return w.filterSyncedBuildingsByRouteLocked(positions, true)
}

func isTurretBlockSyncKind(kind blockSyncKind) bool {
	switch kind {
	case blockSyncTurret,
		blockSyncItemTurret,
		blockSyncContinuousTurret,
		blockSyncPointDefenseTurret,
		blockSyncTractorBeamTurret,
		blockSyncPayloadTurret,
		blockSyncRepairTurret:
		return true
	default:
		return false
	}
}

func isHighFrequencyTransportBlockSyncKind(kind blockSyncKind) bool {
	switch kind {
	case blockSyncConveyor, blockSyncStackConveyor:
		return true
	default:
		return false
	}
}

func (w *World) filterSyncedBuildingsByRouteLocked(positions []int32, itemTurretsOnly bool) []int32 {
	if len(positions) == 0 {
		return nil
	}

	filtered := make([]int32, 0, len(positions))
	filteredOut := 0
	for _, pos := range positions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		if w.blockSyncSuppressedLocked(pos) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || !isCenterBuildingTile(tile) || tile.Build.Team == 0 {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		kind := w.classifyBlockSyncKindLocked(pos, tile, name)
		if itemTurretsOnly {
			if kind != blockSyncItemTurret {
				filteredOut++
				continue
			}
			filtered = append(filtered, pos)
			continue
		}
		if kind == blockSyncItemTurret {
			// Entity-ammo turrets run on a dedicated snapshot route so their ammo
			// cannot be rewritten by generic turret/building snapshot senders.
			filteredOut++
			continue
		}

		// Authoritative correction paths need current runtime state for any block
		// whose writeSync bytes materially affect what a newly joined player sees.
		// This includes conveyor-family transport blocks so connect-time snapshots
		// can seed in-flight item positions instead of forcing the client to
		// restart those animations from a recomputed local state.
		switch kind {
		case blockSyncBaseOnly,
			blockSyncConveyor,
			blockSyncStackConveyor,
			blockSyncStorage,              // container, vault
			blockSyncUnloader,             // unloader / sorter-like storage sync
			blockSyncGenericCrafter,       // factories
			blockSyncHeatProducer,         // heat-producing crafters
			blockSyncSeparator,            // separator
			blockSyncPowerGenerator,       // generators
			blockSyncNuclearReactor,       // thorium reactor
			blockSyncImpactReactor,        // impact reactor
			blockSyncHeaterGenerator,      // neoplasia reactor
			blockSyncVariableReactor,      // flux reactor
			blockSyncDrill,                // drill runtime for connect-time /sync correction
			blockSyncPump,                 // pump runtime / liquid module
			blockSyncIncinerator,          // incinerator liquid/item runtime
			blockSyncMassDriver,           // mass driver
			blockSyncUnitFactory,          // unit factories
			blockSyncPayloadVoid,          // payload void
			blockSyncPayloadDeconstructor, // payload deconstructor
			blockSyncReconstructor,        // reconstructors
			blockSyncTurret,               // all turret types
			blockSyncContinuousTurret,
			blockSyncPointDefenseTurret,
			blockSyncTractorBeamTurret,
			blockSyncPayloadTurret, // payload turrets (container weapons)
			blockSyncRepairTurret:
			filtered = append(filtered, pos)
		default:
			filteredOut++
		}
	}

	if w.blockSyncLogsEnabled {
		log.Printf("[blocksync] filterSynced: input=%d filtered=%d filteredOut=%d itemTurretsOnly=%v", len(positions), len(filtered), filteredOut, itemTurretsOnly)
	}
	return filtered
}

// BlockSyncSnapshotsLiveOnly serializes snapshots from the current runtime state
// without replaying inline map sync bytes loaded from the original msav.
func (w *World) BlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}

	// CRITICAL: Only sync buildings that have BlockFlag.synced set in Java
	// This matches Java's: indexer.getFlagged(team.team, BlockFlag.synced)
	syncedPositions := w.filterSyncedBuildingsLocked(w.activeTilePositions)
	return w.blockSyncSnapshotsForTilePositionsLocked(syncedPositions, false)
}

func (w *World) PeriodicBlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}

	syncedPositions := w.filterSyncedBuildingsLocked(w.activeTilePositions)
	if len(syncedPositions) == 0 {
		return nil
	}

	filtered := make([]int32, 0, len(syncedPositions))
	for _, pos := range syncedPositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		kind := w.classifyBlockSyncKindLocked(pos, tile, name)
		if isTurretBlockSyncKind(kind) || isHighFrequencyTransportBlockSyncKind(kind) {
			continue
		}
		filtered = append(filtered, pos)
	}
	return w.blockSyncSnapshotsForTilePositionsLocked(filtered, false)
}

func (w *World) ItemTurretBlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	positions := make([]int32, 0, len(w.activeTilePositions))
	positions = append(positions, w.activeTilePositions...)
	syncedPositions := w.filterItemTurretBuildingsLocked(positions)
	return w.blockSyncSnapshotsForTilePositionsLocked(syncedPositions, false)
}

func (w *World) ItemTurretBlockSyncSnapshotsForPacked(packedPositions []int32) []BlockSyncSnapshot {
	return w.itemTurretBlockSyncSnapshotsForPacked(packedPositions, true)
}

func (w *World) ItemTurretBlockSyncSnapshotsForPackedLiveOnly(packedPositions []int32) []BlockSyncSnapshot {
	return w.itemTurretBlockSyncSnapshotsForPacked(packedPositions, false)
}

func (w *World) BlockSyncSnapshotsForPacked(packedPositions []int32) []BlockSyncSnapshot {
	return w.blockSyncSnapshotsForPacked(packedPositions, true)
}

func (w *World) BlockSyncSnapshotsForPackedLiveOnly(packedPositions []int32) []BlockSyncSnapshot {
	return w.blockSyncSnapshotsForPacked(packedPositions, false)
}

func (w *World) itemTurretBlockSyncSnapshotsForPacked(packedPositions []int32, allowInlineFallback bool) []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 || len(packedPositions) == 0 {
		return nil
	}
	positions := w.tilePositionsFromPackedLocked(packedPositions)
	syncedPositions := w.filterItemTurretBuildingsLocked(positions)
	return w.blockSyncSnapshotsForTilePositionsLocked(syncedPositions, allowInlineFallback)
}

func (w *World) blockSyncSnapshotsForPacked(packedPositions []int32, allowInlineFallback bool) []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 || len(packedPositions) == 0 {
		return nil
	}

	positions := w.tilePositionsFromPackedLocked(packedPositions)

	// CRITICAL: Only sync buildings that have BlockFlag.synced set in Java
	// This prevents over-synchronization when broadcasting related block snapshots
	syncedPositions := w.filterSyncedBuildingsLocked(positions)
	return w.blockSyncSnapshotsForTilePositionsLocked(syncedPositions, allowInlineFallback)
}

func (w *World) tilePositionsFromPackedLocked(packedPositions []int32) []int32 {
	if len(packedPositions) == 0 {
		return nil
	}
	positions := make([]int32, 0, len(packedPositions))
	seen := make(map[int32]struct{}, len(packedPositions))
	for _, packed := range packedPositions {
		pos, ok := w.buildingIndexFromPackedPosLocked(packed)
		if !ok {
			continue
		}
		if _, dup := seen[pos]; dup {
			continue
		}
		seen[pos] = struct{}{}
		positions = append(positions, pos)
	}
	return positions
}

func (w *World) UnitFactoryBlockSyncSnapshots() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	return w.unitFactoryBlockSyncSnapshotsLocked(true)
}

func (w *World) UnitFactoryBlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	return w.unitFactoryBlockSyncSnapshotsLocked(false)
}

func (w *World) TurretBlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 || len(w.turretTilePositions) == 0 {
		return nil
	}
	positions := make([]int32, 0, len(w.turretTilePositions))
	for _, pos := range w.turretTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || tile.Build.Health <= 0 || tile.Build.Team == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		if w.classifyBlockSyncKindLocked(pos, tile, name) == blockSyncItemTurret {
			continue
		}
		positions = append(positions, pos)
	}
	return w.blockSyncSnapshotsForTilePositionsLocked(positions, false)
}

func (w *World) unitFactoryBlockSyncSnapshotsLocked(allowInlineFallback bool) []BlockSyncSnapshot {
	positions := make([]int32, 0, 16)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 {
			continue
		}
		if classifyBlockSyncKind(strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))) != blockSyncUnitFactory {
			continue
		}
		positions = append(positions, pos)
	}
	return w.blockSyncSnapshotsForTilePositionsLocked(positions, allowInlineFallback)
}

func (w *World) PayloadProcessorBlockSyncSnapshots() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	return w.payloadProcessorBlockSyncSnapshotsLocked(true)
}

func (w *World) PayloadProcessorBlockSyncSnapshotsLiveOnly() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	return w.payloadProcessorBlockSyncSnapshotsLocked(false)
}

func (w *World) payloadProcessorBlockSyncSnapshotsLocked(allowInlineFallback bool) []BlockSyncSnapshot {
	positions := make([]int32, 0, 16)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 {
			continue
		}
		if !isPayloadProcessorBlockSyncKind(classifyBlockSyncKind(strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))))) {
			continue
		}
		positions = append(positions, pos)
	}
	return w.blockSyncSnapshotsForTilePositionsLocked(positions, allowInlineFallback)
}

func isPayloadProcessorBlockSyncKind(kind blockSyncKind) bool {
	switch kind {
	case blockSyncPayloadVoid, blockSyncPayloadDeconstructor, blockSyncReconstructor:
		return true
	default:
		return false
	}
}

func (w *World) RelatedBlockSyncPackedPositions(packedPos int32) []int32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}

	pos, ok := w.buildingIndexFromPackedPosLocked(packedPos)
	if !ok || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil
	}
	if w.blockSyncSuppressedLocked(pos) {
		return nil
	}
	tile := &w.model.Tiles[pos]
	if tile == nil || tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 {
		return nil
	}

	out := make([]int32, 0, 8)
	seen := map[int32]struct{}{}
	add := func(index int32) {
		if centerPos, ok := w.centerBuildingIndexLocked(index); ok {
			index = centerPos
		} else {
			return
		}
		if index < 0 || int(index) >= len(w.model.Tiles) {
			return
		}
		if w.blockSyncSuppressedLocked(index) {
			return
		}
		other := &w.model.Tiles[index]
		if other == nil || !isCenterBuildingTile(other) || other.Build.Team == 0 {
			return
		}
		packed := packTilePos(other.X, other.Y)
		if _, dup := seen[packed]; dup {
			return
		}
		seen[packed] = struct{}{}
		out = append(out, packed)
	}

	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if w.classifyBlockSyncKindLocked(pos, tile, name) == blockSyncNone {
		return nil
	}
	add(pos)
	if !w.isPowerRelevantBuildingLocked(tile) {
		return out
	}
	for _, otherPos := range w.blockSyncPowerLinksLocked(pos, tile, name) {
		add(otherPos)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (w *World) blockSyncSnapshotsForTilePositionsLocked(tilePositions []int32, allowInlineFallback bool) []BlockSyncSnapshot {
	if len(tilePositions) == 0 {
		return nil
	}

	// Debug: Log all tile positions to check for invalid ones
	for _, pos := range tilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			log.Printf("[blocksync] INVALID POSITION: pos=%d totalTiles=%d", pos, len(w.model.Tiles))
		}
	}

	out := make([]BlockSyncSnapshot, 0, 64)
	for _, pos := range tilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		if w.blockSyncSuppressedLocked(pos) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || !isCenterBuildingTile(tile) || tile.Build.Team == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		kind := w.classifyBlockSyncKindLocked(pos, tile, name)
		if kind == blockSyncNone {
			continue
		}

		// DETAILED LOGGING: Before serialization
		itemCount := int32(0)
		if tile.Build.Items != nil {
			for _, stack := range tile.Build.Items {
				itemCount += stack.Amount
			}
		}

		data, ok := w.serializeBlockSyncLocked(pos, tile, name, kind, allowInlineFallback)
		if !ok || len(data) == 0 {
			if w.blockSyncLogsEnabled {
				log.Printf("[blocksync] FAILED serialize pos=%d (%d,%d) block=%s kind=%d team=%d items=%d",
					pos, tile.X, tile.Y, name, kind, tile.Build.Team, itemCount)
			}
			continue
		}

		// DETAILED LOGGING: After serialization
		if w.blockSyncLogsEnabled {
			log.Printf("[blocksync] SUCCESS serialize pos=%d (%d,%d) block=%s kind=%d team=%d items=%d dataLen=%d fallback=%v",
				pos, tile.X, tile.Y, name, kind, tile.Build.Team, itemCount, len(data), allowInlineFallback)
		}

		out = append(out, BlockSyncSnapshot{
			Pos:     packTilePos(tile.X, tile.Y),
			BlockID: int16(tile.Block),
			Data:    data,
		})
	}

	if w.blockSyncLogsEnabled {
		log.Printf("[blocksync] Generated %d snapshots from %d positions (fallback=%v)", len(out), len(tilePositions), allowInlineFallback)
	}
	return out
}

func classifyBlockSyncKind(name string) blockSyncKind {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "battery", "battery-large",
		"unit-repair-tower",
		"power-node", "power-node-large", "surge-tower", "beam-link", "power-source",
		"beam-node", "beam-tower", "power-void", "power-diode",
		"router", "distributor":
		return blockSyncBaseOnly
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return blockSyncConveyor
	case "plastanium-conveyor", "surge-conveyor":
		return blockSyncStackConveyor
	case "mass-driver":
		return blockSyncMassDriver
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis":
		// CRITICAL: Cores should NOT be synced via blockSnapshot
		// Java: CoreBlock.java:79 - sync = false; //core items are synced elsewhere
		return blockSyncNone
	case "container", "vault", "reinforced-container", "reinforced-vault":
		return blockSyncStorage
	case "unloader":
		return blockSyncUnloader
	case "ground-factory", "air-factory", "naval-factory":
		return blockSyncUnitFactory
	case "payload-void":
		return blockSyncPayloadVoid
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		return blockSyncPayloadDeconstructor
	case "separator", "disassembler", "slag-centrifuge":
		return blockSyncSeparator
	case "thorium-reactor":
		return blockSyncNuclearReactor
	case "impact-reactor":
		return blockSyncImpactReactor
	case "neoplasia-reactor":
		return blockSyncHeaterGenerator
	case "flux-reactor":
		return blockSyncVariableReactor
	case "repair-point", "repair-turret":
		return blockSyncRepairTurret
	case "mechanical-drill", "pneumatic-drill", "laser-drill", "blast-drill", "impact-drill", "eruption-drill", "plasma-bore", "large-plasma-bore":
		return blockSyncDrill
	case "mechanical-pump", "rotary-pump", "impulse-pump", "water-extractor", "oil-extractor":
		return blockSyncPump
	case "incinerator", "slag-incinerator":
		return blockSyncIncinerator
	case "combustion-generator", "thermal-generator", "steam-generator", "differential-generator",
		"rtg-generator", "solar-panel", "solar-panel-large", "turbine-condenser",
		"chemical-combustion-chamber", "pyrolysis-generator":
		return blockSyncPowerGenerator
	}
	if isReconstructorBlockName(name) {
		return blockSyncReconstructor
	}
	if prof, ok := crafterProfilesByBlockName[name]; ok {
		if prof.HeatOutput > 0 {
			return blockSyncHeatProducer
		}
		return blockSyncGenericCrafter
	}
	return blockSyncNone
}

func (w *World) classifyBlockSyncKindLocked(pos int32, tile *Tile, name string) blockSyncKind {
	if kind := classifyBlockSyncKind(name); kind != blockSyncNone {
		return kind
	}
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return blockSyncNone
	}
	prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	if !ok {
		return blockSyncNone
	}
	return classifyTurretBlockSyncKind(name, prof)
}

func classifyTurretBlockSyncKind(name string, prof buildingWeaponProfile) blockSyncKind {
	className := strings.ToLower(strings.TrimSpace(prof.ClassName))
	switch {
	case strings.Contains(className, "payloadturret"):
		return blockSyncPayloadTurret
	case strings.Contains(className, "pointdefense"):
		return blockSyncPointDefenseTurret
	case strings.Contains(className, "tractorbeam"):
		return blockSyncTractorBeamTurret
	case strings.Contains(className, "continuous"):
		return blockSyncContinuousTurret
	case strings.Contains(className, "itemturret"):
		return blockSyncItemTurret
	case strings.Contains(className, "turret"):
		return blockSyncTurret
	}

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "segment":
		return blockSyncPointDefenseTurret
	case "parallax":
		return blockSyncTractorBeamTurret
	}
	if prof.ContinuousHold {
		return blockSyncContinuousTurret
	}
	if prof.AmmoCapacity > 0 {
		return blockSyncItemTurret
	}
	if prof.Range > 0 || prof.Damage > 0 || prof.SplashDamage > 0 || prof.StatusID != 0 || strings.TrimSpace(prof.StatusName) != "" {
		return blockSyncTurret
	}
	return blockSyncNone
}

func (w *World) serializeBlockSyncLocked(pos int32, tile *Tile, name string, kind blockSyncKind, allowInlineFallback bool) ([]byte, bool) {
	if w == nil || tile == nil || tile.Build == nil {
		return nil, false
	}

	// CRITICAL: Check config option before using MapSyncData fallback
	if allowInlineFallback {
		if data, ok := w.inlineBlockSyncDataFallbackLocked(pos, tile, name, kind); ok {
			if w.blockSyncLogsEnabled {
				log.Printf("[blocksync] FALLBACK used pos=%d (%d,%d) block=%s kind=%d dataLen=%d",
					pos, tile.X, tile.Y, name, kind, len(data))
			}
			return data, true
		}
	}

	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, kind); err != nil {
		if w.blockSyncLogsEnabled {
			log.Printf("[blocksync] ERROR writeBlockBaseSync pos=%d (%d,%d) block=%s kind=%d err=%v",
				pos, tile.X, tile.Y, name, kind, err)
		}
		return nil, false
	}

	switch kind {
	case blockSyncConveyor:
		if err := w.writeBlockConveyorSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
	case blockSyncStackConveyor:
		if err := w.writeBlockStackConveyorSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
	case blockSyncMassDriver:
		if err := w.writeBlockMassDriverSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
	case blockSyncTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
	case blockSyncItemTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
		if err := w.writeBlockItemTurretAmmoLocked(writer, tile); err != nil {
			return nil, false
		}
	case blockSyncContinuousTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(w.blockSyncContinuousTurretLengthLocked(pos, tile)); err != nil {
			return nil, false
		}
	case blockSyncPointDefenseTurret, blockSyncTractorBeamTurret:
		if err := writer.WriteFloat32(w.blockSyncTurretRotationLocked(pos, tile)); err != nil {
			return nil, false
		}
	case blockSyncPayloadTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return nil, false
		}
		if err := w.writeBlockPayloadTurretAmmoLocked(writer, tile); err != nil {
			return nil, false
		}
	case blockSyncUnloader:
		sortItem := int16(-1)
		if item, ok := w.unloaderCfg[pos]; ok {
			sortItem = int16(item)
		}
		if err := writer.WriteInt16(sortItem); err != nil {
			return nil, false
		}
	case blockSyncUnitFactory:
		payX, payY, payRotation := w.unitFactoryPayloadSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(w.unitFactoryProgressLocked(pos, tile)); err != nil {
			return nil, false
		}
		currentPlan, _ := w.unitFactoryConfigValueLocked(pos, tile)
		if err := writer.WriteInt16(int16(currentPlan)); err != nil {
			return nil, false
		}
		commandPos, command := w.unitFactoryCommandStateLocked(pos)
		if err := protocol.WriteVecNullable(writer, commandPos); err != nil {
			return nil, false
		}
		if err := protocol.WriteCommand(writer, command); err != nil {
			return nil, false
		}
	case blockSyncPayloadVoid:
		payX, payY, payRotation := w.payloadVoidSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return nil, false
		}
	case blockSyncPayloadDeconstructor:
		payX, payY, payRotation := w.payloadDeconstructorSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return nil, false
		}
		state := w.payloadDeconstructorStateLocked(pos)
		if err := writer.WriteFloat32(clampf(state.Progress, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteInt16(int16(len(state.Accum))); err != nil {
			return nil, false
		}
		for _, value := range state.Accum {
			if err := writer.WriteFloat32(value); err != nil {
				return nil, false
			}
		}
		if err := writePayloadDataBytes(writer, state.Deconstructing); err != nil {
			return nil, false
		}
	case blockSyncReconstructor:
		payX, payY, payRotation := w.reconstructorPayloadSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(w.reconstructorProgressLocked(pos)); err != nil {
			return nil, false
		}
		commandPos, command := w.reconstructorCommandStateLocked(pos)
		if err := protocol.WriteVecNullable(writer, commandPos); err != nil {
			return nil, false
		}
		if err := protocol.WriteCommand(writer, command); err != nil {
			return nil, false
		}
	case blockSyncGenericCrafter:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		// Cultivator keeps a legacy warmup slot in vanilla GenericCrafter.read().
		if name == "cultivator" {
			if err := writer.WriteFloat32(0); err != nil {
				return nil, false
			}
		}
	case blockSyncHeatProducer:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(w.heatStates[pos], 0)); err != nil {
			return nil, false
		}
	case blockSyncSeparator:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteInt32(int32(state.Seed)); err != nil {
			return nil, false
		}
	case blockSyncPowerGenerator, blockSyncNuclearReactor, blockSyncImpactReactor, blockSyncHeaterGenerator:
		productionEfficiency, generateTime, extra := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(generateTime, 0)); err != nil {
			return nil, false
		}
		switch kind {
		case blockSyncNuclearReactor, blockSyncHeaterGenerator:
			if err := writer.WriteFloat32(maxf(extra, 0)); err != nil {
				return nil, false
			}
		case blockSyncImpactReactor:
			if err := writer.WriteFloat32(clampf(extra, 0, 1)); err != nil {
				return nil, false
			}
		}
	case blockSyncVariableReactor:
		productionEfficiency, heat, instability, warmup := w.variableReactorSyncFieldsLocked(pos, tile, name)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(0); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(heat, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(instability, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(warmup, 0, 1)); err != nil {
			return nil, false
		}
	case blockSyncRepairTurret:
		rotation := float32(90)
		if state, ok := w.repairTurretStates[pos]; ok && state.Rotation != 0 {
			rotation = state.Rotation
		}
		if err := writer.WriteFloat32(rotation); err != nil {
			return nil, false
		}
	case blockSyncDrill:
		progress, warmup := w.drillSyncStateLocked(pos, name)
		if err := writer.WriteFloat32(maxf(progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(warmup, 0, 1)); err != nil {
			return nil, false
		}
	}

	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) inlineBlockSyncDataFallbackLocked(pos int32, tile *Tile, name string, kind blockSyncKind) ([]byte, bool) {
	if w == nil || tile == nil || tile.Build == nil || len(tile.Build.MapSyncData) == 0 {
		return nil, false
	}
	switch kind {
	case blockSyncConveyor:
		if w.conveyorStates[pos] != nil {
			return nil, false
		}
	case blockSyncStackConveyor:
		if w.stackStates[pos] != nil {
			return nil, false
		}
	case blockSyncMassDriver:
		if w.massDriverStates[pos] != nil || len(w.massDriverShots) > 0 {
			return nil, false
		}
	case blockSyncTurret, blockSyncItemTurret, blockSyncContinuousTurret, blockSyncPointDefenseTurret, blockSyncTractorBeamTurret:
		if controlled, _, _, _ := w.controlledBuildingAimLocked(pos); controlled {
			return nil, false
		}
		if _, ok := w.buildStates[pos]; ok {
			return nil, false
		}
	case blockSyncUnloader:
		if _, ok := w.unloaderCfg[pos]; ok {
			return nil, false
		}
	case blockSyncUnitFactory:
		if _, ok := w.factoryStates[pos]; ok || w.payloadStates[pos] != nil {
			return nil, false
		}
	case blockSyncPayloadVoid:
		if w.payloadStates[pos] != nil {
			return nil, false
		}
	case blockSyncPayloadDeconstructor:
		if w.payloadDeconstructorStates[pos] != nil {
			return nil, false
		}
	case blockSyncReconstructor:
		if _, ok := w.reconstructorStates[pos]; ok || w.payloadStates[pos] != nil {
			return nil, false
		}
	case blockSyncGenericCrafter, blockSyncHeatProducer, blockSyncSeparator:
		if _, ok := w.crafterStates[pos]; ok {
			return nil, false
		}
		// CRITICAL: If runtime state doesn't exist, don't use MapSyncData
		// Generate fresh runtime data instead to avoid sending stale/empty data
		return nil, false
	case blockSyncPowerGenerator, blockSyncNuclearReactor, blockSyncImpactReactor, blockSyncHeaterGenerator, blockSyncVariableReactor:
		if w.powerGeneratorState[pos] != nil {
			return nil, false
		}
		// CRITICAL: If runtime state doesn't exist, don't use MapSyncData
		return nil, false
	case blockSyncRepairTurret:
		if _, ok := w.repairTurretStates[pos]; ok {
			return nil, false
		}
		// CRITICAL: If runtime state doesn't exist, don't use MapSyncData
		return nil, false
	case blockSyncDrill:
		if _, ok := beamDrillProfilesByBlockName[name]; ok {
			if _, ok := w.beamDrillStates[pos]; ok {
				return nil, false
			}
		} else if _, ok := burstDrillProfilesByBlockName[name]; ok {
			if _, ok := w.burstDrillStates[pos]; ok {
				return nil, false
			}
		} else if _, ok := w.drillStates[pos]; ok {
			return nil, false
		}
	case blockSyncPump:
		if _, ok := w.pumpStates[pos]; ok {
			return nil, false
		}
	case blockSyncIncinerator:
		if _, ok := w.incineratorStates[pos]; ok {
			return nil, false
		}
	default:
		return nil, false
	}
	return append([]byte(nil), tile.Build.MapSyncData...), true
}

func (w *World) writeBlockBaseSyncLocked(writer *protocol.Writer, pos int32, tile *Tile, name string, kind blockSyncKind) error {
	build := tile.Build
	hasItems := w.hasItemModuleForBlockSyncLocked(tile, name, kind)
	hasPower := w.hasPowerModuleForBlockSyncLocked(tile, name, kind)
	hasLiquids := w.hasLiquidModuleForBlockSyncLocked(tile, name, kind)

	health := maxf(build.Health, 0)
	if err := writer.WriteFloat32(health); err != nil {
		return err
	}
	if err := writer.WriteByte(byte(int(tile.Rotation)&0x7f | 0x80)); err != nil {
		return err
	}
	if err := writer.WriteByte(byte(build.Team)); err != nil {
		return err
	}
	if err := writer.WriteByte(3); err != nil {
		return err
	}
	if err := writer.WriteByte(1); err != nil {
		return err
	}

	moduleBits := byte(1 << 3)
	if hasItems {
		moduleBits |= 1
	}
	if hasPower {
		moduleBits |= 1 << 1
	}
	if hasLiquids {
		moduleBits |= 1 << 2
	}
	if err := writer.WriteByte(moduleBits); err != nil {
		return err
	}
	if hasItems {
		// Mindustry-157 ItemTurret keeps Block.hasItems=true, so the base item
		// module bit must stay present. Its runtime ammo is serialized separately
		// by ItemTurret.write(...), not through the base ItemModule payload.
		if kind == blockSyncItemTurret {
			if err := writer.WriteInt16(0); err != nil {
				return err
			}
		} else if err := w.writeBlockItemModuleLocked(writer, pos, build); err != nil {
			return err
		}
	}
	if hasPower {
		if err := w.writeBlockPowerModuleLocked(writer, pos, tile, name); err != nil {
			return err
		}
	}
	if hasLiquids {
		if err := writeBlockLiquidModule(writer, build); err != nil {
			return err
		}
	}

	efficiency, optionalEfficiency := w.blockSyncEfficiencyLocked(pos, tile, name, kind)
	if err := writer.WriteByte(byte(clampf(efficiency, 0, 1) * 255)); err != nil {
		return err
	}
	return writer.WriteByte(byte(clampf(optionalEfficiency, 0, 1) * 255))
}

func (w *World) hasItemModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	switch kind {
	case blockSyncItemTurret:
		// Mindustry-157 ItemTurret.java sets hasItems = true.
		return true
	case blockSyncStorage, blockSyncUnloader, blockSyncGenericCrafter, blockSyncNuclearReactor, blockSyncHeaterGenerator, blockSyncImpactReactor:
		return true
	case blockSyncPowerGenerator:
		return w.itemCapacityForBlockLocked(tile) > 0
	default:
		return w.itemCapacityForBlockLocked(tile) > 0
	}
}

func (w *World) hasLiquidModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	switch kind {
	case blockSyncTurret, blockSyncItemTurret, blockSyncContinuousTurret, blockSyncPayloadTurret:
		// Mindustry-157 Turret.java defaults liquidCapacity = 20f for Turret and
		// its subclasses, even when the module currently holds zero liquid.
		return true
	}
	return w.liquidCapacityForBlockLocked(tile) > 0
}

func (w *World) hasPowerModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	return w.isPowerRelevantBuildingLocked(tile)
}

func (w *World) writeBlockItemModuleLocked(writer *protocol.Writer, pos int32, build *Building) error {
	if writer == nil {
		return nil
	}
	src := build
	isShared := false
	if _, _, shared, ok := w.sharedCoreInventoryLocked(pos); ok && shared != nil {
		src = shared
		isShared = true
	}
	if src == nil {
		return writer.WriteInt16(0)
	}

	// DETAILED LOGGING: Before writing items
	itemCount := int32(0)
	for _, stack := range src.Items {
		if stack.Amount > 0 {
			itemCount += stack.Amount
		}
	}

	items := make([]ItemStack, 0, len(src.Items))
	for _, stack := range src.Items {
		if stack.Amount > 0 {
			items = append(items, stack)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Item < items[j].Item
	})

	if w.blockSyncLogsEnabled {
		log.Printf("[blocksync] writeItemModule pos=%d totalItems=%d uniqueTypes=%d shared=%v",
			pos, itemCount, len(items), isShared)
	}

	if err := writer.WriteInt16(int16(len(items))); err != nil {
		return err
	}
	for _, stack := range items {
		if err := writer.WriteInt16(int16(stack.Item)); err != nil {
			return err
		}
		if err := writer.WriteInt32(stack.Amount); err != nil {
			return err
		}
	}
	return nil
}

func writeBlockLiquidModule(writer *protocol.Writer, build *Building) error {
	liquids := make([]LiquidStack, 0, len(build.Liquids))
	for _, stack := range build.Liquids {
		if stack.Amount > 0.0001 {
			liquids = append(liquids, stack)
		}
	}
	sort.Slice(liquids, func(i, j int) bool {
		return liquids[i].Liquid < liquids[j].Liquid
	})
	if err := writer.WriteInt16(int16(len(liquids))); err != nil {
		return err
	}
	for _, stack := range liquids {
		if err := writer.WriteInt16(int16(stack.Liquid)); err != nil {
			return err
		}
		if err := writer.WriteFloat32(stack.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) writeBlockPowerModuleLocked(writer *protocol.Writer, pos int32, tile *Tile, name string) error {
	links := w.blockSyncPowerLinksLocked(pos, tile, name)
	packedLinks := make([]int32, 0, len(links))
	for _, link := range links {
		if link < 0 || w.model == nil || int(link) >= len(w.model.Tiles) {
			continue
		}
		target := &w.model.Tiles[link]
		packedLinks = append(packedLinks, packTilePos(target.X, target.Y))
	}
	if err := writer.WriteInt16(int16(len(packedLinks))); err != nil {
		return err
	}
	for _, link := range packedLinks {
		if err := writer.WriteInt32(link); err != nil {
			return err
		}
	}
	return writer.WriteFloat32(clampf(w.blockSyncPowerStatusLocked(pos, tile, name), 0, 1))
}

func writeBlockPayloadBytes(writer *protocol.Writer, build *Building) error {
	if build == nil || len(build.Payload) == 0 {
		return writer.WriteBool(false)
	}
	return writer.WriteBytes(build.Payload)
}

func writePayloadDataBytes(writer *protocol.Writer, payload *payloadData) error {
	if payload == nil || len(payload.Serialized) == 0 {
		return writer.WriteBool(false)
	}
	return writer.WriteBytes(payload.Serialized)
}

func (w *World) writeBlockTurretSyncLocked(writer *protocol.Writer, pos int32, tile *Tile) error {
	if writer == nil {
		return nil
	}
	if err := writer.WriteFloat32(w.blockSyncTurretReloadCounterLocked(pos, tile)); err != nil {
		return err
	}
	return writer.WriteFloat32(w.blockSyncTurretRotationLocked(pos, tile))
}

func (w *World) writeBlockItemTurretAmmoLocked(writer *protocol.Writer, tile *Tile) error {
	if writer == nil {
		return nil
	}
	if tile == nil || tile.Build == nil {
		return writer.WriteByte(0)
	}
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
		w.normalizeTurretAmmoEntriesLocked(tile, prof)
	}
	items := make([]ItemStack, 0, len(tile.Build.Items))
	for _, entry := range tile.Build.Items {
		if entry.Amount > 0 {
			items = append(items, entry)
		}
	}
	if len(items) > 255 {
		items = items[:255]
	}
	if w.blockSyncLogsEnabled {
		name := ""
		tileTeam := TeamID(0)
		buildTeam := TeamID(0)
		x, y := 0, 0
		if tile != nil {
			x, y = tile.X, tile.Y
			tileTeam = tile.Team
			if tile.Build != nil {
				name = strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block))))
				buildTeam = tile.Build.Team
			}
		}
		log.Printf("[turret-ammo] serialize (%d,%d) block=%s tileTeam=%d buildTeam=%d entries=%d stacks=%s",
			x, y, name, tileTeam, buildTeam, len(items), w.debugItemStacksLocked(items))
	}
	if err := writer.WriteByte(byte(len(items))); err != nil {
		return err
	}
	for _, stack := range items {
		if err := writer.WriteInt16(int16(stack.Item)); err != nil {
			return err
		}
		amount := stack.Amount
		if amount < 0 {
			amount = 0
		}
		if amount > 0x7fff {
			amount = 0x7fff
		}
		if err := writer.WriteInt16(int16(amount)); err != nil {
			return err
		}
	}
	return nil
}

func signedByteFromInt(v int) byte {
	if v < -128 {
		v = -128
	} else if v > 127 {
		v = 127
	}
	return byte(int8(v))
}

func (w *World) writeBlockConveyorSyncLocked(writer *protocol.Writer, pos int32, tile *Tile) error {
	if writer == nil {
		return nil
	}
	st := w.conveyorStateLocked(pos, tile)
	if st == nil || st.Len < 0 {
		return writer.WriteInt32(0)
	}
	if err := writer.WriteInt32(int32(st.Len)); err != nil {
		return err
	}
	for i := 0; i < st.Len; i++ {
		if err := writer.WriteInt16(int16(st.IDs[i])); err != nil {
			return err
		}
		if err := writer.WriteByte(signedByteFromInt(int(st.XS[i] * 127))); err != nil {
			return err
		}
		if err := writer.WriteByte(signedByteFromInt(int(st.YS[i]*255 - 128))); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) writeBlockStackConveyorSyncLocked(writer *protocol.Writer, pos int32, tile *Tile) error {
	if writer == nil {
		return nil
	}
	st := w.stackStateLocked(pos, tile)
	link := int32(-1)
	cooldown := float32(0)
	if st != nil {
		link = w.blockSyncPackedLinkedTileLocked(st.Link)
		cooldown = maxf(st.Cooldown, 0)
	}
	if err := writer.WriteInt32(link); err != nil {
		return err
	}
	return writer.WriteFloat32(cooldown)
}

func (w *World) writeBlockMassDriverSyncLocked(writer *protocol.Writer, pos int32, tile *Tile) error {
	if writer == nil {
		return nil
	}
	link := int32(-1)
	if target, ok := w.massDriverLinks[pos]; ok {
		link = w.blockSyncPackedLinkedTileLocked(target)
	}
	if err := writer.WriteInt32(link); err != nil {
		return err
	}
	if err := writer.WriteFloat32(w.blockSyncMassDriverRotationLocked(pos, tile)); err != nil {
		return err
	}
	return writer.WriteByte(w.blockSyncMassDriverStateLocked(pos, tile))
}

func (w *World) blockSyncPackedLinkedTileLocked(pos int32) int32 {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return -1
	}
	tile := &w.model.Tiles[pos]
	return packTilePos(tile.X, tile.Y)
}

func (w *World) blockSyncMassDriverRotationLocked(pos int32, tile *Tile) float32 {
	if w == nil || w.model == nil || tile == nil {
		return 90
	}
	srcX := float32(tile.X*8 + 4)
	srcY := float32(tile.Y*8 + 4)
	if incoming := w.blockSyncMassDriverIncomingSourceLocked(pos); incoming >= 0 && int(incoming) < len(w.model.Tiles) {
		from := &w.model.Tiles[incoming]
		return lookAt(srcX, srcY, float32(from.X*8+4), float32(from.Y*8+4))
	}
	if target, ok := w.massDriverTargetLocked(pos, tile); ok && target >= 0 && int(target) < len(w.model.Tiles) {
		dst := &w.model.Tiles[target]
		return lookAt(srcX, srcY, float32(dst.X*8+4), float32(dst.Y*8+4))
	}
	return 90
}

func (w *World) blockSyncMassDriverIncomingSourceLocked(pos int32) int32 {
	if w == nil {
		return -1
	}
	for _, shot := range w.massDriverShots {
		if shot.ToPos == pos {
			return shot.FromPos
		}
	}
	return -1
}

func (w *World) blockSyncMassDriverStateLocked(pos int32, tile *Tile) byte {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	if w.massDriverIncomingShotsLocked(pos) > 0 {
		return 1
	}
	if _, ok := w.massDriverTargetLocked(pos, tile); ok {
		return 2
	}
	return 0
}

func (w *World) blockSyncTurretReloadCounterLocked(pos int32, tile *Tile) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	if !ok || prof.Interval <= 0 {
		return 0
	}
	state, exists := w.buildStates[pos]
	if !exists {
		return 0
	}
	return clampf(prof.Interval-state.Cooldown, 0, prof.Interval)
}

func (w *World) blockSyncTurretRotationLocked(pos int32, tile *Tile) float32 {
	if tile == nil {
		return 90
	}
	if controlled, _, aimX, aimY := w.controlledBuildingAimLocked(pos); controlled {
		return lookAt(float32(tile.X*8+4), float32(tile.Y*8+4), aimX, aimY)
	}
	if st, ok := w.buildStates[pos]; ok && st.HasRotation {
		return st.TurretRotation
	}
	return float32(tile.Rotation) * 90
}

func (w *World) blockSyncContinuousTurretLengthLocked(pos int32, tile *Tile) float32 {
	if st, ok := w.buildStates[pos]; ok && st.BeamLastLength > 0 {
		return st.BeamLastLength
	}
	if tile == nil {
		return 0
	}
	size := float32(w.blockSizeForTileLocked(tile))
	if size <= 0 {
		size = 1
	}
	return size * 4
}

func (w *World) unitFactoryPayloadSyncFieldsLocked(pos int32, tile *Tile) (float32, float32, float32) {
	rotation := float32(0)
	if tile != nil {
		rotation = float32(tile.Rotation) * 90
	}
	if tile == nil {
		return 0, 0, rotation
	}
	st := w.payloadStates[pos]
	if st == nil || st.Payload == nil {
		return 0, 0, rotation
	}
	moveTime := w.unitBlockPayloadMoveFramesLocked(tile)
	if moveTime <= 0 {
		return 0, 0, rotation
	}
	progress := clampf(st.Move/moveTime, 0, 1)
	dx, dy := dirDelta(tile.Rotation)
	dist := float32(w.blockSizeForTileLocked(tile)) * 8 / 2
	return float32(dx) * dist * progress, float32(dy) * dist * progress, rotation
}

func (w *World) blockSyncPowerLinksLocked(pos int32, tile *Tile, name string) []int32 {
	if w == nil || tile == nil || tile.Build == nil {
		return nil
	}
	links := make([]int32, 0, 6)
	for _, link := range w.powerNodeLinks[pos] {
		if w.blockSyncPowerLinkPresentLocked(pos, link) {
			links = appendUniquePowerPos(links, link)
		}
	}
	if isBeamNodeBlockName(name) {
		for _, link := range w.beamNodeTargetsLocked(pos, tile) {
			if w.blockSyncPowerLinkPresentLocked(pos, link) {
				links = appendUniquePowerPos(links, link)
			}
		}
	}
	if w.model != nil {
		for _, otherPos := range w.activeTilePositions {
			if otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			other := &w.model.Tiles[otherPos]
			if other.Build == nil || other.Block == 0 || !isBeamNodeBlockName(w.blockNameByID(int16(other.Block))) {
				continue
			}
			for _, link := range w.beamNodeTargetsLocked(otherPos, other) {
				if link == pos && w.blockSyncPowerLinkPresentLocked(otherPos, pos) {
					links = appendUniquePowerPos(links, otherPos)
					break
				}
			}
		}
	}
	if len(links) == 0 {
		return nil
	}
	sort.Slice(links, func(i, j int) bool { return links[i] < links[j] })
	return append([]int32(nil), links...)
}

func (w *World) blockSyncPowerLinkPresentLocked(pos, link int32) bool {
	if w == nil || w.model == nil || pos < 0 || link < 0 || int(pos) >= len(w.model.Tiles) || int(link) >= len(w.model.Tiles) {
		return false
	}
	from := &w.model.Tiles[pos]
	to := &w.model.Tiles[link]
	if from.Build == nil || to.Build == nil || from.Block == 0 || to.Block == 0 {
		return false
	}
	if from.Build.Team == 0 || from.Build.Team != to.Build.Team {
		return false
	}
	if !w.isPowerRelevantBuildingLocked(from) || !w.isPowerRelevantBuildingLocked(to) {
		return false
	}
	if isPowerDiodeBlockName(w.blockNameByID(int16(from.Block))) || isPowerDiodeBlockName(w.blockNameByID(int16(to.Block))) {
		return false
	}
	return true
}

func (w *World) blockSyncPowerStatusLocked(pos int32, tile *Tile, name string) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if capacity := powerStorageCapacityByBlockName(name); capacity > 0 {
		if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
			return 1
		}
		if stored, ok := w.powerStorageState[pos]; ok {
			return clampf(stored/capacity, 0, 1)
		}
		if tile.Build.MapPowerStatusSet {
			return clampf(tile.Build.MapPowerStatus, 0, 1)
		}
		return 0
	}
	if !w.blockConsumesPowerLocked(name) {
		return 0
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
		return 1
	}
	if requested := w.powerRequested[pos]; requested > 0.000001 {
		return clampf(w.powerSupplied[pos]/requested, 0, 1)
	}
	if net, ok := w.powerNetStateForPosLocked(pos); ok {
		// Vanilla only stores status for consumers/buffers; inactive consumers estimate
		// based on whether their graph can currently supply power at all.
		if net.Budget > 0.0001 || net.Produced > 0.0001 {
			return 1
		}
	}
	if tile.Build.MapPowerStatusSet {
		return clampf(tile.Build.MapPowerStatus, 0, 1)
	}
	return 0
}

func (w *World) blockSyncEfficiencyLocked(pos int32, tile *Tile, name string, kind blockSyncKind) (float32, float32) {
	switch kind {
	case blockSyncStorage, blockSyncUnloader:
		return 1, 1
	case blockSyncUnitFactory:
		efficiency := w.unitFactorySyncEfficiencyLocked(pos, tile)
		return efficiency, efficiency
	case blockSyncPayloadVoid:
		return 1, 1
	case blockSyncPayloadDeconstructor:
		efficiency := w.payloadDeconstructorSyncEfficiencyLocked(pos, tile)
		return efficiency, efficiency
	case blockSyncReconstructor:
		efficiency := w.reconstructorSyncEfficiencyLocked(pos, tile)
		return efficiency, efficiency
	case blockSyncGenericCrafter:
		state := w.crafterStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncHeatProducer:
		state := w.crafterStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncRepairTurret:
		return w.repairTurretSyncEfficienciesLocked(pos, tile)
	case blockSyncDrill:
		_, warmup := w.drillSyncStateLocked(pos, name)
		return clampf(warmup, 0, 1), clampf(warmup, 0, 1)
	case blockSyncPump:
		state := w.pumpStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncPowerGenerator, blockSyncNuclearReactor, blockSyncImpactReactor, blockSyncHeaterGenerator:
		productionEfficiency, _, _ := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		return clampf(productionEfficiency, 0, 1), clampf(productionEfficiency, 0, 1)
	case blockSyncVariableReactor:
		productionEfficiency, _, _, _ := w.variableReactorSyncFieldsLocked(pos, tile, name)
		return clampf(productionEfficiency, 0, 1), clampf(productionEfficiency, 0, 1)
	case blockSyncIncinerator:
		heat := clampf(w.incineratorStates[pos], 0, 1)
		return heat, heat
	default:
		return 1, 1
	}
}

func (w *World) drillSyncStateLocked(pos int32, name string) (float32, float32) {
	if _, ok := beamDrillProfilesByBlockName[name]; ok {
		state := w.beamDrillStates[pos]
		return state.Time, state.Warmup
	}
	if _, ok := burstDrillProfilesByBlockName[name]; ok {
		state := w.burstDrillStates[pos]
		return state.Progress, state.Warmup
	}
	state := w.drillStates[pos]
	return state.Progress, state.Warmup
}

func (w *World) repairTurretSyncEfficienciesLocked(pos int32, tile *Tile) (float32, float32) {
	state := w.repairTurretStates[pos]
	efficiency := float32(0)
	if state.TargetID != 0 {
		efficiency = clampf(w.blockSyncPowerStatusLocked(pos, tile, w.blockNameByID(int16(tile.Block))), 0, 1)
		if !w.isPowerRelevantBuildingLocked(tile) {
			efficiency = 1
		}
	}
	name := w.blockNameByID(int16(tile.Block))
	prof, ok := repairTurretProfilesByBlockName[name]
	if !ok || !prof.AcceptCoolant {
		return efficiency, efficiency
	}
	liquid, amount, has := firstBuildingLiquid(tile.Build)
	if has && amount > 0.0001 && repairTurretAcceptsLiquid(liquid) {
		return efficiency, 1
	}
	return efficiency, 0
}

func (w *World) unitFactorySyncEfficiencyLocked(pos int32, tile *Tile) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	if plan, ok := w.unitFactorySelectedPlanLocked(pos, tile); !ok {
		return 0
	} else {
		if st := w.payloadStates[pos]; st != nil && st.Payload != nil {
			return 0
		}
		if w.unitBuildSpeedMultiplierLocked(tile.Build.Team, w.rulesMgr.Get()) <= 0 {
			return 0
		}
		if scaledCost := w.unitFactoryScaledCostLocked(tile.Build.Team, plan.Cost); len(scaledCost) > 0 && !hasRequiredItemsLocked(tile.Build, scaledCost) {
			return 0
		}
		if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
			return 1
		}
		if !w.isPowerRelevantBuildingLocked(tile) {
			return 1
		}
		return clampf(w.blockSyncPowerStatusLocked(pos, tile, w.blockNameByID(int16(tile.Block))), 0, 1)
	}
}

func (w *World) powerGeneratorSyncFieldsLocked(pos int32, tile *Tile, name string, kind blockSyncKind) (float32, float32, float32) {
	st := w.powerGeneratorState[pos]
	switch kind {
	case blockSyncImpactReactor:
		warmup := float32(0)
		generateTime := float32(0)
		if st != nil {
			warmup = clampf(st.Warmup, 0, 1)
			generateTime = maxf(st.FuelFrames, 0)
		}
		return clampf(pow5f(warmup), 0, 1), generateTime, warmup
	case blockSyncNuclearReactor:
		generateTime := float32(0)
		if st != nil {
			generateTime = maxf(st.FuelFrames, 0)
		}
		productionEfficiency := float32(0)
		if tile != nil && tile.Build != nil {
			if tile.Build.ItemAmount(thoriumItemID) > 0 || tile.Build.ItemAmount(legacyThoriumItemID) > 0 {
				productionEfficiency = 1
			}
		}
		if generateTime > 0 {
			productionEfficiency = 1
		}
		return clampf(productionEfficiency, 0, 1), generateTime, maxf(w.heatStates[pos], 0)
	case blockSyncHeaterGenerator:
		heat := maxf(w.heatStates[pos], 0)
		heatOutput := float32(10)
		if name == "neoplasia-reactor" {
			heatOutput = 60
		}
		return clampf(heat/maxf(heatOutput, 0.0001), 0, 1), 0, heat
	case blockSyncPowerGenerator:
		generateTime := float32(0)
		if st != nil {
			generateTime = maxf(st.FuelFrames, 0)
		}
		productionEfficiency := float32(0)
		switch name {
		case "solar-panel", "solar-panel-large":
			productionEfficiency = 1
		case "thermal-generator":
			productionEfficiency = clampf(w.thermalGenerationEfficiencyLocked(tile), 0, 1)
		case "turbine-condenser":
			productionEfficiency = clampf(w.sumFloorAttributeLocked(tile, "steam")/9, 0, 1)
		case "chemical-combustion-chamber":
			if canConsumeLiquidStacksLocked(tile.Build, []LiquidStack{
				{Liquid: ozoneLiquidID, Amount: 2.0 / 60.0},
				{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
			}, 1) {
				productionEfficiency = 1
			}
		case "pyrolysis-generator":
			if canConsumeLiquidStacksLocked(tile.Build, []LiquidStack{
				{Liquid: slagLiquidID, Amount: 20.0 / 60.0},
				{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
			}, 1) {
				productionEfficiency = 1
			}
		}
		if generateTime > 0 {
			productionEfficiency = 1
		}
		return productionEfficiency, generateTime, 0
	default:
		return 0, 0, 0
	}
}

func (w *World) variableReactorSyncFieldsLocked(pos int32, tile *Tile, name string) (productionEfficiency, heat, instability, warmup float32) {
	st := w.powerGeneratorState[pos]
	heat = maxf(w.heatStates[pos], 0)
	if st != nil {
		instability = st.Instability
		warmup = st.Warmup
	}
	maxHeat := float32(0)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "flux-reactor":
		maxHeat = 150
	}
	if maxHeat > 0 {
		productionEfficiency = clampf(heat/maxHeat, 0, 1)
	}
	return clampf(productionEfficiency, 0, 1), heat, clampf(instability, 0, 1), clampf(warmup, 0, 1)
}
