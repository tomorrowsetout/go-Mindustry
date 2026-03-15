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
	model, err := decodeMapChunk(data.Map, content)
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

func decodeMapChunk(chunk []byte, content *protocol.ContentRegistry) (*world.WorldModel, error) {
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
		hadData := (packed & 4) != 0
		if hadData {
			rot, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			team, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			// read data/floor/overlay/extra
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
			t.Rotation = int8(rot)
			t.Team = world.TeamID(team)
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
				if t.Build == nil {
					t.Build = &world.Building{X: t.X, Y: t.Y, Block: t.Block, Team: t.Team}
				}
				t.Build.RawData = chunk
				decodeBuildingRaw(t.Build, content)
			}
		} else if !hadData {
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

// decodeBuildingRaw 解析 MSAV 中的建筑内部状态（物品/液体/电力），最佳努力。
// Mindustry 的 build.readAll 会根据 block 类型动态读取模块，顺序通常是 items -> liquids -> power。
// 这里按该顺序尝试读取，遇到错误会回退并停止，避免破坏后续未知数据。
func decodeBuildingRaw(b *world.Building, _ *protocol.ContentRegistry) {
	if b == nil || len(b.RawData) == 0 {
		return
	}
	r := newJavaReader(b.RawData)

	// consume revision byte
	if _, err := r.ReadByte(); err != nil {
		return
	}

	tryRead := func(fn func(*javaReader) error) {
		start := r.Offset()
		if err := fn(r); err != nil {
			_ = r.SeekAbs(start)
		}
	}

	// items: short count, then (short id, int amount)
	tryRead(func(r *javaReader) error {
		count, err := r.ReadInt16()
		if err != nil || count < 0 {
			return fmt.Errorf("items count err")
		}
		items := make([]world.ItemStack, 0, count)
		for i := 0; i < int(count); i++ {
			id, err := r.ReadInt16()
			if err != nil {
				return err
			}
			amt, err := r.ReadInt32()
			if err != nil {
				return err
			}
			if id >= 0 && amt > 0 {
				items = append(items, world.ItemStack{Item: world.ItemID(id), Amount: amt})
			}
		}
		b.Items = items
		return nil
	})

	// liquids: short count, then (short id, float amount)
	tryRead(func(r *javaReader) error {
		count, err := r.ReadInt16()
		if err != nil || count < 0 {
			return fmt.Errorf("liquids count err")
		}
		liqs := make([]world.LiquidStack, 0, count)
		for i := 0; i < int(count); i++ {
			id, err := r.ReadInt16()
			if err != nil {
				return err
			}
			amt, err := r.ReadFloat32()
			if err != nil {
				return err
			}
			if id >= 0 && amt > 0 {
				liqs = append(liqs, world.LiquidStack{Liquid: world.LiquidID(id), Amount: amt})
			}
		}
		b.Liquids = liqs
		return nil
	})

	// power: short link count, link ints, then float status
	tryRead(func(r *javaReader) error {
		count, err := r.ReadInt16()
		if err != nil || count < 0 {
			return fmt.Errorf("power links err")
		}
		links := make([]int32, 0, count)
		for i := 0; i < int(count); i++ {
			id, err := r.ReadInt32()
			if err != nil {
				return err
			}
			links = append(links, id)
		}
		status, err := r.ReadFloat32()
		if err != nil {
			return err
		}
		b.PowerLinks = links
		b.PowerStatus = status
		return nil
	})
}
