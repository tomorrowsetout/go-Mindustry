package worldstream

import (
	"fmt"

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
	model, err := decodeMapChunk(data.Map)
	if err != nil {
		return nil, err
	}
	model.MSAVVersion = data.Version
	model.Tags = data.Tags
	model.Content = data.Content
	model.Patches = data.Patches
	model.RawMap = data.Map
	model.RawEntities = data.RawEntities
	model.Markers = data.Markers
	model.Custom = data.Custom
	if blockNames, err := readContentBlockNames(data.Content, content); err == nil {
		model.BlockNames = blockNames
	}
	if unitNames, err := readContentUnitNames(data.Content, content); err == nil {
		model.UnitNames = unitNames
	}
	_ = decodeEntitiesChunk(data.RawEntities, model)
	return model, nil
}

func decodeMapChunk(chunk []byte) (*world.WorldModel, error) {
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
		Block:    world.BlockID(blockID),
		Team:     world.TeamID(teamRaw),
		Rotation: rotation,
		X:        tile.X,
		Y:        tile.Y,
		Health:   health,
	}
	if build.Health <= 0 {
		build.Health = 1000
	}
	build.MaxHealth = build.Health

	if (moduleBits & 1) != 0 {
		items, ok := decodeInlineItemModule(r, legacy)
		if !ok {
			return build, true
		}
		build.Items = items
	}
	if (moduleBits & (1 << 1)) != 0 {
		if !skipInlinePowerModule(r) {
			return build, true
		}
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

func skipInlinePowerModule(r *javaReader) bool {
	count, err := r.ReadInt16()
	if err != nil {
		return false
	}
	if count < 0 || count > 4096 {
		return false
	}
	for i := 0; i < int(count); i++ {
		if _, err := r.ReadInt32(); err != nil {
			return false
		}
	}
	if _, err := r.ReadFloat32(); err != nil {
		return false
	}
	return true
}

func decodeEntitiesChunk(chunk []byte, model *world.WorldModel) error {
	if len(chunk) == 0 || model == nil {
		return nil
	}
	r := newJavaReader(chunk)
	rev, err := r.ReadByte()
	if err != nil {
		return err
	}
	model.EntitiesRev = rev
	// We only support minimal parsing for snapshot completeness.
	if int8(rev) < 0 {
		return nil
	}

	amount, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if amount < 0 {
		return ErrInvalidMSAV
	}
	for i := 0; i < int(amount); i++ {
		ent, err := readEntity(r)
		if err != nil {
			return err
		}
		if ent.TypeID < 0 {
			continue
		}
		if ent.ID >= model.NextEntityID {
			model.NextEntityID = ent.ID + 1
		}
		model.Entities = append(model.Entities, ent)
	}

	blocks, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if blocks < 0 {
		return ErrInvalidMSAV
	}
	for i := 0; i < int(blocks); i++ {
		pos, err := r.ReadInt32()
		if err != nil {
			return err
		}
		t, err := model.TileAt(int(pos)%model.Width, int(pos)/model.Width)
		if err != nil {
			return err
		}
		blockID, err := r.ReadInt16()
		if err != nil {
			return err
		}
		team, err := r.ReadByte()
		if err != nil {
			return err
		}
		rot, err := r.ReadByte()
		if err != nil {
			return err
		}
		health, err := r.ReadFloat32()
		if err != nil {
			return err
		}
		configLen, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if configLen < 0 {
			return ErrInvalidMSAV
		}
		var config []byte
		if configLen > 0 {
			config, err = r.ReadBytes(int(configLen))
			if err != nil {
				return err
			}
		}
		payload, err := readBuildingPayload(r)
		if err != nil {
			return err
		}
		// Read maxHealth from MSAV format (added after payload in newer versions)
		// If we run out of data, use health as maxHealth (fallback for older saves)
		maxHealth := health
		// Check if we have at least 4 bytes remaining (float32 size)
		if r.buf.Len() >= 4 {
			if mh, err := r.ReadFloat32(); err == nil {
				maxHealth = mh
			}
		}
		build := &world.Building{
			Block:     world.BlockID(blockID),
			Team:      world.TeamID(team),
			Rotation:  int8(rot),
			X:         t.X,
			Y:         t.Y,
			Health:    health,
			MaxHealth: maxHealth,
			Config:    config,
			Payload:   payload,
		}
		t.Build = build
		t.Block = world.BlockID(blockID)
		t.Team = world.TeamID(team)
		t.Rotation = int8(rot)
	}
	return nil
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
