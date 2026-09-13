package worldstream

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

// LoadWorldModelFromMSAV decodes the map chunk into a WorldModel.
// It does not deserialize full entities/building state yet.
func LoadWorldModelFromMSAV(path string, content *protocol.ContentRegistry) (*world.WorldModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadWorldModelFromMSAVBytes(raw, content)
}

// LoadWorldModelFromMSAVBytes decodes an in-memory .msav payload into a
// WorldModel. This is the byte-stream entry used by the world module contract.
func LoadWorldModelFromMSAVBytes(raw []byte, content *protocol.ContentRegistry) (*world.WorldModel, error) {
	data, err := readMSAVBytes(raw)
	if err != nil {
		return nil, err
	}
	mapBlockNames, _ := readContentBlockNames(data.Content, nil)
	blockNames := mapBlockNames
	blockRemap := buildContentIDRemap(mapBlockNames, nil, nil)
	unitRemap := contentIDRemap{}
	itemRemap := contentIDRemap{}
	liquidRemap := contentIDRemap{}
	statusRemap := contentIDRemap{}
	weatherRemap := contentIDRemap{}
	if content != nil {
		if names := readContentNamesFromRegistry(byte(protocol.ContentBlock), content); len(names) > 0 {
			blockRemap = buildContentIDRemap(mapBlockNames, names, blockNameFallbacks)
			blockNames = blockRemap.names
		}
		if mapUnitNames, _ := readContentUnitNames(data.Content, nil); len(mapUnitNames) > 0 {
			unitRemap = buildContentIDRemap(mapUnitNames, readContentNamesFromRegistry(byte(protocol.ContentUnit), content), nil)
		}
		if mapItemNames, _ := readContentNamesOfType(data.Content, byte(protocol.ContentItem), nil); len(mapItemNames) > 0 {
			itemRemap = buildContentIDRemap(mapItemNames, readContentNamesFromRegistry(byte(protocol.ContentItem), content), nil)
		}
		if mapLiquidNames, _ := readContentNamesOfType(data.Content, byte(protocol.ContentLiquid), nil); len(mapLiquidNames) > 0 {
			liquidRemap = buildContentIDRemap(mapLiquidNames, readContentNamesFromRegistry(byte(protocol.ContentLiquid), content), nil)
		}
		if mapStatusNames, _ := readContentNamesOfType(data.Content, byte(protocol.ContentStatus), nil); len(mapStatusNames) > 0 {
			statusRemap = buildContentIDRemap(mapStatusNames, readContentNamesFromRegistry(byte(protocol.ContentStatus), content), nil)
		}
		if mapWeatherNames, _ := readContentNamesOfType(data.Content, byte(protocol.ContentWeather), nil); len(mapWeatherNames) > 0 {
			weatherRemap = buildContentIDRemap(mapWeatherNames, readContentNamesFromRegistry(byte(protocol.ContentWeather), content), nil)
		}
	}
	model, err := decodeMapChunkForVersion(data.Map, data.Version, mapBlockNames)
	if err != nil {
		return nil, err
	}
	model.MSAVVersion = data.Version
	model.Tags = data.Tags
	model.Content = data.Content
	model.Patches = data.Patches
	model.RawMap = data.Map
	model.EntityMapping = append([]byte(nil), data.EntityMapping...)
	model.TeamBlocks = append([]byte(nil), data.TeamBlocks...)
	model.RawEntities = data.RawEntities
	model.Markers = data.Markers
	model.Custom = data.Custom
	_ = decodeEntitiesData(data, model)
	if content != nil {
		applyContentRemaps(model, blockRemap, unitRemap, itemRemap, liquidRemap)
		if remapped, changed, rerr := remapContentHeader(data.Content, map[byte]contentIDRemap{
			byte(protocol.ContentItem):    itemRemap,
			byte(protocol.ContentBlock):   blockRemap,
			byte(protocol.ContentLiquid):  liquidRemap,
			byte(protocol.ContentStatus):  statusRemap,
			byte(protocol.ContentUnit):    unitRemap,
			byte(protocol.ContentWeather): weatherRemap,
		}); rerr == nil && changed {
			model.Content = remapped
		}
		if blockRemap.changed || unitRemap.changed || itemRemap.changed || liquidRemap.changed {
			model.RawMap = nil
		}
	}
	if len(blockNames) > 0 {
		model.BlockNames = blockNames
		hydrateInlineBuildingConfigs(model)
	}
	if unitRemap.names != nil && len(unitRemap.names) > 0 {
		model.UnitNames = unitRemap.names
	} else if unitNames, err := readContentUnitNames(data.Content, nil); err == nil && len(unitNames) > 0 {
		model.UnitNames = unitNames
	} else if content != nil {
		if names := readContentNamesFromRegistry(byte(protocol.ContentUnit), content); len(names) > 0 {
			model.UnitNames = names
		}
	}
	return model, nil
}

type contentIDRemap struct {
	ids     map[int16]int16
	names   map[int16]string
	changed bool
}

func (r contentIDRemap) mapID(id int16) int16 {
	if r.ids == nil {
		return id
	}
	if mapped, ok := r.ids[id]; ok {
		return mapped
	}
	return id
}

func buildContentIDRemap(mapNames, runtimeNames map[int16]string, fallbacks map[string]string) contentIDRemap {
	if len(mapNames) == 0 {
		if len(runtimeNames) == 0 {
			return contentIDRemap{}
		}
		return contentIDRemap{names: copyContentNames(runtimeNames)}
	}
	if len(runtimeNames) == 0 {
		ids := make(map[int16]int16, len(mapNames))
		for id := range mapNames {
			ids[id] = id
		}
		return contentIDRemap{ids: ids, names: copyContentNames(mapNames)}
	}

	nameToRuntime := make(map[string]int16, len(runtimeNames))
	for id, name := range runtimeNames {
		normalized := normalizeContentName(name)
		if normalized == "" {
			continue
		}
		nameToRuntime[normalized] = id
	}

	ids := make(map[int16]int16, len(mapNames))
	names := copyContentNames(runtimeNames)
	changed := false
	for localID, name := range mapNames {
		normalized := normalizeContentName(name)
		if replacement := normalizeContentName(fallbacks[normalized]); replacement != "" {
			normalized = replacement
		}
		runtimeID, ok := nameToRuntime[normalized]
		if !ok {
			runtimeID = localID
			names[runtimeID] = normalizeContentName(name)
		}
		ids[localID] = runtimeID
		if runtimeID != localID {
			changed = true
		}
	}
	return contentIDRemap{ids: ids, names: names, changed: changed}
}

func copyContentNames(src map[int16]string) map[int16]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[int16]string, len(src))
	for id, name := range src {
		if normalized := normalizeContentName(name); normalized != "" {
			out[id] = normalized
		}
	}
	return out
}

func normalizeContentName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func applyContentRemaps(model *world.WorldModel, blockRemap, unitRemap, itemRemap, liquidRemap contentIDRemap) {
	if model == nil {
		return
	}
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		tile.Floor = world.FloorID(blockRemap.mapID(int16(tile.Floor)))
		tile.Overlay = world.OverlayID(blockRemap.mapID(int16(tile.Overlay)))
		tile.Block = world.BlockID(blockRemap.mapID(int16(tile.Block)))
		if tile.Build != nil {
			tile.Build.Block = world.BlockID(blockRemap.mapID(int16(tile.Build.Block)))
			for j := range tile.Build.Items {
				tile.Build.Items[j].Item = world.ItemID(itemRemap.mapID(int16(tile.Build.Items[j].Item)))
			}
			for j := range tile.Build.Liquids {
				tile.Build.Liquids[j].Liquid = world.LiquidID(liquidRemap.mapID(int16(tile.Build.Liquids[j].Liquid)))
			}
		}
	}
	for i := range model.Entities {
		model.Entities[i].TypeID = unitRemap.mapID(model.Entities[i].TypeID)
	}
}

func remapContentHeader(chunk []byte, remaps map[byte]contentIDRemap) ([]byte, bool, error) {
	if len(chunk) == 0 {
		return nil, false, nil
	}
	r := newJavaReader(chunk)
	mapped, err := r.ReadByte()
	if err != nil {
		return nil, false, err
	}

	type section struct {
		typ   byte
		names []string
	}
	sections := make([]section, 0, int(mapped))
	changed := false
	for i := 0; i < int(mapped); i++ {
		typ, err := r.ReadByte()
		if err != nil {
			return nil, false, err
		}
		total, err := r.ReadInt16()
		if err != nil {
			return nil, false, err
		}
		if total < 0 {
			return nil, false, ErrInvalidMSAV
		}
		names := make([]string, int(total))
		for id := int16(0); id < total; id++ {
			name, err := r.ReadUTF()
			if err != nil {
				return nil, false, err
			}
			names[id] = name
		}
		if remap, ok := remaps[typ]; ok && len(remap.names) > 0 {
			names = denseContentNames(remap.names)
			changed = changed || remap.changed
		}
		sections = append(sections, section{typ: typ, names: names})
	}
	if !changed {
		return append([]byte(nil), chunk...), false, nil
	}

	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteByte(mapped); err != nil {
		return nil, false, err
	}
	for _, section := range sections {
		if err := w.WriteByte(section.typ); err != nil {
			return nil, false, err
		}
		if err := w.WriteInt16(int16(len(section.names))); err != nil {
			return nil, false, err
		}
		for _, name := range section.names {
			if err := w.WriteUTF(name); err != nil {
				return nil, false, err
			}
		}
	}
	return out.Bytes(), true, nil
}

func denseContentNames(names map[int16]string) []string {
	maxID := int16(-1)
	for id := range names {
		if id > maxID {
			maxID = id
		}
	}
	if maxID < 0 {
		return nil
	}
	out := make([]string, int(maxID)+1)
	for id, name := range names {
		if id < 0 {
			continue
		}
		out[id] = normalizeContentName(name)
	}
	return out
}

var blockNameFallbacks = map[string]string{
	"dart-mech-pad":       "legacy-mech-pad",
	"dart-ship-pad":       "legacy-mech-pad",
	"javelin-ship-pad":    "legacy-mech-pad",
	"trident-ship-pad":    "legacy-mech-pad",
	"glaive-ship-pad":     "legacy-mech-pad",
	"alpha-mech-pad":      "legacy-mech-pad",
	"tau-mech-pad":        "legacy-mech-pad",
	"omega-mech-pad":      "legacy-mech-pad",
	"delta-mech-pad":      "legacy-mech-pad",
	"draug-factory":       "legacy-unit-factory",
	"spirit-factory":      "legacy-unit-factory",
	"phantom-factory":     "legacy-unit-factory",
	"wraith-factory":      "legacy-unit-factory",
	"ghoul-factory":       "legacy-unit-factory-air",
	"revenant-factory":    "legacy-unit-factory-air",
	"dagger-factory":      "legacy-unit-factory",
	"crawler-factory":     "legacy-unit-factory",
	"titan-factory":       "legacy-unit-factory-ground",
	"fortress-factory":    "legacy-unit-factory-ground",
	"mass-conveyor":       "payload-conveyor",
	"vestige":             "scepter",
	"turbine-generator":   "steam-generator",
	"rocks":               "stone-wall",
	"sporerocks":          "spore-wall",
	"icerocks":            "ice-wall",
	"dunerocks":           "dune-wall",
	"sandrocks":           "sand-wall",
	"shalerocks":          "shale-wall",
	"snowrocks":           "snow-wall",
	"saltrocks":           "salt-wall",
	"dirtwall":            "dirt-wall",
	"ignarock":            "basalt",
	"holostone":           "dacite",
	"holostone-wall":      "dacite-wall",
	"rock":                "boulder",
	"snowrock":            "snow-boulder",
	"cliffs":              "stone-wall",
	"craters":             "crater-stone",
	"deepwater":           "deep-water",
	"water":               "shallow-water",
	"sand":                "sand-floor",
	"slag":                "molten-slag",
	"cryofluidmixer":      "cryofluid-mixer",
	"block-forge":         "constructor",
	"block-unloader":      "payload-unloader",
	"block-loader":        "payload-loader",
	"thermal-pump":        "impulse-pump",
	"alloy-smelter":       "surge-smelter",
	"steam-vent":          "rhyolite-vent",
	"fabricator":          "tank-fabricator",
	"basic-reconstructor": "refabricator",
}

func decodeMapChunk(chunk []byte) (*world.WorldModel, error) {
	return decodeMapChunkModern(chunk)
}

func decodeMapChunkForVersion(chunk []byte, version int32, blockNames map[int16]string) (*world.WorldModel, error) {
	switch {
	case version >= 10:
		return decodeMapChunkModern(chunk)
	case version >= 6:
		return decodeMapChunkShortChunk(chunk)
	default:
		return decodeMapChunkLegacy(chunk, blockNames)
	}
}

func decodeMapChunkModern(chunk []byte) (*world.WorldModel, error) {
	r := newJavaReader(chunk)
	width, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	height, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidMSAV
	}
	w := int(width)
	h := int(height)
	total := w * h

	model := world.NewWorldModel(w, h)

	// floors + overlays (run-length encoded)
	for i := 0; i < total; i++ {
		floor, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		overlay, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		con, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		run := int(con)
		for j := 0; j <= run; j++ {
			idx := i + j
			if idx >= total {
				return nil, fmt.Errorf("map floor run out of range: %d/%d", idx, total)
			}
			t := &model.Tiles[idx]
			t.Floor = world.FloorID(floor)
			t.Overlay = world.OverlayID(overlay)
		}
		i += run
	}

	// blocks (run-length encoded)
	for i := 0; i < total; i++ {
		blockID, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		packed, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		t := &model.Tiles[i]
		t.Block = world.BlockID(blockID)

		hadEntity := (packed & 1) != 0
		hadDataOld := (packed & 2) != 0
		hadDataNew := (packed & 4) != 0
		if hadDataNew {
			dataByte, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			floorData, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			overlayData, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			extraData, err := r.ReadInt32()
			if err != nil {
				return nil, err
			}
			t.HasData = true
			t.Data = dataByte
			t.FloorData = floorData
			t.OverlayData = overlayData
			t.ExtraData = extraData
		}
		if hadEntity {
			isCenter, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if isCenter == 1 {
				chunkLen, err := r.ReadInt32()
				if err != nil {
					return nil, err
				}
				if chunkLen < 0 {
					return nil, ErrInvalidMSAV
				}
				chunk, err := r.ReadBytes(int(chunkLen))
				if err != nil {
					return nil, err
				}
				if build, ok := decodeInlineBuildingChunk(chunk, t, blockID); ok {
					t.Build = build
					t.Team = build.Team
					t.Rotation = build.Rotation
				}
			}
		} else if hadDataOld || hadDataNew {
			// Old data format (bit 2): one data byte when there is no entity.
			// New data format (bit 3): already consumed above.
			if hadDataOld {
				dataByte, err := r.ReadByte()
				if err != nil {
					return nil, err
				}
				t.HasData = true
				t.Data = dataByte
			}
		} else {
			con, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			run := int(con)
			for j := 0; j <= run; j++ {
				idx := i + j
				if idx >= total {
					return nil, fmt.Errorf("map block run out of range: %d/%d", idx, total)
				}
				tb := &model.Tiles[idx]
				tb.Block = world.BlockID(blockID)
			}
			i += run
		}
	}
	return model, nil
}

func decodeMapChunkShortChunk(chunk []byte) (*world.WorldModel, error) {
	r := newJavaReader(chunk)
	width, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	height, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidMSAV
	}
	w := int(width)
	h := int(height)
	total := w * h

	model := world.NewWorldModel(w, h)
	for i := 0; i < total; i++ {
		floor, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		overlay, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		con, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		run := int(con)
		for j := 0; j <= run; j++ {
			idx := i + j
			if idx >= total {
				return nil, fmt.Errorf("map floor run out of range: %d/%d", idx, total)
			}
			t := &model.Tiles[idx]
			t.Floor = world.FloorID(floor)
			t.Overlay = world.OverlayID(overlay)
		}
		i += run
	}

	for i := 0; i < total; i++ {
		blockID, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		packed, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		t := &model.Tiles[i]
		t.Block = world.BlockID(blockID)

		hadEntity := (packed & 1) != 0
		hadDataOld := (packed & 2) != 0
		hadDataNew := (packed & 4) != 0
		if hadDataNew {
			dataByte, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			floorData, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			overlayData, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			extraData, err := r.ReadInt32()
			if err != nil {
				return nil, err
			}
			t.HasData = true
			t.Data = dataByte
			t.FloorData = floorData
			t.OverlayData = overlayData
			t.ExtraData = extraData
		}
		if hadEntity {
			isCenter, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if isCenter == 1 {
				chunkLen, err := r.ReadUInt16()
				if err != nil {
					return nil, err
				}
				payload, err := r.ReadBytes(int(chunkLen))
				if err != nil {
					return nil, err
				}
				if build, ok := decodeInlineBuildingChunk(payload, t, blockID); ok {
					t.Build = build
					t.Team = build.Team
					t.Rotation = build.Rotation
				}
			}
		} else if hadDataOld || hadDataNew {
			if hadDataOld {
				dataByte, err := r.ReadByte()
				if err != nil {
					return nil, err
				}
				t.HasData = true
				t.Data = dataByte
			}
		} else {
			con, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			run := int(con)
			for j := 0; j <= run; j++ {
				idx := i + j
				if idx >= total {
					return nil, fmt.Errorf("map block run out of range: %d/%d", idx, total)
				}
				model.Tiles[idx].Block = world.BlockID(blockID)
			}
			i += run
		}
	}
	return model, nil
}

func decodeMapChunkLegacy(chunk []byte, blockNames map[int16]string) (*world.WorldModel, error) {
	r := newJavaReader(chunk)
	width, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	height, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidMSAV
	}
	w := int(width)
	h := int(height)
	total := w * h
	model := world.NewWorldModel(w, h)

	for i := 0; i < total; i++ {
		floor, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		overlay, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		con, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		run := int(con)
		for j := 0; j <= run; j++ {
			idx := i + j
			if idx >= total {
				return nil, fmt.Errorf("map floor run out of range: %d/%d", idx, total)
			}
			t := &model.Tiles[idx]
			t.Floor = world.FloorID(floor)
			t.Overlay = world.OverlayID(overlay)
		}
		i += run
	}

	for i := 0; i < total; i++ {
		blockID, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		t := &model.Tiles[i]
		t.Block = world.BlockID(blockID)
		name := strings.ToLower(strings.TrimSpace(blockNames[blockID]))
		if legacyBlockHasEntity(name) {
			chunkLen, err := r.ReadUInt16()
			if err != nil {
				return nil, err
			}
			payload, err := r.ReadBytes(int(chunkLen))
			if err != nil {
				return nil, err
			}
			if build, ok := decodeLegacyBuildingChunk(payload, t, blockID); ok {
				t.Build = build
				t.Team = build.Team
				t.Rotation = build.Rotation
			}
			continue
		}
		con, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		run := int(con)
		for j := 0; j <= run; j++ {
			idx := i + j
			if idx >= total {
				return nil, fmt.Errorf("legacy map block run out of range: %d/%d", idx, total)
			}
			model.Tiles[idx].Block = world.BlockID(blockID)
		}
		i += run
	}
	return model, nil
}

func legacyBlockHasEntity(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "air" {
		return false
	}
	switch {
	case strings.Contains(name, "wall") && !strings.Contains(name, "copper-wall") && !strings.Contains(name, "titanium-wall") && !strings.Contains(name, "thorium-wall") && !strings.Contains(name, "plastanium-wall") && !strings.Contains(name, "phase-wall") && !strings.Contains(name, "surge-wall") && !strings.Contains(name, "beryllium-wall") && !strings.Contains(name, "carbide-wall") && !strings.Contains(name, "shield-wall"):
		return false
	case strings.HasSuffix(name, "-floor"), strings.HasSuffix(name, "-water"), strings.HasSuffix(name, "-vent"):
		return false
	case strings.Contains(name, "ore"), strings.Contains(name, "boulder"), strings.Contains(name, "tree"), strings.Contains(name, "bush"), strings.Contains(name, "rock"):
		return false
	default:
		return true
	}
}

func decodeLegacyBuildingChunk(chunk []byte, tile *world.Tile, blockID int16) (*world.Building, bool) {
	if tile == nil || len(chunk) < 4 {
		return nil, false
	}
	r := newJavaReader(chunk)
	revision, err := r.ReadByte()
	if err != nil {
		return nil, false
	}
	healthRaw, err := r.ReadUInt16()
	if err != nil {
		return nil, false
	}
	packedRot, err := r.ReadByte()
	if err != nil {
		return nil, false
	}
	team := byte((packedRot >> 4) & 0x0f)
	rotation := int8(packedRot & 0x0f)
	if team == 8 {
		teamRaw, err := r.ReadByte()
		if err != nil {
			return nil, false
		}
		team = teamRaw
	}
	build := &world.Building{
		Block:           world.BlockID(blockID),
		Team:            world.TeamID(team),
		Rotation:        rotation,
		X:               tile.X,
		Y:               tile.Y,
		Health:          float32(healthRaw),
		MaxHealth:       float32(healthRaw),
		MapSyncRevision: revision,
	}
	if build.Health <= 0 {
		build.Health = 1000
		build.MaxHealth = 1000
	}
	if tail := chunk[1:]; len(tail) > 0 {
		build.MapSyncData = append([]byte(nil), tail...)
	}
	return build, true
}

func decodeInlineBuildingChunk(chunk []byte, tile *world.Tile, blockID int16) (*world.Building, bool) {
	if tile == nil || len(chunk) < 7 {
		return nil, false
	}
	r := newJavaReader(chunk)
	revision, err := r.ReadByte()
	if err != nil {
		return nil, false
	}
	_ = revision
	health, err := r.ReadFloat32()
	if err != nil {
		return nil, false
	}
	rotRaw, err := r.ReadByte()
	if err != nil {
		return nil, false
	}
	teamRaw, err := r.ReadByte()
	if err != nil {
		return nil, false
	}

	rotation := int8(rotRaw & 0x7f)
	version := byte(0)
	moduleBits := byte(0)
	legacy := true

	if (rotRaw & 0x80) != 0 {
		version, err = r.ReadByte()
		if err != nil {
			return nil, false
		}
		if version >= 1 {
			if _, err := r.ReadByte(); err != nil { // enabled
				return nil, false
			}
		}
		if version >= 2 {
			moduleBits, err = r.ReadByte()
			if err != nil {
				return nil, false
			}
		}
		legacy = false
	}

	build := &world.Building{
		Block:           world.BlockID(blockID),
		Team:            world.TeamID(teamRaw),
		Rotation:        rotation,
		X:               tile.X,
		Y:               tile.Y,
		Health:          health,
		MapSyncRevision: revision,
	}
	if build.Health <= 0 {
		build.Health = 1000
	}
	build.MaxHealth = build.Health
	if len(chunk) > 1 {
		build.MapSyncData = append([]byte(nil), chunk[1:]...)
	}

	if (moduleBits & 1) != 0 {
		items, ok := decodeInlineItemModule(r, legacy)
		if !ok {
			return build, true
		}
		build.Items = items
	}
	if (moduleBits & (1 << 1)) != 0 {
		links, status, ok := decodeInlinePowerModule(r)
		if !ok {
			return build, true
		}
		build.MapPowerLinks = append([]int32(nil), links...)
		build.MapPowerStatus = status
		build.MapPowerStatusSet = true
	}
	if (moduleBits & (1 << 2)) != 0 {
		liquids, ok := decodeInlineLiquidModule(r, legacy)
		if !ok {
			return build, true
		}
		build.Liquids = liquids
	}
	if (moduleBits & (1 << 4)) != 0 {
		if _, err := r.ReadFloat32(); err != nil {
			return build, true
		}
		if _, err := r.ReadFloat32(); err != nil {
			return build, true
		}
	}
	if (moduleBits & (1 << 5)) != 0 {
		if _, err := r.ReadInt32(); err != nil {
			return build, true
		}
	}
	if version <= 2 {
		if _, err := r.ReadByte(); err != nil {
			return build, true
		}
	}
	if version >= 3 {
		if _, err := r.ReadByte(); err != nil {
			return build, true
		}
		if _, err := r.ReadByte(); err != nil {
			return build, true
		}
	}
	if version == 4 {
		if _, err := r.ReadInt64(); err != nil {
			return build, true
		}
	}
	if tail := chunk[r.Offset():]; len(tail) > 0 {
		build.MapSyncTail = append([]byte(nil), tail...)
	}

	return build, true
}

func decodeInlineItemModule(r *javaReader, legacy bool) ([]world.ItemStack, bool) {
	if legacy {
		countRaw, err := r.ReadByte()
		if err != nil {
			return nil, false
		}
		count := int(countRaw)
		if count < 0 || count > 4096 {
			return nil, false
		}
		items := make([]world.ItemStack, 0, count)
		for i := 0; i < count; i++ {
			itemIDRaw, err := r.ReadByte()
			if err != nil {
				return nil, false
			}
			amount, err := r.ReadInt32()
			if err != nil {
				return nil, false
			}
			if amount <= 0 {
				continue
			}
			items = append(items, world.ItemStack{Item: world.ItemID(itemIDRaw), Amount: amount})
		}
		return items, true
	}
	countRaw, err := r.ReadInt16()
	if err != nil {
		return nil, false
	}
	count := int(countRaw)
	if count < 0 || count > 4096 {
		return nil, false
	}
	items := make([]world.ItemStack, 0, count)
	for i := 0; i < count; i++ {
		itemIDRaw, err := r.ReadInt16()
		if err != nil {
			return nil, false
		}
		amount, err := r.ReadInt32()
		if err != nil {
			return nil, false
		}
		if amount <= 0 || itemIDRaw < 0 {
			continue
		}
		items = append(items, world.ItemStack{Item: world.ItemID(itemIDRaw), Amount: amount})
	}
	return items, true
}

func decodeInlineLiquidModule(r *javaReader, legacy bool) ([]world.LiquidStack, bool) {
	if legacy {
		countRaw, err := r.ReadByte()
		if err != nil {
			return nil, false
		}
		count := int(countRaw)
		if count < 0 || count > 4096 {
			return nil, false
		}
		liquids := make([]world.LiquidStack, 0, count)
		for i := 0; i < count; i++ {
			liqIDRaw, err := r.ReadByte()
			if err != nil {
				return nil, false
			}
			amount, err := r.ReadFloat32()
			if err != nil {
				return nil, false
			}
			if amount <= 0 {
				continue
			}
			liquids = append(liquids, world.LiquidStack{Liquid: world.LiquidID(liqIDRaw), Amount: amount})
		}
		return liquids, true
	}
	countRaw, err := r.ReadInt16()
	if err != nil {
		return nil, false
	}
	count := int(countRaw)
	if count < 0 || count > 4096 {
		return nil, false
	}
	liquids := make([]world.LiquidStack, 0, count)
	for i := 0; i < count; i++ {
		liqIDRaw, err := r.ReadInt16()
		if err != nil {
			return nil, false
		}
		amount, err := r.ReadFloat32()
		if err != nil {
			return nil, false
		}
		if amount <= 0 || liqIDRaw < 0 {
			continue
		}
		liquids = append(liquids, world.LiquidStack{Liquid: world.LiquidID(liqIDRaw), Amount: amount})
	}
	return liquids, true
}

func decodeInlinePowerModule(r *javaReader) ([]int32, float32, bool) {
	count, err := r.ReadInt16()
	if err != nil {
		return nil, 0, false
	}
	if count < 0 || count > 4096 {
		return nil, 0, false
	}
	links := make([]int32, 0, count)
	for i := 0; i < int(count); i++ {
		link, err := r.ReadInt32()
		if err != nil {
			return nil, 0, false
		}
		links = append(links, link)
	}
	status, err := r.ReadFloat32()
	if err != nil {
		return nil, 0, false
	}
	return links, status, true
}

func skipInlinePowerModule(r *javaReader) bool {
	_, _, ok := decodeInlinePowerModule(r)
	return ok
}

func hydrateInlineBuildingConfigs(model *world.WorldModel) {
	if model == nil || len(model.BlockNames) == 0 {
		return
	}
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		build := tile.Build
		if build == nil || tile.Block == 0 || len(build.Config) > 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Block)]))
		switch name {
		case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
			if cfg, ok := encodePointSeqConfigFromPackedLinks(model, tile.X, tile.Y, build.MapPowerLinks); ok {
				build.Config = cfg
			}
		case "bridge-conveyor", "phase-conveyor", "bridge-conduit", "phase-conduit",
			"mass-driver", "payload-mass-driver", "large-payload-mass-driver":
			if target, ok := decodeLeadingPackedLink(build.MapSyncTail); ok {
				if cfg, ok := encodePointConfigFromPackedLink(model, tile.X, tile.Y, target); ok {
					build.Config = cfg
				}
			}
		case "item-source",
			"sorter",
			"inverted-sorter",
			"duct-router",
			"surge-router",
			"unloader",
			"duct-unloader":
			if itemID, ok := decodeLeadingConfigInt16(build.MapSyncTail); ok {
				if cfg, ok := encodeConfigObject(protocol.ItemRef{ItmID: itemID}); ok {
					build.Config = cfg
				}
			}
		case "liquid-source":
			if liquidID, ok := decodeLeadingConfigInt16(build.MapSyncTail); ok {
				if cfg, ok := encodeConfigObject(configContentRef{typ: protocol.ContentLiquid, id: liquidID}); ok {
					build.Config = cfg
				}
			}
		case "payload-router", "reinforced-payload-router":
			if content, ok := decodeLeadingPayloadContent(build.MapSyncTail); ok {
				if cfg, ok := encodeConfigObject(content); ok {
					build.Config = cfg
				}
			}
		}
	}
}

func decodeLeadingPackedLink(tail []byte) (int32, bool) {
	if len(tail) < 4 {
		return 0, false
	}
	value, err := newJavaReader(tail).ReadInt32()
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func decodeLeadingConfigInt16(tail []byte) (int16, bool) {
	if len(tail) < 2 {
		return 0, false
	}
	value, err := newJavaReader(tail).ReadInt16()
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

type configContentRef struct {
	typ protocol.ContentType
	id  int16
}

func (c configContentRef) ContentType() protocol.ContentType { return c.typ }
func (c configContentRef) ID() int16                         { return c.id }
func (c configContentRef) Name() string                      { return "" }

func decodeLeadingPayloadContent(tail []byte) (protocol.Content, bool) {
	if len(tail) < 3 {
		return nil, false
	}
	r := newJavaReader(tail)
	ctypeRaw, err := r.ReadByte()
	if err != nil {
		return nil, false
	}
	contentID, err := r.ReadInt16()
	if err != nil || contentID < 0 {
		return nil, false
	}
	switch ctype := protocol.ContentType(int8(ctypeRaw)); ctype {
	case protocol.ContentBlock, protocol.ContentUnit:
		return configContentRef{typ: ctype, id: contentID}, true
	default:
		return nil, false
	}
}

func encodePointConfigFromPackedLink(model *world.WorldModel, srcX, srcY int, packed int32) ([]byte, bool) {
	pt := protocol.UnpackPoint2(packed)
	if model == nil || !model.InBounds(int(pt.X), int(pt.Y)) {
		return nil, false
	}
	return encodeConfigObject(protocol.Point2{
		X: pt.X - int32(srcX),
		Y: pt.Y - int32(srcY),
	})
}

func encodePointSeqConfigFromPackedLinks(model *world.WorldModel, srcX, srcY int, packedLinks []int32) ([]byte, bool) {
	if len(packedLinks) == 0 {
		return nil, false
	}
	points := make([]protocol.Point2, 0, len(packedLinks))
	seen := make(map[protocol.Point2]struct{}, len(packedLinks))
	for _, packed := range packedLinks {
		pt := protocol.UnpackPoint2(packed)
		if model == nil || !model.InBounds(int(pt.X), int(pt.Y)) {
			continue
		}
		rel := protocol.Point2{
			X: pt.X - int32(srcX),
			Y: pt.Y - int32(srcY),
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		points = append(points, rel)
	}
	if len(points) == 0 {
		return nil, false
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].X == points[j].X {
			return points[i].Y < points[j].Y
		}
		return points[i].X < points[j].X
	})
	return encodeConfigObject(points)
}

func encodeConfigObject(value any) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := protocol.WriteObject(writer, value, nil); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

type msavWorldEntityChunk struct {
	ClassID byte
	ID      int32
	Raw     []byte
}

func splitMSAVEntitiesChunk(raw []byte) ([]byte, []byte, []msavWorldEntityChunk, error) {
	if len(raw) == 0 {
		return nil, nil, nil, nil
	}
	r := newJavaReader(raw)
	entityMapCount, err := r.ReadInt16()
	if err != nil {
		return nil, nil, nil, err
	}
	if entityMapCount < 0 {
		return nil, nil, nil, ErrInvalidMSAV
	}
	for i := 0; i < int(entityMapCount); i++ {
		if _, err := r.ReadInt16(); err != nil {
			return nil, nil, nil, err
		}
		if err := r.SkipUTF(); err != nil {
			return nil, nil, nil, err
		}
	}
	mappingEnd := r.Offset()
	teamStart := mappingEnd

	teamCount, err := r.ReadInt32()
	if err != nil {
		return nil, nil, nil, err
	}
	if teamCount < 0 {
		return nil, nil, nil, ErrInvalidMSAV
	}
	for i := 0; i < int(teamCount); i++ {
		if _, err := r.ReadInt32(); err != nil {
			return nil, nil, nil, err
		}
		blockCount, err := r.ReadInt32()
		if err != nil {
			return nil, nil, nil, err
		}
		if blockCount < 0 {
			return nil, nil, nil, ErrInvalidMSAV
		}
		for j := 0; j < int(blockCount); j++ {
			if err := r.Skip(8); err != nil {
				return nil, nil, nil, err
			}
			if err := skipTypeIOObject(r); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	teamEnd := r.Offset()

	worldCount, err := r.ReadInt32()
	if err != nil {
		return nil, nil, nil, err
	}
	if worldCount < 0 {
		return nil, nil, nil, ErrInvalidMSAV
	}
	chunks := make([]msavWorldEntityChunk, 0, worldCount)
	for i := 0; i < int(worldCount); i++ {
		size, err := r.ReadInt32()
		if err != nil {
			return nil, nil, nil, err
		}
		if size < 0 {
			return nil, nil, nil, ErrInvalidMSAV
		}
		payload, err := r.ReadBytes(int(size))
		if err != nil {
			return nil, nil, nil, err
		}
		entry := msavWorldEntityChunk{Raw: append([]byte(nil), payload...)}
		if len(payload) >= 5 {
			entry.ClassID = payload[0]
			entry.ID = int32(binary.BigEndian.Uint32(payload[1:5]))
		}
		chunks = append(chunks, entry)
	}

	mapping := append([]byte(nil), raw[:mappingEnd]...)
	teamBlocks := append([]byte(nil), raw[teamStart:teamEnd]...)
	return mapping, teamBlocks, chunks, nil
}

func decodeEntitiesChunk(chunk []byte, model *world.WorldModel) error {
	return decodeEntitiesData(MSAVData{
		RawEntities:          chunk,
		WorldEntityChunks:    nil,
		WorldEntitiesHaveIDs: true,
	}, model)
}

func decodeEntitiesData(data MSAVData, model *world.WorldModel) error {
	if model == nil {
		return nil
	}
	model.Entities = model.Entities[:0]
	model.EntitiesRev = 0
	if model.NextEntityID <= 0 {
		model.NextEntityID = 1
	}
	chunks := data.WorldEntityChunks
	if len(chunks) == 0 && len(data.RawEntities) > 0 {
		section, err := parseModernEntitySection(data.RawEntities)
		if err != nil {
			return err
		}
		chunks = section.worldChunks
		data.WorldEntitiesHaveIDs = section.worldHaveIDs
	}
	var firstErr error
	for _, chunk := range chunks {
		offset := 1
		entityID := chunk.ID
		if data.WorldEntitiesHaveIDs {
			offset = 5
			// Vanilla entities allocate a transient EntityGroup ID in the
			// constructor before SaveVersion overwrites it with the saved ID.
			// This affects the next runtime-spawned entity ID when saved chunks
			// are not sorted by ascending ID.
			model.NextEntityID++
			if chunk.ID > 0 && chunk.ID >= model.NextEntityID {
				model.NextEntityID = chunk.ID + 1
			}
		} else {
			entityID = model.NextEntityID
			model.NextEntityID++
		}
		if !protocol.IsKnownUnitEntityClassID(chunk.ClassID) || len(chunk.Raw) <= offset {
			continue
		}
		unit := &protocol.UnitEntitySync{
			IDValue:      entityID,
			ClassIDValue: chunk.ClassID,
			ClassIDSet:   true,
		}
		if err := unit.ReadEntity(protocol.NewReaderWithContext(chunk.Raw[offset:], nil)); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		model.Entities = append(model.Entities, world.RawEntityFromUnitEntitySave(unit))
	}
	return firstErr
}

func readEntity(r *javaReader) (world.RawEntity, error) {
	typ, err := r.ReadInt16()
	if err != nil {
		return world.RawEntity{}, err
	}
	if typ == -1 {
		return world.RawEntity{TypeID: -1}, nil
	}
	id, err := r.ReadInt32()
	if err != nil {
		return world.RawEntity{}, err
	}
	x, err := r.ReadFloat32()
	if err != nil {
		return world.RawEntity{}, err
	}
	y, err := r.ReadFloat32()
	if err != nil {
		return world.RawEntity{}, err
	}
	rot, err := r.ReadFloat32()
	if err != nil {
		return world.RawEntity{}, err
	}
	team, err := r.ReadByte()
	if err != nil {
		return world.RawEntity{}, err
	}
	size, err := r.ReadInt16()
	if err != nil {
		return world.RawEntity{}, err
	}
	if size < 0 {
		return world.RawEntity{}, ErrInvalidMSAV
	}
	var payload []byte
	if size > 0 {
		payload, err = r.ReadBytes(int(size))
		if err != nil {
			return world.RawEntity{}, err
		}
	}
	return world.RawEntity{
		TypeID:   typ,
		ID:       id,
		X:        x,
		Y:        y,
		Rotation: rot,
		Team:     world.TeamID(team),
		Payload:  payload,
	}, nil
}

func readBuildingPayload(r *javaReader) ([]byte, error) {
	// Payload length-prefixed by int32 in Mindustry save format.
	size, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if size < 0 {
		return nil, ErrInvalidMSAV
	}
	if size == 0 {
		return nil, nil
	}
	return r.ReadBytes(int(size))
}
