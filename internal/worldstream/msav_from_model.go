package worldstream

import (
	"bytes"
	"compress/zlib"
	"os"
	"strconv"

	"mdt-server/internal/world"
)

// WriteMSAVFromModel writes a .msav using data stored in the model.
// Map chunk is always encoded from current model tiles so runtime build/remove changes
// are persisted and reloaded correctly.
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

	mapChunk, err := encodeMapChunkMinimal(model)
	if err != nil {
		return err
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
	if rebuilt, err := writeEntitiesChunkFromModel(model); err == nil && len(rebuilt) > 0 {
		entitiesChunk = rebuilt
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

// BuildWorldStreamFromModel builds handshake world stream payload from the current runtime model.
// Unlike MSAV-based path, this always encodes map chunk from model tiles so late-joiners
// can see buildings placed before they connected.
func BuildWorldStreamFromModel(model *world.WorldModel) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}

	var out bytes.Buffer
	w := &javaWriter{buf: &out}

	rules := "{}"
	if model.Tags != nil {
		if v := model.Tags["rules"]; v != "" {
			rules = v
		}
	}
	locales := "{}"
	if model.Tags != nil {
		if v := model.Tags["locales"]; v != "" {
			locales = v
		}
	}
	if err := w.WriteUTF(rules); err != nil {
		return nil, err
	}
	if err := w.WriteUTF(locales); err != nil {
		return nil, err
	}

	tags := make(map[string]string, len(model.Tags))
	for k, v := range model.Tags {
		tags[k] = v
	}
	if err := w.WriteStringMap(tags); err != nil {
		return nil, err
	}

	wave := int32(1)
	if v, ok := tags["wave"]; ok {
		if parsed, err := strconv.Atoi(v); err == nil {
			wave = int32(parsed)
		}
	}
	if err := w.WriteInt32(wave); err != nil {
		return nil, err
	}

	wavetime := float32(0)
	if v, ok := tags["wavetime"]; ok {
		if parsed, err := strconv.ParseFloat(v, 32); err == nil {
			wavetime = float32(parsed)
		}
	}
	if err := w.WriteFloat32(wavetime); err != nil {
		return nil, err
	}

	tick := float64(0)
	if v, ok := tags["tick"]; ok {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			tick = parsed
		}
	}
	if err := w.WriteFloat64(tick); err != nil {
		return nil, err
	}

	if err := w.WriteInt64(0); err != nil {
		return nil, err
	}
	if err := w.WriteInt64(0); err != nil {
		return nil, err
	}

	if err := w.WriteInt32(1); err != nil {
		return nil, err
	}
	if err := writeMinimalPlayer(w); err != nil {
		return nil, err
	}

	if err := w.WriteBytes(model.Content); err != nil {
		return nil, err
	}
	if err := w.WriteByte(0); err != nil {
		return nil, err
	}

	mapChunk, err := encodeMapChunkMinimal(model)
	if err != nil {
		return nil, err
	}
	if err := w.WriteBytes(mapChunk); err != nil {
		return nil, err
	}
	if err := writeMinimalTeamBlocks(w); err != nil {
		return nil, err
	}

	markers := model.Markers
	if len(markers) == 0 {
		markers = []byte{0x7B, 0x7D}
	}
	if err := w.WriteBytes(markers); err != nil {
		return nil, err
	}
	if err := writeMinimalCustomChunks(w); err != nil {
		return nil, err
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(out.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
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
		// packed: no entity/data
		if err := w.WriteByte(0); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func writeEntitiesChunkFromModel(model *world.WorldModel) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	// entities revision
	if err := w.WriteByte(model.EntitiesRev); err != nil {
		return nil, err
	}
	// entities
	if err := w.WriteInt32(int32(len(model.Entities))); err != nil {
		return nil, err
	}
	for _, e := range model.Entities {
		if err := w.WriteInt16(e.TypeID); err != nil {
			return nil, err
		}
		if err := w.WriteInt32(e.ID); err != nil {
			return nil, err
		}
		if err := w.WriteFloat32(e.X); err != nil {
			return nil, err
		}
		if err := w.WriteFloat32(e.Y); err != nil {
			return nil, err
		}
		if err := w.WriteFloat32(e.Rotation); err != nil {
			return nil, err
		}
		if err := w.WriteByte(byte(e.Team)); err != nil {
			return nil, err
		}
		if e.Payload == nil {
			if err := w.WriteInt16(0); err != nil {
				return nil, err
			}
		} else {
			if len(e.Payload) > 32767 {
				return nil, ErrInvalidMSAV
			}
			if err := w.WriteInt16(int16(len(e.Payload))); err != nil {
				return nil, err
			}
			if err := w.WriteBytes(e.Payload); err != nil {
				return nil, err
			}
		}
	}

	// collect buildings from tiles
	var builds []*world.Building
	for i := range model.Tiles {
		if b := model.Tiles[i].Build; b != nil {
			builds = append(builds, b)
		}
	}

	if err := w.WriteInt32(int32(len(builds))); err != nil {
		return nil, err
	}
	for _, b := range builds {
		pos := int32(b.Y*model.Width + b.X)
		if err := w.WriteInt32(pos); err != nil {
			return nil, err
		}
		if err := w.WriteInt16(int16(b.Block)); err != nil {
			return nil, err
		}
		if err := w.WriteByte(byte(b.Team)); err != nil {
			return nil, err
		}
		if err := w.WriteByte(byte(b.Rotation)); err != nil {
			return nil, err
		}
		if err := w.WriteFloat32(b.Health); err != nil {
			return nil, err
		}
		if b.Config == nil {
			if err := w.WriteInt32(0); err != nil {
				return nil, err
			}
		} else {
			if err := w.WriteInt32(int32(len(b.Config))); err != nil {
				return nil, err
			}
			if err := w.WriteBytes(b.Config); err != nil {
				return nil, err
			}
		}
		if b.Payload == nil {
			if err := w.WriteInt32(0); err != nil {
				return nil, err
			}
		} else {
			if err := w.WriteInt32(int32(len(b.Payload))); err != nil {
				return nil, err
			}
			if err := w.WriteBytes(b.Payload); err != nil {
				return nil, err
			}
		}
		if err := w.WriteFloat32(b.MaxHealth); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}
