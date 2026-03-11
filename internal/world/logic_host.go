package world

import (
	"math"
	"strings"
	"time"

	"mdt-server/internal/logic"
	"mdt-server/internal/protocol"
)

// LogicHost implements logic.MlogHost against the current World state.
// All methods assume the world lock is already held by the caller.
type LogicHost struct {
	W        *World
	BuildPos int32
	Links    []logicLink
}

func (h *LogicHost) Now() time.Time {
	return time.Now()
}

func (h *LogicHost) Sensor(target any, sensor string) (any, bool) {
	if h == nil || h.W == nil {
		return nil, false
	}
	switch t := target.(type) {
	case *Building:
		return h.sensorBuilding(t, sensor)
	case Building:
		return h.sensorBuilding(&t, sensor)
	case *RawEntity:
		return h.sensorEntity(t, sensor)
	case RawEntity:
		return h.sensorEntity(&t, sensor)
	default:
		return nil, false
	}
}

func (h *LogicHost) Control(target any, action string, a, b, c, d *logic.MlogVar) bool {
	if h == nil || h.W == nil {
		return false
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch t := target.(type) {
	case *Building:
		return h.controlBuilding(t, action, a, b, c, d)
	case Building:
		return h.controlBuilding(&t, action, a, b, c, d)
	default:
		return false
	}
}

func (h *LogicHost) UControl(target any, action string, a, b, c, d *logic.MlogVar) bool {
	if h == nil || h.W == nil {
		return false
	}
	e := h.findEntity(target)
	if e == nil {
		return false
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "move":
		x := float32(numOrZeroVar(a) * 8)
		y := float32(numOrZeroVar(b) * 8)
		speed := float32(numOrZeroVar(c))
		if speed <= 0 {
			speed = e.MoveSpeed
			if speed <= 0 {
				speed = 1
			}
		}
		dx := x - e.X
		dy := y - e.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist <= 0.01 {
			e.VelX, e.VelY = 0, 0
			return true
		}
		e.VelX = speed * dx / dist
		e.VelY = speed * dy / dist
		return true
	case "approach":
		x := float32(numOrZeroVar(a) * 8)
		y := float32(numOrZeroVar(b) * 8)
		dst := float32(numOrZeroVar(c) * 8)
		if dst < 0 {
			dst = 0
		}
		dx := x - e.X
		dy := y - e.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist <= dst {
			e.VelX, e.VelY = 0, 0
			return true
		}
		speed := e.MoveSpeed
		if speed <= 0 {
			speed = 1
		}
		e.VelX = speed * dx / dist
		e.VelY = speed * dy / dist
		return true
	case "stop":
		e.VelX, e.VelY = 0, 0
		return true
	case "flag":
		if h.W.logicUnitFlags == nil {
			h.W.logicUnitFlags = map[int32]float64{}
		}
		h.W.logicUnitFlags[e.ID] = numOrZeroVar(a)
		return true
	default:
		return false
	}
}

func (h *LogicHost) Radar(target any, filter string, sort string, count int) (any, bool) {
	return h.findRadarTarget(target, filter, sort)
}

func (h *LogicHost) URadar(target any, filter string, sort string, count int) (any, bool) {
	return h.findRadarTarget(target, filter, sort)
}

func (h *LogicHost) Locate(target any, locate string, flag string, enemy bool, ore any) (found bool, x, y float64, obj any) {
	_ = target
	_ = locate
	_ = flag
	_ = enemy
	_ = ore
	return false, 0, 0, nil
}

func (h *LogicHost) GetLink(index int) (any, bool) {
	if h == nil || h.W == nil || index < 0 || index >= len(h.Links) {
		return nil, false
	}
	link := h.Links[index]
	if h.W.model == nil || !h.W.model.InBounds(link.X, link.Y) {
		return nil, false
	}
	t := &h.W.model.Tiles[link.Y*h.W.model.Width+link.X]
	if t.Build == nil {
		return nil, false
	}
	return t.Build, true
}

func (h *LogicHost) ReadCell(cell any, addr int) float64 {
	if addr < 0 {
		return 0
	}
	switch c := cell.(type) {
	case *logic.MlogCell:
		return c.Get(addr)
	case *Building:
		return h.readMemoryCell(c, addr)
	case Building:
		return h.readMemoryCell(&c, addr)
	default:
		return 0
	}
}

func (h *LogicHost) WriteCell(cell any, addr int, value float64) {
	if addr < 0 {
		return
	}
	switch c := cell.(type) {
	case *logic.MlogCell:
		c.Set(addr, value)
	case *Building:
		h.writeMemoryCell(c, addr, value)
	case Building:
		h.writeMemoryCell(&c, addr, value)
	}
}

func (h *LogicHost) Lookup(kind string, id int) (any, bool) {
	if h == nil || h.W == nil || h.W.logicIDs == nil || id < 0 {
		return nil, false
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "block":
		if id >= len(h.W.logicIDs.blockNames) {
			return nil, false
		}
		name := h.W.logicIDs.blockNames[id]
		if name == "" || h.W.logicIDs.blockByName == nil {
			return nil, false
		}
		if blk, ok := h.W.logicIDs.blockByName[name]; ok {
			return blk, true
		}
	case "unit":
		if id >= len(h.W.logicIDs.unitNames) {
			return nil, false
		}
		name := h.W.logicIDs.unitNames[id]
		if name == "" || h.W.logicIDs.unitByName == nil {
			return nil, false
		}
		if u, ok := h.W.logicIDs.unitByName[name]; ok {
			return u, true
		}
	case "item":
		if id >= len(h.W.logicIDs.itemNames) {
			return nil, false
		}
		name := h.W.logicIDs.itemNames[id]
		if name == "" || h.W.logicIDs.itemByName == nil {
			return nil, false
		}
		if it, ok := h.W.logicIDs.itemByName[name]; ok {
			return it, true
		}
	case "liquid":
		if id >= len(h.W.logicIDs.liquidNames) {
			return nil, false
		}
		name := h.W.logicIDs.liquidNames[id]
		if name == "" || h.W.logicIDs.liquidByName == nil {
			return nil, false
		}
		if liq, ok := h.W.logicIDs.liquidByName[name]; ok {
			return liq, true
		}
	}
	return nil, false
}

func (h *LogicHost) GetBlock(x, y int, layer string) (any, bool) {
	if h == nil || h.W == nil || h.W.model == nil {
		return nil, false
	}
	if !h.W.model.InBounds(x, y) {
		return nil, false
	}
	t := &h.W.model.Tiles[y*h.W.model.Width+x]
	switch strings.ToLower(strings.TrimSpace(layer)) {
	case "floor":
		if h.W.content != nil {
			return h.W.content.Block(int16(t.Floor)), true
		}
		return t.Floor, true
	case "ore", "overlay":
		if h.W.content != nil {
			return h.W.content.Block(int16(t.Overlay)), true
		}
		return t.Overlay, true
	case "building":
		if t.Build == nil {
			return nil, false
		}
		return t.Build, true
	default:
		if h.W.content != nil {
			return h.W.content.Block(int16(t.Block)), true
		}
		return t.Block, true
	}
}

func (h *LogicHost) SetBlock(x, y int, block any, team int, rotation int) bool {
	if h == nil || h.W == nil || h.W.model == nil {
		return false
	}
	if !h.W.model.InBounds(x, y) {
		return false
	}
	t := &h.W.model.Tiles[y*h.W.model.Width+x]
	blockID := int16(0)
	switch b := block.(type) {
	case protocol.Block:
		blockID = b.ID()
	case BlockID:
		blockID = int16(b)
	case int16:
		blockID = b
	case int32:
		blockID = int16(b)
	case int:
		blockID = int16(b)
	case string:
		if h.W.logicIDs != nil && h.W.logicIDs.blockByName != nil {
			name := strings.ToLower(strings.TrimSpace(b))
			if blk, ok := h.W.logicIDs.blockByName[name]; ok {
				blockID = blk.ID()
			}
		}
	}
	if blockID == 0 {
		t.Block = 0
		t.Build = nil
		return true
	}
	t.Block = BlockID(blockID)
	t.Team = TeamID(byte(team))
	t.Rotation = int8(rotation & 3)
	if t.Build == nil {
		t.Build = &Building{
			Block:     t.Block,
			Team:      t.Team,
			Rotation:  t.Rotation,
			X:         t.X,
			Y:         t.Y,
			Health:    1,
			MaxHealth: estimateBuildMaxHealth(blockID, h.W.model),
		}
	} else {
		t.Build.Block = t.Block
		t.Build.Team = t.Team
		t.Build.Rotation = t.Rotation
	}
	return true
}

func (h *LogicHost) Spawn(unit any, x, y float64, team int, rot float64) (any, bool) {
	if h == nil || h.W == nil {
		return nil, false
	}
	typeID := int16(0)
	switch u := unit.(type) {
	case protocol.UnitType:
		typeID = u.ID()
	case int16:
		typeID = u
	case int32:
		typeID = int16(u)
	case int:
		typeID = int16(u)
	}
	if typeID <= 0 {
		return nil, false
	}
	ent, err := h.W.AddEntity(typeID, float32(x*8), float32(y*8), TeamID(byte(team)))
	if err != nil {
		return nil, false
	}
	ent.Rotation = float32(rot)
	return &ent, true
}

func (h *LogicHost) Apply(status any, target any, duration float64) bool {
	_ = status
	_ = target
	_ = duration
	return false
}

func (h *LogicHost) Explosion(x, y float64, damage, radius float64, team int) bool {
	_ = x
	_ = y
	_ = damage
	_ = radius
	_ = team
	return false
}

func (h *LogicHost) Status(target any) (health, maxHealth float64, team int, ok bool) {
	switch t := target.(type) {
	case *Building:
		return float64(t.Health), float64(t.MaxHealth), int(t.Team), true
	case Building:
		return float64(t.Health), float64(t.MaxHealth), int(t.Team), true
	case *RawEntity:
		return float64(t.Health), float64(t.MaxHealth), int(t.Team), true
	case RawEntity:
		return float64(t.Health), float64(t.MaxHealth), int(t.Team), true
	default:
		return 0, 0, 0, false
	}
}

func (h *LogicHost) PrintFlush(target any, text string) bool {
	if h == nil || h.W == nil {
		return false
	}
	b := h.getBuilding(target)
	if b == nil {
		return false
	}
	name := h.buildingName(b)
	if !strings.Contains(name, "message") {
		return false
	}
	h.W.setBuildingConfigValueLocked(packTilePos(b.X, b.Y), text)
	return true
}

func (h *LogicHost) DrawFlush(target any, buffer []uint64) bool {
	if h == nil || h.W == nil || len(buffer) == 0 {
		return false
	}
	b := h.getBuilding(target)
	if b == nil {
		return false
	}
	name := h.buildingName(b)
	if !strings.Contains(name, "logic-display") && !strings.Contains(name, "display") && !strings.Contains(name, "canvas") {
		return false
	}
	if h.W.logicDisplayBuffers == nil {
		h.W.logicDisplayBuffers = map[int32][]uint64{}
	}
	pos := packTilePos(b.X, b.Y)
	cp := make([]uint64, len(buffer))
	copy(cp, buffer)
	h.W.logicDisplayBuffers[pos] = cp
	return true
}

func (h *LogicHost) ClientData(channel string, value any, reliable bool) bool {
	if h == nil || h.W == nil {
		return false
	}
	rules := h.W.rulesMgr.Get()
	if rules == nil || !rules.AllowLogicData {
		return false
	}
	if channel == "" {
		return false
	}
	h.W.logicClientData = append(h.W.logicClientData, logicClientDataEvent{
		Channel:  channel,
		Value:    value,
		Reliable: reliable,
	})
	return true
}

func (h *LogicHost) Fetch(kind string, team any, extra any, idx int) (any, bool) {
	if h == nil || h.W == nil || h.W.model == nil {
		return nil, false
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "unitbind":
		for i := range h.W.model.Entities {
			e := &h.W.model.Entities[i]
			if e == nil || e.Health <= 0 {
				continue
			}
			if idx > 0 && int(e.TypeID) != idx {
				continue
			}
			return e, true
		}
	case "unit":
		return h.fetchUnit(team, extra, idx)
	case "unitcount":
		return h.fetchUnitCount(team, extra), true
	case "build":
		return h.fetchBuilding(team, extra, idx)
	case "buildcount":
		return h.fetchBuildingCount(team, extra), true
	}
	return nil, false
}

func (h *LogicHost) SyncVar(id int, value any) bool {
	if h == nil || h.W == nil || id < 0 {
		return false
	}
	rules := h.W.rulesMgr.Get()
	if rules == nil || !rules.AllowLogicData {
		return false
	}
	h.W.logicSyncVars = append(h.W.logicSyncVars, logicSyncEvent{
		BuildPos: h.BuildPos,
		VarID:    int32(id),
		Value:    value,
	})
	return true
}

func (h *LogicHost) sensorBuilding(b *Building, sensor string) (any, bool) {
	if b == nil {
		return nil, false
	}
	sensor = strings.ToLower(strings.TrimSpace(sensor))
	pos := packTilePos(b.X, b.Y)
	switch sensor {
	case "x":
		return float64(b.X), true
	case "y":
		return float64(b.Y), true
	case "health":
		return float64(b.Health), true
	case "maxhealth":
		return float64(b.MaxHealth), true
	case "team":
		return float64(b.Team), true
	case "rotation":
		return float64(b.Rotation), true
	case "totalitems":
		sum := float64(0)
		for _, st := range b.Items {
			sum += float64(st.Amount)
		}
		return sum, true
	case "totalliquids":
		sum := float64(0)
		for _, st := range b.Liquids {
			sum += float64(st.Amount)
		}
		return sum, true
	case "itemcapacity":
		if h.W.blockNamesByID != nil && h.W.blockPropsByName != nil {
			if name, ok := h.W.blockNamesByID[int16(b.Block)]; ok {
				props := h.W.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]
				return float64(props.ItemCapacity), true
			}
		}
	case "liquidcapacity":
		if h.W.blockNamesByID != nil && h.W.blockPropsByName != nil {
			if name, ok := h.W.blockNamesByID[int16(b.Block)]; ok {
				props := h.W.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]
				return float64(props.LiquidCapacity), true
			}
		}
	case "powercapacity":
		if h.W.blockNamesByID != nil && h.W.blockPropsByName != nil {
			if name, ok := h.W.blockNamesByID[int16(b.Block)]; ok {
				props := h.W.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]
				return float64(props.PowerCapacity), true
			}
		}
	case "totalpower":
		if h.W.blockNamesByID != nil && h.W.blockPropsByName != nil {
			if name, ok := h.W.blockNamesByID[int16(b.Block)]; ok {
				props := h.W.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]
				if props.PowerCapacity > 0 {
					if h.W.powerStoredByPos != nil {
						return float64(h.W.powerStoredByPos[pos]), true
					}
					return float64(0), true
				}
				if h.W.powerStatusByPos != nil {
					return float64(h.W.powerStatusByPos[pos]), true
				}
			}
		}
	case "powernetstored":
		if h.W.powerNetByPos != nil {
			if net := h.W.powerNetByPos[pos]; net != nil {
				return float64(net.Energy), true
			}
		}
	case "powernetcapacity":
		if h.W.powerNetByPos != nil {
			if net := h.W.powerNetByPos[pos]; net != nil {
				return float64(net.Capacity), true
			}
		}
	case "powernetin":
		if h.W.powerNetByPos != nil {
			if net := h.W.powerNetByPos[pos]; net != nil {
				return float64(net.Produced), true
			}
		}
	case "powernetout":
		if h.W.powerNetByPos != nil {
			if net := h.W.powerNetByPos[pos]; net != nil {
				return float64(net.Consumed), true
			}
		}
	case "firstitem":
		if len(b.Items) > 0 && h.W.content != nil {
			return h.W.content.Item(int16(b.Items[0].Item)), true
		}
	case "buffersize":
		if v, ok := h.W.buildingConfigValueLocked(pos); ok {
			if s, ok2 := v.(string); ok2 {
				return float64(len(s)), true
			}
		}
	case "enabled":
		if v, ok := h.W.buildingConfigValueLocked(pos); ok {
			if b, ok2 := v.(bool); ok2 {
				if b {
					return float64(1), true
				}
				return float64(0), true
			}
		}
	}
	return nil, false
}

func (h *LogicHost) sensorEntity(e *RawEntity, sensor string) (any, bool) {
	if e == nil {
		return nil, false
	}
	sensor = strings.ToLower(strings.TrimSpace(sensor))
	switch sensor {
	case "x":
		return float64(e.X / 8), true
	case "y":
		return float64(e.Y / 8), true
	case "health":
		return float64(e.Health), true
	case "maxhealth":
		return float64(e.MaxHealth), true
	case "team":
		return float64(e.Team), true
	case "rotation":
		return float64(e.Rotation), true
	case "velocityx":
		return float64(e.VelX), true
	case "velocityy":
		return float64(e.VelY), true
	case "dead":
		if e.Health <= 0 {
			return float64(1), true
		}
		return float64(0), true
	case "type":
		if h.W.content != nil {
			return h.W.content.UnitType(e.TypeID), true
		}
	case "id":
		return float64(e.ID), true
	case "flag":
		if h.W.logicUnitFlags != nil {
			return h.W.logicUnitFlags[e.ID], true
		}
	}
	return nil, false
}

func (h *LogicHost) controlBuilding(b *Building, action string, a, bvar, c, d *logic.MlogVar) bool {
	if b == nil {
		return false
	}
	pos := packTilePos(b.X, b.Y)
	switch action {
	case "enabled":
		val := numOrZeroVar(a) != 0
		h.W.setBuildingConfigValueLocked(pos, val)
		return true
	case "config":
		if a != nil && a.IsObj {
			h.W.setBuildingConfigValueLocked(pos, a.ObjVal)
		} else {
			h.W.setBuildingConfigValueLocked(pos, numOrZeroVar(a))
		}
		return true
	case "rotate", "rotation":
		rot := int8(int(numOrZeroVar(a)) & 3)
		b.Rotation = rot
		if t, ok := h.W.tileForPosLocked(pos); ok && t != nil {
			t.Rotation = rot
		}
		return true
	default:
		return false
	}
}

func (h *LogicHost) findRadarTarget(target any, filter string, sort string) (any, bool) {
	if h == nil || h.W == nil || h.W.model == nil {
		return nil, false
	}
	var baseX, baseY float32
	var baseTeam TeamID
	var rangeLimit float32 = 8 * 10
	switch t := target.(type) {
	case *Building:
		baseX, baseY = float32(t.X*8), float32(t.Y*8)
		baseTeam = t.Team
		rangeLimit = h.W.logicRangeForBlock(h.buildingName(t))
	case Building:
		baseX, baseY = float32(t.X*8), float32(t.Y*8)
		baseTeam = t.Team
		rangeLimit = h.W.logicRangeForBlock(h.buildingName(&t))
	case *RawEntity:
		baseX, baseY = t.X, t.Y
		baseTeam = t.Team
		if t.AttackRange > 0 {
			rangeLimit = t.AttackRange
		}
	case RawEntity:
		baseX, baseY = t.X, t.Y
		baseTeam = t.Team
		if t.AttackRange > 0 {
			rangeLimit = t.AttackRange
		}
	default:
		return nil, false
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	sort = strings.ToLower(strings.TrimSpace(sort))
	bestScore := float64(0)
	var best *RawEntity
	for i := range h.W.model.Entities {
		e := &h.W.model.Entities[i]
		if e == nil || e.Health <= 0 {
			continue
		}
		if filter == "enemy" && e.Team == baseTeam {
			continue
		}
		if filter == "ally" && e.Team != baseTeam {
			continue
		}
		dx := e.X - baseX
		dy := e.Y - baseY
		dist2 := dx*dx + dy*dy
		if rangeLimit > 0 && dist2 > rangeLimit*rangeLimit {
			continue
		}
		var score float64
		switch sort {
		case "health":
			score = float64(e.Health)
		case "maxhealth":
			score = float64(e.MaxHealth)
		default:
			score = -float64(dist2)
		}
		if best == nil || score > bestScore {
			best = e
			bestScore = score
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

func (h *LogicHost) buildingName(b *Building) string {
	if h == nil || h.W == nil || b == nil || h.W.blockNamesByID == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(h.W.blockNamesByID[int16(b.Block)]))
}

func (h *LogicHost) getBuilding(v any) *Building {
	switch t := v.(type) {
	case *Building:
		return t
	case Building:
		return &t
	default:
		return nil
	}
}

func (h *LogicHost) readMemoryCell(b *Building, addr int) float64 {
	if b == nil || h.W == nil {
		return 0
	}
	pos := packTilePos(b.X, b.Y)
	cell := h.memoryCellForBuilding(b, pos)
	if cell == nil {
		return 0
	}
	return cell.Get(addr)
}

func (h *LogicHost) writeMemoryCell(b *Building, addr int, value float64) {
	if b == nil || h.W == nil {
		return
	}
	pos := packTilePos(b.X, b.Y)
	cell := h.memoryCellForBuilding(b, pos)
	if cell == nil {
		return
	}
	cell.Set(addr, value)
}

func (h *LogicHost) memoryCellForBuilding(b *Building, pos int32) *logic.MlogCell {
	if h.W.logicMemory == nil {
		h.W.logicMemory = map[int32]*logic.MlogCell{}
	}
	cell := h.W.logicMemory[pos]
	if cell != nil {
		return cell
	}
	name := h.buildingName(b)
	capacity := 64
	switch {
	case strings.Contains(name, "memory-bank"):
		capacity = 512
	case strings.Contains(name, "world-cell"):
		capacity = 512
	case strings.Contains(name, "memory-cell"):
		capacity = 64
	}
	cell = logic.NewMlogCell(capacity)
	h.W.logicMemory[pos] = cell
	return cell
}

func (h *LogicHost) findEntity(target any) *RawEntity {
	switch t := target.(type) {
	case *RawEntity:
		return t
	case RawEntity:
		return &t
	case int32:
		if h.W == nil || h.W.model == nil {
			return nil
		}
		for i := range h.W.model.Entities {
			if h.W.model.Entities[i].ID == t {
				return &h.W.model.Entities[i]
			}
		}
	}
	return nil
}

func numOrZeroVar(v *logic.MlogVar) float64 {
	if v == nil {
		return 0
	}
	return v.Num()
}

func (h *LogicHost) fetchUnit(team any, extra any, idx int) (any, bool) {
	if h == nil || h.W == nil || h.W.model == nil {
		return nil, false
	}
	teamID := parseTeamID(team)
	unitType := parseUnitTypeID(extra)
	count := 0
	for i := range h.W.model.Entities {
		e := &h.W.model.Entities[i]
		if e == nil || e.Health <= 0 {
			continue
		}
		if teamID >= 0 && int(e.Team) != teamID {
			continue
		}
		if unitType > 0 && int(e.TypeID) != unitType {
			continue
		}
		if count == idx {
			return e, true
		}
		count++
	}
	return nil, false
}

func (h *LogicHost) fetchUnitCount(team any, extra any) int {
	teamID := parseTeamID(team)
	unitType := parseUnitTypeID(extra)
	count := 0
	for i := range h.W.model.Entities {
		e := &h.W.model.Entities[i]
		if e == nil || e.Health <= 0 {
			continue
		}
		if teamID >= 0 && int(e.Team) != teamID {
			continue
		}
		if unitType > 0 && int(e.TypeID) != unitType {
			continue
		}
		count++
	}
	return count
}

func (h *LogicHost) fetchBuilding(team any, extra any, idx int) (any, bool) {
	if h == nil || h.W == nil || h.W.model == nil {
		return nil, false
	}
	teamID := parseTeamID(team)
	blockID := parseBlockID(extra, h.W)
	count := 0
	for i := range h.W.model.Tiles {
		t := &h.W.model.Tiles[i]
		if t == nil || t.Build == nil || t.Block == 0 || t.Build.Health <= 0 {
			continue
		}
		if teamID >= 0 && int(t.Build.Team) != teamID {
			continue
		}
		if blockID > 0 && int(t.Block) != blockID {
			continue
		}
		if count == idx {
			return t.Build, true
		}
		count++
	}
	return nil, false
}

func (h *LogicHost) fetchBuildingCount(team any, extra any) int {
	if h == nil || h.W == nil || h.W.model == nil {
		return 0
	}
	teamID := parseTeamID(team)
	blockID := parseBlockID(extra, h.W)
	count := 0
	for i := range h.W.model.Tiles {
		t := &h.W.model.Tiles[i]
		if t == nil || t.Build == nil || t.Block == 0 || t.Build.Health <= 0 {
			continue
		}
		if teamID >= 0 && int(t.Build.Team) != teamID {
			continue
		}
		if blockID > 0 && int(t.Block) != blockID {
			continue
		}
		count++
	}
	return count
}

func parseTeamID(v any) int {
	switch t := v.(type) {
	case nil:
		return -1
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case protocol.Team:
		return int(t.ID)
	default:
		return -1
	}
}

func parseUnitTypeID(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case protocol.UnitType:
		return int(t.ID())
	default:
		return 0
	}
}

func parseBlockID(v any, w *World) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case protocol.Block:
		return int(t.ID())
	case string:
		if w != nil && w.logicIDs != nil && w.logicIDs.blockByName != nil {
			name := strings.ToLower(strings.TrimSpace(t))
			if b, ok := w.logicIDs.blockByName[name]; ok {
				return int(b.ID())
			}
		}
	}
	return 0
}
