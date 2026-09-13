package worldstream

import (
	"bytes"
	"compress/zlib"
	"os"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

// WriteMSAVFromModel writes a .msav using data stored in the model.
// If model.RawMap is present, it is used verbatim; otherwise a minimal map chunk is encoded.
func WriteMSAVFromModel(dstPath string, model *world.WorldModel, updates map[string]string) error {
	if model == nil {
		return ErrInvalidMSAV
	}
	tags := make(map[string]string, len(model.Tags)+len(updates))
	for k, v := range model.Tags {
		tags[k] = v
	}
	for k, v := range updates {
		tags[k] = v
	}

	var meta bytes.Buffer
	metaWriter := &javaWriter{buf: &meta}
	if err := metaWriter.WriteStringMap(tags); err != nil {
		return err
	}

	mapChunk := model.RawMap
	if len(mapChunk) == 0 {
		encoded, err := encodeMapChunkMinimal(model)
		if err != nil {
			return err
		}
		mapChunk = encoded
	}

	var raw bytes.Buffer
	if _, err := raw.Write([]byte("MSAV")); err != nil {
		return err
	}
	if err := writeBE(&raw, model.MSAVVersion); err != nil {
		return err
	}
	if err := writeChunk(&raw, meta.Bytes()); err != nil {
		return err
	}
	if err := writeChunk(&raw, model.Content); err != nil {
		return err
	}
	if model.MSAVVersion >= 11 {
		if err := writeChunk(&raw, model.Patches); err != nil {
			return err
		}
	}
	if err := writeChunk(&raw, mapChunk); err != nil {
		return err
	}
	entitiesChunk := model.RawEntities
	if model.MSAVVersion >= 10 || len(entitiesChunk) == 0 {
		if rebuilt, err := writeEntitiesChunkFromModel(model); err == nil && len(rebuilt) > 0 {
			entitiesChunk = rebuilt
		}
	}
	if err := writeChunk(&raw, entitiesChunk); err != nil {
		return err
	}
	if model.MSAVVersion >= 8 {
		if err := writeChunk(&raw, model.Markers); err != nil {
			return err
		}
	}
	if model.MSAVVersion >= 7 {
		if err := writeChunk(&raw, model.Custom); err != nil {
			return err
		}
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(dstPath, out.Bytes(), 0644)
}

func encodeMapChunkMinimal(model *world.WorldModel) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt16(int16(model.Width)); err != nil {
		return nil, err
	}
	if err := w.WriteInt16(int16(model.Height)); err != nil {
		return nil, err
	}
	total := model.Width * model.Height

	// Floors + overlays (no run-length compression for now).
	for i := 0; i < total; i++ {
		t := model.Tiles[i]
		if err := w.WriteInt16(int16(t.Floor)); err != nil {
			return nil, err
		}
		if err := w.WriteInt16(int16(t.Overlay)); err != nil {
			return nil, err
		}
		if err := w.WriteByte(0); err != nil {
			return nil, err
		}
	}

	// Blocks (no run-length compression for now).
	for i := 0; i < total; i++ {
		t := model.Tiles[i]
		if err := w.WriteInt16(int16(t.Block)); err != nil {
			return nil, err
		}
		hasSaveData := t.HasData
		isCenter := t.Build != nil && t.Build.X == t.X && t.Build.Y == t.Y
		hasEntity := t.Build != nil
		packed := byte(0)
		if hasEntity {
			packed |= 1
		}
		if hasSaveData {
			packed |= 4
		}
		if err := w.WriteByte(packed); err != nil {
			return nil, err
		}
		if hasSaveData {
			if err := w.WriteByte(t.Data); err != nil {
				return nil, err
			}
			if err := w.WriteByte(t.FloorData); err != nil {
				return nil, err
			}
			if err := w.WriteByte(t.OverlayData); err != nil {
				return nil, err
			}
			if err := w.WriteInt32(t.ExtraData); err != nil {
				return nil, err
			}
		}
		if hasEntity && !isCenter {
			if err := w.WriteByte(0); err != nil {
				return nil, err
			}
			continue
		}
		if hasEntity && isCenter {
			if err := w.WriteByte(1); err != nil {
				return nil, err
			}
			chunk, err := encodeInlineBuildingChunkForModel(t.Build)
			if err != nil {
				return nil, err
			}
			if err := w.WriteInt32(int32(len(chunk))); err != nil {
				return nil, err
			}
			if err := w.WriteBytes(chunk); err != nil {
				return nil, err
			}
			continue
		}
		if hasSaveData {
			continue
		}
		if err := w.WriteByte(0); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func encodeInlineBuildingChunkForModel(build *world.Building) ([]byte, error) {
	if build == nil {
		return nil, ErrInvalidMSAV
	}
	payload := build.MapSyncData
	if len(payload) == 0 {
		var err error
		payload, err = encodeMinimalBuildingMapSyncData(build)
		if err != nil {
			return nil, err
		}
	}
	chunk := make([]byte, 0, len(payload)+1)
	chunk = append(chunk, build.MapSyncRevision)
	chunk = append(chunk, payload...)
	return chunk, nil
}

func encodeMinimalBuildingMapSyncData(build *world.Building) ([]byte, error) {
	if build == nil {
		return nil, ErrInvalidMSAV
	}
	var out bytes.Buffer
	w := &javaWriter{buf: &out}

	health := build.Health
	switch {
	case health > 0:
	case build.MaxHealth > 0:
		health = build.MaxHealth
	default:
		health = 1000
	}
	if err := w.WriteFloat32(health); err != nil {
		return nil, err
	}
	if err := w.WriteByte(byte(int(build.Rotation)&0x7f | 0x80)); err != nil {
		return nil, err
	}
	if err := w.WriteByte(byte(build.Team)); err != nil {
		return nil, err
	}
	if err := w.WriteByte(3); err != nil {
		return nil, err
	}
	if err := w.WriteByte(1); err != nil {
		return nil, err
	}

	moduleBits := byte(1 << 3)
	itemCount := 0
	for _, stack := range build.Items {
		if stack.Amount > 0 {
			itemCount++
		}
	}
	liquidCount := 0
	for _, stack := range build.Liquids {
		if stack.Amount > 0 {
			liquidCount++
		}
	}
	if itemCount > 0 {
		moduleBits |= 1
	}
	if len(build.MapPowerLinks) > 0 || build.MapPowerStatusSet {
		moduleBits |= 1 << 1
	}
	if liquidCount > 0 {
		moduleBits |= 1 << 2
	}
	if err := w.WriteByte(moduleBits); err != nil {
		return nil, err
	}
	if itemCount > 0 {
		if err := w.WriteInt16(int16(itemCount)); err != nil {
			return nil, err
		}
		for _, stack := range build.Items {
			if stack.Amount <= 0 {
				continue
			}
			if err := w.WriteInt16(int16(stack.Item)); err != nil {
				return nil, err
			}
			if err := w.WriteInt32(stack.Amount); err != nil {
				return nil, err
			}
		}
	}
	if (moduleBits & (1 << 1)) != 0 {
		if err := w.WriteInt16(int16(len(build.MapPowerLinks))); err != nil {
			return nil, err
		}
		for _, link := range build.MapPowerLinks {
			if err := w.WriteInt32(link); err != nil {
				return nil, err
			}
		}
		if err := w.WriteFloat32(build.MapPowerStatus); err != nil {
			return nil, err
		}
	}
	if liquidCount > 0 {
		if err := w.WriteInt16(int16(liquidCount)); err != nil {
			return nil, err
		}
		for _, stack := range build.Liquids {
			if stack.Amount <= 0 {
				continue
			}
			if err := w.WriteInt16(int16(stack.Liquid)); err != nil {
				return nil, err
			}
			if err := w.WriteFloat32(stack.Amount); err != nil {
				return nil, err
			}
		}
	}
	if err := w.WriteByte(255); err != nil {
		return nil, err
	}
	if err := w.WriteByte(255); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeEntitiesChunkFromModel(model *world.WorldModel) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}

	mapping := []byte{0, 0}
	teamBlocks := []byte{0, 0, 0, 0}
	if len(model.EntityMapping) > 0 {
		mapping = append([]byte(nil), model.EntityMapping...)
	}
	if len(model.TeamBlocks) > 0 {
		teamBlocks = append([]byte(nil), model.TeamBlocks...)
	}
	preserved := make([]msavWorldEntityChunk, 0)
	if len(model.RawEntities) > 0 {
		if rawMapping, rawTeamBlocks, rawChunks, err := splitMSAVEntitiesChunk(model.RawEntities); err == nil {
			if len(rawMapping) > 0 {
				mapping = rawMapping
			}
			if len(rawTeamBlocks) > 0 {
				teamBlocks = rawTeamBlocks
			}
			for _, chunk := range rawChunks {
				if protocol.IsKnownUnitEntityClassID(chunk.ClassID) {
					continue
				}
				preserved = append(preserved, chunk)
			}
		}
	}

	rebuilt := make([][]byte, 0, len(model.Entities))
	for _, entity := range model.Entities {
		if entity.ID == 0 || entity.TypeID <= 0 {
			continue
		}
		unitName := ""
		if model.UnitNames != nil {
			unitName = model.UnitNames[entity.TypeID]
		}
		unit := world.UnitEntitySyncFromRawEntitySave(entity, unitName)
		if unit == nil {
			continue
		}
		writer := protocol.NewWriter()
		if err := writer.WriteByte(unit.ClassID()); err != nil {
			return nil, err
		}
		if err := writer.WriteInt32(unit.ID()); err != nil {
			return nil, err
		}
		if err := unit.WriteEntity(writer); err != nil {
			return nil, err
		}
		rebuilt = append(rebuilt, append([]byte(nil), writer.Bytes()...))
	}

	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteBytes(mapping); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(teamBlocks); err != nil {
		return nil, err
	}
	if err := w.WriteInt32(int32(len(preserved) + len(rebuilt))); err != nil {
		return nil, err
	}
	for _, chunk := range preserved {
		if err := w.WriteInt32(int32(len(chunk.Raw))); err != nil {
			return nil, err
		}
		if err := w.WriteBytes(chunk.Raw); err != nil {
			return nil, err
		}
	}
	for _, raw := range rebuilt {
		if err := w.WriteInt32(int32(len(raw))); err != nil {
			return nil, err
		}
		if err := w.WriteBytes(raw); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}
