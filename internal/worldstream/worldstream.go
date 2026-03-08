package worldstream

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"mdt-server/internal/protocol"
)

var ErrInvalidMSAV = errors.New("invalid msav file")
var ErrUnsupportedMSAVVersion = errors.New("unsupported msav save version")

type MSAVData struct {
	Version     int32
	Tags        map[string]string
	Content     []byte
	Patches     []byte
	Map         []byte
	Markers     []byte
	Custom      []byte
	RawMeta     []byte
	RawEntities []byte
}

func BuildWorldStreamFromMSAV(path string) ([]byte, error) {
	data, err := readMSAV(path)
	if err != nil {
		return nil, err
	}
	if data.Version < 11 {
		return nil, fmt.Errorf("%w: %d (need >= 11)", ErrUnsupportedMSAVVersion, data.Version)
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
	if err := writeMinimalPlayer(w); err != nil {
		return nil, err
	}

	if err := w.WriteBytes(data.Content); err != nil {
		return nil, err
	}
	// Always write empty content patches for compatibility across builds.
	if err := w.WriteByte(0); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(data.Map); err != nil {
		return nil, err
	}
	if err := writeMinimalTeamBlocks(w); err != nil {
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

	meta, err := r.ReadChunk()
	if err != nil {
		return MSAVData{}, err
	}
	tags, err := readStringMap(meta)
	if err != nil {
		return MSAVData{}, err
	}
	content, err := r.ReadChunk()
	if err != nil {
		return MSAVData{}, err
	}

	var patches []byte
	if version >= 11 {
		patches, err = r.ReadChunk()
		if err != nil {
			return MSAVData{}, err
		}
	}

	mapChunk, err := r.ReadChunk()
	if err != nil {
		return MSAVData{}, err
	}

	entities, err := r.ReadChunk()
	if err != nil {
		return MSAVData{}, err
	}

	var markers []byte
	if version >= 8 {
		markers, err = r.ReadChunk()
		if err != nil {
			return MSAVData{}, err
		}
	}

	var custom []byte
	if version >= 7 {
		custom, err = r.ReadChunk()
		if err != nil {
			return MSAVData{}, err
		}
	}

	return MSAVData{
		Version:     version,
		Tags:        tags,
		Content:     content,
		Patches:     patches,
		Map:         mapChunk,
		Markers:     markers,
		Custom:      custom,
		RawMeta:     meta,
		RawEntities: entities,
	}, nil
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
	// 优先使用 content registry 中的映射，忽略 MSAV 文件中的 content chunk
	// 因为 MSAV 文件的 content chunk 可能使用旧版 Mindustry 的 content 顺序
	if registry != nil {
		out := map[int16]string{}
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
		return out, nil
	}

	// 如果没有 registry，使用 MSAV 文件中的 content chunk（向后兼容）
	r := newJavaReader(chunk)
	mapped, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	out := map[int16]string{}
	for i := 0; i < int(mapped); i++ {
		ct, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		total, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		for id := int16(0); id < total; id++ {
			name, err := r.ReadUTF()
			if err != nil {
				return nil, err
			}
			if ct == typeID {
				out[id] = name
			}
		}
	}
	return out, nil
}

func isCoreBlockName(name string) bool {
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
	total := int(width) * int(height)

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

	var cores []protocol.Point2

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
		if _, ok := coreIDs[blockID]; ok {
			x := int32(i % int(width))
			y := int32(i / int(width))
			cores = append(cores, protocol.Point2{X: x, Y: y})
		}
		hadEntity := (packed & 1) != 0
		hadData := (packed & 4) != 0
		if hadData {
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
		} else if !hadData {
			con, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			i += int(con)
		}
	}
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

type javaWriter struct {
	buf *bytes.Buffer
}

func (w *javaWriter) WriteBytes(b []byte) error {
	_, err := w.buf.Write(b)
	return err
}

func (w *javaWriter) WriteByte(v byte) error {
	return w.buf.WriteByte(v)
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

func writeMinimalPlayer(w *javaWriter) error {
	pw := protocol.NewWriter()
	if err := pw.WriteInt16(1); err != nil {
		return err
	}
	if err := pw.WriteBool(false); err != nil {
		return err
	}
	if err := pw.WriteBool(false); err != nil {
		return err
	}
	if err := protocol.WriteColor(pw, protocol.Color{RGBA: 0}); err != nil {
		return err
	}
	if err := protocol.WriteCommand(pw, nil); err != nil {
		return err
	}
	if err := pw.WriteFloat32(0); err != nil {
		return err
	}
	if err := pw.WriteFloat32(0); err != nil {
		return err
	}
	empty := ""
	if err := protocol.WriteString(pw, &empty); err != nil {
		return err
	}
	if err := pw.WriteBool(false); err != nil {
		return err
	}
	if err := protocol.WriteTeam(pw, &protocol.Team{ID: 1}); err != nil {
		return err
	}
	if err := pw.WriteBool(false); err != nil {
		return err
	}
	if err := protocol.WriteUnit(pw, nil); err != nil {
		return err
	}
	if err := pw.WriteFloat32(0); err != nil {
		return err
	}
	if err := pw.WriteFloat32(0); err != nil {
		return err
	}
	return w.WriteBytes(pw.Bytes())
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
	// Fallback: old minimal payload (may be incompatible with newer clients).
	return writeMinimalPlayer(w)
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

	playerStart, err := locatePlayerStart(raw)
	if err != nil {
		return nil, err
	}
	if playerStart < 0 || playerStart >= len(raw) {
		return nil, io.ErrUnexpectedEOF
	}

	// Prefer exact content header match: locate the content chunk bytes in template raw.
	if idx := bytes.Index(raw[playerStart:], content); idx >= 0 {
		payload := append([]byte(nil), raw[playerStart:playerStart+idx]...)
		templatePlayerCacheMu.Lock()
		templatePlayerCache[key] = payload
		templatePlayerCacheMu.Unlock()
		return payload, nil
	}

	// Fallback: heuristic scan when content differs (e.g., mods) or template mismatch.
	if payload, err := extractPlayerPayloadFromWorldStream(raw); err == nil && len(payload) > 0 {
		templatePlayerCacheMu.Lock()
		templatePlayerCache[key] = payload
		templatePlayerCacheMu.Unlock()
		return payload, nil
	}
	return nil, errors.New("unable to locate player payload in template world stream")
}

func loadTemplateWorldRaw() ([]byte, error) {
	templateRawOnce.Do(func() {
		templateRaw, templateRawErr = readBootstrapWorldRaw()
	})
	return templateRaw, templateRawErr
}

func readBootstrapWorldRaw() ([]byte, error) {
	candidates := []string{
		filepath.Join("assets", "bootstrap-world.bin"),
		filepath.Join("go-server", "assets", "bootstrap-world.bin"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			continue
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
	return nil, errors.New("template world stream not found")
}

func extractPlayerPayloadFromWorldStream(raw []byte) ([]byte, error) {
	playerStart, err := locatePlayerStart(raw)
	if err != nil {
		return nil, err
	}
	if playerStart >= len(raw) {
		return nil, io.ErrUnexpectedEOF
	}

	const minPlayerLen = 8
	const maxScan = 8192
	limit := maxScan
	if playerStart+limit > len(raw) {
		limit = len(raw) - playerStart
	}
	for delta := minPlayerLen; delta <= limit; delta++ {
		rr := newJavaReader(raw[playerStart+delta:])
		if err := skipContentHeader(rr); err != nil {
			continue
		}
		if err := skipContentPatches(rr); err != nil {
			continue
		}
		if err := skipMapData(rr); err != nil {
			continue
		}
		out := make([]byte, delta)
		copy(out, raw[playerStart:playerStart+delta])
		return out, nil
	}
	return nil, errors.New("unable to locate content header in template world stream")
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

// RewritePlayerToLegacyRev1 rewrites only the player entity blob in a
// zlib-compressed NetworkIO.writeWorld payload, replacing it with a minimal
// revision-1 player payload while preserving all subsequent bytes exactly.
func RewritePlayerToLegacyRev1(payload []byte) ([]byte, error) {
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
	if _, err := r.ReadInt32(); err != nil { // wave
		return nil, err
	}
	if _, err := r.ReadFloat32(); err != nil { // wavetime
		return nil, err
	}
	if _, err := r.ReadFloat64(); err != nil { // tick
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed0
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed1
		return nil, err
	}
	if _, err := r.ReadInt32(); err != nil { // player id
		return nil, err
	}

	playerStart := r.Offset()
	playerRev, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return nil, err
	}
	playerEnd := r.Offset()

	var outRaw bytes.Buffer
	w := &javaWriter{buf: &outRaw}
	if err := w.WriteBytes(raw[:playerStart]); err != nil {
		return nil, err
	}
	if err := writeMinimalPlayer(w); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(raw[playerEnd:]); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(outRaw.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RebuildLegacyCompatibleWorldStream rewrites a network world-stream .bin payload
// into a conservative format compatible with older clients:
// - player entity serialized as revision 1 minimal payload
// - team blocks replaced with an empty plan set
// - markers/custom chunks replaced with empty values
// Content header/patches/map are preserved from source payload.
func RebuildLegacyCompatibleWorldStream(payload []byte) ([]byte, error) {
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

	if _, err := r.ReadInt32(); err != nil { // wave
		return nil, err
	}
	if _, err := r.ReadFloat32(); err != nil { // wavetime
		return nil, err
	}
	if _, err := r.ReadFloat64(); err != nil { // tick
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed0
		return nil, err
	}
	if _, err := r.ReadInt64(); err != nil { // seed1
		return nil, err
	}
	if _, err := r.ReadInt32(); err != nil { // player id
		return nil, err
	}

	playerStart := r.Offset()
	playerRev, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return nil, err
	}
	playerEnd := r.Offset()

	contentStart := playerEnd
	if err := skipContentHeader(r); err != nil {
		return nil, err
	}
	contentEnd := r.Offset()

	patchesStart := contentEnd
	if err := skipContentPatches(r); err != nil {
		return nil, err
	}
	patchesEnd := r.Offset()

	mapStart := patchesEnd
	if err := skipMapData(r); err != nil {
		return nil, err
	}
	mapEnd := r.Offset()

	var outRaw bytes.Buffer
	w := &javaWriter{buf: &outRaw}

	// Prefix through player id.
	if err := w.WriteBytes(raw[:playerStart]); err != nil {
		return nil, err
	}
	if err := writeMinimalPlayer(w); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(raw[contentStart:contentEnd]); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(raw[patchesStart:patchesEnd]); err != nil {
		return nil, err
	}
	if err := w.WriteBytes(raw[mapStart:mapEnd]); err != nil {
		return nil, err
	}
	if err := writeMinimalTeamBlocks(w); err != nil {
		return nil, err
	}
	// Empty marker map in UBJSON.
	if err := w.WriteBytes([]byte{0x7B, 0x7D}); err != nil {
		return nil, err
	}
	if err := writeMinimalCustomChunks(w); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(outRaw.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func skipPlayerPayload(r *javaReader, rev int16) error {
	// admin, boosting, color, command, mouseX, mouseY, name, [selected], shooting, team, typing, unit, x, y
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
		if err := r.SkipUTF(); err != nil {
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
		if err := r.SkipUTF(); err != nil {
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
		if err := r.SkipUTF(); err != nil {
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
