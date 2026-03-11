package world

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"

	"mdt-server/internal/protocol"
)

const (
	payloadMagic   = "MDTP"
	payloadVersion = byte(1)

	payloadKindUnit  = byte(1)
	payloadKindBuild = byte(2)
)

type PayloadDropKind byte

const (
	PayloadDropNone PayloadDropKind = iota
	PayloadDropUnit
	PayloadDropBuild
)

type PayloadDropResult struct {
	Kind     PayloadDropKind
	UnitID   int32
	BuildPos int32
	BlockID  int16
	Team     TeamID
	Rotation int8
}

func (w *World) EntityHasPayload(id int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || id == 0 {
		return false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == id {
			return len(w.model.Entities[i].Payload) > 0
		}
	}
	return false
}

func (w *World) SetEntityPayload(id int32, payload []byte) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || id == 0 {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		out := append([]byte(nil), payload...)
		w.model.Entities[i].Payload = out
		return w.model.Entities[i], true
	}
	return RawEntity{}, false
}

func (w *World) ClearEntityPayload(id int32) (RawEntity, bool) {
	return w.SetEntityPayload(id, nil)
}

func (w *World) BuildingPayload(buildPos int32) ([]byte, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil || t.Build == nil {
		return nil, false
	}
	if len(t.Build.Payload) == 0 {
		return nil, true
	}
	out := append([]byte(nil), t.Build.Payload...)
	return out, true
}

func (w *World) SetBuildingPayload(buildPos int32, payload []byte) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	if len(payload) == 0 {
		b.Payload = nil
		return true
	}
	b.Payload = append([]byte(nil), payload...)
	return true
}

func (w *World) ClearBuildingPayload(buildPos int32) bool {
	return w.SetBuildingPayload(buildPos, nil)
}

func (w *World) RemoveEntitySilent(id int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || id == 0 {
		return RawEntity{}, false
	}
	ent, ok := w.model.RemoveEntity(id)
	if ok {
		delete(w.unitMountCDs, id)
		delete(w.unitTargets, id)
	}
	return ent, ok
}

func (w *World) PickUnitPayload(carrierID, targetID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || carrierID == 0 || targetID == 0 || carrierID == targetID {
		return false
	}
	carrierIdx := -1
	targetIdx := -1
	for i := range w.model.Entities {
		id := w.model.Entities[i].ID
		if id == carrierID {
			carrierIdx = i
		} else if id == targetID {
			targetIdx = i
		}
		if carrierIdx >= 0 && targetIdx >= 0 {
			break
		}
	}
	if carrierIdx < 0 || targetIdx < 0 {
		return false
	}
	if len(w.model.Entities[carrierIdx].Payload) > 0 {
		return false
	}
	if w.model.Entities[carrierIdx].Team != w.model.Entities[targetIdx].Team {
		return false
	}
	payload := encodePayloadUnit(w.model.Entities[targetIdx])
	if len(payload) == 0 {
		return false
	}
	if _, ok := w.model.RemoveEntity(targetID); !ok {
		return false
	}
	delete(w.unitMountCDs, targetID)
	delete(w.unitTargets, targetID)
	w.model.Entities[carrierIdx].Payload = payload
	return true
}

func (w *World) PickBuildPayload(carrierID int32, buildPos int32) (blockID int16, team TeamID, rot int8, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || carrierID == 0 {
		return 0, 0, 0, false
	}
	carrierIdx := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == carrierID {
			carrierIdx = i
			break
		}
	}
	if carrierIdx < 0 || len(w.model.Entities[carrierIdx].Payload) > 0 {
		return 0, 0, 0, false
	}
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil || t.Build == nil || t.Block == 0 {
		return 0, 0, 0, false
	}
	if t.Build.Team != w.model.Entities[carrierIdx].Team {
		return 0, 0, 0, false
	}
	payload := encodePayloadBuild(w, t)
	if len(payload) == 0 {
		return 0, 0, 0, false
	}
	blockID = int16(t.Block)
	team = t.Build.Team
	rot = t.Build.Rotation
	w.model.Entities[carrierIdx].Payload = payload
	removeBuildingLocked(w, t)
	return blockID, team, rot, true
}

func (w *World) DropEntityPayload(carrierID int32, x, y float32) (PayloadDropResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || carrierID == 0 {
		return PayloadDropResult{}, false
	}
	carrierIdx := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == carrierID {
			carrierIdx = i
			break
		}
	}
	if carrierIdx < 0 {
		return PayloadDropResult{}, false
	}
	payload := w.model.Entities[carrierIdx].Payload
	if len(payload) == 0 {
		return PayloadDropResult{}, false
	}
	result, ok := w.dropPayloadAtLocked(payload, x, y, w.model.Entities[carrierIdx].Team)
	if !ok {
		return PayloadDropResult{}, false
	}
	w.model.Entities[carrierIdx].Payload = nil
	return result, true
}

func (w *World) dropPayloadAtLocked(payload []byte, x, y float32, fallbackTeam TeamID) (PayloadDropResult, bool) {
	if w.model == nil || len(payload) == 0 {
		return PayloadDropResult{}, false
	}
	if unitData, ok := decodePayloadUnit(payload); ok {
		unitData.X = x
		unitData.Y = y
		if unitData.Team == 0 {
			unitData.Team = fallbackTeam
		}
		ent := addEntityFromPayloadLocked(w, unitData)
		return PayloadDropResult{Kind: PayloadDropUnit, UnitID: ent.ID}, true
	}
	if buildData, ok := decodePayloadBuild(payload); ok {
		tx := int(math.Round(float64(x / 8)))
		ty := int(math.Round(float64(y / 8)))
		if !w.model.InBounds(tx, ty) {
			return PayloadDropResult{}, false
		}
		t, err := w.model.TileAt(tx, ty)
		if err != nil || t == nil || t.Block != 0 || t.Build != nil {
			return PayloadDropResult{}, false
		}
		if buildData.Team == 0 {
			buildData.Team = fallbackTeam
		}
		buildPos := packTilePos(tx, ty)
		placeBuildingLocked(w, t, buildData)
		return PayloadDropResult{
			Kind:     PayloadDropBuild,
			BuildPos: buildPos,
			BlockID:  buildData.BlockID,
			Team:     buildData.Team,
			Rotation: buildData.Rotation,
		}, true
	}
	return PayloadDropResult{}, false
}

func (w *World) EnterUnitIntoPayload(unitID int32, buildPos int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || unitID == 0 {
		return false
	}
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil || t.Build == nil || t.Block == 0 {
		return false
	}
	if !buildCanAcceptPayloadLocked(w, t) {
		return false
	}
	if len(t.Build.Payload) > 0 {
		return false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != unitID {
			continue
		}
		payload := encodePayloadUnit(w.model.Entities[i])
		if len(payload) == 0 {
			return false
		}
		if _, ok := w.model.RemoveEntity(unitID); !ok {
			return false
		}
		delete(w.unitMountCDs, unitID)
		delete(w.unitTargets, unitID)
		t.Build.Payload = payload
		return true
	}
	return false
}

func buildCanAcceptPayloadLocked(w *World, t *Tile) bool {
	if w == nil || t == nil || w.blockNamesByID == nil {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	if name == "" {
		return true
	}
	return strings.Contains(name, "payload")
}

func (w *World) payloadSourceEnsurePayloadLocked(t *Tile) bool {
	if w == nil || t == nil || t.Build == nil {
		return false
	}
	if len(t.Build.Payload) > 0 {
		return true
	}
	cfg := any(nil)
	if w.tileConfigValues != nil {
		cfg = w.tileConfigValues[packTilePos(t.X, t.Y)]
	}
	payload := w.payloadFromConfigLocked(t.Build.Team, cfg)
	if len(payload) == 0 {
		return false
	}
	t.Build.Payload = payload
	return true
}

func (w *World) payloadFromConfigLocked(team TeamID, cfg any) []byte {
	if cfg == nil {
		return nil
	}
	switch v := cfg.(type) {
	case protocol.Block:
		return w.payloadFromBlockLocked(team, v.ID())
	case protocol.UnitType:
		return w.payloadFromUnitLocked(team, v.ID())
	case protocol.Content:
		switch v.ContentType() {
		case protocol.ContentBlock:
			return w.payloadFromBlockLocked(team, v.ID())
		case protocol.ContentUnit:
			return w.payloadFromUnitLocked(team, v.ID())
		}
	case protocol.BlockRef:
		return w.payloadFromBlockLocked(team, v.ID())
	case protocol.UnitCommand:
	case protocol.UnitStance:
	case protocol.UnitBox:
		return w.payloadFromUnitLocked(team, int16(v.ID()))
	case int16:
		return w.payloadFromAnyIDLocked(team, v)
	case int32:
		return w.payloadFromAnyIDLocked(team, int16(v))
	case int:
		return w.payloadFromAnyIDLocked(team, int16(v))
	case uint16:
		return w.payloadFromAnyIDLocked(team, int16(v))
	case uint32:
		return w.payloadFromAnyIDLocked(team, int16(v))
	case byte:
		return w.payloadFromAnyIDLocked(team, int16(v))
	}
	return nil
}

func (w *World) payloadFromAnyIDLocked(team TeamID, id int16) []byte {
	if id <= 0 {
		return nil
	}
	if w.blockNamesByID != nil {
		if _, ok := w.blockNamesByID[id]; ok {
			return w.payloadFromBlockLocked(team, id)
		}
	}
	if w.unitNamesByID != nil {
		if _, ok := w.unitNamesByID[id]; ok {
			return w.payloadFromUnitLocked(team, id)
		}
	}
	return nil
}

func (w *World) payloadFromBlockLocked(team TeamID, blockID int16) []byte {
	if blockID <= 0 {
		return nil
	}
	return encodePayloadBuild(w, &Tile{Build: &Building{Block: BlockID(blockID), Team: team, Rotation: 0, Health: 100, MaxHealth: 100}})
}

func (w *World) payloadFromUnitLocked(team TeamID, typeID int16) []byte {
	if typeID <= 0 {
		return nil
	}
	ent := RawEntity{
		TypeID:      typeID,
		Team:        team,
		Health:      100,
		MaxHealth:   100,
		Shield:      0,
		ShieldMax:   0,
		ShieldRegen: 0,
		Armor:       0,
		SlowMul:     1,
		RuntimeInit: true,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	return encodePayloadUnit(ent)
}

type payloadUnitData struct {
	ID        int32
	TypeID    int16
	Team      TeamID
	Health    float32
	MaxHealth float32
	Rotation  float32
	X         float32
	Y         float32
	VelX      float32
	VelY      float32
	RotVel    float32
	Shield    float32
	ShieldMax float32
	Payload   []byte
}

type payloadBuildData struct {
	BlockID     int16
	Team        TeamID
	Rotation    int8
	Health      float32
	MaxHealth   float32
	Config      []byte
	Items       []ItemStack
	Liquids     []LiquidStack
	Payload     []byte
	PowerStatus float32
	Extra       []byte
}

func encodePayloadUnit(ent RawEntity) []byte {
	buf := newPayloadWriter(payloadKindUnit)
	writeInt32(buf, ent.ID)
	writeInt16(buf, ent.TypeID)
	writeByte(buf, byte(ent.Team))
	writeFloat32(buf, ent.Health)
	writeFloat32(buf, ent.MaxHealth)
	writeFloat32(buf, ent.Rotation)
	writeFloat32(buf, ent.X)
	writeFloat32(buf, ent.Y)
	writeFloat32(buf, ent.VelX)
	writeFloat32(buf, ent.VelY)
	writeFloat32(buf, ent.RotVel)
	writeFloat32(buf, ent.Shield)
	writeFloat32(buf, ent.ShieldMax)
	writeBytes(buf, ent.Payload)
	return buf.Bytes()
}

func encodePayloadBuild(w *World, t *Tile) []byte {
	if t == nil || t.Build == nil {
		return nil
	}
	if payload := encodePayloadBuildVanilla(w, t); len(payload) > 0 {
		return payload
	}
	return encodePayloadBuildCustom(t)
}

func encodePayloadBuildCustom(t *Tile) []byte {
	if t == nil || t.Build == nil {
		return nil
	}
	buf := newPayloadWriter(payloadKindBuild)
	writeInt16(buf, int16(t.Build.Block))
	writeByte(buf, byte(t.Build.Team))
	writeByte(buf, byte(t.Build.Rotation))
	writeFloat32(buf, t.Build.Health)
	writeFloat32(buf, t.Build.MaxHealth)
	writeBytes(buf, t.Build.Config)
	if len(t.Build.Items) > 0 {
		writeInt16(buf, int16(len(t.Build.Items)))
		for _, st := range t.Build.Items {
			writeInt16(buf, int16(st.Item))
			writeInt32(buf, st.Amount)
		}
	} else {
		writeInt16(buf, 0)
	}
	if len(t.Build.Liquids) > 0 {
		writeInt16(buf, int16(len(t.Build.Liquids)))
		for _, st := range t.Build.Liquids {
			writeInt16(buf, int16(st.Liquid))
			writeFloat32(buf, st.Amount)
		}
	} else {
		writeInt16(buf, 0)
	}
	writeBytes(buf, t.Build.Payload)
	return buf.Bytes()
}

func decodePayloadUnit(payload []byte) (payloadUnitData, bool) {
	if data, ok := decodePayloadUnitCustom(payload); ok {
		return data, true
	}
	return decodePayloadUnitVanilla(payload)
}

func decodePayloadBuild(payload []byte) (payloadBuildData, bool) {
	if data, ok := decodePayloadBuildCustom(payload); ok {
		return data, true
	}
	return decodePayloadBuildVanilla(payload)
}

func decodePayloadUnitCustom(payload []byte) (payloadUnitData, bool) {
	reader := newPayloadReader(payload, payloadKindUnit)
	if reader == nil {
		return payloadUnitData{}, false
	}
	id, ok := reader.readInt32()
	if !ok {
		return payloadUnitData{}, false
	}
	typeID, ok := reader.readInt16()
	if !ok {
		return payloadUnitData{}, false
	}
	team, ok := reader.readByte()
	if !ok {
		return payloadUnitData{}, false
	}
	health, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	maxHealth, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	rot, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	x, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	y, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	vx, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	vy, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	rotVel, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	shield, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	shieldMax, ok := reader.readFloat32()
	if !ok {
		return payloadUnitData{}, false
	}
	nested, ok := reader.readBytes()
	if !ok {
		return payloadUnitData{}, false
	}
	return payloadUnitData{
		ID:        id,
		TypeID:    typeID,
		Team:      TeamID(team),
		Health:    health,
		MaxHealth: maxHealth,
		Rotation:  rot,
		X:         x,
		Y:         y,
		VelX:      vx,
		VelY:      vy,
		RotVel:    rotVel,
		Shield:    shield,
		ShieldMax: shieldMax,
		Payload:   nested,
	}, true
}

func decodePayloadBuildCustom(payload []byte) (payloadBuildData, bool) {
	reader := newPayloadReader(payload, payloadKindBuild)
	if reader == nil {
		return payloadBuildData{}, false
	}
	blockID, ok := reader.readInt16()
	if !ok {
		return payloadBuildData{}, false
	}
	team, ok := reader.readByte()
	if !ok {
		return payloadBuildData{}, false
	}
	rot, ok := reader.readByte()
	if !ok {
		return payloadBuildData{}, false
	}
	health, ok := reader.readFloat32()
	if !ok {
		return payloadBuildData{}, false
	}
	maxHealth, ok := reader.readFloat32()
	if !ok {
		return payloadBuildData{}, false
	}
	config, ok := reader.readBytes()
	if !ok {
		return payloadBuildData{}, false
	}
	itemCount, ok := reader.readInt16()
	if !ok {
		return payloadBuildData{}, false
	}
	items := make([]ItemStack, 0, maxInt16(itemCount))
	for i := int16(0); i < itemCount; i++ {
		itemID, ok := reader.readInt16()
		if !ok {
			return payloadBuildData{}, false
		}
		amt, ok := reader.readInt32()
		if !ok {
			return payloadBuildData{}, false
		}
		if amt > 0 {
			items = append(items, ItemStack{Item: ItemID(itemID), Amount: amt})
		}
	}
	liqCount, ok := reader.readInt16()
	if !ok {
		return payloadBuildData{}, false
	}
	liquids := make([]LiquidStack, 0, maxInt16(liqCount))
	for i := int16(0); i < liqCount; i++ {
		liqID, ok := reader.readInt16()
		if !ok {
			return payloadBuildData{}, false
		}
		amt, ok := reader.readFloat32()
		if !ok {
			return payloadBuildData{}, false
		}
		if amt > 0 {
			liquids = append(liquids, LiquidStack{Liquid: LiquidID(liqID), Amount: amt})
		}
	}
	nested, ok := reader.readBytes()
	if !ok {
		return payloadBuildData{}, false
	}
	return payloadBuildData{
		BlockID:   blockID,
		Team:      TeamID(team),
		Rotation:  int8(rot),
		Health:    health,
		MaxHealth: maxHealth,
		Config:    config,
		Items:     items,
		Liquids:   liquids,
		Payload:   nested,
	}, true
}

func encodePayloadBuildVanilla(w *World, t *Tile) []byte {
	if t == nil || t.Build == nil {
		return nil
	}
	writer := protocol.NewWriter()
	if err := writer.WriteByte(protocol.PayloadBlock); err != nil {
		return nil
	}
	if err := writer.WriteInt16(int16(t.Build.Block)); err != nil {
		return nil
	}
	if err := writer.WriteByte(0); err != nil { // build version (block-specific)
		return nil
	}
	if !writeBuildingBaseVanilla(writer, w, t) {
		return nil
	}
	return writer.Bytes()
}

type vanillaBuildBase struct {
	Health      float32
	Rotation    int8
	Team        TeamID
	Items       []ItemStack
	Liquids     []LiquidStack
	PowerStatus float32
	Enabled     bool
	Efficiency  float32
	OptionalEff float32
	Extra       []byte
}

func decodePayloadBuildVanilla(payload []byte) (payloadBuildData, bool) {
	r := protocol.NewReader(payload)
	typ, err := r.ReadByte()
	if err != nil || typ != protocol.PayloadBlock {
		return payloadBuildData{}, false
	}
	blockID, err := r.ReadInt16()
	if err != nil {
		return payloadBuildData{}, false
	}
	if _, err := r.ReadByte(); err != nil { // build version
		return payloadBuildData{}, false
	}
	base, ok := readBuildingBaseVanilla(r)
	if !ok {
		return payloadBuildData{}, false
	}
	extra := []byte(nil)
	if rem := r.Remaining(); rem > 0 {
		extra, _ = r.ReadBytes(rem)
	}
	maxHealth := base.Health
	if maxHealth <= 0 {
		maxHealth = 1
	}
	return payloadBuildData{
		BlockID:     blockID,
		Team:        base.Team,
		Rotation:    base.Rotation,
		Health:      maxf(base.Health, 1),
		MaxHealth:   maxHealth,
		Items:       base.Items,
		Liquids:     base.Liquids,
		PowerStatus: base.PowerStatus,
		Extra:       extra,
	}, true
}

func decodePayloadUnitVanilla(payload []byte) (payloadUnitData, bool) {
	r := protocol.NewReader(payload)
	typ, err := r.ReadByte()
	if err != nil || typ != protocol.PayloadUnit {
		return payloadUnitData{}, false
	}
	classID, err := r.ReadByte()
	if err != nil {
		return payloadUnitData{}, false
	}
	_ = classID
	u := &protocol.UnitEntitySync{}
	if err := u.ReadSync(r); err != nil {
		return payloadUnitData{}, false
	}
	return payloadUnitData{
		TypeID:    u.TypeID,
		Team:      TeamID(u.TeamID),
		Health:    u.Health,
		MaxHealth: u.Health,
		Rotation:  u.Rotation,
		X:         u.X,
		Y:         u.Y,
		VelX:      u.Vel.X,
		VelY:      u.Vel.Y,
		Shield:    u.Shield,
		ShieldMax: 0,
	}, true
}

func addEntityFromPayloadLocked(w *World, data payloadUnitData) RawEntity {
	ent := RawEntity{
		TypeID:      data.TypeID,
		ID:          data.ID,
		X:           data.X,
		Y:           data.Y,
		Rotation:    data.Rotation,
		VelX:        data.VelX,
		VelY:        data.VelY,
		RotVel:      data.RotVel,
		Health:      data.Health,
		MaxHealth:   data.MaxHealth,
		Shield:      data.Shield,
		ShieldMax:   data.ShieldMax,
		Team:        data.Team,
		Payload:     append([]byte(nil), data.Payload...),
		RuntimeInit: true,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	return w.model.AddEntity(ent)
}

func placeBuildingLocked(w *World, t *Tile, data payloadBuildData) {
	if w == nil || t == nil {
		return
	}
	t.Block = BlockID(data.BlockID)
	t.Team = data.Team
	t.Rotation = data.Rotation
	t.Build = &Building{
		Block:     t.Block,
		Team:      data.Team,
		Rotation:  data.Rotation,
		X:         t.X,
		Y:         t.Y,
		Health:    maxf(data.Health, 1),
		MaxHealth: maxf(data.MaxHealth, maxf(data.Health, 1)),
		Config:    append([]byte(nil), data.Config...),
		Payload:   append([]byte(nil), data.Payload...),
	}
	if len(data.Items) > 0 {
		t.Build.Items = append([]ItemStack(nil), data.Items...)
	}
	if len(data.Liquids) > 0 {
		t.Build.Liquids = append([]LiquidStack(nil), data.Liquids...)
	}
	if data.PowerStatus > 0 && w.blockNamesByID != nil && w.blockPropsByName != nil {
		pos := packTilePos(t.X, t.Y)
		if name, ok := w.blockNamesByID[int16(t.Block)]; ok {
			n := strings.ToLower(strings.TrimSpace(name))
			if props, ok := w.blockPropsByName[n]; ok {
				if w.powerStatusByPos == nil {
					w.powerStatusByPos = map[int32]float32{}
				}
				w.powerStatusByPos[pos] = data.PowerStatus
				if props.PowerCapacity > 0 {
					if w.powerStoredByPos == nil {
						w.powerStoredByPos = map[int32]float32{}
					}
					w.powerStoredByPos[pos] = data.PowerStatus * props.PowerCapacity
				}
			}
		}
	}
}

func writeBuildingBaseVanilla(wr *protocol.Writer, w *World, t *Tile) bool {
	if wr == nil || t == nil || t.Build == nil {
		return false
	}
	health := t.Build.Health
	rotation := byte(t.Build.Rotation) | 0x80
	team := byte(t.Build.Team)
	version := byte(3)
	enabled := byte(1)
	hasItems := len(t.Build.Items) > 0
	hasLiquids := len(t.Build.Liquids) > 0
	hasPower := false
	status := float32(0)
	if w != nil && w.blockNamesByID != nil && w.blockPropsByName != nil {
		if name, ok := w.blockNamesByID[int16(t.Build.Block)]; ok {
			n := strings.ToLower(strings.TrimSpace(name))
			if props, ok := w.blockPropsByName[n]; ok {
				if props.ItemCapacity > 0 {
					hasItems = true
				}
				if props.LiquidCapacity > 0 {
					hasLiquids = true
				}
				if props.PowerCapacity > 0 || props.PowerUse > 0 || props.PowerProduction > 0 || props.LinkRangeTiles > 0 ||
					strings.Contains(n, "power-node") || strings.Contains(n, "surge-tower") ||
					strings.Contains(n, "beam-link") || strings.Contains(n, "power-diode") {
					hasPower = true
				}
				if hasPower {
					pos := packTilePos(t.X, t.Y)
					if props.PowerCapacity > 0 {
						if w.powerStoredByPos != nil && props.PowerCapacity > 0 {
							status = w.powerStoredByPos[pos] / props.PowerCapacity
						}
					} else if w.powerStatusByPos != nil {
						status = w.powerStatusByPos[pos]
					}
				}
			}
		}
	}
	if status < 0 {
		status = 0
	}
	if status > 1 {
		status = 1
	}
	if !hasPower && status > 0 {
		hasPower = true
	}
	moduleBits := byte(0)
	if hasItems {
		moduleBits |= 1
	}
	if hasPower {
		moduleBits |= 1 << 1
	}
	if hasLiquids {
		moduleBits |= 1 << 2
	}
	moduleBits |= 1 << 3

	if err := wr.WriteFloat32(health); err != nil {
		return false
	}
	if err := wr.WriteByte(rotation); err != nil {
		return false
	}
	if err := wr.WriteByte(team); err != nil {
		return false
	}
	if err := wr.WriteByte(version); err != nil {
		return false
	}
	if err := wr.WriteByte(enabled); err != nil {
		return false
	}
	if err := wr.WriteByte(moduleBits); err != nil {
		return false
	}
	if hasItems {
		if !writeItemModuleVanilla(wr, t.Build.Items) {
			return false
		}
	}
	if hasPower {
		if !writePowerModuleVanilla(wr, status) {
			return false
		}
	}
	if hasLiquids {
		if !writeLiquidModuleVanilla(wr, t.Build.Liquids) {
			return false
		}
	}
	if err := wr.WriteByte(0xFF); err != nil { // efficiency=1.0
		return false
	}
	if err := wr.WriteByte(0xFF); err != nil { // optionalEfficiency=1.0
		return false
	}
	return true
}

func readBuildingBaseVanilla(r *protocol.Reader) (vanillaBuildBase, bool) {
	if r == nil {
		return vanillaBuildBase{}, false
	}
	health, err := r.ReadFloat32()
	if err != nil {
		return vanillaBuildBase{}, false
	}
	rot, err := r.ReadByte()
	if err != nil {
		return vanillaBuildBase{}, false
	}
	team, err := r.ReadByte()
	if err != nil {
		return vanillaBuildBase{}, false
	}
	rotation := int8(rot & 0x7F)
	moduleBits := byte(0)
	enabled := true
	version := byte(0)
	legacy := true
	if (rot & 0x80) != 0 {
		v, err := r.ReadByte()
		if err != nil {
			return vanillaBuildBase{}, false
		}
		version = v
		if version >= 1 {
			on, err := r.ReadByte()
			if err != nil {
				return vanillaBuildBase{}, false
			}
			enabled = on == 1
		}
		if version >= 2 {
			if v2, err := r.ReadUByte(); err == nil {
				moduleBits = byte(v2)
			} else {
				return vanillaBuildBase{}, false
			}
		}
		legacy = false
	}
	items := []ItemStack(nil)
	liquids := []LiquidStack(nil)
	powerStatus := float32(0)
	if moduleBits&1 != 0 {
		it, ok := readItemModuleVanilla(r, legacy)
		if !ok {
			return vanillaBuildBase{}, false
		}
		items = it
	}
	if moduleBits&(1<<1) != 0 {
		status, ok := readPowerModuleVanilla(r)
		if !ok {
			return vanillaBuildBase{}, false
		}
		powerStatus = status
	}
	if moduleBits&(1<<2) != 0 {
		lq, ok := readLiquidModuleVanilla(r, legacy)
		if !ok {
			return vanillaBuildBase{}, false
		}
		liquids = lq
	}
	if moduleBits&(1<<4) != 0 {
		if _, err := r.ReadFloat32(); err != nil {
			return vanillaBuildBase{}, false
		}
		if _, err := r.ReadFloat32(); err != nil {
			return vanillaBuildBase{}, false
		}
	}
	if moduleBits&(1<<5) != 0 {
		if _, err := r.ReadInt32(); err != nil {
			return vanillaBuildBase{}, false
		}
	}
	if version <= 2 {
		if _, err := r.ReadBool(); err != nil {
			return vanillaBuildBase{}, false
		}
	}
	eff := float32(1)
	optional := float32(1)
	if version >= 3 {
		if v, err := r.ReadUByte(); err == nil {
			eff = float32(v) / 255
		} else {
			return vanillaBuildBase{}, false
		}
		if v, err := r.ReadUByte(); err == nil {
			optional = float32(v) / 255
		} else {
			return vanillaBuildBase{}, false
		}
	}
	if version == 4 {
		if _, err := r.ReadInt64(); err != nil {
			return vanillaBuildBase{}, false
		}
	}
	return vanillaBuildBase{
		Health:      health,
		Rotation:    rotation,
		Team:        TeamID(team),
		Items:       items,
		Liquids:     liquids,
		PowerStatus: powerStatus,
		Enabled:     enabled,
		Efficiency:  eff,
		OptionalEff: optional,
	}, true
}

func writeItemModuleVanilla(wr *protocol.Writer, items []ItemStack) bool {
	if wr == nil {
		return false
	}
	count := int16(0)
	for _, st := range items {
		if st.Amount > 0 {
			count++
		}
	}
	if err := wr.WriteInt16(count); err != nil {
		return false
	}
	if count == 0 {
		return true
	}
	for _, st := range items {
		if st.Amount <= 0 {
			continue
		}
		if err := wr.WriteInt16(int16(st.Item)); err != nil {
			return false
		}
		if err := wr.WriteInt32(st.Amount); err != nil {
			return false
		}
	}
	return true
}

func readItemModuleVanilla(r *protocol.Reader, legacy bool) ([]ItemStack, bool) {
	if r == nil {
		return nil, false
	}
	var count int16
	if legacy {
		v, err := r.ReadUByte()
		if err != nil {
			return nil, false
		}
		count = int16(v)
	} else {
		v, err := r.ReadInt16()
		if err != nil {
			return nil, false
		}
		count = v
	}
	if count <= 0 {
		return nil, true
	}
	out := make([]ItemStack, 0, maxInt16(count))
	for i := int16(0); i < count; i++ {
		var itemID int16
		if legacy {
			v, err := r.ReadUByte()
			if err != nil {
				return nil, false
			}
			itemID = int16(v)
		} else {
			v, err := r.ReadInt16()
			if err != nil {
				return nil, false
			}
			itemID = v
		}
		amt, err := r.ReadInt32()
		if err != nil {
			return nil, false
		}
		if amt > 0 {
			out = append(out, ItemStack{Item: ItemID(itemID), Amount: amt})
		}
	}
	return out, true
}

func writeLiquidModuleVanilla(wr *protocol.Writer, liquids []LiquidStack) bool {
	if wr == nil {
		return false
	}
	count := int16(0)
	for _, st := range liquids {
		if st.Amount > 0 {
			count++
		}
	}
	if err := wr.WriteInt16(count); err != nil {
		return false
	}
	if count == 0 {
		return true
	}
	for _, st := range liquids {
		if st.Amount <= 0 {
			continue
		}
		if err := wr.WriteInt16(int16(st.Liquid)); err != nil {
			return false
		}
		if err := wr.WriteFloat32(st.Amount); err != nil {
			return false
		}
	}
	return true
}

func readLiquidModuleVanilla(r *protocol.Reader, legacy bool) ([]LiquidStack, bool) {
	if r == nil {
		return nil, false
	}
	var count int16
	if legacy {
		v, err := r.ReadUByte()
		if err != nil {
			return nil, false
		}
		count = int16(v)
	} else {
		v, err := r.ReadInt16()
		if err != nil {
			return nil, false
		}
		count = v
	}
	if count <= 0 {
		return nil, true
	}
	out := make([]LiquidStack, 0, maxInt16(count))
	for i := int16(0); i < count; i++ {
		var liquidID int16
		if legacy {
			v, err := r.ReadUByte()
			if err != nil {
				return nil, false
			}
			liquidID = int16(v)
		} else {
			v, err := r.ReadInt16()
			if err != nil {
				return nil, false
			}
			liquidID = v
		}
		amt, err := r.ReadFloat32()
		if err != nil {
			return nil, false
		}
		if amt > 0 {
			out = append(out, LiquidStack{Liquid: LiquidID(liquidID), Amount: amt})
		}
	}
	return out, true
}

func writePowerModuleVanilla(wr *protocol.Writer, status float32) bool {
	if wr == nil {
		return false
	}
	if err := wr.WriteInt16(0); err != nil {
		return false
	}
	if err := wr.WriteFloat32(status); err != nil {
		return false
	}
	return true
}

func readPowerModuleVanilla(r *protocol.Reader) (float32, bool) {
	if r == nil {
		return 0, false
	}
	links, err := r.ReadInt16()
	if err != nil {
		return 0, false
	}
	if links > 0 {
		for i := int16(0); i < links; i++ {
			if _, err := r.ReadInt32(); err != nil {
				return 0, false
			}
		}
	}
	status, err := r.ReadFloat32()
	if err != nil {
		return 0, false
	}
	if status < 0 {
		status = 0
	}
	return status, true
}

func removeBuildingLocked(w *World, t *Tile) {
	if w == nil || t == nil {
		return
	}
	buildPos := int32(t.Y*w.model.Width + t.X)
	t.Build = nil
	t.Block = 0
	t.Team = 0
	t.Rotation = 0
	delete(w.buildStates, buildPos)
	delete(w.pendingBuilds, buildPos)
	delete(w.tileConfigValues, packTilePos(t.X, t.Y))
}

func newPayloadWriter(kind byte) *bytes.Buffer {
	buf := &bytes.Buffer{}
	_, _ = buf.WriteString(payloadMagic)
	_ = buf.WriteByte(payloadVersion)
	_ = buf.WriteByte(kind)
	return buf
}

type payloadReader struct {
	r *bytes.Reader
}

func newPayloadReader(payload []byte, wantKind byte) *payloadReader {
	if len(payload) < 6 {
		return nil
	}
	if string(payload[:4]) != payloadMagic {
		return nil
	}
	if payload[4] != payloadVersion {
		return nil
	}
	if payload[5] != wantKind {
		return nil
	}
	return &payloadReader{r: bytes.NewReader(payload[6:])}
}

func (p *payloadReader) readByte() (byte, bool) {
	if p == nil || p.r == nil {
		return 0, false
	}
	b, err := p.r.ReadByte()
	return b, err == nil
}

func (p *payloadReader) readInt16() (int16, bool) {
	u, ok := p.readUint16()
	return int16(u), ok
}

func (p *payloadReader) readInt32() (int32, bool) {
	u, ok := p.readUint32()
	return int32(u), ok
}

func (p *payloadReader) readUint16() (uint16, bool) {
	if p == nil || p.r == nil {
		return 0, false
	}
	var buf [2]byte
	if _, err := p.r.Read(buf[:]); err != nil {
		return 0, false
	}
	return binary.BigEndian.Uint16(buf[:]), true
}

func (p *payloadReader) readUint32() (uint32, bool) {
	if p == nil || p.r == nil {
		return 0, false
	}
	var buf [4]byte
	if _, err := p.r.Read(buf[:]); err != nil {
		return 0, false
	}
	return binary.BigEndian.Uint32(buf[:]), true
}

func (p *payloadReader) readFloat32() (float32, bool) {
	u, ok := p.readUint32()
	if !ok {
		return 0, false
	}
	return math.Float32frombits(u), true
}

func (p *payloadReader) readBytes() ([]byte, bool) {
	n, ok := p.readInt32()
	if !ok || n < 0 {
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	if p.r.Len() < int(n) {
		return nil, false
	}
	out := make([]byte, n)
	if _, err := p.r.Read(out); err != nil {
		return nil, false
	}
	return out, true
}

func writeByte(buf *bytes.Buffer, v byte) {
	_ = buf.WriteByte(v)
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	_, _ = buf.Write(b[:])
}

func writeInt16(buf *bytes.Buffer, v int16) {
	writeUint16(buf, uint16(v))
}

func writeUint32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	_, _ = buf.Write(b[:])
}

func writeInt32(buf *bytes.Buffer, v int32) {
	writeUint32(buf, uint32(v))
}

func writeFloat32(buf *bytes.Buffer, v float32) {
	writeUint32(buf, math.Float32bits(v))
}

func writeBytes(buf *bytes.Buffer, payload []byte) {
	if len(payload) == 0 {
		writeInt32(buf, 0)
		return
	}
	writeInt32(buf, int32(len(payload)))
	_, _ = buf.Write(payload)
}

func maxInt16(v int16) int {
	if v <= 0 {
		return 0
	}
	return int(v)
}
