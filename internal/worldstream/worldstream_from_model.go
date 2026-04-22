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
	return buildWorldStreamFromModelState(model, tags, playerID, 1, 0, 0, 0, 0)
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
	return buildWorldStreamFromModelState(model, tags, playerID, snap.Wave, snap.WaveTime*60, float64(snap.Tick), snap.Rand0, snap.Rand1)
}

func buildWorldStreamFromModelState(model *world.WorldModel, tags map[string]string, playerID int32, wave int32, wavetimeTicks float32, tick float64, rand0, rand1 int64) ([]byte, error) {
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
	if err := w.WriteInt32(wave); err != nil {
		return nil, err
	}

	if err := w.WriteFloat32(wavetimeTicks); err != nil {
		return nil, err
	}
	if err := w.WriteFloat64(tick); err != nil {
		return nil, err
	}

	// rand seeds
	if err := w.WriteInt64(rand0); err != nil {
		return nil, err
	}
	if err := w.WriteInt64(rand1); err != nil {
		return nil, err
	}

	if playerID <= 0 {
		playerID = 1
	}
	if err := w.WriteInt32(playerID); err != nil {
		return nil, err
	}
	if err := writeDirectPlayerPayload(w); err != nil {
		return nil, err
	}

	if err := w.WriteBytes(model.Content); err != nil {
		return nil, err
	}
	patches := model.Patches
	if len(patches) == 0 {
		patches = []byte{0}
	}
	if err := w.WriteBytes(patches); err != nil {
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
	var teamBlocks bytes.Buffer
	if err := writeMinimalTeamBlocks(&javaWriter{buf: &teamBlocks}); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(teamBlocks.Bytes()); err != nil {
		return nil, err
	}
	markers := model.Markers
	if len(markers) == 0 {
		markers = []byte{0x7B, 0x7D}
	}
	if err := w.WriteBytes(markers); err != nil {
		return nil, err
	}
	custom := model.Custom
	if len(custom) == 0 {
		custom = []byte{0, 0, 0, 0}
	}
	if err := w.WriteBytes(custom); err != nil {
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
