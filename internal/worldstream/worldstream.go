package worldstream

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"mdt-server/internal/protocol"
	"mdt-server/internal/runtimeassets"
	"mdt-server/internal/world"
)

var ErrInvalidMSAV = errors.New("invalid msav file")
var ErrUnsupportedMSAVVersion = errors.New("unsupported msav save version")

type MSAVData struct {
	Version                  int32
	Tags                     map[string]string
	Content                  []byte
	Patches                  []byte
	Map                      []byte
	Markers                  []byte
	Custom                   []byte
	RawMeta                  []byte
	RawEntities              []byte
	EntityMapping            []byte
	TeamBlocks               []byte
	WorldEntityChunks        []msavWorldEntityChunk
	WorldEntitiesHaveIDs     bool
	WorldEntitiesShortChunks bool
	LegacyEntityGroups       bool
}

func BuildWorldStreamFromMSAV(path string) ([]byte, error) {
	data, err := readMSAV(path)
	if err != nil {
		return nil, err
	}
	mapChunk := data.Map
	if err := skipMapData(newJavaReader(mapChunk)); err != nil {
		model, merr := LoadWorldModelFromMSAV(path, nil)
		if merr == nil {
			clearInlineBuildingSyncData(model)
			normalized, nerr := encodeMapChunkMinimal(model)
			if nerr == nil {
				mapChunk = normalized
			}
		}
	}
	// NetworkIO.writeWorld sends current runtime team plans, not the raw team-block
	// section preserved inside an arbitrary save file. Old saves can carry plan config
	// objects that deserialize differently clientside, so keep join-world payloads on
	// the safest legal representation until runtime-owned team plans are tracked.
	var teamBlocks bytes.Buffer
	if err := writeMinimalTeamBlocks(&javaWriter{buf: &teamBlocks}); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	w := &javaWriter{buf: &out}

	rules := data.Tags["rules"]
	if rules == "" {
		rules = "{}"
	}
	locales := data.Tags["locales"]
	if locales == "" {
		locales = "{}"
	}
	if err := w.WriteUTF(rules); err != nil {
		return nil, err
	}
	if err := w.WriteUTF(locales); err != nil {
		return nil, err
	}

	if err := w.WriteStringMap(data.Tags); err != nil {
		return nil, err
	}

	wave := int32(1)
	if v, ok := data.Tags["wave"]; ok {
		if parsed, err := strconv.Atoi(v); err == nil {
			wave = int32(parsed)
		}
	}
	if err := w.WriteInt32(wave); err != nil {
		return nil, err
	}

	wavetime := float32(0)
	if v, ok := data.Tags["wavetime"]; ok {
		if parsed, err := strconv.ParseFloat(v, 32); err == nil {
			wavetime = float32(parsed)
		}
	}
	if err := w.WriteFloat32(wavetime); err != nil {
		return nil, err
	}

	tick := float64(0)
	if v, ok := data.Tags["tick"]; ok {
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

	// player id
	if err := w.WriteInt32(1); err != nil {
		return nil, err
	}
	if err := writeDirectPlayerPayload(w); err != nil {
		return nil, err
	}

	if err := w.WriteBytes(data.Content); err != nil {
		return nil, err
	}
	patches := data.Patches
	if len(patches) == 0 {
		patches = []byte{0}
	}
	if err := w.WriteBytes(patches); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(mapChunk); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(teamBlocks.Bytes()); err != nil {
		return nil, err
	}
	markers := data.Markers
	if len(markers) == 0 {
		// UBJSON empty object - decodes as an empty IntMap for markers.
		markers = []byte{0x7B, 0x7D}
	}
	if err := w.WriteBytes(markers); err != nil {
		return nil, err
	}
	custom := data.Custom
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

func clearInlineBuildingSyncData(model *world.WorldModel) {
	if model == nil {
		return
	}
	for i := range model.Tiles {
		build := model.Tiles[i].Build
		if build == nil {
			continue
		}
		build.MapSyncRevision = 0
		build.MapSyncData = nil
		build.MapSyncTail = nil
		build.MapSyncAmmoLoaded = false
	}
}

func ReadMSAVVersion(path string) (int32, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		return 0, err
	}
	r := newJavaReader(decompressed)
	header, err := r.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	if string(header) != "MSAV" {
		return 0, ErrInvalidMSAV
	}
	version, err := r.ReadInt32()
	if err != nil {
		return 0, err
	}
	return version, nil
}

func readMSAV(path string) (MSAVData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return MSAVData{}, err
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return MSAVData{}, err
	}
	defer zr.Close()
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		return MSAVData{}, err
	}

	r := newJavaReader(decompressed)
	header, err := r.ReadBytes(4)
	if err != nil {
		return MSAVData{}, err
	}
	if string(header) != "MSAV" {
		return MSAVData{}, ErrInvalidMSAV
	}
	version, err := r.ReadInt32()
	if err != nil {
		return MSAVData{}, err
	}
	return readMSAVVersioned(r, version)
}

// FindCoreTileFromMSAV tries to locate the first core tile position in a .msav map.
// It returns (pos, true, nil) on success, (zero, false, nil) if not found.
// Deprecated: Use FindCoreTilesFromMSAV instead.
func FindCoreTileFromMSAV(path string) (protocol.Point2, bool, error) {
	coreTiles, err := FindCoreTilesFromMSAV(path)
	if err != nil || len(coreTiles) == 0 {
		return protocol.Point2{}, false, err
	}
	return coreTiles[0], true, nil
}

// FindCoreTilesFromMSAV tries to locate all core tile positions in a .msav map.
// It returns a list of positions, empty if no cores found.
func FindCoreTilesFromMSAV(path string) ([]protocol.Point2, error) {
	data, err := readMSAV(path)
	if err != nil {
		return nil, err
	}
	// 使用 nil registry，fallback 到索引
	blockNames, err := readContentBlockNames(data.Content, nil)
	if err != nil {
		return nil, err
	}
	coreIDs := make(map[int16]struct{})
	for id, name := range blockNames {
		if isCoreBlockName(name) {
			coreIDs[id] = struct{}{}
		}
	}
	// 调试日志：输出找到的核心 ID
	if len(coreIDs) > 0 {
		fmt.Printf("[worldstream] found cores in content: ")
		for id := range coreIDs {
			fmt.Printf("%d(%s) ", id, blockNames[id])
		}
		fmt.Println()
	} else {
		fmt.Printf("[worldstream] no cores found in content, blockNames count=%d\n", len(blockNames))
		// 输出所有块名称用于调试
		count := 0
		for id, name := range blockNames {
			if count < 10 {
				fmt.Printf("[worldstream] block %d: %s\n", id, name)
				count++
			}
		}
	}
	if len(coreIDs) == 0 {
		return []protocol.Point2{}, nil
	}
	return findCoresInMapChunk(data.Map, coreIDs)
}

type javaReader struct {
	buf *bytes.Reader
}

func newJavaReader(b []byte) *javaReader {
	return &javaReader{buf: bytes.NewReader(b)}
}

func (r *javaReader) ReadBytes(n int) ([]byte, error) {
	out := make([]byte, n)
	_, err := io.ReadFull(r.buf, out)
	return out, err
}

func (r *javaReader) ReadByte() (byte, error) {
	return r.buf.ReadByte()
}

func (r *javaReader) Skip(n int) error {
	if n < 0 || r.buf.Len() < n {
		return io.ErrUnexpectedEOF
	}
	_, err := r.buf.Seek(int64(n), io.SeekCurrent)
	return err
}

func (r *javaReader) Offset() int {
	return int(r.buf.Size()) - r.buf.Len()
}

func (r *javaReader) ReadInt32() (int32, error) {
	var v int32
	err := readBE(r.buf, &v)
	return v, err
}

func (r *javaReader) ReadInt64() (int64, error) {
	var v int64
	err := readBE(r.buf, &v)
	return v, err
}

func (r *javaReader) ReadFloat32() (float32, error) {
	var v float32
	err := readBE(r.buf, &v)
	return v, err
}

func (r *javaReader) ReadFloat64() (float64, error) {
	var v float64
	err := readBE(r.buf, &v)
	return v, err
}

func (r *javaReader) ReadChunk() ([]byte, error) {
	var length int32
	if err := readBE(r.buf, &length); err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, ErrInvalidMSAV
	}
	return r.ReadBytes(int(length))
}

func readStringMap(chunk []byte) (map[string]string, error) {
	r := newJavaReader(chunk)
	size, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, size)
	for i := 0; i < int(size); i++ {
		k, err := r.ReadUTF()
		if err != nil {
			return nil, err
		}
		v, err := r.ReadUTF()
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func readStringMapInline(r *javaReader) (map[string]string, error) {
	size, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, size)
	for i := 0; i < int(size); i++ {
		k, err := r.ReadUTF()
		if err != nil {
			return nil, err
		}
		v, err := r.ReadUTF()
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func readContentBlockNames(chunk []byte, registry *protocol.ContentRegistry) (map[int16]string, error) {
	return readContentNamesOfType(chunk, 1, registry) // ContentType.block
}

func readContentUnitNames(chunk []byte, registry *protocol.ContentRegistry) (map[int16]string, error) {
	return readContentNamesOfType(chunk, 6, registry) // ContentType.unit
}

func readContentNamesOfType(chunk []byte, typeID byte, registry *protocol.ContentRegistry) (map[int16]string, error) {
	// Prefer the map's own content-id mapping first.
	// This avoids runtime registry order drift from breaking block/unit name resolution.
	r := newJavaReader(chunk)
	mapped, err := r.ReadByte()
	if err != nil {
		if registry != nil {
			return readContentNamesFromRegistry(typeID, registry), nil
		}
		return nil, err
	}
	out := map[int16]string{}
	for i := 0; i < int(mapped); i++ {
		ct, err := r.ReadByte()
		if err != nil {
			if registry != nil {
				return readContentNamesFromRegistry(typeID, registry), nil
			}
			return nil, err
		}
		total, err := r.ReadInt16()
		if err != nil {
			if registry != nil {
				return readContentNamesFromRegistry(typeID, registry), nil
			}
			return nil, err
		}
		for id := int16(0); id < total; id++ {
			name, err := r.ReadUTF()
			if err != nil {
				if registry != nil {
					return readContentNamesFromRegistry(typeID, registry), nil
				}
				return nil, err
			}
			if ct == typeID {
				out[id] = strings.ToLower(strings.TrimSpace(name))
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if registry != nil {
		return readContentNamesFromRegistry(typeID, registry), nil
	}
	return out, nil
}

func readContentNamesFromRegistry(typeID byte, registry *protocol.ContentRegistry) map[int16]string {
	out := map[int16]string{}
	if registry == nil {
		return out
	}
	switch typeID {
	case 1: // ContentType.block
		registry.IterateBlocks(func(b protocol.Block) bool {
			out[b.ID()] = strings.ToLower(strings.TrimSpace(b.Name()))
			return true
		})
	case 6: // ContentType.unit
		registry.IterateUnitTypes(func(u protocol.UnitType) bool {
			out[u.ID()] = strings.ToLower(strings.TrimSpace(u.Name()))
			return true
		})
	}
	return out
}

func buildContentHeaderFromRegistry(registry *protocol.ContentRegistry) ([]byte, error) {
	if registry == nil {
		return nil, errors.New("nil content registry")
	}
	snapshot := registry.SnapshotContentNames()
	if len(snapshot) == 0 {
		return nil, errors.New("empty content registry")
	}

	type section struct {
		typ   protocol.ContentType
		names map[int16]string
	}
	sections := make([]section, 0, len(snapshot))
	for typ, names := range snapshot {
		if len(names) == 0 {
			continue
		}
		sections = append(sections, section{typ: typ, names: names})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].typ < sections[j].typ })

	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteByte(byte(len(sections))); err != nil {
		return nil, err
	}
	for _, section := range sections {
		if err := w.WriteByte(byte(section.typ)); err != nil {
			return nil, err
		}
		maxID := int16(-1)
		for id := range section.names {
			if id > maxID {
				maxID = id
			}
		}
		total := int(maxID) + 1
		if total < 0 {
			total = 0
		}
		if err := w.WriteInt16(int16(total)); err != nil {
			return nil, err
		}
		for id := int16(0); id < int16(total); id++ {
			name := strings.TrimSpace(section.names[id])
			if err := w.WriteUTF(name); err != nil {
				return nil, err
			}
		}
	}
	return out.Bytes(), nil
}

func isCoreBlockName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	// core-zone is a floor marker in Erekir, not an actual CoreBlock.
	if name == "core-zone" {
		return false
	}
	return strings.HasPrefix(name, "core-")
}

func findCoresInMapChunk(chunk []byte, coreIDs map[int16]struct{}) ([]protocol.Point2, error) {
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

	// floors + overlays
	for i := 0; i < total; i++ {
		if err := r.Skip(2 + 2); err != nil {
			return nil, err
		}
		con, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		i += int(con)
	}

	coreMask := make([]bool, total)

	// blocks
	for i := 0; i < total; i++ {
		blockID, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		packed, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		isCore := false
		if _, ok := coreIDs[blockID]; ok {
			isCore = true
			coreMask[i] = true
		}
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
				chunkLen, err := r.ReadInt32()
				if err != nil {
					return nil, err
				}
				if chunkLen < 0 {
					return nil, ErrInvalidMSAV
				}
				if err := r.Skip(int(chunkLen)); err != nil {
					return nil, err
				}
			}
		} else if hadDataOld || hadDataNew {
			if hadDataOld {
				if err := r.Skip(1); err != nil {
					return nil, err
				}
			}
		} else {
			con, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			run := int(con)
			if isCore {
				for j := 1; j <= run; j++ {
					idx := i + j
					if idx >= total {
						return nil, fmt.Errorf("map core run out of range: %d/%d", idx, total)
					}
					coreMask[idx] = true
				}
			}
			i += run
		}
	}

	visited := make([]bool, total)
	cores := make([]protocol.Point2, 0)
	queue := make([]int, 0, 16)
	push := func(idx int) {
		if idx < 0 || idx >= total || visited[idx] || !coreMask[idx] {
			return
		}
		visited[idx] = true
		queue = append(queue, idx)
	}

	for start := 0; start < total; start++ {
		if visited[start] || !coreMask[start] {
			continue
		}
		queue = queue[:0]
		push(start)
		minX, maxX := start%w, start%w
		minY, maxY := start/w, start/w
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			x, y := idx%w, idx/w
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			if x > 0 {
				push(idx - 1)
			}
			if x+1 < w {
				push(idx + 1)
			}
			if y > 0 {
				push(idx - w)
			}
			if y+1 < h {
				push(idx + w)
			}
		}
		cores = append(cores, protocol.Point2{
			X: int32((minX + maxX) / 2),
			Y: int32((minY + maxY) / 2),
		})
	}

	sort.Slice(cores, func(i, j int) bool {
		if cores[i].Y != cores[j].Y {
			return cores[i].Y < cores[j].Y
		}
		return cores[i].X < cores[j].X
	})
	return cores, nil
}

func findCoreInMapChunk(chunk []byte, coreIDs map[int16]struct{}) (protocol.Point2, bool, error) {
	cores, err := findCoresInMapChunk(chunk, coreIDs)
	if err != nil || len(cores) == 0 {
		return protocol.Point2{}, false, err
	}
	return cores[0], true, nil
}

func (r *javaReader) ReadInt16() (int16, error) {
	var v int16
	err := readBE(r.buf, &v)
	return v, err
}

func (r *javaReader) ReadUTF() (string, error) {
	var n uint16
	if err := readBE(r.buf, &n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	b, err := r.ReadBytes(int(n))
	if err != nil {
		return "", err
	}
	return decodeModifiedUTF8(b)
}

func (r *javaReader) SkipUTF() error {
	var n uint16
	if err := readBE(r.buf, &n); err != nil {
		return err
	}
	return r.Skip(int(n))
}

func (r *javaReader) SkipTypeIOString() error {
	exists, err := r.ReadByte()
	if err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	return r.SkipUTF()
}

func skipTypeIOObject(r *javaReader) error {
	typ, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch typ {
	case 0:
		return nil
	case 1, 3, 12, 17:
		return r.Skip(4)
	case 2, 11:
		return r.Skip(8)
	case 4:
		return r.SkipTypeIOString()
	case 5, 9:
		return r.Skip(3)
	case 6:
		count, err := r.ReadInt16()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		return r.Skip(int(count) * 4)
	case 7, 19:
		return r.Skip(8)
	case 8:
		count, err := r.ReadByte()
		if err != nil {
			return err
		}
		return r.Skip(int(count) * 4)
	case 10, 15, 20:
		return r.Skip(1)
	case 13, 23:
		return r.Skip(2)
	case 14, 16:
		count, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		if typ == 16 {
			return r.Skip(int(count))
		}
		return r.Skip(int(count))
	case 18:
		count, err := r.ReadInt16()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		return r.Skip(int(count) * 8)
	case 21:
		count, err := r.ReadInt16()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		return r.Skip(int(count) * 4)
	case 22:
		count, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		for i := 0; i < int(count); i++ {
			if err := skipTypeIOObject(r); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported typeio object type: %d", typ)
	}
}

type javaWriter struct {
	buf *bytes.Buffer
}

func (w *javaWriter) WriteBytes(b []byte) error {
	_, err := w.buf.Write(b)
	return err
}

func (w *javaWriter) WriteChunkBytes(b []byte) error {
	if err := w.WriteInt32(int32(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

func (w *javaWriter) WriteByte(v byte) error {
	return w.buf.WriteByte(v)
}

func (w *javaWriter) WriteBool(v bool) error {
	if v {
		return w.WriteByte(1)
	}
	return w.WriteByte(0)
}

func (w *javaWriter) WriteInt16(v int16) error {
	return writeBE(w.buf, v)
}

func (w *javaWriter) WriteInt32(v int32) error {
	return writeBE(w.buf, v)
}

func (w *javaWriter) WriteInt64(v int64) error {
	return writeBE(w.buf, v)
}

func (w *javaWriter) WriteFloat32(v float32) error {
	return writeBE(w.buf, v)
}

func (w *javaWriter) WriteFloat64(v float64) error {
	return writeBE(w.buf, v)
}

func (w *javaWriter) WriteUTF(s string) error {
	encoded := encodeModifiedUTF8(s)
	if len(encoded) > 0xFFFF {
		return fmt.Errorf("string too long: %d", len(encoded))
	}
	if err := writeBE(w.buf, uint16(len(encoded))); err != nil {
		return err
	}
	_, err := w.buf.Write(encoded)
	return err
}

func (w *javaWriter) WriteStringMap(m map[string]string) error {
	if len(m) > 0x7FFF {
		return fmt.Errorf("string map too large: %d", len(m))
	}
	if err := w.WriteInt16(int16(len(m))); err != nil {
		return err
	}
	for k, v := range m {
		if err := w.WriteUTF(k); err != nil {
			return err
		}
		if err := w.WriteUTF(v); err != nil {
			return err
		}
	}
	return nil
}

func (w *javaWriter) WriteTypeIOString(s *string) error {
	if s == nil {
		return w.WriteByte(0)
	}
	if err := w.WriteByte(1); err != nil {
		return err
	}
	return w.WriteUTF(*s)
}

func writeMinimalTeamBlocks(w *javaWriter) error {
	if err := w.WriteInt32(1); err != nil {
		return err
	}
	if err := w.WriteInt32(1); err != nil {
		return err
	}
	return w.WriteInt32(0)
}

func writeMinimalCustomChunks(w *javaWriter) error {
	return w.WriteInt32(0)
}

func writeDirectPlayerPayload(w *javaWriter) error {
	return writeMinimalPlayer(w)
}

func writeMinimalPlayer(w *javaWriter) error {
	empty := ""
	if err := w.WriteInt16(2); err != nil {
		return err
	}
	if err := w.WriteBool(false); err != nil {
		return err
	}
	if err := w.WriteBool(false); err != nil {
		return err
	}
	if err := w.WriteInt32(0); err != nil {
		return err
	}
	if err := w.WriteByte(255); err != nil {
		return err
	}
	if err := w.WriteFloat32(0); err != nil {
		return err
	}
	if err := w.WriteFloat32(0); err != nil {
		return err
	}
	if err := w.WriteTypeIOString(&empty); err != nil {
		return err
	}
	if err := w.WriteInt16(-1); err != nil {
		return err
	}
	if err := w.WriteInt32(0); err != nil {
		return err
	}
	if err := w.WriteBool(false); err != nil {
		return err
	}
	if err := w.WriteByte(1); err != nil {
		return err
	}
	if err := w.WriteBool(false); err != nil {
		return err
	}
	if err := w.WriteByte(0); err != nil {
		return err
	}
	if err := w.WriteInt32(0); err != nil {
		return err
	}
	if err := w.WriteFloat32(0); err != nil {
		return err
	}
	return w.WriteFloat32(0)
}

var templateRawOnce sync.Once
var templateRaw []byte
var templateRawErr error
var templatePlayerCacheMu sync.Mutex
var templatePlayerCache = map[uint64][]byte{}

func writeTemplatePlayerForContent(w *javaWriter, content []byte) error {
	payload, err := templatePlayerPayloadForContent(content)
	if err == nil && len(payload) > 0 {
		return w.WriteBytes(payload)
	}
	// Fallback: direct placeholder payload when template extraction fails.
	return writeDirectPlayerPayload(w)
}

func templatePlayerPayloadForContent(content []byte) ([]byte, error) {
	if len(content) == 0 {
		return nil, errors.New("empty content header")
	}
	key := hash64(content)
	templatePlayerCacheMu.Lock()
	if cached, ok := templatePlayerCache[key]; ok {
		templatePlayerCacheMu.Unlock()
		return cached, nil
	}
	templatePlayerCacheMu.Unlock()

	raw, err := loadTemplateWorldRaw()
	if err != nil {
		return nil, err
	}
	payload, err := extractPlayerPayloadFromWorldStream(raw)
	if err != nil {
		return nil, err
	}
	templatePlayerCacheMu.Lock()
	templatePlayerCache[key] = payload
	templatePlayerCacheMu.Unlock()
	return payload, nil
}

func loadTemplateWorldRaw() ([]byte, error) {
	templateRawOnce.Do(func() {
		templateRaw, templateRawErr = readBootstrapWorldRaw()
	})
	return templateRaw, templateRawErr
}

func readBootstrapWorldRaw() ([]byte, error) {
	data, _, err := runtimeassets.LoadBootstrapWorld("")
	if err != nil || len(data) == 0 {
		return nil, errors.New("template world stream not found")
	}
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func extractPlayerPayloadFromWorldStream(raw []byte) ([]byte, error) {
	playerStart, err := locatePlayerStart(raw)
	if err != nil {
		return nil, err
	}
	if playerStart >= len(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	r := newJavaReader(raw[playerStart:])
	playerRev, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return nil, err
	}
	playerEnd := playerStart + r.Offset()
	if playerEnd > len(raw) {
		return nil, io.ErrUnexpectedEOF
	}

	if _, _, _, _, _, _, _, err := inspectWorldSections(raw, playerEnd); err != nil {
		return nil, fmt.Errorf("template world section validation failed: %w", err)
	}

	out := make([]byte, playerEnd-playerStart)
	copy(out, raw[playerStart:playerEnd])
	return out, nil
}

func inspectWorldSections(raw []byte, start int) (contentEnd, patchesEnd, mapEnd, teamBlocksLen, markersLen, customLen int, chunked bool, err error) {
	if start < 0 || start > len(raw) {
		return 0, 0, 0, 0, 0, 0, false, io.ErrUnexpectedEOF
	}

	// Fallback for legacy/bootstrap assets that still store raw world sections without
	// chunk prefixes after the player payload.
	validate := newJavaReader(raw[start:])
	if err := skipContentHeader(validate); err != nil {
		// Some older locally generated payloads incorrectly wrapped sections with chunk
		// lengths. Keep a compatibility fallback so diagnostics and legacy tests can still
		// inspect those payloads.
		chunkReader := newJavaReader(raw[start:])
		contentChunk, cErr := chunkReader.ReadChunk()
		if cErr == nil && skipContentHeader(newJavaReader(contentChunk)) == nil {
			contentEnd = start + chunkReader.Offset()
			patchesChunk, pErr := chunkReader.ReadChunk()
			if pErr == nil && skipContentPatches(newJavaReader(patchesChunk)) == nil {
				patchesEnd = start + chunkReader.Offset()
				mapChunk, mErr := chunkReader.ReadChunk()
				if mErr == nil && skipMapData(newJavaReader(mapChunk)) == nil {
					mapEnd = start + chunkReader.Offset()
					teamBlocks, tErr := chunkReader.ReadChunk()
					if tErr == nil {
						markers, mkErr := chunkReader.ReadChunk()
						if mkErr == nil {
							custom, cuErr := chunkReader.ReadChunk()
							if cuErr == nil {
								return contentEnd, patchesEnd, mapEnd, len(teamBlocks), len(markers), len(custom), true, nil
							}
						}
					}
				}
			}
		}
		return 0, 0, 0, 0, 0, 0, false, err
	}
	contentEnd = start + validate.Offset()
	if err := skipContentPatches(validate); err != nil {
		return 0, 0, 0, 0, 0, 0, false, err
	}
	patchesEnd = start + validate.Offset()
	if err := skipMapData(validate); err != nil {
		return 0, 0, 0, 0, 0, 0, false, err
	}
	mapEnd = start + validate.Offset()
	teamReader := newJavaReader(raw[mapEnd:])
	teamBlocks, _, err := readModernTeamBlocksRaw(teamReader, raw[mapEnd:])
	if err != nil {
		return 0, 0, 0, 0, 0, 0, false, err
	}
	tail := raw[mapEnd+len(teamBlocks):]
	markersLen, customLen, err = splitRawMarkersAndCustom(tail)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, false, err
	}
	return contentEnd, patchesEnd, mapEnd, len(teamBlocks), markersLen, customLen, false, nil
}

func splitRawMarkersAndCustom(tail []byte) (markersLen int, customLen int, err error) {
	if len(tail) == 0 {
		return 0, 0, nil
	}
	for split := 0; split <= len(tail); split++ {
		custom := tail[split:]
		r := newJavaReader(custom)
		if err := skipCustomChunks(r); err != nil {
			continue
		}
		if r.Offset() != len(custom) {
			continue
		}
		return split, len(custom), nil
	}
	return 0, 0, fmt.Errorf("unable to split markers/custom tail len=%d", len(tail))
}

func locatePlayerStart(raw []byte) (int, error) {
	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return 0, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return 0, err
	}
	if _, err := readStringMapInline(r); err != nil {
		return 0, err
	}
	if _, err := r.ReadInt32(); err != nil { // wave
		return 0, err
	}
	if _, err := r.ReadFloat32(); err != nil { // wavetime
		return 0, err
	}
	if _, err := r.ReadFloat64(); err != nil { // tick
		return 0, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed0
		return 0, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed1
		return 0, err
	}
	if _, err := r.ReadInt32(); err != nil { // player id
		return 0, err
	}
	return r.Offset(), nil
}

func hash64(b []byte) uint64 {
	var h uint64 = 1469598103934665603
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

func encodeModifiedUTF8(s string) []byte {
	if s == "" {
		return nil
	}
	var out []byte
	for _, r := range s {
		switch {
		case r == 0:
			out = append(out, 0xC0, 0x80)
		case r <= 0x7F:
			out = append(out, byte(r))
		case r <= 0x7FF:
			out = append(out, 0xC0|byte(r>>6))
			out = append(out, 0x80|byte(r&0x3F))
		case r <= 0xFFFF:
			out = append(out, 0xE0|byte(r>>12))
			out = append(out, 0x80|byte((r>>6)&0x3F))
			out = append(out, 0x80|byte(r&0x3F))
		default:
			r -= 0x10000
			hi := rune(0xD800 + (r >> 10))
			lo := rune(0xDC00 + (r & 0x3FF))
			out = append(out, 0xE0|byte(hi>>12))
			out = append(out, 0x80|byte((hi>>6)&0x3F))
			out = append(out, 0x80|byte(hi&0x3F))
			out = append(out, 0xE0|byte(lo>>12))
			out = append(out, 0x80|byte((lo>>6)&0x3F))
			out = append(out, 0x80|byte(lo&0x3F))
		}
	}
	return out
}

func decodeModifiedUTF8(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	var units []uint16
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c>>7 == 0:
			units = append(units, uint16(c))
			i++
		case (c & 0xE0) == 0xC0:
			if i+1 >= len(b) {
				return "", ErrInvalidMSAV
			}
			c2 := b[i+1]
			ch := uint16(c&0x1F)<<6 | uint16(c2&0x3F)
			units = append(units, ch)
			i += 2
		case (c & 0xF0) == 0xE0:
			if i+2 >= len(b) {
				return "", ErrInvalidMSAV
			}
			c2 := b[i+1]
			c3 := b[i+2]
			ch := uint16(c&0x0F)<<12 | uint16(c2&0x3F)<<6 | uint16(c3&0x3F)
			units = append(units, ch)
			i += 3
		default:
			return "", ErrInvalidMSAV
		}
	}
	runes := utf16.Decode(units)
	return string(runes), nil
}

func readBE(r io.Reader, data any) error {
	return binaryReadBE(r, data)
}

func writeBE(w io.Writer, data any) error {
	return binaryWriteBE(w, data)
}

func binaryReadBE(r io.Reader, data any) error {
	return binary.Read(r, binary.BigEndian, data)
}

func binaryWriteBE(w io.Writer, data any) error {
	return binary.Write(w, binary.BigEndian, data)
}

// Trim a .msav.msav suffix to keep map names readable.
func TrimMapName(name string) string {
	name = strings.TrimSuffix(name, ".msav")
	name = strings.TrimSuffix(name, ".msav")
	return name
}

// NormalizePlayerRevisionInWorldStream rewrites the player revision field in a
// zlib-compressed NetworkIO.writeWorld payload for compatibility across builds.
func NormalizePlayerRevisionInWorldStream(payload []byte, rev int16) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, err
	}

	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	entries, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if entries < 0 {
		return nil, ErrInvalidMSAV
	}
	for i := 0; i < int(entries); i++ {
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
	}

	// wave, wavetime, tick, seed0, seed1, player id
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}
	if _, err := r.ReadFloat32(); err != nil {
		return nil, err
	}
	if _, err := r.ReadFloat64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}

	revPos := len(raw) - r.buf.Len()
	if revPos+2 > len(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint16(raw[revPos:revPos+2], uint16(rev))

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RewritePlayerIDInWorldStream rewrites the player ID field in a
// zlib-compressed NetworkIO.writeWorld payload.
func RewritePlayerIDInWorldStream(payload []byte, playerID int32) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, err
	}

	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	entries, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if entries < 0 {
		return nil, ErrInvalidMSAV
	}
	for i := 0; i < int(entries); i++ {
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
	}

	// wave, wavetime, tick, seed0, seed1
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}
	if _, err := r.ReadFloat32(); err != nil {
		return nil, err
	}
	if _, err := r.ReadFloat64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}

	idPos := r.Offset()
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}
	if idPos+4 > len(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint32(raw[idPos:idPos+4], uint32(playerID))

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RewriteRulesInWorldStream rewrites the leading rules JSON blob in a
// zlib-compressed NetworkIO.writeWorld payload while preserving the rest.
func RewriteRulesInWorldStream(payload []byte, rules string) ([]byte, error) {
	if strings.TrimSpace(rules) == "" {
		rules = "{}"
	}
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, err
	}

	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	oldRulesEnd := r.Offset()
	if oldRulesEnd > len(raw) {
		return nil, io.ErrUnexpectedEOF
	}

	var head bytes.Buffer
	w := &javaWriter{buf: &head}
	if err := w.WriteUTF(rules); err != nil {
		return nil, err
	}

	patched := make([]byte, 0, head.Len()+len(raw)-oldRulesEnd)
	patched = append(patched, head.Bytes()...)
	patched = append(patched, raw[oldRulesEnd:]...)

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(patched); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RewriteRuntimeStateInWorldStream rewrites wave/wavetime/tick/playerID fields in a
// zlib-compressed NetworkIO.writeWorld payload while preserving the rest of the payload.
func RewriteRuntimeStateInWorldStream(payload []byte, wave int32, wavetimeTicks float32, tick float64, playerID int32) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, err
	}

	r := newJavaReader(raw)
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	if _, err := r.ReadUTF(); err != nil {
		return nil, err
	}
	entries, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if entries < 0 {
		return nil, ErrInvalidMSAV
	}
	for i := 0; i < int(entries); i++ {
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
		if err := r.SkipUTF(); err != nil {
			return nil, err
		}
	}

	wavePos := r.Offset()
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}
	wavetimePos := r.Offset()
	if _, err := r.ReadFloat32(); err != nil {
		return nil, err
	}
	tickPos := r.Offset()
	if _, err := r.ReadFloat64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil {
		return nil, err
	}
	playerIDPos := r.Offset()
	if _, err := r.ReadInt32(); err != nil {
		return nil, err
	}

	if wavePos+4 > len(raw) || wavetimePos+4 > len(raw) || tickPos+8 > len(raw) || playerIDPos+4 > len(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint32(raw[wavePos:wavePos+4], uint32(wave))
	binary.BigEndian.PutUint32(raw[wavetimePos:wavetimePos+4], math.Float32bits(wavetimeTicks))
	binary.BigEndian.PutUint64(raw[tickPos:tickPos+8], math.Float64bits(tick))
	binary.BigEndian.PutUint32(raw[playerIDPos:playerIDPos+4], uint32(playerID))

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func skipPlayerPayload(r *javaReader, rev int16) error {
	// Matches observed Player.write()/read() revisions in current official payloads:
	// rev0: admin, boosting, color, mouseX, mouseY, name, shooting, team, typing, unit, x, y
	// rev1: admin, boosting, color, lastCommand, mouseX, mouseY, name, shooting, team, typing, unit, x, y
	// rev2: admin, boosting, color, lastCommand, mouseX, mouseY, name, selectedBlock, selectedRotation, shooting, team, typing, unit, x, y
	switch rev {
	case 0:
		// admin + boosting
		if err := r.Skip(2); err != nil {
			return err
		}
		// color
		if err := r.Skip(4); err != nil {
			return err
		}
		// mouse x/y
		if err := r.Skip(8); err != nil {
			return err
		}
		if err := r.SkipTypeIOString(); err != nil {
			return err
		}
	case 1:
		if err := r.Skip(2); err != nil {
			return err
		}
		if err := r.Skip(4); err != nil {
			return err
		}
		// command byte
		if err := r.Skip(1); err != nil {
			return err
		}
		if err := r.Skip(8); err != nil {
			return err
		}
		if err := r.SkipTypeIOString(); err != nil {
			return err
		}
	case 2:
		if err := r.Skip(2); err != nil {
			return err
		}
		if err := r.Skip(4); err != nil {
			return err
		}
		if err := r.Skip(1); err != nil {
			return err
		}
		if err := r.Skip(8); err != nil {
			return err
		}
		if err := r.SkipTypeIOString(); err != nil {
			return err
		}
		// selectedBlock short + selectedRotation int
		if err := r.Skip(2 + 4); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported player revision: %d", rev)
	}

	// shooting, team, typing
	if err := r.Skip(3); err != nil {
		return err
	}
	// unit reference: byte type + int id
	if err := r.Skip(1 + 4); err != nil {
		return err
	}
	// x/y
	return r.Skip(8)
}

func skipContentHeader(r *javaReader) error {
	mapped, err := r.ReadByte()
	if err != nil {
		return err
	}
	for i := 0; i < int(mapped); i++ {
		if _, err := r.ReadByte(); err != nil {
			return err
		}
		total, err := r.ReadInt16()
		if err != nil {
			return err
		}
		if total < 0 {
			return ErrInvalidMSAV
		}
		for j := 0; j < int(total); j++ {
			if err := r.SkipUTF(); err != nil {
				return err
			}
		}
	}
	return nil
}

func skipContentPatches(r *javaReader) error {
	amount, err := r.ReadByte()
	if err != nil {
		return err
	}
	for i := 0; i < int(amount); i++ {
		l, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if l < 0 {
			return ErrInvalidMSAV
		}
		if err := r.Skip(int(l)); err != nil {
			return err
		}
	}
	return nil
}

func skipCustomChunks(r *javaReader) error {
	count, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if count < 0 {
		return ErrInvalidMSAV
	}
	for i := 0; i < int(count); i++ {
		if err := r.SkipUTF(); err != nil {
			return err
		}
		length, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if length < 0 {
			return ErrInvalidMSAV
		}
		if err := r.Skip(int(length)); err != nil {
			return err
		}
	}
	return nil
}

func skipMapData(r *javaReader) error {
	width, err := r.ReadInt16()
	if err != nil {
		return err
	}
	height, err := r.ReadInt16()
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 {
		return ErrInvalidMSAV
	}
	total := int(width) * int(height)

	// floors + overlays
	for i := 0; i < total; i++ {
		if err := r.Skip(2 + 2); err != nil {
			return err
		}
		con, err := r.ReadByte()
		if err != nil {
			return err
		}
		i += int(con)
	}

	// blocks
	for i := 0; i < total; i++ {
		if err := r.Skip(2); err != nil { // block id
			return err
		}
		packed, err := r.ReadByte()
		if err != nil {
			return err
		}
		hadEntity := (packed & 1) != 0
		hadData := (packed & 4) != 0
		if hadData {
			if err := r.Skip(1 + 1 + 1 + 4); err != nil {
				return err
			}
		}
		if hadEntity {
			isCenter, err := r.ReadByte()
			if err != nil {
				return err
			}
			if isCenter == 1 {
				chunkLen, err := r.ReadInt32()
				if err != nil {
					return err
				}
				if chunkLen < 0 {
					return ErrInvalidMSAV
				}
				if err := r.Skip(int(chunkLen)); err != nil {
					return err
				}
			}
		} else if !hadData {
			con, err := r.ReadByte()
			if err != nil {
				return err
			}
			i += int(con)
		}
	}
	return nil
}
