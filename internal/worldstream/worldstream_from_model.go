package worldstream

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strconv"

	"mdt-server/internal/world"
)

// BuildWorldStreamFromModel builds a join-world payload from the in-memory world model.
// It preserves map-derived tags/content while reflecting runtime tile/build changes.
func BuildWorldStreamFromModel(model *world.WorldModel, playerID int32) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}

	tags := make(map[string]string, len(model.Tags))
	for k, v := range model.Tags {
		tags[k] = v
	}
	return buildWorldStreamFromModelTags(model, tags, playerID)
}

func BuildWorldStreamFromModelSnapshot(model *world.WorldModel, playerID int32, snap world.Snapshot) ([]byte, error) {
	if model == nil {
		return nil, ErrInvalidMSAV
	}
	tags := make(map[string]string, len(model.Tags)+3)
	for k, v := range model.Tags {
		tags[k] = v
	}
	tags["wave"] = strconv.Itoa(int(snap.Wave))
	tags["wavetime"] = fmt.Sprintf("%.2f", snap.WaveTime*60)
	tags["tick"] = fmt.Sprintf("%.0f", float64(snap.Tick))
	return buildWorldStreamFromModelTags(model, tags, playerID)
}

func buildWorldStreamFromModelTags(model *world.WorldModel, tags map[string]string, playerID int32) ([]byte, error) {
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	rules := tags["rules"]
	if rules == "" {
		rules = "{}"
	}
	locales := tags["locales"]
	if locales == "" {
		locales = "{}"
	}
	if err := w.WriteUTF(rules); err != nil {
		return nil, err
	}
	if err := w.WriteUTF(locales); err != nil {
		return nil, err
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

	// rand seeds
	if err := w.WriteInt64(0); err != nil {
		return nil, err
	}
	if err := w.WriteInt64(0); err != nil {
		return nil, err
	}

	if playerID <= 0 {
		playerID = 1
	}
	if err := w.WriteInt32(playerID); err != nil {
		return nil, err
	}
	if err := writeTemplatePlayerForContent(w, model.Content); err != nil {
		return nil, err
	}

	if err := w.WriteBytes(model.Content); err != nil {
		return nil, err
	}
	// Keep compatibility with current decoder expectation.
	if err := w.WriteByte(0); err != nil {
		return nil, err
	}

	// Always rebuild map chunk from current tiles, otherwise runtime build changes are lost.
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
