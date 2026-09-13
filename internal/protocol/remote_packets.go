package protocol

// Regenerated from Mindustry build-159.7 @Remote method signatures by
// tools/gen_packets.py. The factory list is method-name ordered and is NOT
// guaranteed to equal the official wire table until gen_registry.py is re-run
// against Mindustry 160.3. TextureStream is framework id 6; the seven new
// 160.3 remotes live in remote_packets_160.go. Each Java parameter type is
// mapped to the matching protocol Read*/Write* helper.


func readLenBytes(r *Reader) ([]byte, error) {
	// TypeIO.readBytes uses a signed short length (Java write.s / read.s).
	n, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	return r.ReadBytes(int(n))
}
func writeLenBytes(w *Writer, b []byte) error {
	// TypeIO.writeBytes: write.s((short)bytes.length); write.b(bytes)
	if b == nil {
		b = []byte{}
	}
	if err := w.WriteInt16(int16(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

// readTypeIOString reads a TypeIO nullable string into a plain Go string.
func readTypeIOString(r *Reader) (string, error) {
	s, err := ReadString(r)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", nil
	}
	return *s, nil
}
func readLenInt16s(r *Reader) ([]int16, error) {
	// TypeIO int/short arrays use a short length prefix.
	n, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	out := make([]int16, n)
	for i := 0; i < int(n); i++ {
		v, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
func writeLenInt16s(w *Writer, v []int16) error {
	if v == nil {
		v = []int16{}
	}
	if err := w.WriteInt16(int16(len(v))); err != nil {
		return err
	}
	for _, x := range v {
		if err := w.WriteInt16(x); err != nil {
			return err
		}
	}
	return nil
}

type Remote_WaveSpawner_spawnEffect_0 struct {
	X float32
	Y float32
	Rotation float32
	U UnitType
}

func (p *Remote_WaveSpawner_spawnEffect_0) Read(r *Reader, _ int) error {
	v0, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Rotation = v2
	v3, err := ReadUnitType(r, r.Ctx)
	if err != nil {
		return err
	}
	p.U = v3
	return nil
}

func (p *Remote_WaveSpawner_spawnEffect_0) Write(w *Writer) error {
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Rotation); err != nil {
		return err
	}
	if err := WriteUnitType(w, p.U); err != nil {
		return err
	}
	return nil
}

func (p *Remote_WaveSpawner_spawnEffect_0) Priority() int { return PriorityNormal }

type Remote_Logic_gameOver_3 struct {
	Winner Team
}

func (p *Remote_Logic_gameOver_3) Read(r *Reader, _ int) error {
	v0, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Winner = v0
	return nil
}

func (p *Remote_Logic_gameOver_3) Write(w *Writer) error {
	if err := WriteTeam(w, &p.Winner); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Logic_gameOver_3) Priority() int { return PriorityNormal }

type Remote_Logic_researched_4 struct {
	Content Content
}

func (p *Remote_Logic_researched_4) Read(r *Reader, _ int) error {
	v0, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Content = v0
	return nil
}

func (p *Remote_Logic_researched_4) Write(w *Writer) error {
	if err := WriteContent(w, p.Content); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Logic_researched_4) Priority() int { return PriorityNormal }

type Remote_Logic_sectorCapture_1 struct {
	// no payload
}

func (p *Remote_Logic_sectorCapture_1) Read(r *Reader, _ int) error {
	// no fields
	return nil
}

func (p *Remote_Logic_sectorCapture_1) Write(w *Writer) error {
	// no fields
	return nil
}

func (p *Remote_Logic_sectorCapture_1) Priority() int { return PriorityNormal }

type Remote_Logic_updateGameOver_2 struct {
	Winner Team
}

func (p *Remote_Logic_updateGameOver_2) Read(r *Reader, _ int) error {
	v0, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Winner = v0
	return nil
}

func (p *Remote_Logic_updateGameOver_2) Write(w *Writer) error {
	if err := WriteTeam(w, &p.Winner); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Logic_updateGameOver_2) Priority() int { return PriorityNormal }

type Remote_NetClient_blockSnapshot_34 struct {
	Amount int16
	Data []byte
}

func (p *Remote_NetClient_blockSnapshot_34) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt16()
	if err != nil {
		return err
	}
	p.Amount = v0
	v1, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Data = v1
	return nil
}

func (p *Remote_NetClient_blockSnapshot_34) Write(w *Writer) error {
	if err := w.WriteInt16(p.Amount); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Data); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_blockSnapshot_34) Priority() int { return PriorityNormal }

type Remote_NetClient_clearObjectives_26 struct {
	// no payload
}

func (p *Remote_NetClient_clearObjectives_26) Read(r *Reader, _ int) error {
	// no fields
	return nil
}

func (p *Remote_NetClient_clearObjectives_26) Write(w *Writer) error {
	// no fields
	return nil
}

func (p *Remote_NetClient_clearObjectives_26) Priority() int { return PriorityNormal }

type Remote_NetClient_clientBinaryPacketReliable_5 struct {
	Type string
	Contents []byte
}

func (p *Remote_NetClient_clientBinaryPacketReliable_5) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v0
	v1, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Contents = v1
	return nil
}

func (p *Remote_NetClient_clientBinaryPacketReliable_5) Write(w *Writer) error {
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_clientBinaryPacketReliable_5) Priority() int { return PriorityNormal }

type Remote_NetClient_clientBinaryPacketUnreliable_6 struct {
	Type string
	Contents []byte
}

func (p *Remote_NetClient_clientBinaryPacketUnreliable_6) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v0
	v1, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Contents = v1
	return nil
}

func (p *Remote_NetClient_clientBinaryPacketUnreliable_6) Write(w *Writer) error {
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_clientBinaryPacketUnreliable_6) Priority() int { return PriorityNormal }

type Remote_NetClient_clientPacketReliable_7 struct {
	Type string
	Contents string
}

func (p *Remote_NetClient_clientPacketReliable_7) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Contents = v1
	return nil
}

func (p *Remote_NetClient_clientPacketReliable_7) Write(w *Writer) error {
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := WriteString(w, &p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_clientPacketReliable_7) Priority() int { return PriorityNormal }

type Remote_NetClient_clientPacketUnreliable_8 struct {
	Type string
	Contents string
}

func (p *Remote_NetClient_clientPacketUnreliable_8) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Contents = v1
	return nil
}

func (p *Remote_NetClient_clientPacketUnreliable_8) Write(w *Writer) error {
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := WriteString(w, &p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_clientPacketUnreliable_8) Priority() int { return PriorityNormal }

type Remote_NetClient_completeObjective_27 struct {
	Index int32
}

func (p *Remote_NetClient_completeObjective_27) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Index = v0
	return nil
}

func (p *Remote_NetClient_completeObjective_27) Write(w *Writer) error {
	if err := w.WriteInt32(p.Index); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_completeObjective_27) Priority() int { return PriorityNormal }

type Remote_NetClient_connect_17 struct {
	Ip string
	Port int32
}

func (p *Remote_NetClient_connect_17) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Ip = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Port = v1
	return nil
}

func (p *Remote_NetClient_connect_17) Write(w *Writer) error {
	if err := WriteString(w, &p.Ip); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Port); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_connect_17) Priority() int { return PriorityNormal }

type Remote_NetClient_effect_11 struct {
	Effect Effect
	X float32
	Y float32
	Rotation float32
	Color Color
}

func (p *Remote_NetClient_effect_11) Read(r *Reader, _ int) error {
	v0, err := ReadEffect(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Effect = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Rotation = v3
	v4, err := ReadColor(r)
	if err != nil {
		return err
	}
	p.Color = v4
	return nil
}

func (p *Remote_NetClient_effect_11) Write(w *Writer) error {
	if err := WriteEffect(w, p.Effect); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Rotation); err != nil {
		return err
	}
	if err := WriteColor(w, p.Color); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_effect_11) Priority() int { return PriorityNormal }

type Remote_NetClient_effect_12 struct {
	Effect Effect
	X float32
	Y float32
	Rotation float32
	Color Color
	Data any
}

func (p *Remote_NetClient_effect_12) Read(r *Reader, _ int) error {
	v0, err := ReadEffect(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Effect = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Rotation = v3
	v4, err := ReadColor(r)
	if err != nil {
		return err
	}
	p.Color = v4
	v5, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Data = v5
	return nil
}

func (p *Remote_NetClient_effect_12) Write(w *Writer) error {
	if err := WriteEffect(w, p.Effect); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Rotation); err != nil {
		return err
	}
	if err := WriteColor(w, p.Color); err != nil {
		return err
	}
	if err := WriteObject(w, p.Data, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_effect_12) Priority() int { return PriorityNormal }

type Remote_NetClient_effectReliable_13 struct {
	Effect Effect
	X float32
	Y float32
	Rotation float32
	Color Color
}

func (p *Remote_NetClient_effectReliable_13) Read(r *Reader, _ int) error {
	v0, err := ReadEffect(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Effect = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Rotation = v3
	v4, err := ReadColor(r)
	if err != nil {
		return err
	}
	p.Color = v4
	return nil
}

func (p *Remote_NetClient_effectReliable_13) Write(w *Writer) error {
	if err := WriteEffect(w, p.Effect); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Rotation); err != nil {
		return err
	}
	if err := WriteColor(w, p.Color); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_effectReliable_13) Priority() int { return PriorityNormal }

type Remote_NetClient_entitySnapshot_32 struct {
	Amount int16
	Data []byte
}

func (p *Remote_NetClient_entitySnapshot_32) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt16()
	if err != nil {
		return err
	}
	p.Amount = v0
	v1, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Data = v1
	return nil
}

func (p *Remote_NetClient_entitySnapshot_32) Write(w *Writer) error {
	if err := w.WriteInt16(p.Amount); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Data); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_entitySnapshot_32) Priority() int { return PriorityNormal }

type Remote_NetClient_hiddenSnapshot_33 struct {
	Ids IntSeq
}

func (p *Remote_NetClient_hiddenSnapshot_33) Read(r *Reader, _ int) error {
	v0, err := ReadIntSeq(r)
	if err != nil {
		return err
	}
	p.Ids = v0
	return nil
}

func (p *Remote_NetClient_hiddenSnapshot_33) Write(w *Writer) error {
	if err := WriteIntSeq(w, p.Ids); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_hiddenSnapshot_33) Priority() int { return PriorityNormal }

type Remote_NetClient_kick_22 struct {
	Reason string
}

func (p *Remote_NetClient_kick_22) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Reason = v0
	return nil
}

func (p *Remote_NetClient_kick_22) Write(w *Writer) error {
	if err := WriteString(w, &p.Reason); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_kick_22) Priority() int { return PriorityNormal }

type Remote_NetClient_kick_21 struct {
	Reason KickReason
}

func (p *Remote_NetClient_kick_21) Read(r *Reader, _ int) error {
	v0, err := ReadKick(r)
	if err != nil {
		return err
	}
	p.Reason = v0
	return nil
}

func (p *Remote_NetClient_kick_21) Write(w *Writer) error {
	if err := WriteKick(w, p.Reason); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_kick_21) Priority() int { return PriorityNormal }

type Remote_NetClient_ping_18 struct {
	Player Entity
	Time int64
}

func (p *Remote_NetClient_ping_18) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := r.ReadInt64()
	if err != nil {
		return err
	}
	p.Time = v1
	return nil
}

func (p *Remote_NetClient_ping_18) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteInt64(p.Time); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_ping_18) Priority() int { return PriorityNormal }

type Remote_NetClient_pingResponse_19 struct {
	Time int64
}

func (p *Remote_NetClient_pingResponse_19) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt64()
	if err != nil {
		return err
	}
	p.Time = v0
	return nil
}

func (p *Remote_NetClient_pingResponse_19) Write(w *Writer) error {
	if err := w.WriteInt64(p.Time); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_pingResponse_19) Priority() int { return PriorityNormal }

type Remote_NetClient_playerDisconnect_31 struct {
	Playerid int32
}

func (p *Remote_NetClient_playerDisconnect_31) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Playerid = v0
	return nil
}

func (p *Remote_NetClient_playerDisconnect_31) Write(w *Writer) error {
	if err := w.WriteInt32(p.Playerid); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_playerDisconnect_31) Priority() int { return PriorityNormal }

type Remote_NetClient_sendChatMessage_16 struct {
	// Player IS on the S→C wire (server broadcast).
	Player Entity
	Message string
}

func (p *Remote_NetClient_sendChatMessage_16) Read(r *Reader, _ int) error {
	// C→S omits the injected Player; wire is Message only.
	p.Player = nil
	v1, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v1 != nil {
		p.Message = *v1
	}
	return nil
}

func (p *Remote_NetClient_sendChatMessage_16) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	msg := p.Message
	return w.WriteStringNullable(&msg)
}

func (p *Remote_NetClient_sendChatMessage_16) Priority() int { return PriorityNormal }

type Remote_NetClient_sendMessage_14 struct {
	Message string
}

func (p *Remote_NetClient_sendMessage_14) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	return nil
}

func (p *Remote_NetClient_sendMessage_14) Write(w *Writer) error {
	msg := p.Message
	return w.WriteStringNullable(&msg)
}

func (p *Remote_NetClient_sendMessage_14) Priority() int { return PriorityNormal }

type Remote_NetClient_sendMessage_15 struct {
	Message string
	Unformatted string
	Playersender Entity
}

func (p *Remote_NetClient_sendMessage_15) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	v1, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v1 != nil {
		p.Unformatted = *v1
	}
	v2, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Playersender = v2
	return nil
}

func (p *Remote_NetClient_sendMessage_15) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Unformatted); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Playersender); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_sendMessage_15) Priority() int { return PriorityNormal }

type Remote_NetClient_setCameraPosition_30 struct {
	X float32
	Y float32
}

func (p *Remote_NetClient_setCameraPosition_30) Read(r *Reader, _ int) error {
	v0, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v1
	return nil
}

func (p *Remote_NetClient_setCameraPosition_30) Write(w *Writer) error {
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_setCameraPosition_30) Priority() int { return PriorityNormal }

type Remote_NetClient_setObjectives_25 struct {
	Executor MapObjectives
}

func (p *Remote_NetClient_setObjectives_25) Read(r *Reader, _ int) error {
	v0, err := ReadObjectives(r)
	if err != nil {
		return err
	}
	p.Executor = v0
	return nil
}

func (p *Remote_NetClient_setObjectives_25) Write(w *Writer) error {
	if err := WriteObjectives(w, p.Executor); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_setObjectives_25) Priority() int { return PriorityNormal }

type Remote_NetClient_setPosition_29 struct {
	X float32
	Y float32
}

func (p *Remote_NetClient_setPosition_29) Read(r *Reader, _ int) error {
	v0, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v1
	return nil
}

func (p *Remote_NetClient_setPosition_29) Write(w *Writer) error {
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_setPosition_29) Priority() int { return PriorityNormal }

type Remote_NetClient_setRule_24 struct {
	Rule string
	JsonData string
}

func (p *Remote_NetClient_setRule_24) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Rule = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.JsonData = v1
	return nil
}

func (p *Remote_NetClient_setRule_24) Write(w *Writer) error {
	if err := WriteString(w, &p.Rule); err != nil {
		return err
	}
	if err := WriteString(w, &p.JsonData); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_setRule_24) Priority() int { return PriorityNormal }

type Remote_NetClient_setRules_23 struct {
	Rules Rules
}

func (p *Remote_NetClient_setRules_23) Read(r *Reader, _ int) error {
	v0, err := ReadRules(r)
	if err != nil {
		return err
	}
	p.Rules = v0
	return nil
}

func (p *Remote_NetClient_setRules_23) Write(w *Writer) error {
	if err := WriteRules(w, p.Rules); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_setRules_23) Priority() int { return PriorityNormal }

type Remote_NetClient_sound_9 struct {
	Sound Sound
	Volume float32
	Pitch float32
	Pan float32
}

func (p *Remote_NetClient_sound_9) Read(r *Reader, _ int) error {
	v0, err := ReadSound(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Sound = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Volume = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Pitch = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Pan = v3
	return nil
}

func (p *Remote_NetClient_sound_9) Write(w *Writer) error {
	if err := WriteSound(w, p.Sound); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Volume); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Pitch); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Pan); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_sound_9) Priority() int { return PriorityNormal }

type Remote_NetClient_soundAt_10 struct {
	Sound Sound
	X float32
	Y float32
	Volume float32
	Pitch float32
}

func (p *Remote_NetClient_soundAt_10) Read(r *Reader, _ int) error {
	v0, err := ReadSound(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Sound = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Volume = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Pitch = v4
	return nil
}

func (p *Remote_NetClient_soundAt_10) Write(w *Writer) error {
	if err := WriteSound(w, p.Sound); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Volume); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Pitch); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_soundAt_10) Priority() int { return PriorityNormal }

type Remote_NetClient_stateSnapshot_35 struct {
	WaveTime float32
	Wave int32
	Enemies int32
	Paused bool
	GameOver bool
	TimeData int32
	Tps byte
	Rand0 int64
	Rand1 int64
	CoreData []byte
}

func (p *Remote_NetClient_stateSnapshot_35) Read(r *Reader, _ int) error {
	v0, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.WaveTime = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Wave = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Enemies = v2
	v3, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Paused = v3
	v4, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.GameOver = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TimeData = v5
	v6, err := r.ReadByte()
	if err != nil {
		return err
	}
	p.Tps = v6
	v7, err := r.ReadInt64()
	if err != nil {
		return err
	}
	p.Rand0 = v7
	v8, err := r.ReadInt64()
	if err != nil {
		return err
	}
	p.Rand1 = v8
	v9, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.CoreData = v9
	return nil
}

func (p *Remote_NetClient_stateSnapshot_35) Write(w *Writer) error {
	if err := w.WriteFloat32(p.WaveTime); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Wave); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Enemies); err != nil {
		return err
	}
	if err := w.WriteBool(p.Paused); err != nil {
		return err
	}
	if err := w.WriteBool(p.GameOver); err != nil {
		return err
	}
	if err := w.WriteInt32(p.TimeData); err != nil {
		return err
	}
	if err := w.WriteByte(p.Tps); err != nil {
		return err
	}
	if err := w.WriteInt64(p.Rand0); err != nil {
		return err
	}
	if err := w.WriteInt64(p.Rand1); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.CoreData); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_stateSnapshot_35) Priority() int { return PriorityNormal }

type Remote_NetClient_traceInfo_20 struct {
	Player Entity
	Info TraceInfo
}

func (p *Remote_NetClient_traceInfo_20) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := ReadTraceInfo(r)
	if err != nil {
		return err
	}
	p.Info = v1
	return nil
}

func (p *Remote_NetClient_traceInfo_20) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteTraceInfo(w, p.Info); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetClient_traceInfo_20) Priority() int { return PriorityNormal }

type Remote_NetClient_worldDataBegin_28 struct {
	// no payload
}

func (p *Remote_NetClient_worldDataBegin_28) Read(r *Reader, _ int) error {
	// no fields
	return nil
}

func (p *Remote_NetClient_worldDataBegin_28) Write(w *Writer) error {
	// no fields
	return nil
}

func (p *Remote_NetClient_worldDataBegin_28) Priority() int { return PriorityNormal }

type Remote_NetServer_adminRequest_49 struct {
	Player Entity
	Other Entity
	Action AdminAction
	Params any
}

func (p *Remote_NetServer_adminRequest_49) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Other = v1
	v2, err := ReadAction(r)
	if err != nil {
		return err
	}
	p.Action = v2
	v3, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Params = v3
	return nil
}

func (p *Remote_NetServer_adminRequest_49) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Other); err != nil {
		return err
	}
	if err := WriteAction(w, p.Action); err != nil {
		return err
	}
	if err := WriteObject(w, p.Params, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_adminRequest_49) Priority() int { return PriorityNormal }

type Remote_NetServer_clientLogicDataReliable_43 struct {
	Player Entity
	Channel string
	Value any
}

func (p *Remote_NetServer_clientLogicDataReliable_43) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Channel = v1
	v2, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Value = v2
	return nil
}

func (p *Remote_NetServer_clientLogicDataReliable_43) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Channel); err != nil {
		return err
	}
	if err := WriteObject(w, p.Value, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_clientLogicDataReliable_43) Priority() int { return PriorityNormal }

type Remote_NetServer_clientLogicDataUnreliable_44 struct {
	Player Entity
	Channel string
	Value any
}

func (p *Remote_NetServer_clientLogicDataUnreliable_44) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Channel = v1
	v2, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Value = v2
	return nil
}

func (p *Remote_NetServer_clientLogicDataUnreliable_44) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Channel); err != nil {
		return err
	}
	if err := WriteObject(w, p.Value, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_clientLogicDataUnreliable_44) Priority() int { return PriorityNormal }

type Remote_NetServer_clientPlanSnapshot_46 struct {
	Player Entity
	GroupId int32
	Plans []*BuildPlan
}

func (p *Remote_NetServer_clientPlanSnapshot_46) Read(r *Reader, _ int) error {
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.GroupId = v1
	v2, err := ReadClientPlans(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Plans = v2
	return nil
}

func (p *Remote_NetServer_clientPlanSnapshot_46) Write(w *Writer) error {
	if err := w.WriteInt32(p.GroupId); err != nil {
		return err
	}
	if err := WriteClientPlans(w, p.Plans, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_clientPlanSnapshot_46) Priority() int { return PriorityNormal }

type Remote_NetServer_clientPlanSnapshotReceived_47 struct {
	Player Entity
	GroupId int32
	Plans []*BuildPlan
}

func (p *Remote_NetServer_clientPlanSnapshotReceived_47) Read(r *Reader, _ int) error {
	// S→C includes the injected Player on the wire.
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.GroupId = v1
	v2, err := ReadClientPlans(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Plans = v2
	return nil
}

func (p *Remote_NetServer_clientPlanSnapshotReceived_47) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteInt32(p.GroupId); err != nil {
		return err
	}
	if err := WriteClientPlans(w, p.Plans, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_clientPlanSnapshotReceived_47) Priority() int { return PriorityNormal }

type Remote_NetServer_clientSnapshot_48 struct {
	// Player is injected by the server from the connection; NOT on the C→S wire.
	Player Entity
	SnapshotID int32
	UnitID int32
	Dead bool
	X float32
	Y float32
	PointerX float32
	PointerY float32
	Rotation float32
	BaseRotation float32
	XVelocity float32
	YVelocity float32
	Mining Tile
	Boosting bool
	Shooting bool
	Chatting bool
	Building bool
	SelectedBlock Content
	SelectedRotation int32
	Plans []*BuildPlan
	ViewX float32
	ViewY float32
	ViewWidth float32
	ViewHeight float32
}

func (p *Remote_NetServer_clientSnapshot_48) Read(r *Reader, _ int) error {
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.SnapshotID = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.UnitID = v2
	v3, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Dead = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v4
	v5, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v5
	v6, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.PointerX = v6
	v7, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.PointerY = v7
	v8, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Rotation = v8
	v9, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.BaseRotation = v9
	v10, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.XVelocity = v10
	v11, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.YVelocity = v11
	v12, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Mining = v12
	v13, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Boosting = v13
	v14, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Shooting = v14
	v15, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Chatting = v15
	v16, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Building = v16
	// Java TypeIO.writeBlock: signed short id (-1 = null), not writeContent.
	v17, err := ReadBlock(r, r.Ctx)
	if err != nil {
		return err
	}
	p.SelectedBlock = v17
	v18, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.SelectedRotation = v18
	v19, err := ReadPlansQueue(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Plans = v19
	v20, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.ViewX = v20
	v21, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.ViewY = v21
	v22, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.ViewWidth = v22
	v23, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.ViewHeight = v23
	return nil
}

func (p *Remote_NetServer_clientSnapshot_48) Write(w *Writer) error {
	if err := w.WriteInt32(p.SnapshotID); err != nil {
		return err
	}
	if err := w.WriteInt32(p.UnitID); err != nil {
		return err
	}
	if err := w.WriteBool(p.Dead); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.PointerX); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.PointerY); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Rotation); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.BaseRotation); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.XVelocity); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.YVelocity); err != nil {
		return err
	}
	if err := WriteTile(w, p.Mining); err != nil {
		return err
	}
	if err := w.WriteBool(p.Boosting); err != nil {
		return err
	}
	if err := w.WriteBool(p.Shooting); err != nil {
		return err
	}
	if err := w.WriteBool(p.Chatting); err != nil {
		return err
	}
	if err := w.WriteBool(p.Building); err != nil {
		return err
	}
	if err := WriteBlock(w, p.SelectedBlock); err != nil {
		return err
	}
	if err := w.WriteInt32(p.SelectedRotation); err != nil {
		return err
	}
	if err := WritePlansQueueNet(w, p.Plans, w.Ctx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.ViewX); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.ViewY); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.ViewWidth); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.ViewHeight); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_clientSnapshot_48) Priority() int { return PriorityNormal }

type Remote_NetServer_connectConfirm_50 struct {
	// Player is injected by the server from the connection; it is NOT on the wire.
	Player Entity
}

func (p *Remote_NetServer_connectConfirm_50) Read(r *Reader, _ int) error {
	return nil
}

func (p *Remote_NetServer_connectConfirm_50) Write(w *Writer) error {
	return nil
}

func (p *Remote_NetServer_connectConfirm_50) Priority() int { return PriorityNormal }

type Remote_NetServer_debugStatusClient_37 struct {
	Value int32
	LastClientSnapshot int32
}

func (p *Remote_NetServer_debugStatusClient_37) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Value = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.LastClientSnapshot = v1
	return nil
}

func (p *Remote_NetServer_debugStatusClient_37) Write(w *Writer) error {
	if err := w.WriteInt32(p.Value); err != nil {
		return err
	}
	if err := w.WriteInt32(p.LastClientSnapshot); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_debugStatusClient_37) Priority() int { return PriorityNormal }

type Remote_NetServer_debugStatusClientUnreliable_38 struct {
	Value int32
	LastClientSnapshot int32
}

func (p *Remote_NetServer_debugStatusClientUnreliable_38) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Value = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.LastClientSnapshot = v1
	return nil
}

func (p *Remote_NetServer_debugStatusClientUnreliable_38) Write(w *Writer) error {
	if err := w.WriteInt32(p.Value); err != nil {
		return err
	}
	if err := w.WriteInt32(p.LastClientSnapshot); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_debugStatusClientUnreliable_38) Priority() int { return PriorityNormal }

type Remote_NetServer_requestBlockSnapshot_45 struct {
	Player Entity
	Pos int32
}

func (p *Remote_NetServer_requestBlockSnapshot_45) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Pos = v1
	return nil
}

func (p *Remote_NetServer_requestBlockSnapshot_45) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Pos); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_requestBlockSnapshot_45) Priority() int { return PriorityNormal }

type Remote_NetServer_requestDebugStatus_36 struct {
	Player Entity
}

func (p *Remote_NetServer_requestDebugStatus_36) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	return nil
}

func (p *Remote_NetServer_requestDebugStatus_36) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_requestDebugStatus_36) Priority() int { return PriorityNormal }

type Remote_NetServer_serverBinaryPacketReliable_41 struct {
	Player Entity
	Type string
	Contents []byte
}

func (p *Remote_NetServer_serverBinaryPacketReliable_41) Read(r *Reader, _ int) error {
	// S→C includes the injected Player.
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v1
	v2, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Contents = v2
	return nil
}

func (p *Remote_NetServer_serverBinaryPacketReliable_41) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_serverBinaryPacketReliable_41) Priority() int { return PriorityNormal }

type Remote_NetServer_serverBinaryPacketUnreliable_42 struct {
	Player Entity
	Type string
	Contents []byte
}

func (p *Remote_NetServer_serverBinaryPacketUnreliable_42) Read(r *Reader, _ int) error {
	// S→C includes the injected Player.
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v1
	v2, err := readLenBytes(r)
	if err != nil {
		return err
	}
	p.Contents = v2
	return nil
}

func (p *Remote_NetServer_serverBinaryPacketUnreliable_42) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := writeLenBytes(w, p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_serverBinaryPacketUnreliable_42) Priority() int { return PriorityNormal }

type Remote_NetServer_serverPacketReliable_39 struct {
	Player Entity
	Type string
	Contents string
}

func (p *Remote_NetServer_serverPacketReliable_39) Read(r *Reader, _ int) error {
	// S→C includes the injected Player.
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v1
	v2, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Contents = v2
	return nil
}

func (p *Remote_NetServer_serverPacketReliable_39) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := WriteString(w, &p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_serverPacketReliable_39) Priority() int { return PriorityNormal }

type Remote_NetServer_serverPacketUnreliable_40 struct {
	Player Entity
	Type string
	Contents string
}

func (p *Remote_NetServer_serverPacketUnreliable_40) Read(r *Reader, _ int) error {
	// S→C includes the injected Player.
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Type = v1
	v2, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Contents = v2
	return nil
}

func (p *Remote_NetServer_serverPacketUnreliable_40) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteString(w, &p.Type); err != nil {
		return err
	}
	if err := WriteString(w, &p.Contents); err != nil {
		return err
	}
	return nil
}

func (p *Remote_NetServer_serverPacketUnreliable_40) Priority() int { return PriorityNormal }

type Remote_Units_unitCapDeath_52 struct {
	Unit Entity
}

func (p *Remote_Units_unitCapDeath_52) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	return nil
}

func (p *Remote_Units_unitCapDeath_52) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitCapDeath_52) Priority() int { return PriorityNormal }

type Remote_Units_unitDeath_54 struct {
	Uid int32
}

func (p *Remote_Units_unitDeath_54) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Uid = v0
	return nil
}

func (p *Remote_Units_unitDeath_54) Write(w *Writer) error {
	if err := w.WriteInt32(p.Uid); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitDeath_54) Priority() int { return PriorityNormal }

type Remote_Units_unitDespawn_56 struct {
	Unit Entity
}

func (p *Remote_Units_unitDespawn_56) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	return nil
}

func (p *Remote_Units_unitDespawn_56) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitDespawn_56) Priority() int { return PriorityNormal }

type Remote_Units_unitDestroy_55 struct {
	Uid int32
}

func (p *Remote_Units_unitDestroy_55) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Uid = v0
	return nil
}

func (p *Remote_Units_unitDestroy_55) Write(w *Writer) error {
	if err := w.WriteInt32(p.Uid); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitDestroy_55) Priority() int { return PriorityNormal }

type Remote_Units_unitEnvDeath_53 struct {
	Unit Entity
}

func (p *Remote_Units_unitEnvDeath_53) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	return nil
}

func (p *Remote_Units_unitEnvDeath_53) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitEnvDeath_53) Priority() int { return PriorityNormal }

type Remote_Units_unitSafeDeath_57 struct {
	Unit Entity
}

func (p *Remote_Units_unitSafeDeath_57) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	return nil
}

func (p *Remote_Units_unitSafeDeath_57) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitSafeDeath_57) Priority() int { return PriorityNormal }

type Remote_Units_unitSpawn_51 struct {
	Container Entity
}

func (p *Remote_Units_unitSpawn_51) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Container = v0
	return nil
}

func (p *Remote_Units_unitSpawn_51) Write(w *Writer) error {
	if err := WriteEntity(w, p.Container); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Units_unitSpawn_51) Priority() int { return PriorityNormal }

type Remote_BulletType_createBullet_58 struct {
	Type BulletType
	Team Team
	X float32
	Y float32
	Angle float32
	Damage float32
	VelocityScl float32
	LifetimeScl float32
}

func (p *Remote_BulletType_createBullet_58) Read(r *Reader, _ int) error {
	v0, err := ReadBulletType(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Type = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Angle = v4
	v5, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Damage = v5
	v6, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.VelocityScl = v6
	v7, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.LifetimeScl = v7
	return nil
}

func (p *Remote_BulletType_createBullet_58) Write(w *Writer) error {
	if err := WriteBulletType(w, p.Type); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Angle); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Damage); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.VelocityScl); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.LifetimeScl); err != nil {
		return err
	}
	return nil
}

func (p *Remote_BulletType_createBullet_58) Priority() int { return PriorityNormal }

type Remote_Teams_destroyPayload_59 struct {
	Build Entity
}

func (p *Remote_Teams_destroyPayload_59) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	return nil
}

func (p *Remote_Teams_destroyPayload_59) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Teams_destroyPayload_59) Priority() int { return PriorityNormal }

type Remote_InputHandler_buildingControlSelect_92 struct {
	Player Entity
	Build Entity
}

func (p *Remote_InputHandler_buildingControlSelect_92) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	return nil
}

func (p *Remote_InputHandler_buildingControlSelect_92) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return WriteEntity(w, p.Build)
}

func (p *Remote_InputHandler_buildingControlSelect_92) Priority() int { return PriorityNormal }

type Remote_InputHandler_clearItems_66 struct {
	Build Entity
}

func (p *Remote_InputHandler_clearItems_66) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	return nil
}

func (p *Remote_InputHandler_clearItems_66) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_clearItems_66) Priority() int { return PriorityNormal }

type Remote_InputHandler_clearLiquids_70 struct {
	Build Entity
}

func (p *Remote_InputHandler_clearLiquids_70) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	return nil
}

func (p *Remote_InputHandler_clearLiquids_70) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_clearLiquids_70) Priority() int { return PriorityNormal }

type Remote_InputHandler_commandBuilding_77 struct {
	Player Entity
	Buildings []int32
	Target *Vec2
}

func (p *Remote_InputHandler_commandBuilding_77) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Buildings = v1
	v2, err := ReadVecNullable(r)
	if err != nil {
		return err
	}
	p.Target = v2
	return nil
}

func (p *Remote_InputHandler_commandBuilding_77) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteInts(w, p.Buildings); err != nil {
		return err
	}
	if err := WriteVecNullable(w, p.Target); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_commandBuilding_77) Priority() int { return PriorityNormal }

type Remote_InputHandler_commandUnits_74 struct {
	Player Entity
	UnitIds []int32
	BuildTarget Entity
	UnitTarget Entity
	PosTarget *Vec2
	QueueCommand bool
	FinalBatch bool
}

func (p *Remote_InputHandler_commandUnits_74) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.UnitIds = v1
	v2, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.BuildTarget = v2
	v3, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.UnitTarget = v3
	v4, err := ReadVecNullable(r)
	if err != nil {
		return err
	}
	p.PosTarget = v4
	v5, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.QueueCommand = v5
	v6, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.FinalBatch = v6
	return nil
}

func (p *Remote_InputHandler_commandUnits_74) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteInts(w, p.UnitIds); err != nil {
		return err
	}
	if err := WriteEntity(w, p.BuildTarget); err != nil {
		return err
	}
	if err := WriteEntity(w, p.UnitTarget); err != nil {
		return err
	}
	if err := WriteVecNullable(w, p.PosTarget); err != nil {
		return err
	}
	if err := w.WriteBool(p.QueueCommand); err != nil {
		return err
	}
	if err := w.WriteBool(p.FinalBatch); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_commandUnits_74) Priority() int { return PriorityNormal }

type Remote_InputHandler_deletePlans_72 struct {
	Player Entity
	Positions []int32
}

func (p *Remote_InputHandler_deletePlans_72) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v1
	return nil
}

func (p *Remote_InputHandler_deletePlans_72) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_deletePlans_72) Priority() int { return PriorityNormal }

type Remote_InputHandler_dropItem_88 struct {
	Player Entity
	Angle float32
}

func (p *Remote_InputHandler_dropItem_88) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Angle = v1
	return nil
}

func (p *Remote_InputHandler_dropItem_88) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Angle); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_dropItem_88) Priority() int { return PriorityNormal }

type Remote_InputHandler_payloadDropped_86 struct {
	Unit Entity
	X float32
	Y float32
}

func (p *Remote_InputHandler_payloadDropped_86) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	return nil
}

func (p *Remote_InputHandler_payloadDropped_86) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_payloadDropped_86) Priority() int { return PriorityNormal }

type Remote_InputHandler_pickedBuildPayload_84 struct {
	Unit Entity
	Build Entity
	OnGround bool
}

func (p *Remote_InputHandler_pickedBuildPayload_84) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	v2, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.OnGround = v2
	return nil
}

func (p *Remote_InputHandler_pickedBuildPayload_84) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := w.WriteBool(p.OnGround); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_pickedBuildPayload_84) Priority() int { return PriorityNormal }

type Remote_InputHandler_pickedUnitPayload_83 struct {
	Unit Entity
	Target Entity
}

func (p *Remote_InputHandler_pickedUnitPayload_83) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Target = v1
	return nil
}

func (p *Remote_InputHandler_pickedUnitPayload_83) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Target); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_pickedUnitPayload_83) Priority() int { return PriorityNormal }

type Remote_InputHandler_pingLocation_73 struct {
	Player Entity
	X float32
	Y float32
	Text string
}

func (p *Remote_InputHandler_pingLocation_73) Read(r *Reader, _ int) error {
	// S→C includes the player; C→S omits it (serializer special-cases inbound).
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v3 != nil {
		p.Text = *v3
	}
	return nil
}

func (p *Remote_InputHandler_pingLocation_73) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Text); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_pingLocation_73) Priority() int { return PriorityNormal }

type Remote_InputHandler_removeQueueBlock_80 struct {
	X int32
	Y int32
	Breaking bool
}

func (p *Remote_InputHandler_removeQueueBlock_80) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.X = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Y = v1
	v2, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Breaking = v2
	return nil
}

func (p *Remote_InputHandler_removeQueueBlock_80) Write(w *Writer) error {
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y); err != nil {
		return err
	}
	if err := w.WriteBool(p.Breaking); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_removeQueueBlock_80) Priority() int { return PriorityNormal }

type Remote_InputHandler_requestBuildPayload_82 struct {
	Player Entity
	Build Entity
}

func (p *Remote_InputHandler_requestBuildPayload_82) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	return nil
}

func (p *Remote_InputHandler_requestBuildPayload_82) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_requestBuildPayload_82) Priority() int { return PriorityNormal }

type Remote_InputHandler_requestDropPayload_85 struct {
	Player Entity
	X float32
	Y float32
}

func (p *Remote_InputHandler_requestDropPayload_85) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	return nil
}

func (p *Remote_InputHandler_requestDropPayload_85) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_requestDropPayload_85) Priority() int { return PriorityNormal }

type Remote_InputHandler_requestItem_78 struct {
	// Player is injected on the server from the connection; client→server
	// wire payload is Building, Item, amount only.
	Player Entity
	Build  Entity
	Item   Item
	Amount int32
}

func (p *Remote_InputHandler_requestItem_78) Read(r *Reader, _ int) error {
	// Official C→S omits the injected Player parameter.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	v2, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Amount = v3
	return nil
}

func (p *Remote_InputHandler_requestItem_78) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Amount); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_requestItem_78) Priority() int { return PriorityNormal }

type Remote_InputHandler_requestUnitPayload_81 struct {
	Player Entity
	Target Entity
}

func (p *Remote_InputHandler_requestUnitPayload_81) Read(r *Reader, _ int) error {
	v1, err := ReadUnit(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Target = v1
	return nil
}

func (p *Remote_InputHandler_requestUnitPayload_81) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return WriteUnit(w, p.Target)
}

func (p *Remote_InputHandler_requestUnitPayload_81) Priority() int { return PriorityNormal }

type Remote_InputHandler_rotateBlock_89 struct {
	Player Entity
	Build Entity
	Direction bool
}

func (p *Remote_InputHandler_rotateBlock_89) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	v2, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Direction = v2
	return nil
}

func (p *Remote_InputHandler_rotateBlock_89) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := w.WriteBool(p.Direction); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_rotateBlock_89) Priority() int { return PriorityNormal }

type Remote_InputHandler_setItem_63 struct {
	Build Entity
	Item Item
	Amount int32
}

func (p *Remote_InputHandler_setItem_63) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Amount = v2
	return nil
}

func (p *Remote_InputHandler_setItem_63) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Amount); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setItem_63) Priority() int { return PriorityNormal }

type Remote_InputHandler_setItems_64 struct {
	Build Entity
	Items []ItemStack
}

func (p *Remote_InputHandler_setItems_64) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadItemStacks(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Items = v1
	return nil
}

func (p *Remote_InputHandler_setItems_64) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteItemStacks(w, p.Items); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setItems_64) Priority() int { return PriorityNormal }

type Remote_InputHandler_setLiquid_67 struct {
	Build Entity
	Liquid Liquid
	Amount float32
}

func (p *Remote_InputHandler_setLiquid_67) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadLiquid(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Liquid = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Amount = v2
	return nil
}

func (p *Remote_InputHandler_setLiquid_67) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteLiquid(w, p.Liquid); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Amount); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setLiquid_67) Priority() int { return PriorityNormal }

type Remote_InputHandler_setLiquids_68 struct {
	Build Entity
	Liquids []LiquidStack
}

func (p *Remote_InputHandler_setLiquids_68) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadLiquidStacks(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Liquids = v1
	return nil
}

func (p *Remote_InputHandler_setLiquids_68) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteLiquidStacks(w, p.Liquids); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setLiquids_68) Priority() int { return PriorityNormal }

type Remote_InputHandler_setTileItems_65 struct {
	Item Item
	Amount int32
	Positions []int32
}

func (p *Remote_InputHandler_setTileItems_65) Read(r *Reader, _ int) error {
	v0, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Amount = v1
	v2, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v2
	return nil
}

func (p *Remote_InputHandler_setTileItems_65) Write(w *Writer) error {
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Amount); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setTileItems_65) Priority() int { return PriorityNormal }

type Remote_InputHandler_setTileLiquids_69 struct {
	Liquid Liquid
	Amount float32
	Positions []int32
}

func (p *Remote_InputHandler_setTileLiquids_69) Read(r *Reader, _ int) error {
	v0, err := ReadLiquid(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Liquid = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Amount = v1
	v2, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v2
	return nil
}

func (p *Remote_InputHandler_setTileLiquids_69) Write(w *Writer) error {
	if err := WriteLiquid(w, p.Liquid); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Amount); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setTileLiquids_69) Priority() int { return PriorityNormal }

type Remote_InputHandler_setUnitCommand_75 struct {
	Player Entity
	UnitIds []int32
	Command *UnitCommand
}

func (p *Remote_InputHandler_setUnitCommand_75) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.UnitIds = v1
	v2, err := ReadCommand(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Command = v2
	return nil
}

func (p *Remote_InputHandler_setUnitCommand_75) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteInts(w, p.UnitIds); err != nil {
		return err
	}
	if err := WriteCommand(w, p.Command); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setUnitCommand_75) Priority() int { return PriorityNormal }

type Remote_InputHandler_setUnitStance_76 struct {
	Player Entity
	UnitIds []int32
	Stance UnitStance
	Enable bool
}

func (p *Remote_InputHandler_setUnitStance_76) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.UnitIds = v1
	v2, err := ReadStance(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Stance = v2
	v3, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Enable = v3
	return nil
}

func (p *Remote_InputHandler_setUnitStance_76) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteInts(w, p.UnitIds); err != nil {
		return err
	}
	if err := WriteStance(w, &p.Stance); err != nil {
		return err
	}
	if err := w.WriteBool(p.Enable); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_setUnitStance_76) Priority() int { return PriorityNormal }

type Remote_InputHandler_takeItems_61 struct {
	Build Entity
	Item Item
	Amount int32
	To Entity
}

func (p *Remote_InputHandler_takeItems_61) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Amount = v2
	v3, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.To = v3
	return nil
}

func (p *Remote_InputHandler_takeItems_61) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Amount); err != nil {
		return err
	}
	if err := WriteEntity(w, p.To); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_takeItems_61) Priority() int { return PriorityNormal }

type Remote_InputHandler_tileConfig_90 struct {
	Player Entity
	Build Entity
	Value any
}

func (p *Remote_InputHandler_tileConfig_90) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	v2, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Value = v2
	return nil
}

func (p *Remote_InputHandler_tileConfig_90) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteObject(w, p.Value, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_tileConfig_90) Priority() int { return PriorityNormal }

type Remote_InputHandler_tileTap_91 struct {
	Player Entity
	Tile Tile
}

func (p *Remote_InputHandler_tileTap_91) Read(r *Reader, _ int) error {
	// C→S omits the injected Player; wire is Tile only (packed pos int32).
	p.Player = nil
	v1, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v1
	return nil
}

func (p *Remote_InputHandler_tileTap_91) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_tileTap_91) Priority() int { return PriorityNormal }

type Remote_InputHandler_transferInventory_79 struct {
	Player Entity
	Build Entity
}

func (p *Remote_InputHandler_transferInventory_79) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	return nil
}

func (p *Remote_InputHandler_transferInventory_79) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_transferInventory_79) Priority() int { return PriorityNormal }

type Remote_InputHandler_transferItemEffect_60 struct {
	Item Item
	X float32
	Y float32
	To any
}

func (p *Remote_InputHandler_transferItemEffect_60) Read(r *Reader, _ int) error {
	v0, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.To = v3
	return nil
}

func (p *Remote_InputHandler_transferItemEffect_60) Write(w *Writer) error {
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := WriteObject(w, p.To, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_transferItemEffect_60) Priority() int { return PriorityNormal }

type Remote_InputHandler_transferItemTo_71 struct {
	Unit Entity
	Item Item
	Amount int32
	X float32
	Y float32
	Build Entity
}

func (p *Remote_InputHandler_transferItemTo_71) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Amount = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v4
	v5, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v5
	return nil
}

func (p *Remote_InputHandler_transferItemTo_71) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Amount); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_transferItemTo_71) Priority() int { return PriorityNormal }

type Remote_InputHandler_transferItemToUnit_62 struct {
	Item Item
	X float32
	Y float32
	To any
}

func (p *Remote_InputHandler_transferItemToUnit_62) Read(r *Reader, _ int) error {
	v0, err := ReadItem(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Item = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.To = v3
	return nil
}

func (p *Remote_InputHandler_transferItemToUnit_62) Write(w *Writer) error {
	if err := WriteItem(w, p.Item); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := WriteObject(w, p.To, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_transferItemToUnit_62) Priority() int { return PriorityNormal }

type Remote_InputHandler_unitBuildingControlSelect_93 struct {
	Unit Entity
	Build Entity
}

func (p *Remote_InputHandler_unitBuildingControlSelect_93) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	return nil
}

func (p *Remote_InputHandler_unitBuildingControlSelect_93) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_unitBuildingControlSelect_93) Priority() int { return PriorityNormal }

type Remote_InputHandler_unitClear_95 struct {
	Player Entity
}

func (p *Remote_InputHandler_unitClear_95) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	return nil
}

func (p *Remote_InputHandler_unitClear_95) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_unitClear_95) Priority() int { return PriorityNormal }

type Remote_InputHandler_unitControl_94 struct {
	// Player is injected by the server from the connection; NOT on the C→S wire.
	Player Entity
	Unit Entity
}

func (p *Remote_InputHandler_unitControl_94) Read(r *Reader, _ int) error {
	v1, err := ReadUnit(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v1
	return nil
}

func (p *Remote_InputHandler_unitControl_94) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return WriteUnit(w, p.Unit)
}

func (p *Remote_InputHandler_unitControl_94) Priority() int { return PriorityNormal }

type Remote_InputHandler_unitEnteredPayload_87 struct {
	Unit Entity
	Build Entity
}

func (p *Remote_InputHandler_unitEnteredPayload_87) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v1
	return nil
}

func (p *Remote_InputHandler_unitEnteredPayload_87) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_InputHandler_unitEnteredPayload_87) Priority() int { return PriorityNormal }

type Remote_LExecutor_createMarker_100 struct {
	Id int32
	Marker ObjectiveMarker
}

func (p *Remote_LExecutor_createMarker_100) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	v1, err := ReadObjectiveMarker(r)
	if err != nil {
		return err
	}
	p.Marker = v1
	return nil
}

func (p *Remote_LExecutor_createMarker_100) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := WriteObjectiveMarker(w, p.Marker); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_createMarker_100) Priority() int { return PriorityNormal }

type Remote_LExecutor_logicExplosion_97 struct {
	Team Team
	X float32
	Y float32
	Radius float32
	Damage float32
	Air bool
	Ground bool
	Pierce bool
	Effect bool
}

func (p *Remote_LExecutor_logicExplosion_97) Read(r *Reader, _ int) error {
	v0, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.X = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Y = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Radius = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Damage = v4
	v5, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Air = v5
	v6, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Ground = v6
	v7, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Pierce = v7
	v8, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Effect = v8
	return nil
}

func (p *Remote_LExecutor_logicExplosion_97) Write(w *Writer) error {
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Y); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Radius); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Damage); err != nil {
		return err
	}
	if err := w.WriteBool(p.Air); err != nil {
		return err
	}
	if err := w.WriteBool(p.Ground); err != nil {
		return err
	}
	if err := w.WriteBool(p.Pierce); err != nil {
		return err
	}
	if err := w.WriteBool(p.Effect); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_logicExplosion_97) Priority() int { return PriorityNormal }

type Remote_LExecutor_removeMarker_101 struct {
	Id int32
}

func (p *Remote_LExecutor_removeMarker_101) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	return nil
}

func (p *Remote_LExecutor_removeMarker_101) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_removeMarker_101) Priority() int { return PriorityNormal }

type Remote_LExecutor_setFlag_99 struct {
	Flag string
	Add bool
}

func (p *Remote_LExecutor_setFlag_99) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Flag = v0
	v1, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Add = v1
	return nil
}

func (p *Remote_LExecutor_setFlag_99) Write(w *Writer) error {
	if err := WriteString(w, &p.Flag); err != nil {
		return err
	}
	if err := w.WriteBool(p.Add); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_setFlag_99) Priority() int { return PriorityNormal }

type Remote_LExecutor_setMapArea_96 struct {
	X int32
	Y int32
	W int32
	H int32
}

func (p *Remote_LExecutor_setMapArea_96) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.X = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Y = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.W = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.H = v3
	return nil
}

func (p *Remote_LExecutor_setMapArea_96) Write(w *Writer) error {
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y); err != nil {
		return err
	}
	if err := w.WriteInt32(p.W); err != nil {
		return err
	}
	if err := w.WriteInt32(p.H); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_setMapArea_96) Priority() int { return PriorityNormal }

type Remote_LExecutor_syncVariable_98 struct {
	Building Entity
	Variable int32
	Value any
}

func (p *Remote_LExecutor_syncVariable_98) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Building = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Variable = v1
	v2, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Value = v2
	return nil
}

func (p *Remote_LExecutor_syncVariable_98) Write(w *Writer) error {
	if err := WriteEntity(w, p.Building); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Variable); err != nil {
		return err
	}
	if err := WriteObject(w, p.Value, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_syncVariable_98) Priority() int { return PriorityNormal }

type Remote_LExecutor_updateMarker_102 struct {
	Id int32
	Control LMarkerControl
	P1 float64
	P2 float64
	P3 float64
}

func (p *Remote_LExecutor_updateMarker_102) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	v1, err := ReadMarkerControl(r)
	if err != nil {
		return err
	}
	p.Control = v1
	v2, err := r.ReadFloat64()
	if err != nil {
		return err
	}
	p.P1 = v2
	v3, err := r.ReadFloat64()
	if err != nil {
		return err
	}
	p.P2 = v3
	v4, err := r.ReadFloat64()
	if err != nil {
		return err
	}
	p.P3 = v4
	return nil
}

func (p *Remote_LExecutor_updateMarker_102) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := WriteMarkerControl(w, p.Control); err != nil {
		return err
	}
	if err := w.WriteFloat64(p.P1); err != nil {
		return err
	}
	if err := w.WriteFloat64(p.P2); err != nil {
		return err
	}
	if err := w.WriteFloat64(p.P3); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_updateMarker_102) Priority() int { return PriorityNormal }

type Remote_LExecutor_updateMarkerText_103 struct {
	Id int32
	Type LMarkerControl
	Fetch bool
	Text string
}

func (p *Remote_LExecutor_updateMarkerText_103) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	v1, err := ReadMarkerControl(r)
	if err != nil {
		return err
	}
	p.Type = v1
	v2, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Fetch = v2
	v3, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Text = v3
	return nil
}

func (p *Remote_LExecutor_updateMarkerText_103) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := WriteMarkerControl(w, p.Type); err != nil {
		return err
	}
	if err := w.WriteBool(p.Fetch); err != nil {
		return err
	}
	if err := WriteString(w, &p.Text); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_updateMarkerText_103) Priority() int { return PriorityNormal }

type Remote_LExecutor_updateMarkerTexture_104 struct {
	Id int32
	Texture any
}

func (p *Remote_LExecutor_updateMarkerTexture_104) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	v1, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Texture = v1
	return nil
}

func (p *Remote_LExecutor_updateMarkerTexture_104) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := WriteObject(w, p.Texture, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LExecutor_updateMarkerTexture_104) Priority() int { return PriorityNormal }

type Remote_Weather_createWeather_105 struct {
	Weather Weather
	Intensity float32
	Duration float32
	WindX float32
	WindY float32
}

func (p *Remote_Weather_createWeather_105) Read(r *Reader, _ int) error {
	v0, err := ReadWeather(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Weather = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Intensity = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.WindX = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.WindY = v4
	return nil
}

func (p *Remote_Weather_createWeather_105) Write(w *Writer) error {
	if err := WriteWeather(w, p.Weather); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Intensity); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.WindX); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.WindY); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Weather_createWeather_105) Priority() int { return PriorityNormal }

type Remote_Menus_announce_116 struct {
	Message string
}

func (p *Remote_Menus_announce_116) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	return nil
}

func (p *Remote_Menus_announce_116) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_announce_116) Priority() int { return PriorityNormal }

type Remote_Menus_copyToClipboard_129 struct {
	Text string
}

func (p *Remote_Menus_copyToClipboard_129) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Text = v0
	return nil
}

func (p *Remote_Menus_copyToClipboard_129) Write(w *Writer) error {
	if err := WriteString(w, &p.Text); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_copyToClipboard_129) Priority() int { return PriorityNormal }

type Remote_Menus_followUpMenu_107 struct {
	MenuId int32
	Title string
	Message string
	Options [][]string
}

func (p *Remote_Menus_followUpMenu_107) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.MenuId = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Title = v1
	v2, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v2
	v3, err := ReadStringArray(r)
	if err != nil {
		return err
	}
	p.Options = v3
	return nil
}

func (p *Remote_Menus_followUpMenu_107) Write(w *Writer) error {
	if err := w.WriteInt32(p.MenuId); err != nil {
		return err
	}
	if err := WriteString(w, &p.Title); err != nil {
		return err
	}
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	if err := WriteStringArray(w, p.Options); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_followUpMenu_107) Priority() int { return PriorityNormal }

type Remote_Menus_hideFollowUpMenu_108 struct {
	MenuId int32
}

func (p *Remote_Menus_hideFollowUpMenu_108) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.MenuId = v0
	return nil
}

func (p *Remote_Menus_hideFollowUpMenu_108) Write(w *Writer) error {
	if err := w.WriteInt32(p.MenuId); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_hideFollowUpMenu_108) Priority() int { return PriorityNormal }

type Remote_Menus_hideHudText_114 struct {
	// no payload
}

func (p *Remote_Menus_hideHudText_114) Read(r *Reader, _ int) error {
	// no fields
	return nil
}

func (p *Remote_Menus_hideHudText_114) Write(w *Writer) error {
	// no fields
	return nil
}

func (p *Remote_Menus_hideHudText_114) Priority() int { return PriorityNormal }

type Remote_Menus_infoMessage_117 struct {
	Message string
}

func (p *Remote_Menus_infoMessage_117) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	return nil
}

func (p *Remote_Menus_infoMessage_117) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoMessage_117) Priority() int { return PriorityNormal }

type Remote_Menus_infoPopup_118 struct {
	Message string
	Duration float32
	Align int32
	Top int32
	Left int32
	Bottom int32
	Right int32
}

func (p *Remote_Menus_infoPopup_118) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Align = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Top = v3
	v4, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Left = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Bottom = v5
	v6, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Right = v6
	return nil
}

func (p *Remote_Menus_infoPopup_118) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Align); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Top); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Left); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Bottom); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Right); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoPopup_118) Priority() int { return PriorityNormal }

type Remote_Menus_infoPopup_120 struct {
	Message string
	Id string
	Duration float32
	Align int32
	Top int32
	Left int32
	Bottom int32
	Right int32
}

func (p *Remote_Menus_infoPopup_120) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v1 != nil {
		p.Id = *v1
	}
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Align = v3
	v4, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Top = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Left = v5
	v6, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Bottom = v6
	v7, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Right = v7
	return nil
}

func (p *Remote_Menus_infoPopup_120) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Align); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Top); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Left); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Bottom); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Right); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoPopup_120) Priority() int { return PriorityNormal }

type Remote_Menus_infoPopupReliable_119 struct {
	Message string
	Duration float32
	Align int32
	Top int32
	Left int32
	Bottom int32
	Right int32
}

func (p *Remote_Menus_infoPopupReliable_119) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Align = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Top = v3
	v4, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Left = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Bottom = v5
	v6, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Right = v6
	return nil
}

func (p *Remote_Menus_infoPopupReliable_119) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Align); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Top); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Left); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Bottom); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Right); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoPopupReliable_119) Priority() int { return PriorityNormal }

type Remote_Menus_infoPopupReliable_121 struct {
	Message string
	Id string
	Duration float32
	Align int32
	Top int32
	Left int32
	Bottom int32
	Right int32
}

func (p *Remote_Menus_infoPopupReliable_121) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v1 != nil {
		p.Id = *v1
	}
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Align = v3
	v4, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Top = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Left = v5
	v6, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Bottom = v6
	v7, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Right = v7
	return nil
}

func (p *Remote_Menus_infoPopupReliable_121) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Align); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Top); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Left); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Bottom); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Right); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoPopupReliable_121) Priority() int { return PriorityNormal }

type Remote_Menus_infoToast_126 struct {
	Message string
	Duration float32
}

func (p *Remote_Menus_infoToast_126) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	v1, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v1
	return nil
}

func (p *Remote_Menus_infoToast_126) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_infoToast_126) Priority() int { return PriorityNormal }

type Remote_Menus_label_124 struct {
	Message string
	Id int32
	Duration float32
	Worldx float32
	Worldy float32
}

func (p *Remote_Menus_label_124) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldx = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldy = v4
	return nil
}

func (p *Remote_Menus_label_124) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_label_124) Priority() int { return PriorityNormal }

type Remote_Menus_label_122 struct {
	Message string
	Id int32
	Duration float32
	Worldx float32
	Worldy float32
	Flags int32
}

func (p *Remote_Menus_label_122) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldx = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldy = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Flags = v5
	return nil
}

func (p *Remote_Menus_label_122) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Flags); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_label_122) Priority() int { return PriorityNormal }

type Remote_Menus_labelReliable_125 struct {
	Message string
	Id int32
	Duration float32
	Worldx float32
	Worldy float32
}

func (p *Remote_Menus_labelReliable_125) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldx = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldy = v4
	return nil
}

func (p *Remote_Menus_labelReliable_125) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_labelReliable_125) Priority() int { return PriorityNormal }

type Remote_Menus_labelReliable_123 struct {
	Message string
	Id int32
	Duration float32
	Worldx float32
	Worldy float32
	Flags int32
}

func (p *Remote_Menus_labelReliable_123) Read(r *Reader, _ int) error {
	v0, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v0 != nil {
		p.Message = *v0
	}
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	v2, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Duration = v2
	v3, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldx = v3
	v4, err := r.ReadFloat32()
	if err != nil {
		return err
	}
	p.Worldy = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Flags = v5
	return nil
}

func (p *Remote_Menus_labelReliable_123) Write(w *Writer) error {
	if err := w.WriteStringNullable(&p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Flags); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_labelReliable_123) Priority() int { return PriorityNormal }

type Remote_Menus_menu_106 struct {
	MenuId int32
	Title string
	Message string
	Options [][]string
}

func (p *Remote_Menus_menu_106) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.MenuId = v0
	v1, err := ReadString(r)
	if err != nil {
		return err
	}
	if v1 != nil {
		p.Title = *v1
	}
	v2, err := ReadString(r)
	if err != nil {
		return err
	}
	if v2 != nil {
		p.Message = *v2
	}
	v3, err := ReadStringArray(r)
	if err != nil {
		return err
	}
	p.Options = v3
	return nil
}

func (p *Remote_Menus_menu_106) Write(w *Writer) error {
	if err := w.WriteInt32(p.MenuId); err != nil {
		return err
	}
	// Java TypeIO.writeString: exists-byte + write.str
	title := p.Title
	if err := WriteString(w, &title); err != nil {
		return err
	}
	message := p.Message
	if err := WriteString(w, &message); err != nil {
		return err
	}
	if err := WriteStringArray(w, p.Options); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_menu_106) Priority() int { return PriorityNormal }

type Remote_Menus_menuChoose_109 struct {
	Player Entity
	MenuId int32
	Option int32
}

func (p *Remote_Menus_menuChoose_109) Read(r *Reader, _ int) error {
	// C→S omits the injected Player.
	p.Player = nil
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.MenuId = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Option = v2
	return nil
}

func (p *Remote_Menus_menuChoose_109) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteInt32(p.MenuId); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Option); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_menuChoose_109) Priority() int { return PriorityNormal }

type Remote_Menus_openURI_128 struct {
	Uri string
}

func (p *Remote_Menus_openURI_128) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Uri = v0
	return nil
}

func (p *Remote_Menus_openURI_128) Write(w *Writer) error {
	if err := WriteString(w, &p.Uri); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_openURI_128) Priority() int { return PriorityNormal }

type Remote_Menus_removeWorldLabel_130 struct {
	Id int32
}

func (p *Remote_Menus_removeWorldLabel_130) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v0
	return nil
}

func (p *Remote_Menus_removeWorldLabel_130) Write(w *Writer) error {
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_removeWorldLabel_130) Priority() int { return PriorityNormal }

type Remote_Menus_setHudText_113 struct {
	Message string
}

func (p *Remote_Menus_setHudText_113) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	return nil
}

func (p *Remote_Menus_setHudText_113) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_setHudText_113) Priority() int { return PriorityNormal }

type Remote_Menus_setHudTextReliable_115 struct {
	Message string
}

func (p *Remote_Menus_setHudTextReliable_115) Read(r *Reader, _ int) error {
	v0, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v0
	return nil
}

func (p *Remote_Menus_setHudTextReliable_115) Write(w *Writer) error {
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_setHudTextReliable_115) Priority() int { return PriorityNormal }

type Remote_Menus_textInput_110 struct {
	TextInputId int32
	Title string
	Message string
	TextLength int32
	Def string
	Numeric bool
}

func (p *Remote_Menus_textInput_110) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TextInputId = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Title = v1
	v2, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TextLength = v3
	v4, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Def = v4
	v5, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Numeric = v5
	return nil
}

func (p *Remote_Menus_textInput_110) Write(w *Writer) error {
	if err := w.WriteInt32(p.TextInputId); err != nil {
		return err
	}
	if err := WriteString(w, &p.Title); err != nil {
		return err
	}
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.TextLength); err != nil {
		return err
	}
	if err := WriteString(w, &p.Def); err != nil {
		return err
	}
	if err := w.WriteBool(p.Numeric); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_textInput_110) Priority() int { return PriorityNormal }

type Remote_Menus_textInput_111 struct {
	TextInputId int32
	Title string
	Message string
	TextLength int32
	Def string
	Numeric bool
	AllowEmpty bool
}

func (p *Remote_Menus_textInput_111) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TextInputId = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Title = v1
	v2, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Message = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TextLength = v3
	v4, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Def = v4
	v5, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Numeric = v5
	v6, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.AllowEmpty = v6
	return nil
}

func (p *Remote_Menus_textInput_111) Write(w *Writer) error {
	if err := w.WriteInt32(p.TextInputId); err != nil {
		return err
	}
	if err := WriteString(w, &p.Title); err != nil {
		return err
	}
	if err := WriteString(w, &p.Message); err != nil {
		return err
	}
	if err := w.WriteInt32(p.TextLength); err != nil {
		return err
	}
	if err := WriteString(w, &p.Def); err != nil {
		return err
	}
	if err := w.WriteBool(p.Numeric); err != nil {
		return err
	}
	if err := w.WriteBool(p.AllowEmpty); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_textInput_111) Priority() int { return PriorityNormal }

type Remote_Menus_textInputResult_112 struct {
	Player Entity
	TextInputId int32
	Text string
}

func (p *Remote_Menus_textInputResult_112) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.TextInputId = v1
	v2, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	if v2 != nil {
		p.Text = *v2
	}
	return nil
}

func (p *Remote_Menus_textInputResult_112) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := w.WriteInt32(p.TextInputId); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Text); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_textInputResult_112) Priority() int { return PriorityNormal }

type Remote_Menus_warningToast_127 struct {
	Unicode int32
	Text string
}

func (p *Remote_Menus_warningToast_127) Read(r *Reader, _ int) error {
	v0, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Unicode = v0
	v1, err := readTypeIOString(r)
	if err != nil {
		return err
	}
	p.Text = v1
	return nil
}

func (p *Remote_Menus_warningToast_127) Write(w *Writer) error {
	if err := w.WriteInt32(p.Unicode); err != nil {
		return err
	}
	if err := WriteString(w, &p.Text); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Menus_warningToast_127) Priority() int { return PriorityNormal }

type Remote_HudFragment_setPlayerTeamEditor_131 struct {
	Player Entity
	Team Team
}

func (p *Remote_HudFragment_setPlayerTeamEditor_131) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	return nil
}

func (p *Remote_HudFragment_setPlayerTeamEditor_131) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	return nil
}

func (p *Remote_HudFragment_setPlayerTeamEditor_131) Priority() int { return PriorityNormal }

type Remote_Build_beginBreak_132 struct {
	Unit Entity
	Team Team
	X int32
	Y int32
}

func (p *Remote_Build_beginBreak_132) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	v2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.X = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Y = v3
	return nil
}

func (p *Remote_Build_beginBreak_132) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Build_beginBreak_132) Priority() int { return PriorityNormal }

type Remote_Build_beginPlace_133 struct {
	Unit Entity
	Result Content
	Team Team
	X int32
	Y int32
	Rotation int32
	PlaceConfig any
}

func (p *Remote_Build_beginPlace_133) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Unit = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Result = v1
	v2, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.X = v3
	v4, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Y = v4
	v5, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Rotation = v5
	v6, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.PlaceConfig = v6
	return nil
}

func (p *Remote_Build_beginPlace_133) Write(w *Writer) error {
	if err := WriteEntity(w, p.Unit); err != nil {
		return err
	}
	if err := WriteContent(w, p.Result); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Rotation); err != nil {
		return err
	}
	if err := WriteObject(w, p.PlaceConfig, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Build_beginPlace_133) Priority() int { return PriorityNormal }

type Remote_Tile_buildDestroyed_143 struct {
	Build Entity
}

func (p *Remote_Tile_buildDestroyed_143) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	return nil
}

func (p *Remote_Tile_buildDestroyed_143) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_buildDestroyed_143) Priority() int { return PriorityNormal }

type Remote_Tile_buildHealthUpdate_144 struct {
	Buildings IntSeq
}

func (p *Remote_Tile_buildHealthUpdate_144) Read(r *Reader, _ int) error {
	v0, err := ReadIntSeq(r)
	if err != nil {
		return err
	}
	p.Buildings = v0
	return nil
}

func (p *Remote_Tile_buildHealthUpdate_144) Write(w *Writer) error {
	if err := WriteIntSeq(w, p.Buildings); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_buildHealthUpdate_144) Priority() int { return PriorityNormal }

type Remote_Tile_removeTile_139 struct {
	Tile Tile
}

func (p *Remote_Tile_removeTile_139) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	return nil
}

func (p *Remote_Tile_removeTile_139) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_removeTile_139) Priority() int { return PriorityNormal }

type Remote_Tile_setFloor_137 struct {
	Tile Tile
	Floor Content
	Overlay Content
}

func (p *Remote_Tile_setFloor_137) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Floor = v1
	v2, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Overlay = v2
	return nil
}

func (p *Remote_Tile_setFloor_137) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteContent(w, p.Floor); err != nil {
		return err
	}
	if err := WriteContent(w, p.Overlay); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setFloor_137) Priority() int { return PriorityNormal }

type Remote_Tile_setOverlay_138 struct {
	Tile Tile
	Overlay Content
}

func (p *Remote_Tile_setOverlay_138) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Overlay = v1
	return nil
}

func (p *Remote_Tile_setOverlay_138) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteContent(w, p.Overlay); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setOverlay_138) Priority() int { return PriorityNormal }

type Remote_Tile_setTeam_141 struct {
	Build Entity
	Team Team
}

func (p *Remote_Tile_setTeam_141) Read(r *Reader, _ int) error {
	v0, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Build = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	return nil
}

func (p *Remote_Tile_setTeam_141) Write(w *Writer) error {
	if err := WriteEntity(w, p.Build); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTeam_141) Priority() int { return PriorityNormal }

type Remote_Tile_setTeams_142 struct {
	Positions []int32
	Team Team
}

func (p *Remote_Tile_setTeams_142) Read(r *Reader, _ int) error {
	v0, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	return nil
}

func (p *Remote_Tile_setTeams_142) Write(w *Writer) error {
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTeams_142) Priority() int { return PriorityNormal }

type Remote_Tile_setTile_140 struct {
	Tile Tile
	Block Content
	Team Team
	Rotation int32
}

func (p *Remote_Tile_setTile_140) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v1
	v2, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v2
	v3, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Rotation = v3
	return nil
}

func (p *Remote_Tile_setTile_140) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Rotation); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTile_140) Priority() int { return PriorityNormal }

type Remote_Tile_setTileBlocks_134 struct {
	Block Content
	Team Team
	Positions []int32
}

func (p *Remote_Tile_setTileBlocks_134) Read(r *Reader, _ int) error {
	v0, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v0
	v1, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v1
	v2, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v2
	return nil
}

func (p *Remote_Tile_setTileBlocks_134) Write(w *Writer) error {
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTileBlocks_134) Priority() int { return PriorityNormal }

type Remote_Tile_setTileFloors_135 struct {
	Block Content
	Positions []int32
}

func (p *Remote_Tile_setTileFloors_135) Read(r *Reader, _ int) error {
	v0, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v0
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v1
	return nil
}

func (p *Remote_Tile_setTileFloors_135) Write(w *Writer) error {
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTileFloors_135) Priority() int { return PriorityNormal }

type Remote_Tile_setTileOverlays_136 struct {
	Block Content
	Positions []int32
}

func (p *Remote_Tile_setTileOverlays_136) Read(r *Reader, _ int) error {
	v0, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v0
	v1, err := ReadInts(r)
	if err != nil {
		return err
	}
	p.Positions = v1
	return nil
}

func (p *Remote_Tile_setTileOverlays_136) Write(w *Writer) error {
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteInts(w, p.Positions); err != nil {
		return err
	}
	return nil
}

func (p *Remote_Tile_setTileOverlays_136) Priority() int { return PriorityNormal }

type Remote_ConstructBlock_constructFinish_146 struct {
	Tile Tile
	Block Content
	Builder Entity
	Rotation byte
	Team Team
	Config any
}

func (p *Remote_ConstructBlock_constructFinish_146) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v1
	v2, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Builder = v2
	v3, err := r.ReadByte()
	if err != nil {
		return err
	}
	p.Rotation = v3
	v4, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Team = v4
	v5, err := ReadObject(r, false, r.Ctx)
	if err != nil {
		return err
	}
	p.Config = v5
	return nil
}

func (p *Remote_ConstructBlock_constructFinish_146) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Builder); err != nil {
		return err
	}
	if err := w.WriteByte(p.Rotation); err != nil {
		return err
	}
	if err := WriteTeam(w, &p.Team); err != nil {
		return err
	}
	if err := WriteObject(w, p.Config, w.Ctx); err != nil {
		return err
	}
	return nil
}

func (p *Remote_ConstructBlock_constructFinish_146) Priority() int { return PriorityNormal }

type Remote_ConstructBlock_deconstructFinish_145 struct {
	Tile Tile
	Block Content
	Builder Entity
}

func (p *Remote_ConstructBlock_deconstructFinish_145) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadContent(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Block = v1
	v2, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Builder = v2
	return nil
}

func (p *Remote_ConstructBlock_deconstructFinish_145) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteContent(w, p.Block); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Builder); err != nil {
		return err
	}
	return nil
}

func (p *Remote_ConstructBlock_deconstructFinish_145) Priority() int { return PriorityNormal }

type Remote_LandingPad_landingPadLanded_147 struct {
	Tile Tile
}

func (p *Remote_LandingPad_landingPadLanded_147) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	return nil
}

func (p *Remote_LandingPad_landingPadLanded_147) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	return nil
}

func (p *Remote_LandingPad_landingPadLanded_147) Priority() int { return PriorityNormal }

type Remote_AutoDoor_autoDoorToggle_148 struct {
	Tile Tile
	Open bool
}

func (p *Remote_AutoDoor_autoDoorToggle_148) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := r.ReadBool()
	if err != nil {
		return err
	}
	p.Open = v1
	return nil
}

func (p *Remote_AutoDoor_autoDoorToggle_148) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := w.WriteBool(p.Open); err != nil {
		return err
	}
	return nil
}

func (p *Remote_AutoDoor_autoDoorToggle_148) Priority() int { return PriorityNormal }

type Remote_CoreBlock_playerSpawn_149 struct {
	Tile Tile
	Player Entity
}

func (p *Remote_CoreBlock_playerSpawn_149) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := ReadEntity(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Player = v1
	return nil
}

func (p *Remote_CoreBlock_playerSpawn_149) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	return nil
}

func (p *Remote_CoreBlock_playerSpawn_149) Priority() int { return PriorityNormal }

type Remote_UnitAssembler_assemblerDroneSpawned_151 struct {
	Tile Tile
	Id int32
}

func (p *Remote_UnitAssembler_assemblerDroneSpawned_151) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	return nil
}

func (p *Remote_UnitAssembler_assemblerDroneSpawned_151) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	return nil
}

func (p *Remote_UnitAssembler_assemblerDroneSpawned_151) Priority() int { return PriorityNormal }

type Remote_UnitAssembler_assemblerUnitSpawned_150 struct {
	Tile Tile
}

func (p *Remote_UnitAssembler_assemblerUnitSpawned_150) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	return nil
}

func (p *Remote_UnitAssembler_assemblerUnitSpawned_150) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	return nil
}

func (p *Remote_UnitAssembler_assemblerUnitSpawned_150) Priority() int { return PriorityNormal }

type Remote_UnitBlock_unitBlockSpawn_152 struct {
	Tile Tile
}

func (p *Remote_UnitBlock_unitBlockSpawn_152) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	return nil
}

func (p *Remote_UnitBlock_unitBlockSpawn_152) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	return nil
}

func (p *Remote_UnitBlock_unitBlockSpawn_152) Priority() int { return PriorityNormal }

type Remote_UnitCargoLoader_unitTetherBlockSpawned_153 struct {
	Tile Tile
	Id int32
}

func (p *Remote_UnitCargoLoader_unitTetherBlockSpawned_153) Read(r *Reader, _ int) error {
	v0, err := ReadTile(r, r.Ctx)
	if err != nil {
		return err
	}
	p.Tile = v0
	v1, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.Id = v1
	return nil
}

func (p *Remote_UnitCargoLoader_unitTetherBlockSpawned_153) Write(w *Writer) error {
	if err := WriteTile(w, p.Tile); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	return nil
}

func (p *Remote_UnitCargoLoader_unitTetherBlockSpawned_153) Priority() int { return PriorityNormal }

func remotePacketFactories() []PacketFactory {
	return []PacketFactory{
		func() Packet { return &Remote_NetServer_adminRequest_49{} }, // 7 AdminRequest
		func() Packet { return &Remote_Menus_announce_116{} }, // 8 Announce
		func() Packet { return &Remote_UnitAssembler_assemblerDroneSpawned_151{} }, // 9 AssemblerDroneSpawned
		func() Packet { return &Remote_UnitAssembler_assemblerUnitSpawned_150{} }, // 10 AssemblerUnitSpawned
		func() Packet { return &Remote_AutoDoor_autoDoorToggle_148{} }, // 11 AutoDoorToggle
		func() Packet { return &Remote_Build_beginBreak_132{} }, // 12 BeginBreak
		func() Packet { return &Remote_Build_beginPlace_133{} }, // 13 BeginPlace
		func() Packet { return &Remote_NetClient_blockSnapshot_34{} }, // 14 BlockSnapshot
		func() Packet { return &Remote_Tile_buildDestroyed_143{} }, // 15 BuildDestroyed
		func() Packet { return &Remote_Tile_buildHealthUpdate_144{} }, // 16 BuildHealthUpdate
		func() Packet { return &Remote_InputHandler_buildingControlSelect_92{} }, // 17 BuildingControlSelect
		func() Packet { return &Remote_InputHandler_clearItems_66{} }, // 18 ClearItems
		func() Packet { return &Remote_InputHandler_clearLiquids_70{} }, // 19 ClearLiquids
		func() Packet { return &Remote_NetClient_clearObjectives_26{} }, // 20 ClearObjectives
		func() Packet { return &Remote_NetClient_clientBinaryPacketReliable_5{} }, // 21 ClientBinaryPacketReliable
		func() Packet { return &Remote_NetClient_clientBinaryPacketUnreliable_6{} }, // 22 ClientBinaryPacketUnreliable
		func() Packet { return &Remote_NetServer_clientLogicDataReliable_43{} }, // 23 ClientLogicDataReliable
		func() Packet { return &Remote_NetServer_clientLogicDataUnreliable_44{} }, // 24 ClientLogicDataUnreliable
		func() Packet { return &Remote_NetClient_clientPacketReliable_7{} }, // 25 ClientPacketReliable
		func() Packet { return &Remote_NetClient_clientPacketUnreliable_8{} }, // 26 ClientPacketUnreliable
		func() Packet { return &Remote_NetServer_clientPlanSnapshot_46{} }, // 27 ClientPlanSnapshot
		func() Packet { return &Remote_NetServer_clientPlanSnapshotReceived_47{} }, // 28 ClientPlanSnapshotReceived
		func() Packet { return &Remote_NetServer_clientSnapshot_48{} }, // 29 ClientSnapshot
		func() Packet { return &Remote_InputHandler_commandBuilding_77{} }, // 30 CommandBuilding
		func() Packet { return &Remote_InputHandler_commandUnits_74{} }, // 31 CommandUnits
		func() Packet { return &Remote_NetClient_completeObjective_27{} }, // 32 CompleteObjective
		func() Packet { return &Remote_NetClient_connect_17{} }, // 33 Connect
		func() Packet { return &Remote_NetServer_connectConfirm_50{} }, // 34 ConnectConfirm
		func() Packet { return &Remote_ConstructBlock_constructFinish_146{} }, // 35 ConstructFinish
		func() Packet { return &Remote_Menus_copyToClipboard_129{} }, // 36 CopyToClipboard
		func() Packet { return &Remote_BulletType_createBullet_58{} }, // 37 CreateBullet
		func() Packet { return &Remote_LExecutor_createMarker_100{} }, // 38 CreateMarker
		func() Packet { return &Remote_Weather_createWeather_105{} }, // 39 CreateWeather
		func() Packet { return &Remote_NetServer_debugStatusClient_37{} }, // 40 DebugStatusClient
		func() Packet { return &Remote_NetServer_debugStatusClientUnreliable_38{} }, // 41 DebugStatusClientUnreliable
		func() Packet { return &Remote_ConstructBlock_deconstructFinish_145{} }, // 42 DeconstructFinish
		func() Packet { return &Remote_InputHandler_deletePlans_72{} }, // 43 DeletePlans
		func() Packet { return &Remote_Teams_destroyPayload_59{} }, // 44 DestroyPayload
		func() Packet { return &Remote_InputHandler_dropItem_88{} }, // 45 DropItem
		func() Packet { return &Remote_NetClient_effect_11{} }, // 46 Effect
		func() Packet { return &Remote_NetClient_effect_12{} }, // 47 Effect
		func() Packet { return &Remote_NetClient_effectReliable_13{} }, // 48 EffectReliable
		func() Packet { return &Remote_NetClient_entitySnapshot_32{} }, // 49 EntitySnapshot
		func() Packet { return &Remote_Tile_fillTileBlocks_160{} }, // 50 FillTileBlocks
		func() Packet { return &Remote_Tile_fillTileFloors_160{} }, // 51 FillTileFloors
		func() Packet { return &Remote_Tile_fillTileOverlays_160{} }, // 52 FillTileOverlays
		func() Packet { return &Remote_Menus_followUpMenu_107{} }, // 53 FollowUpMenu
		func() Packet { return &Remote_Logic_gameOver_3{} }, // 54 GameOver
		func() Packet { return &Remote_NetClient_hiddenSnapshot_33{} }, // 55 HiddenSnapshot
		func() Packet { return &Remote_Menus_hideFollowUpMenu_108{} }, // 56 HideFollowUpMenu
		func() Packet { return &Remote_Menus_hideHudText_114{} }, // 57 HideHudText
		func() Packet { return &Remote_Menus_hideMenuBuilder_160{} }, // 58 HideMenuBuilder
		func() Packet { return &Remote_Menus_infoMessage_117{} }, // 59 InfoMessage
		func() Packet { return &Remote_Menus_infoPopup_118{} }, // 60 InfoPopup
		func() Packet { return &Remote_Menus_infoPopup_120{} }, // 61 InfoPopup
		func() Packet { return &Remote_Menus_infoPopupReliable_119{} }, // 62 InfoPopupReliable
		func() Packet { return &Remote_Menus_infoPopupReliable_121{} }, // 63 InfoPopupReliable
		func() Packet { return &Remote_Menus_infoToast_126{} }, // 64 InfoToast
		func() Packet { return &Remote_NetClient_kick_22{} }, // 65 Kick
		func() Packet { return &Remote_NetClient_kick_21{} }, // 66 Kick
		func() Packet { return &Remote_Menus_label_124{} }, // 67 Label
		func() Packet { return &Remote_Menus_label_122{} }, // 68 Label
		func() Packet { return &Remote_Menus_label3{} }, // 69 Label
		func() Packet { return &Remote_Menus_labelReliable_125{} }, // 70 LabelReliable
		func() Packet { return &Remote_Menus_labelReliable_123{} }, // 71 LabelReliable
		func() Packet { return &Remote_Menus_labelReliable3{} }, // 72 LabelReliable
		func() Packet { return &Remote_LandingPad_landingPadLanded_147{} }, // 73 LandingPadLanded
		func() Packet { return &Remote_LExecutor_logicExplosion_97{} }, // 74 LogicExplosion
		func() Packet { return &Remote_Menus_menu_106{} }, // 75 Menu
		func() Packet { return &Remote_Menus_menuBuilder_160{} }, // 76 MenuBuilder
		func() Packet { return &Remote_Menus_menuBuilderChoose_160{} }, // 77 MenuBuilderChoose
		func() Packet { return &Remote_Menus_menuBuilderUpdate_160{} }, // 78 MenuBuilderUpdate
		func() Packet { return &Remote_Menus_menuChoose_109{} }, // 79 MenuChoose
		func() Packet { return &Remote_Menus_openURI_128{} }, // 80 OpenURI
		func() Packet { return &Remote_InputHandler_payloadDropped_86{} }, // 81 PayloadDropped
		func() Packet { return &Remote_InputHandler_pickedBuildPayload_84{} }, // 82 PickedBuildPayload
		func() Packet { return &Remote_InputHandler_pickedUnitPayload_83{} }, // 83 PickedUnitPayload
		func() Packet { return &Remote_NetClient_ping_18{} }, // 84 Ping
		func() Packet { return &Remote_InputHandler_pingLocation_73{} }, // 85 PingLocation
		func() Packet { return &Remote_NetClient_pingResponse_19{} }, // 86 PingResponse
		func() Packet { return &Remote_NetClient_playMusic{} }, // 87 PlayMusic
		func() Packet { return &Remote_NetClient_playerDisconnect_31{} }, // 88 PlayerDisconnect
		func() Packet { return &Remote_CoreBlock_playerSpawn_149{} }, // 89 PlayerSpawn
		func() Packet { return &Remote_LExecutor_removeMarker_101{} }, // 90 RemoveMarker
		func() Packet { return &Remote_InputHandler_removeQueueBlock_80{} }, // 91 RemoveQueueBlock
		func() Packet { return &Remote_Tile_removeTile_139{} }, // 92 RemoveTile
		func() Packet { return &Remote_Menus_removeWorldLabel_130{} }, // 93 RemoveWorldLabel
		func() Packet { return &Remote_NetServer_requestAssets{} }, // 94 RequestAssets
		func() Packet { return &Remote_NetServer_requestBlockSnapshot_45{} }, // 95 RequestBlockSnapshot
		func() Packet { return &Remote_InputHandler_requestBuildPayload_82{} }, // 96 RequestBuildPayload
		func() Packet { return &Remote_NetServer_requestDebugStatus_36{} }, // 97 RequestDebugStatus
		func() Packet { return &Remote_InputHandler_requestDropPayload_85{} }, // 98 RequestDropPayload
		func() Packet { return &Remote_InputHandler_requestItem_78{} }, // 99 RequestItem
		func() Packet { return &Remote_InputHandler_requestUnitPayload_81{} }, // 100 RequestUnitPayload
		func() Packet { return &Remote_NetServer_requestWorld{} }, // 101 RequestWorld
		func() Packet { return &Remote_Logic_researched_4{} }, // 102 Researched
		func() Packet { return &Remote_InputHandler_rotateBlock_89{} }, // 103 RotateBlock
		func() Packet { return &Remote_Logic_sectorCapture_1{} }, // 104 SectorCapture
		func() Packet { return &Remote_NetClient_sendChatMessage_16{} }, // 105 SendChatMessage
		func() Packet { return &Remote_NetClient_sendMessage_14{} }, // 106 SendMessage
		func() Packet { return &Remote_NetClient_sendMessage_15{} }, // 107 SendMessage
		func() Packet { return &Remote_NetServer_serverBinaryPacketReliable_41{} }, // 108 ServerBinaryPacketReliable
		func() Packet { return &Remote_NetServer_serverBinaryPacketUnreliable_42{} }, // 109 ServerBinaryPacketUnreliable
		func() Packet { return &Remote_NetServer_serverPacketReliable_39{} }, // 110 ServerPacketReliable
		func() Packet { return &Remote_NetServer_serverPacketUnreliable_40{} }, // 111 ServerPacketUnreliable
		func() Packet { return &Remote_NetClient_setCameraPosition_30{} }, // 112 SetCameraPosition
		func() Packet { return &Remote_LExecutor_setFlag_99{} }, // 113 SetFlag
		func() Packet { return &Remote_Tile_setFloor_137{} }, // 114 SetFloor
		func() Packet { return &Remote_Menus_setHudText_113{} }, // 115 SetHudText
		func() Packet { return &Remote_Menus_setHudTextReliable_115{} }, // 116 SetHudTextReliable
		func() Packet { return &Remote_InputHandler_setItem_63{} }, // 117 SetItem
		func() Packet { return &Remote_InputHandler_setItems_64{} }, // 118 SetItems
		func() Packet { return &Remote_InputHandler_setLiquid_67{} }, // 119 SetLiquid
		func() Packet { return &Remote_InputHandler_setLiquids_68{} }, // 120 SetLiquids
		func() Packet { return &Remote_LExecutor_setMapArea_96{} }, // 121 SetMapArea
		func() Packet { return &Remote_NetClient_setObjectives_25{} }, // 122 SetObjectives
		func() Packet { return &Remote_Tile_setOverlay_138{} }, // 123 SetOverlay
		func() Packet { return &Remote_HudFragment_setPlayerTeamEditor_131{} }, // 124 SetPlayerTeamEditor
		func() Packet { return &Remote_NetClient_setPosition_29{} }, // 125 SetPosition
		func() Packet { return &Remote_NetClient_setRule_24{} }, // 126 SetRule
		func() Packet { return &Remote_NetClient_setRules_23{} }, // 127 SetRules
		func() Packet { return &Remote_Tile_setTeam_141{} }, // 128 SetTeam
		func() Packet { return &Remote_Tile_setTeams_142{} }, // 129 SetTeams
		func() Packet { return &Remote_Tile_setTile_140{} }, // 130 SetTile
		func() Packet { return &Remote_Tile_setTileBlocks_134{} }, // 131 SetTileBlocks
		func() Packet { return &Remote_Tile_setTileFloors_135{} }, // 132 SetTileFloors
		func() Packet { return &Remote_InputHandler_setTileItems_65{} }, // 133 SetTileItems
		func() Packet { return &Remote_InputHandler_setTileLiquids_69{} }, // 134 SetTileLiquids
		func() Packet { return &Remote_Tile_setTileOverlays_136{} }, // 135 SetTileOverlays
		func() Packet { return &Remote_InputHandler_setUnitCommand_75{} }, // 136 SetUnitCommand
		func() Packet { return &Remote_InputHandler_setUnitStance_76{} }, // 137 SetUnitStance
		func() Packet { return &Remote_NetClient_sound_9{} }, // 138 Sound
		func() Packet { return &Remote_NetClient_soundAt_10{} }, // 139 SoundAt
		func() Packet { return &Remote_WaveSpawner_spawnEffect_0{} }, // 140 SpawnEffect
		func() Packet { return &Remote_NetClient_stateSnapshot_35{} }, // 141 StateSnapshot
		func() Packet { return &Remote_LExecutor_syncVariable_98{} }, // 142 SyncVariable
		func() Packet { return &Remote_InputHandler_takeItems_61{} }, // 143 TakeItems
		func() Packet { return &Remote_Menus_textInput_110{} }, // 144 TextInput
		func() Packet { return &Remote_Menus_textInput_111{} }, // 145 TextInput
		func() Packet { return &Remote_Menus_textInputResult_112{} }, // 146 TextInputResult
		func() Packet { return &Remote_InputHandler_tileConfig_90{} }, // 147 TileConfig
		func() Packet { return &Remote_InputHandler_tileTap_91{} }, // 148 TileTap
		func() Packet { return &Remote_NetClient_traceInfo_20{} }, // 149 TraceInfo
		func() Packet { return &Remote_InputHandler_transferInventory_79{} }, // 150 TransferInventory
		func() Packet { return &Remote_InputHandler_transferItemEffect_60{} }, // 151 TransferItemEffect
		func() Packet { return &Remote_InputHandler_transferItemTo_71{} }, // 152 TransferItemTo
		func() Packet { return &Remote_InputHandler_transferItemToUnit_62{} }, // 153 TransferItemToUnit
		func() Packet { return &Remote_UnitBlock_unitBlockSpawn_152{} }, // 154 UnitBlockSpawn
		func() Packet { return &Remote_InputHandler_unitBuildingControlSelect_93{} }, // 155 UnitBuildingControlSelect
		func() Packet { return &Remote_Units_unitCapDeath_52{} }, // 156 UnitCapDeath
		func() Packet { return &Remote_InputHandler_unitClear_95{} }, // 157 UnitClear
		func() Packet { return &Remote_InputHandler_unitControl_94{} }, // 158 UnitControl
		func() Packet { return &Remote_Units_unitDeath_54{} }, // 159 UnitDeath
		func() Packet { return &Remote_Units_unitDespawn_56{} }, // 160 UnitDespawn
		func() Packet { return &Remote_Units_unitDestroy_55{} }, // 161 UnitDestroy
		func() Packet { return &Remote_InputHandler_unitEnteredPayload_87{} }, // 162 UnitEnteredPayload
		func() Packet { return &Remote_Units_unitEnvDeath_53{} }, // 163 UnitEnvDeath
		func() Packet { return &Remote_Units_unitSafeDeath_57{} }, // 164 UnitSafeDeath
		func() Packet { return &Remote_Units_unitSpawn_51{} }, // 165 UnitSpawn
		func() Packet { return &Remote_UnitCargoLoader_unitTetherBlockSpawned_153{} }, // 166 UnitTetherBlockSpawned
		func() Packet { return &Remote_Logic_updateGameOver_2{} }, // 167 UpdateGameOver
		func() Packet { return &Remote_LExecutor_updateMarker_102{} }, // 168 UpdateMarker
		func() Packet { return &Remote_LExecutor_updateMarkerText_103{} }, // 169 UpdateMarkerText
		func() Packet { return &Remote_LExecutor_updateMarkerTexture_104{} }, // 170 UpdateMarkerTexture
		func() Packet { return &Remote_Menus_warningToast_127{} }, // 171 WarningToast
		func() Packet { return &Remote_NetClient_worldDataBegin_28{} }, // 172 WorldDataBegin
	}
}

func initRemotePackets(r *PacketRegistry) {
	for _, f := range remotePacketFactories() {
		r.Register(f)
	}
}
