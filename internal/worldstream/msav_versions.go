package worldstream

import (
	"bytes"
	"fmt"
	"sort"

	"mdt-server/internal/protocol"
)

type msavVersionReader interface {
	read(r *javaReader, version int32) (MSAVData, error)
}

type msavChunkFlags struct {
	patches bool
	markers bool
	custom  bool
}

type msavEntitySection struct {
	mapping          []byte
	teamBlocks       []byte
	worldChunks      []msavWorldEntityChunk
	worldHaveIDs     bool
	worldShortChunks bool
	legacyGroups     bool
}

type basicVersionReader struct {
	chunks   msavChunkFlags
	entities func([]byte) (msavEntitySection, error)
}

func (r basicVersionReader) read(reader *javaReader, version int32) (MSAVData, error) {
	data, err := readMSAVBaseChunks(reader, version, r.chunks)
	if err != nil {
		return MSAVData{}, err
	}
	section, err := r.entities(data.RawEntities)
	if err != nil {
		return MSAVData{}, err
	}
	data.EntityMapping = section.mapping
	data.TeamBlocks = section.teamBlocks
	data.WorldEntityChunks = section.worldChunks
	data.WorldEntitiesHaveIDs = section.worldHaveIDs
	data.WorldEntitiesShortChunks = section.worldShortChunks
	data.LegacyEntityGroups = section.legacyGroups
	return data, nil
}

func readMSAVVersioned(r *javaReader, version int32) (MSAVData, error) {
	readers := map[int32]msavVersionReader{
		1:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave1Entities},
		2:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave2Entities},
		3:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave3Entities},
		4:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave4Entities},
		5:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave5Entities},
		6:  basicVersionReader{chunks: msavChunkFlags{}, entities: parseSave6Entities},
		7:  basicVersionReader{chunks: msavChunkFlags{custom: true}, entities: parseSave7Entities},
		8:  basicVersionReader{chunks: msavChunkFlags{markers: true, custom: true}, entities: parseSave8Entities},
		9:  basicVersionReader{chunks: msavChunkFlags{markers: true, custom: true}, entities: parseSave9Entities},
		10: basicVersionReader{chunks: msavChunkFlags{markers: true, custom: true}, entities: parseSave10Entities},
		11: basicVersionReader{chunks: msavChunkFlags{patches: true, markers: true, custom: true}, entities: parseSave11Entities},
	}
	reader, ok := readers[version]
	if !ok {
		return MSAVData{}, fmt.Errorf("%w: %d", ErrUnsupportedMSAVVersion, version)
	}
	return reader.read(r, version)
}

func readMSAVBaseChunks(r *javaReader, version int32, flags msavChunkFlags) (MSAVData, error) {
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
	if flags.patches {
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
	if flags.markers {
		markers, err = r.ReadChunk()
		if err != nil {
			return MSAVData{}, err
		}
	}

	var custom []byte
	if flags.custom {
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

func parseSave1Entities(raw []byte) (msavEntitySection, error) {
	return parseLegacyGroupEntities(raw, false)
}

func parseSave2Entities(raw []byte) (msavEntitySection, error) {
	return parseLegacyGroupEntities(raw, false)
}

func parseSave3Entities(raw []byte) (msavEntitySection, error) {
	return parseLegacyGroupEntities(raw, true)
}

func parseSave4Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, false, false)
}

func parseSave5Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, true, false)
}

func parseSave6Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, true, true)
}

func parseSave7Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, true, true)
}

func parseSave8Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, true, true)
}

func parseSave9Entities(raw []byte) (msavEntitySection, error) {
	return parseShortChunkEntitySection(raw, true, true)
}

func parseSave10Entities(raw []byte) (msavEntitySection, error) {
	return parseModernEntitySection(raw)
}

func parseSave11Entities(raw []byte) (msavEntitySection, error) {
	return parseModernEntitySection(raw)
}

func parseModernEntitySection(raw []byte) (msavEntitySection, error) {
	mapping, teamBlocks, chunks, err := splitMSAVEntitiesChunk(raw)
	if err != nil {
		return msavEntitySection{}, err
	}
	return msavEntitySection{
		mapping:          mapping,
		teamBlocks:       teamBlocks,
		worldChunks:      chunks,
		worldHaveIDs:     true,
		worldShortChunks: false,
	}, nil
}

func parseShortChunkEntitySection(raw []byte, withMapping bool, withIDs bool) (msavEntitySection, error) {
	if len(raw) == 0 {
		return msavEntitySection{
			worldHaveIDs:     withIDs,
			worldShortChunks: true,
		}, nil
	}
	r := newJavaReader(raw)

	mappingStart := 0
	mappingEnd := 0
	if withMapping {
		end, err := skipEntityMappingInline(r)
		if err != nil {
			return msavEntitySection{}, err
		}
		mappingEnd = end
	}

	teamStart := r.Offset()
	teamBlocks, _, err := readModernTeamBlocksRaw(r, raw)
	if err != nil {
		return msavEntitySection{}, err
	}
	_ = teamStart
	chunks, err := readWorldEntityShortChunks(r, withIDs)
	if err != nil {
		return msavEntitySection{}, err
	}

	var mapping []byte
	if withMapping {
		mapping = append([]byte(nil), raw[mappingStart:mappingEnd]...)
	}
	return msavEntitySection{
		mapping:          mapping,
		teamBlocks:       append([]byte(nil), teamBlocks...),
		worldChunks:      chunks,
		worldHaveIDs:     withIDs,
		worldShortChunks: true,
	}, nil
}

func parseLegacyGroupEntities(raw []byte, withTeamBlocks bool) (msavEntitySection, error) {
	if len(raw) == 0 {
		return msavEntitySection{legacyGroups: true}, nil
	}
	r := newJavaReader(raw)
	section := msavEntitySection{legacyGroups: true}
	if withTeamBlocks {
		plans, err := readLegacySave3TeamBlocks(r)
		if err != nil {
			return msavEntitySection{}, err
		}
		teamRaw, err := encodeTeamBlocks(plans)
		if err != nil {
			return msavEntitySection{}, err
		}
		section.teamBlocks = teamRaw
	}
	if err := skipLegacyEntityGroups(r); err != nil {
		return msavEntitySection{}, err
	}
	return section, nil
}

func skipEntityMappingInline(r *javaReader) (int, error) {
	countRaw, err := r.ReadInt16()
	if err != nil {
		return 0, err
	}
	if countRaw < 0 {
		return 0, ErrInvalidMSAV
	}
	for i := 0; i < int(countRaw); i++ {
		if _, err := r.ReadInt16(); err != nil {
			return 0, err
		}
		if err := r.SkipUTF(); err != nil {
			return 0, err
		}
	}
	return r.Offset(), nil
}

func readModernTeamBlocksRaw(r *javaReader, raw []byte) ([]byte, int, error) {
	start := r.Offset()
	teamCount, err := r.ReadInt32()
	if err != nil {
		return nil, 0, err
	}
	if teamCount < 0 {
		return nil, 0, ErrInvalidMSAV
	}
	for i := 0; i < int(teamCount); i++ {
		if _, err := r.ReadInt32(); err != nil {
			return nil, 0, err
		}
		blockCount, err := r.ReadInt32()
		if err != nil {
			return nil, 0, err
		}
		if blockCount < 0 {
			return nil, 0, ErrInvalidMSAV
		}
		for j := 0; j < int(blockCount); j++ {
			if err := r.Skip(8); err != nil {
				return nil, 0, err
			}
			if err := skipTypeIOObject(r); err != nil {
				return nil, 0, err
			}
		}
	}
	end := r.Offset()
	return raw[start:end], end, nil
}

func readWorldEntityShortChunks(r *javaReader, withIDs bool) ([]msavWorldEntityChunk, error) {
	count, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, ErrInvalidMSAV
	}
	chunks := make([]msavWorldEntityChunk, 0, count)
	for i := 0; i < int(count); i++ {
		size, err := r.ReadUInt16()
		if err != nil {
			return nil, err
		}
		payload, err := r.ReadBytes(int(size))
		if err != nil {
			return nil, err
		}
		entry := msavWorldEntityChunk{Raw: append([]byte(nil), payload...)}
		if len(payload) > 0 {
			entry.ClassID = payload[0]
		}
		if withIDs && len(payload) >= 5 {
			entry.ID = int32(binaryBigEndianUint32(payload[1:5]))
		}
		chunks = append(chunks, entry)
	}
	return chunks, nil
}

func skipLegacyEntityGroups(r *javaReader) error {
	groupsRaw, err := r.ReadByte()
	if err != nil {
		return err
	}
	groups := int(groupsRaw)
	for i := 0; i < groups; i++ {
		count, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if count < 0 {
			return ErrInvalidMSAV
		}
		for j := 0; j < int(count); j++ {
			size, err := r.ReadUInt16()
			if err != nil {
				return err
			}
			if err := r.Skip(int(size)); err != nil {
				return err
			}
		}
	}
	return nil
}

type msavTeamBlockPlan struct {
	teamID byte
	x      int16
	y      int16
	rot    int16
	block  int16
	config any
}

func readLegacySave3TeamBlocks(r *javaReader) ([]msavTeamBlockPlan, error) {
	count, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, ErrInvalidMSAV
	}
	out := make([]msavTeamBlockPlan, 0, count)
	for i := 0; i < int(count); i++ {
		teamRaw, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		blockCount, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		if blockCount < 0 {
			return nil, ErrInvalidMSAV
		}
		for j := 0; j < int(blockCount); j++ {
			x, err := r.ReadInt16()
			if err != nil {
				return nil, err
			}
			y, err := r.ReadInt16()
			if err != nil {
				return nil, err
			}
			rot, err := r.ReadInt16()
			if err != nil {
				return nil, err
			}
			blockID, err := r.ReadInt16()
			if err != nil {
				return nil, err
			}
			config, err := r.ReadInt32()
			if err != nil {
				return nil, err
			}
			out = append(out, msavTeamBlockPlan{
				teamID: byte(teamRaw),
				x:      x,
				y:      y,
				rot:    rot,
				block:  blockID,
				config: config,
			})
		}
	}
	return out, nil
}

func encodeTeamBlocks(plans []msavTeamBlockPlan) ([]byte, error) {
	if len(plans) == 0 {
		var out bytes.Buffer
		w := &javaWriter{buf: &out}
		if err := writeMinimalTeamBlocks(w); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	byTeam := make(map[byte][]msavTeamBlockPlan)
	teamIDs := make([]int, 0)
	for _, plan := range plans {
		if _, ok := byTeam[plan.teamID]; !ok {
			teamIDs = append(teamIDs, int(plan.teamID))
		}
		byTeam[plan.teamID] = append(byTeam[plan.teamID], plan)
	}
	sort.Ints(teamIDs)
	var out bytes.Buffer
	w := &javaWriter{buf: &out}
	if err := w.WriteInt32(int32(len(teamIDs))); err != nil {
		return nil, err
	}
	for _, rawTeam := range teamIDs {
		teamID := byte(rawTeam)
		entries := byTeam[teamID]
		if err := w.WriteInt32(int32(teamID)); err != nil {
			return nil, err
		}
		if err := w.WriteInt32(int32(len(entries))); err != nil {
			return nil, err
		}
		for _, plan := range entries {
			if err := w.WriteInt16(plan.x); err != nil {
				return nil, err
			}
			if err := w.WriteInt16(plan.y); err != nil {
				return nil, err
			}
			if err := w.WriteInt16(plan.rot); err != nil {
				return nil, err
			}
			if err := w.WriteInt16(plan.block); err != nil {
				return nil, err
			}
			tmp := protocol.NewWriter()
			if err := protocol.WriteObject(tmp, plan.config, nil); err != nil {
				return nil, err
			}
			if err := w.WriteBytes(tmp.Bytes()); err != nil {
				return nil, err
			}
		}
	}
	return out.Bytes(), nil
}

func binaryBigEndianUint32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func (r *javaReader) ReadUInt16() (uint16, error) {
	var v uint16
	err := readBE(r.buf, &v)
	return v, err
}
