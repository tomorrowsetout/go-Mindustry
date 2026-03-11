package world

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"math"
	"strings"

	"mdt-server/internal/logic"
	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
)

type logicLink struct {
	X    int
	Y    int
	Name string
}

type logicRuntime struct {
	Exec        *logic.MlogExecutor
	Program     *logic.MlogProgram
	Code        string
	Links       []logicLink
	Accumulator float32
	Ipt         int
	Privileged  bool
	ConfigHash  uint32
	Host        *LogicHost
}

type logicClientDataEvent struct {
	Channel  string
	Value    any
	Reliable bool
}

type logicSyncEvent struct {
	BuildPos int32
	VarID    int32
	Value    any
}

type logicIDMaps struct {
	blockNames  []string
	unitNames   []string
	itemNames   []string
	liquidNames []string

	blockByName  map[string]protocol.Block
	unitByName   map[string]protocol.UnitType
	itemByName   map[string]protocol.Item
	liquidByName map[string]protocol.Liquid
}

func newLogicIDMaps(ids *vanilla.ContentIDsFile) *logicIDMaps {
	if ids == nil {
		return nil
	}
	m := &logicIDMaps{
		blockNames:  make([]string, len(ids.LogicBlocks)),
		unitNames:   make([]string, len(ids.LogicUnits)),
		itemNames:   make([]string, len(ids.LogicItems)),
		liquidNames: make([]string, len(ids.LogicLiquids)),
	}
	for i, e := range ids.LogicBlocks {
		m.blockNames[i] = strings.ToLower(strings.TrimSpace(e.Name))
	}
	for i, e := range ids.LogicUnits {
		m.unitNames[i] = strings.ToLower(strings.TrimSpace(e.Name))
	}
	for i, e := range ids.LogicItems {
		m.itemNames[i] = strings.ToLower(strings.TrimSpace(e.Name))
	}
	for i, e := range ids.LogicLiquids {
		m.liquidNames[i] = strings.ToLower(strings.TrimSpace(e.Name))
	}
	return m
}

func (m *logicIDMaps) bindContent(reg *protocol.ContentRegistry) {
	if m == nil || reg == nil {
		return
	}
	m.blockByName = map[string]protocol.Block{}
	m.unitByName = map[string]protocol.UnitType{}
	m.itemByName = map[string]protocol.Item{}
	m.liquidByName = map[string]protocol.Liquid{}
	reg.IterateBlocks(func(b protocol.Block) bool {
		name := strings.ToLower(strings.TrimSpace(b.Name()))
		if name != "" {
			m.blockByName[name] = b
		}
		return true
	})
	reg.IterateUnitTypes(func(u protocol.UnitType) bool {
		name := strings.ToLower(strings.TrimSpace(u.Name()))
		if name != "" {
			m.unitByName[name] = u
		}
		return true
	})
	reg.IterateItems(func(it protocol.Item) bool {
		name := strings.ToLower(strings.TrimSpace(it.Name()))
		if name != "" {
			m.itemByName[name] = it
		}
		return true
	})
	reg.IterateLiquids(func(liq protocol.Liquid) bool {
		name := strings.ToLower(strings.TrimSpace(liq.Name()))
		if name != "" {
			m.liquidByName[name] = liq
		}
		return true
	})
}

func (w *World) stepLogic(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || dt <= 0 {
		return
	}
	if w.logicRuntime == nil {
		w.logicRuntime = map[int32]*logicRuntime{}
	}
	if w.logicMemory == nil {
		w.logicMemory = map[int32]*logic.MlogCell{}
	}
	if w.logicUnitFlags == nil {
		w.logicUnitFlags = map[int32]float64{}
	}
	if w.logicIDs != nil && w.content != nil && (w.logicIDs.blockByName == nil || w.logicIDs.unitByName == nil) {
		w.logicIDs.bindContent(w.content)
	}

	if w.tick%30 == 0 || w.logicProcessorPos == nil {
		w.logicProcessorPos = w.collectLogicProcessorsLocked()
	}

	rules := w.rulesMgr.Get()
	for _, pos := range w.logicProcessorPos {
		t, ok := w.tileForPosLocked(pos)
		if !ok || t == nil || t.Block == 0 || t.Build == nil {
			delete(w.logicRuntime, pos)
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		if !isLogicProcessorName(blockName) {
			delete(w.logicRuntime, pos)
			continue
		}
		privileged := strings.Contains(blockName, "world-processor")
		if privileged && rules != nil && rules.DisableWorldProcessors {
			continue
		}

		code, links, cfgHash, ok := w.logicConfigForTileLocked(t)
		if !ok || strings.TrimSpace(code) == "" {
			delete(w.logicRuntime, pos)
			continue
		}

		rt := w.logicRuntime[pos]
		if rt == nil || rt.ConfigHash != cfgHash {
			asm := logic.NewMlogAssembler()
			prog, err := asm.Assemble(code)
			if err != nil || prog == nil {
				delete(w.logicRuntime, pos)
				continue
			}
			host := &LogicHost{W: w, BuildPos: pos, Links: links}
			exec := logic.NewMlogExecutor(prog, host)
			rt = &logicRuntime{
				Exec:       exec,
				Program:    prog,
				Code:       code,
				Links:      links,
				Ipt:        logicIptForBlock(blockName),
				Privileged: privileged,
				ConfigHash: cfgHash,
				Host:       host,
			}
			w.logicRuntime[pos] = rt
		} else {
			rt.Links = links
			if rt.Host != nil {
				rt.Host.Links = links
			}
		}

		rt.Ipt = logicIptForBlock(blockName)
		if rt.Program != nil {
			setLogicProgramConst(rt.Program, "@this", t.Build)
			setLogicProgramConst(rt.Program, "@thisx", float64(t.X))
			setLogicProgramConst(rt.Program, "@thisy", float64(t.Y))
			setLogicProgramConst(rt.Program, "@links", float64(len(rt.Links)))
			setLogicProgramConst(rt.Program, "@ipt", float64(rt.Ipt))
			for _, link := range rt.Links {
				if link.Name == "" {
					continue
				}
				lt, err := w.model.TileAt(link.X, link.Y)
				if err != nil || lt == nil || lt.Build == nil {
					setLogicProgramConst(rt.Program, link.Name, nil)
				} else {
					setLogicProgramConst(rt.Program, link.Name, lt.Build)
				}
			}
		}

		if rt.Exec == nil {
			continue
		}
		rt.Accumulator += dt * float32(rt.Ipt)
		maxScale := float32(rt.Ipt * 5)
		if rt.Accumulator > maxScale {
			rt.Accumulator = maxScale
		}
		for rt.Accumulator >= 1 {
			rt.Exec.RunOnce()
			if rt.Exec.Yield {
				rt.Exec.Yield = false
				break
			}
			rt.Accumulator--
		}
	}
}

func setLogicProgramConst(p *logic.MlogProgram, name string, v any) {
	if p == nil {
		return
	}
	varVar := p.EnsureVar(name)
	if varVar == nil {
		return
	}
	varVar.Constant = true
	varVar.SetConst(v)
}

func (w *World) collectLogicProcessorsLocked() []int32 {
	if w.model == nil || w.blockNamesByID == nil {
		return nil
	}
	out := make([]int32, 0)
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		if !isLogicProcessorName(blockName) {
			continue
		}
		out = append(out, packTilePos(t.X, t.Y))
	}
	return out
}

func isLogicProcessorName(name string) bool {
	if name == "" {
		return false
	}
	return strings.Contains(name, "processor")
}

func logicIptForBlock(name string) int {
	switch {
	case strings.Contains(name, "hyper-processor"):
		return 25
	case strings.Contains(name, "logic-processor"):
		return 8
	case strings.Contains(name, "micro-processor"):
		return 2
	case strings.Contains(name, "world-processor"):
		return 8
	default:
		return 1
	}
}

func (w *World) logicConfigForTileLocked(t *Tile) (string, []logicLink, uint32, bool) {
	if t == nil || t.Build == nil {
		return "", nil, 0, false
	}
	pos := packTilePos(t.X, t.Y)
	var rawCfg []byte
	if v, ok := w.buildingConfigValueLocked(pos); ok {
		if b, ok2 := v.([]byte); ok2 {
			rawCfg = b
		}
	}
	if rawCfg == nil && t.Build.Config != nil && w.typeIO != nil {
		if v, ok := w.decodeConfigValueLocked(t.Build.Config); ok {
			if b, ok2 := v.([]byte); ok2 {
				rawCfg = b
			}
		}
	}
	if rawCfg == nil && t.Build.Config != nil {
		if _, _, err := decodeLogicConfig(t.Build.Config, t.X, t.Y); err == nil {
			rawCfg = t.Build.Config
		}
	}
	if rawCfg == nil {
		return "", nil, 0, false
	}
	code, links, err := decodeLogicConfig(rawCfg, t.X, t.Y)
	if err != nil {
		return "", nil, 0, false
	}
	return code, links, hashBytes(rawCfg), true
}

func (w *World) decodeConfigValueLocked(raw []byte) (any, bool) {
	if len(raw) == 0 || w.typeIO == nil {
		return nil, false
	}
	r := protocol.NewReaderWithContext(raw, w.typeIO)
	v, err := protocol.ReadObject(r, false, w.typeIO)
	if err != nil {
		return nil, false
	}
	return v, true
}

func hashBytes(b []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(b)
	return h.Sum32()
}

func decodeLogicConfig(raw []byte, baseX, baseY int) (string, []logicLink, error) {
	if len(raw) == 0 {
		return "", nil, errors.New("empty")
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		return "", nil, err
	}
	if len(data) < 5 {
		return "", nil, errors.New("short")
	}
	r := bytes.NewReader(data)
	if _, err := r.ReadByte(); err != nil {
		return "", nil, err
	}
	var codeLen int32
	if err := binary.Read(r, binary.BigEndian, &codeLen); err != nil {
		return "", nil, err
	}
	if codeLen < 0 || int(codeLen) > len(data) {
		return "", nil, errors.New("bad code len")
	}
	codeBytes := make([]byte, codeLen)
	if _, err := io.ReadFull(r, codeBytes); err != nil {
		return "", nil, err
	}
	var linkCount int32
	if err := binary.Read(r, binary.BigEndian, &linkCount); err != nil {
		return "", nil, err
	}
	if linkCount < 0 {
		return "", nil, errors.New("bad link count")
	}
	links := make([]logicLink, 0, linkCount)
	for i := int32(0); i < linkCount; i++ {
		name, err := readJavaUTF(r)
		if err != nil {
			return "", nil, err
		}
		x, err := readInt16BE(r)
		if err != nil {
			return "", nil, err
		}
		y, err := readInt16BE(r)
		if err != nil {
			return "", nil, err
		}
		links = append(links, logicLink{
			X:    baseX + int(x),
			Y:    baseY + int(y),
			Name: name,
		})
	}
	return string(codeBytes), links, nil
}

func readInt16BE(r *bytes.Reader) (int16, error) {
	var v int16
	if err := binary.Read(r, binary.BigEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func readJavaUTF(r *bytes.Reader) (string, error) {
	var size uint16
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return "", err
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	out := make([]rune, 0, size)
	for i := 0; i < len(buf); {
		b := buf[i]
		switch {
		case b>>7 == 0:
			out = append(out, rune(b))
			i++
		case (b >> 5) == 0x6:
			if i+1 >= len(buf) {
				return "", errors.New("utf short")
			}
			b2 := buf[i+1]
			ch := rune(b&0x1f)<<6 | rune(b2&0x3f)
			out = append(out, ch)
			i += 2
		case (b >> 4) == 0xE:
			if i+2 >= len(buf) {
				return "", errors.New("utf short")
			}
			b2 := buf[i+1]
			b3 := buf[i+2]
			ch := rune(b&0x0f)<<12 | rune(b2&0x3f)<<6 | rune(b3&0x3f)
			out = append(out, ch)
			i += 3
		default:
			return "", errors.New("utf invalid")
		}
	}
	return string(out), nil
}

func (w *World) logicRangeForBlock(name string) float32 {
	switch {
	case strings.Contains(name, "hyper-processor"):
		return 8 * 42
	case strings.Contains(name, "logic-processor"):
		return 8 * 22
	case strings.Contains(name, "micro-processor"):
		return 8 * 10
	case strings.Contains(name, "world-processor"):
		return math.MaxFloat32
	default:
		return 8 * 10
	}
}

func (w *World) DrainLogicClientData() []logicClientDataEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.logicClientData) == 0 {
		return nil
	}
	out := append([]logicClientDataEvent(nil), w.logicClientData...)
	w.logicClientData = w.logicClientData[:0]
	return out
}

func (w *World) DrainLogicSyncVars() []logicSyncEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.logicSyncVars) == 0 {
		return nil
	}
	out := append([]logicSyncEvent(nil), w.logicSyncVars...)
	w.logicSyncVars = w.logicSyncVars[:0]
	return out
}
