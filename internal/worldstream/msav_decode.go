package worldstream

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

// LoadWorldModelFromMSAV decodes the map chunk into a WorldModel.
// It does not deserialize full entities/building state yet.
func LoadWorldModelFromMSAV(path string, content *protocol.ContentRegistry) (*world.WorldModel, error) {
	data, err := readMSAV(path)
	if err != nil {
		return nil, err
	}
	blockNames, _ := readContentBlockNames(data.Content, nil)
	if len(blockNames) == 0 && content != nil {
		if names := readContentNamesFromRegistry(1, content); len(names) > 0 {
			blockNames = names
		}
	}
	model, err := decodeMapChunkForVersion(data.Map, data.Version, blockNames)
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
	if len(blockNames) > 0 {
		model.BlockNames = blockNames
		hydrateInlineBuildingConfigs(model)
	}
	if unitNames, err := readContentUnitNames(data.Content, nil); err == nil && len(unitNames) > 0 {
		model.UnitNames = unitNames
	} else if content != nil {
		if names := readContentNamesFromRegistry(6, content); len(names) > 0 {
			model.UnitNames = names
		}
	}
	_ = decodeEntitiesData(data, model)
	return model, nil
}

func decodeMapChunk(chunk []byte) (*world.WorldModel, error) {
	return decodeMapChunkModern(chunk, nil)
}

func decodeMapChunkForVersion(chunk []byte, version int32, blockNames map[int16]string) (*world.WorldModel, error) {
	switch {
	case version >= 10:
		return decodeMapChunkModern(chunk, blockNames)
	case version >= 6:
		return decodeMapChunkShortChunk(chunk, blockNames)
	default:
		return decodeMapChunkLegacy(chunk, blockNames)
	}
}

func decodeMapChunkModern(chunk []byte, blockNames map[int16]string) (*world.WorldModel, error) {
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
			// New data format: data + floorData + overlayData + extraData(int32).
			if err := r.Skip(1 + 1 + 1 + 4); err != nil {
				return nil, err
			}
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

				if !modernBlockHasEntity(blockNames, blockID) {
					continue
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
				if _, err := r.ReadByte(); err != nil {
					return nil, err
				}
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

func decodeMapChunkShortChunk(chunk []byte, blockNames map[int16]string) (*world.WorldModel, error) {
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
			if err := r.Skip(1 + 1 + 1 + 4); err != nil {
				return nil, err
			}
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
				if !modernBlockHasEntity(blockNames, blockID) {
					continue
				}
				if build, ok := decodeInlineBuildingChunk(payload, t, blockID); ok {
					t.Build = build
					t.Team = build.Team
					t.Rotation = build.Rotation
				}
			}
		} else if hadDataOld || hadDataNew {
			if hadDataOld {
				if _, err := r.ReadByte(); err != nil {
					return nil, err
				}
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
	case strings.HasPrefix(name, "ore-"),
		strings.Contains(name, "-ore-"),
		strings.HasSuffix(name, "-ore"),
		strings.Contains(name, "boulder"),
		strings.Contains(name, "tree"),
		strings.Contains(name, "bush"),
		strings.Contains(name, "rock"):
		return false
	default:
		return true
	}
}

func modernBlockHasEntity(blockNames map[int16]string, blockID int16) bool {
	if len(blockNames) == 0 {
		return true
	}
	name := strings.TrimSpace(blockNames[blockID])
	if name == "" {
		return true
	}
	return legacyBlockHasEntity(name)
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
		if data.WorldEntitiesHaveIDs && chunk.ID > 0 && chunk.ID >= model.NextEntityID {
			model.NextEntityID = chunk.ID + 1
		}
		offset := 1
		entityID := chunk.ID
		if data.WorldEntitiesHaveIDs {
			offset = 5
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
