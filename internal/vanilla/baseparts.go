package vanilla

import (
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

const (
	basePartHeaderSize = 4
	basePartVersion    = 1
)

const (
	BasePartContentItem        = byte(0)
	BasePartContentBlock       = byte(1)
	BasePartContentBullet      = byte(3)
	BasePartContentLiquid      = byte(4)
	BasePartContentStatus      = byte(5)
	BasePartContentUnit        = byte(6)
	BasePartContentWeather     = byte(7)
	BasePartContentPlanet      = byte(13)
	BasePartContentTeam        = byte(15)
	BasePartContentUnitCommand = byte(16)
	BasePartContentUnitStance  = byte(17)
)

type BasePartContentRef struct {
	ContentType byte  `json:"content_type"`
	ID          int16 `json:"id"`
}

type BasePartPoint struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

type BasePartVec2 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

type BasePartTile struct {
	Block    string `json:"block"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Rotation int8   `json:"rotation"`
	Config   any    `json:"config,omitempty"`
}

type BasePartSchematic struct {
	Name   string         `json:"name"`
	Width  int            `json:"width"`
	Height int            `json:"height"`
	Tiles  []BasePartTile `json:"tiles"`
}

//go:embed data/baseparts/*.msch
var embeddedBasePartsFS embed.FS

var (
	basePartOnce   sync.Once
	basePartCache  []BasePartSchematic
	basePartErr    error
)

func LoadEmbeddedBasePartSchematics() ([]BasePartSchematic, error) {
	basePartOnce.Do(func() {
		basePartCache, basePartErr = loadEmbeddedBasePartSchematics()
	})
	if basePartErr != nil {
		return nil, basePartErr
	}
	out := make([]BasePartSchematic, len(basePartCache))
	copy(out, basePartCache)
	return out, nil
}

func loadEmbeddedBasePartSchematics() ([]BasePartSchematic, error) {
	entries, err := fs.ReadDir(embeddedBasePartsFS, "data/baseparts")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(entry.Name(), ".msch") && !strings.HasSuffix(strings.ToLower(entry.Name()), ".msch") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	out := make([]BasePartSchematic, 0, len(names))
	for _, name := range names {
		raw, err := embeddedBasePartsFS.ReadFile("data/baseparts/" + name)
		if err != nil {
			return nil, err
		}
		part, err := parseBasePartSchematic(name, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, part)
	}
	return out, nil
}

func parseBasePartSchematic(name string, raw []byte) (BasePartSchematic, error) {
	if len(raw) < basePartHeaderSize+1 {
		return BasePartSchematic{}, fmt.Errorf("basepart %s: file too short", name)
	}
	if string(raw[:basePartHeaderSize]) != "msch" {
		return BasePartSchematic{}, fmt.Errorf("basepart %s: invalid header", name)
	}
	if raw[basePartHeaderSize] > basePartVersion {
		return BasePartSchematic{}, fmt.Errorf("basepart %s: unsupported version %d", name, raw[basePartHeaderSize])
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[basePartHeaderSize+1:]))
	if err != nil {
		return BasePartSchematic{}, err
	}
	defer zr.Close()
	br := &basePartReader{r: zr}

	width, err := br.readInt16()
	if err != nil {
		return BasePartSchematic{}, err
	}
	height, err := br.readInt16()
	if err != nil {
		return BasePartSchematic{}, err
	}
	tagCount, err := br.readUint8()
	if err != nil {
		return BasePartSchematic{}, err
	}
	for i := 0; i < int(tagCount); i++ {
		if _, err := br.readUTF(); err != nil {
			return BasePartSchematic{}, err
		}
		if _, err := br.readUTF(); err != nil {
			return BasePartSchematic{}, err
		}
	}
	blockCount, err := br.readUint8()
	if err != nil {
		return BasePartSchematic{}, err
	}
	blocks := make([]string, int(blockCount))
	for i := range blocks {
		block, err := br.readUTF()
		if err != nil {
			return BasePartSchematic{}, err
		}
		blocks[i] = strings.ToLower(strings.TrimSpace(block))
	}
	total, err := br.readInt32()
	if err != nil {
		return BasePartSchematic{}, err
	}
	if total < 0 || total > 128*128 {
		return BasePartSchematic{}, fmt.Errorf("basepart %s: invalid tile count %d", name, total)
	}
	part := BasePartSchematic{
		Name:   name,
		Width:  int(width),
		Height: int(height),
		Tiles:  make([]BasePartTile, 0, int(total)),
	}
	for i := 0; i < int(total); i++ {
		blockIndex, err := br.readUint8()
		if err != nil {
			return BasePartSchematic{}, err
		}
		position, err := br.readInt32()
		if err != nil {
			return BasePartSchematic{}, err
		}
		config, err := br.readTypeIOObject(true)
		if err != nil {
			return BasePartSchematic{}, err
		}
		rotation, err := br.readInt8()
		if err != nil {
			return BasePartSchematic{}, err
		}
		if int(blockIndex) >= len(blocks) {
			return BasePartSchematic{}, fmt.Errorf("basepart %s: invalid block index %d", name, blockIndex)
		}
		part.Tiles = append(part.Tiles, BasePartTile{
			Block:    blocks[int(blockIndex)],
			X:        int(uint16(position)),
			Y:        int(uint32(position) >> 16),
			Rotation: rotation,
			Config:   config,
		})
	}
	return part, nil
}

type basePartReader struct {
	r io.Reader
}

func (r *basePartReader) readUint8() (uint8, error) {
	var out [1]byte
	_, err := io.ReadFull(r.r, out[:])
	return out[0], err
}

func (r *basePartReader) readInt8() (int8, error) {
	v, err := r.readUint8()
	return int8(v), err
}

func (r *basePartReader) readBool() (bool, error) {
	v, err := r.readUint8()
	return v != 0, err
}

func (r *basePartReader) readInt16() (int16, error) {
	var out int16
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readUint16() (uint16, error) {
	var out uint16
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readInt32() (int32, error) {
	var out int32
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readInt64() (int64, error) {
	var out int64
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readFloat32() (float32, error) {
	var out float32
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readFloat64() (float64, error) {
	var out float64
	err := binary.Read(r.r, binary.BigEndian, &out)
	return out, err
}

func (r *basePartReader) readUTF() (string, error) {
	size, err := r.readUint16()
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(r.r, buf); err != nil {
		return "", err
	}
	// Mindustry schematics use Java UTF; basepart metadata is ASCII content names/tags,
	// so direct UTF-8 decode is sufficient here.
	return string(buf), nil
}

func (r *basePartReader) readTypeIOObject(allowArrays bool) (any, error) {
	kind, err := r.readUint8()
	if err != nil {
		return nil, err
	}
	switch kind {
	case 0:
		return nil, nil
	case 1:
		return r.readInt32()
	case 2:
		return r.readInt64()
	case 3:
		return r.readFloat32()
	case 4:
		exists, err := r.readUint8()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, nil
		}
		return r.readUTF()
	case 5:
		contentType, err := r.readUint8()
		if err != nil {
			return nil, err
		}
		id, err := r.readInt16()
		if err != nil {
			return nil, err
		}
		return BasePartContentRef{ContentType: contentType, ID: id}, nil
	case 6:
		if !allowArrays {
			return nil, fmt.Errorf("nested int array not allowed")
		}
		size, err := r.readInt16()
		if err != nil {
			return nil, err
		}
		out := make([]int32, 0, int(size))
		for i := 0; i < int(size); i++ {
			value, err := r.readInt32()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case 7:
		x, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		y, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		return BasePartPoint{X: x, Y: y}, nil
	case 8:
		if !allowArrays {
			return nil, fmt.Errorf("nested point array not allowed")
		}
		size, err := r.readUint8()
		if err != nil {
			return nil, err
		}
		out := make([]BasePartPoint, 0, int(size))
		for i := 0; i < int(size); i++ {
			packed, err := r.readInt32()
			if err != nil {
				return nil, err
			}
			out = append(out, BasePartPoint{
				X: int32(uint16(packed)),
				Y: int32(uint32(packed) >> 16),
			})
		}
		return out, nil
	case 9:
		contentType, err := r.readUint8()
		if err != nil {
			return nil, err
		}
		id, err := r.readInt16()
		if err != nil {
			return nil, err
		}
		return BasePartContentRef{ContentType: contentType, ID: id}, nil
	case 10:
		return r.readBool()
	case 11:
		return r.readFloat64()
	case 12:
		return r.readInt32()
	case 13:
		return r.readInt16()
	case 14:
		if !allowArrays {
			return nil, fmt.Errorf("nested byte array not allowed")
		}
		size, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, fmt.Errorf("invalid byte array size %d", size)
		}
		out := make([]byte, int(size))
		_, err = io.ReadFull(r.r, out)
		return out, err
	case 15:
		_, err := r.readUint8()
		return nil, err
	case 16:
		if !allowArrays {
			return nil, fmt.Errorf("nested bool array not allowed")
		}
		size, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, fmt.Errorf("invalid bool array size %d", size)
		}
		out := make([]bool, 0, int(size))
		for i := 0; i < int(size); i++ {
			value, err := r.readBool()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case 17:
		return r.readInt32()
	case 18:
		if !allowArrays {
			return nil, fmt.Errorf("nested vec2 array not allowed")
		}
		size, err := r.readInt16()
		if err != nil {
			return nil, err
		}
		out := make([]BasePartVec2, 0, int(size))
		for i := 0; i < int(size); i++ {
			x, err := r.readFloat32()
			if err != nil {
				return nil, err
			}
			y, err := r.readFloat32()
			if err != nil {
				return nil, err
			}
			out = append(out, BasePartVec2{X: x, Y: y})
		}
		return out, nil
	case 19:
		x, err := r.readFloat32()
		if err != nil {
			return nil, err
		}
		y, err := r.readFloat32()
		if err != nil {
			return nil, err
		}
		return BasePartVec2{X: x, Y: y}, nil
	case 20:
		return r.readUint8()
	case 21:
		if !allowArrays {
			return nil, fmt.Errorf("nested int array not allowed")
		}
		size, err := r.readInt16()
		if err != nil {
			return nil, err
		}
		out := make([]int32, 0, int(size))
		for i := 0; i < int(size); i++ {
			value, err := r.readInt32()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case 22:
		if !allowArrays {
			return nil, fmt.Errorf("nested object array not allowed")
		}
		size, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, fmt.Errorf("invalid object array size %d", size)
		}
		out := make([]any, 0, int(size))
		for i := 0; i < int(size); i++ {
			value, err := r.readTypeIOObject(false)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case 23:
		return r.readInt16()
	default:
		return nil, fmt.Errorf("unsupported TypeIO kind %d", kind)
	}
}
