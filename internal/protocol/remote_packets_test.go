package protocol

import "testing"

func TestRemoteNetServerClientSnapshotUsesOfficialPlansQueueLayout(t *testing.T) {
	ctx := &TypeIOContext{
		BlockLookup: func(id int16) Block {
			switch id {
			case 45:
				return BlockRef{BlkID: 45, BlkName: "router"}
			default:
				return BlockRef{BlkID: id, BlkName: "block"}
			}
		},
	}
	packet := Remote_NetServer_clientSnapshot_48{
		SnapshotID:       3,
		UnitID:           9,
		X:                64,
		Y:                96,
		PointerX:         64,
		PointerY:         96,
		SelectedBlock:    BlockRef{BlkID: 45, BlkName: "router"},
		SelectedRotation: 2,
		Plans: []*BuildPlan{{
			X:        12,
			Y:        34,
			Rotation: 3,
			Block:    BlockRef{BlkID: 45, BlkName: "router"},
			Config:   int32(99),
		}},
	}

	wire := NewWriterWithContext(ctx)
	if err := packet.Write(wire); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	var decoded Remote_NetServer_clientSnapshot_48
	if err := decoded.Read(NewReaderWithContext(wire.Bytes(), ctx), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if decoded.Player != nil {
		t.Fatalf("expected no implicit player in client->server snapshot payload, got %T", decoded.Player)
	}

	plans, ok := decoded.Plans.([]*BuildPlan)
	if !ok {
		t.Fatalf("expected decoded plans slice, got %T", decoded.Plans)
	}
	if len(plans) != 1 || plans[0] == nil {
		t.Fatalf("expected one decoded plan, got %#v", plans)
	}
	if plans[0].X != 12 || plans[0].Y != 34 {
		t.Fatalf("expected plan at (12,34), got (%d,%d)", plans[0].X, plans[0].Y)
	}
	if plans[0].Rotation != 3 {
		t.Fatalf("expected rotation=3, got %d", plans[0].Rotation)
	}
	if plans[0].Block == nil || plans[0].Block.ID() != 45 {
		t.Fatalf("expected block id=45, got %#v", plans[0].Block)
	}
	cfg, ok := plans[0].Config.(int32)
	if !ok || cfg != 99 {
		t.Fatalf("expected int32 config=99, got %T %#v", plans[0].Config, plans[0].Config)
	}

	reader := NewReaderWithContext(wire.Bytes(), ctx)
	if _, err := reader.ReadInt32(); err != nil {
		t.Fatalf("skip snapshotID: %v", err)
	}
	if _, err := reader.ReadInt32(); err != nil {
		t.Fatalf("skip unitID: %v", err)
	}
	if _, err := reader.ReadBool(); err != nil {
		t.Fatalf("skip dead: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := reader.ReadFloat32(); err != nil {
			t.Fatalf("skip float[%d]: %v", i, err)
		}
	}
	if _, err := ReadTile(reader, ctx); err != nil {
		t.Fatalf("skip mining: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := reader.ReadBool(); err != nil {
			t.Fatalf("skip state bool[%d]: %v", i, err)
		}
	}
	if _, err := ReadBlock(reader, ctx); err != nil {
		t.Fatalf("skip selectedBlock: %v", err)
	}
	if _, err := reader.ReadInt32(); err != nil {
		t.Fatalf("skip selectedRotation: %v", err)
	}
	planCount, err := reader.ReadInt32()
	if err != nil {
		t.Fatalf("read plansQueue count: %v", err)
	}
	if planCount != 1 {
		t.Fatalf("expected plansQueue int32 count=1, got %d", planCount)
	}
}

func TestRemoteInputHandlerTileConfigServerReadMatchesOfficialClientLayout(t *testing.T) {
	pos := PackPoint2(113, 298)

	wire := NewWriter()
	if err := WriteBuilding(wire, BuildingBox{PosValue: pos}); err != nil {
		t.Fatalf("write building: %v", err)
	}
	if err := WriteObject(wire, ItemRef{ItmID: 11}, nil); err != nil {
		t.Fatalf("write object: %v", err)
	}

	var packet Remote_InputHandler_tileConfig_90
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no player in client->server tileConfig packet, got %T", packet.Player)
	}
	if packet.Build == nil || packet.Build.Pos() != pos {
		t.Fatalf("expected build pos=%d, got %#v", pos, packet.Build)
	}
	content, ok := packet.Value.(Content)
	if !ok || content.ContentType() != ContentItem || content.ID() != 11 {
		t.Fatalf("expected item config content id=11, got %T %#v", packet.Value, packet.Value)
	}
}

func TestRemoteInputHandlerTileConfigServerWriteMatchesOfficialServerLayout(t *testing.T) {
	pos := PackPoint2(12, 34)
	packet := Remote_InputHandler_tileConfig_90{
		Player: UnitBox{IDValue: 77},
		Build:  BuildingBox{PosValue: pos},
		Value:  ItemRef{ItmID: 6},
	}

	wire := NewWriter()
	if err := packet.Write(wire); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	reader := NewReader(wire.Bytes())
	player, err := ReadEntity(reader, nil)
	if err != nil {
		t.Fatalf("read player: %v", err)
	}
	if player == nil || player.ID() != 77 {
		t.Fatalf("expected player id=77, got %#v", player)
	}
	build, err := ReadBuilding(reader, nil)
	if err != nil {
		t.Fatalf("read build: %v", err)
	}
	if build == nil || build.Pos() != pos {
		t.Fatalf("expected build pos=%d, got %#v", pos, build)
	}
	value, err := ReadObject(reader, false, nil)
	if err != nil {
		t.Fatalf("read value: %v", err)
	}
	content, ok := value.(Content)
	if !ok || content.ContentType() != ContentItem || content.ID() != 6 {
		t.Fatalf("expected item content id=6, got %T %#v", value, value)
	}
}

func TestRemoteInputHandlerRotateBlockServerReadMatchesOfficialClientLayout(t *testing.T) {
	pos := PackPoint2(110, 304)

	wire := NewWriter()
	if err := WriteBuilding(wire, BuildingBox{PosValue: pos}); err != nil {
		t.Fatalf("write building: %v", err)
	}
	if err := wire.WriteBool(true); err != nil {
		t.Fatalf("write direction: %v", err)
	}

	var packet Remote_InputHandler_rotateBlock_89
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no player in client->server rotateBlock packet, got %T", packet.Player)
	}
	if packet.Build == nil || packet.Build.Pos() != pos {
		t.Fatalf("expected build pos=%d, got %#v", pos, packet.Build)
	}
	if !packet.Direction {
		t.Fatalf("expected direction=true")
	}
}

func TestRemoteNetServerClientPlanSnapshotReadMatchesOfficialClientLayout(t *testing.T) {
	ctx := &TypeIOContext{
		BlockLookup: func(id int16) Block {
			return BlockRef{BlkID: id, BlkName: "router"}
		},
	}
	wire := NewWriter()
	if err := wire.WriteInt32(17); err != nil {
		t.Fatalf("write group id: %v", err)
	}
	plans := []*BuildPlan{{
		X:        5,
		Y:        9,
		Rotation: 1,
		Block:    BlockRef{BlkID: 45, BlkName: "router"},
	}}
	if err := WriteClientPlans(wire, plans, ctx); err != nil {
		t.Fatalf("write plans: %v", err)
	}

	var packet Remote_NetServer_clientPlanSnapshot_46
	if err := packet.Read(NewReaderWithContext(wire.Bytes(), ctx), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in clientPlanSnapshot payload, got %T", packet.Player)
	}
	if packet.GroupId != 17 {
		t.Fatalf("expected groupId=17, got %d", packet.GroupId)
	}
	decodedPlans, ok := packet.Plans.([]*BuildPlan)
	if !ok || len(decodedPlans) != 1 || decodedPlans[0] == nil {
		t.Fatalf("expected one decoded build plan, got %#v", packet.Plans)
	}
	if decodedPlans[0].X != 5 || decodedPlans[0].Y != 9 || decodedPlans[0].Rotation != 1 {
		t.Fatalf("unexpected decoded plan: %#v", decodedPlans[0])
	}
}

func TestRemoteNetClientSendChatMessageReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	message := "hello world"
	if err := WriteString(wire, &message); err != nil {
		t.Fatalf("write message: %v", err)
	}

	var packet Remote_NetClient_sendChatMessage_16
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in sendChatMessage payload, got %T", packet.Player)
	}
	if packet.Message != message {
		t.Fatalf("expected message %q, got %q", message, packet.Message)
	}
}

func TestRemoteNetServerConnectConfirmReadMatchesOfficialClientLayout(t *testing.T) {
	var packet Remote_NetServer_connectConfirm_50
	if err := packet.Read(NewReader(nil), 0); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no player in connectConfirm payload, got %T", packet.Player)
	}
}

func TestRemoteNetClientSendMessage14MatchesOfficialSingleStringLayout(t *testing.T) {
	wire := NewWriter()
	message := "server online"
	if err := WriteString(wire, &message); err != nil {
		t.Fatalf("write message: %v", err)
	}

	var packet Remote_NetClient_sendMessage_14
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Message != message {
		t.Fatalf("expected message %q, got %q", message, packet.Message)
	}

	out := NewWriter()
	if err := packet.Write(out); err != nil {
		t.Fatalf("rewrite packet: %v", err)
	}
	if string(out.Bytes()) != string(wire.Bytes()) {
		t.Fatalf("expected identical single-string wire payload")
	}
}

func TestRemoteNetClientSendMessage15MatchesOfficialRichLayout(t *testing.T) {
	wire := NewWriter()
	message := "[cyan]foo[white]: hi"
	unformatted := "hi"
	if err := WriteString(wire, &message); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := WriteString(wire, &unformatted); err != nil {
		t.Fatalf("write unformatted: %v", err)
	}
	if err := WriteEntity(wire, &EntityBox{IDValue: 9}); err != nil {
		t.Fatalf("write playersender: %v", err)
	}

	var packet Remote_NetClient_sendMessage_15
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Message != message {
		t.Fatalf("expected message %q, got %q", message, packet.Message)
	}
	if packet.Unformatted != unformatted {
		t.Fatalf("expected unformatted %q, got %q", unformatted, packet.Unformatted)
	}
	if packet.Playersender == nil || packet.Playersender.ID() != 9 {
		t.Fatalf("expected playersender id=9, got %#v", packet.Playersender)
	}
}

func TestRemoteInputHandlerRequestUnitPayloadReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	if err := WriteUnit(wire, UnitBox{IDValue: 41}); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	var packet Remote_InputHandler_requestUnitPayload_81
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in requestUnitPayload payload, got %T", packet.Player)
	}
	if packet.Target == nil || packet.Target.ID() != 41 {
		t.Fatalf("expected target unit id=41, got %#v", packet.Target)
	}
}

func TestRemoteInputHandlerRequestBuildPayloadReadMatchesOfficialClientLayout(t *testing.T) {
	pos := PackPoint2(22, 71)
	wire := NewWriter()
	if err := WriteBuilding(wire, BuildingBox{PosValue: pos}); err != nil {
		t.Fatalf("write building: %v", err)
	}

	var packet Remote_InputHandler_requestBuildPayload_82
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in requestBuildPayload payload, got %T", packet.Player)
	}
	if packet.Build == nil || packet.Build.Pos() != pos {
		t.Fatalf("expected build pos=%d, got %#v", pos, packet.Build)
	}
}

func TestRemoteInputHandlerRequestDropPayloadReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	if err := wire.WriteFloat32(14.5); err != nil {
		t.Fatalf("write x: %v", err)
	}
	if err := wire.WriteFloat32(29.25); err != nil {
		t.Fatalf("write y: %v", err)
	}

	var packet Remote_InputHandler_requestDropPayload_85
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in requestDropPayload payload, got %T", packet.Player)
	}
	if packet.X != 14.5 || packet.Y != 29.25 {
		t.Fatalf("expected coords (14.5,29.25), got (%v,%v)", packet.X, packet.Y)
	}
}

func TestRemoteInputHandlerDropItemReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	if err := wire.WriteFloat32(33.75); err != nil {
		t.Fatalf("write angle: %v", err)
	}

	var packet Remote_InputHandler_dropItem_88
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in dropItem payload, got %T", packet.Player)
	}
	if packet.Angle != 33.75 {
		t.Fatalf("expected angle=33.75, got %v", packet.Angle)
	}
}

func TestRemoteInputHandlerPingLocationReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	if err := wire.WriteFloat32(144.5); err != nil {
		t.Fatalf("write x: %v", err)
	}
	if err := wire.WriteFloat32(88.25); err != nil {
		t.Fatalf("write y: %v", err)
	}
	text := "danger"
	if err := WriteObject(wire, text, nil); err != nil {
		t.Fatalf("write text: %v", err)
	}

	var packet Remote_InputHandler_pingLocation_73
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in pingLocation payload, got %T", packet.Player)
	}
	if packet.X != 144.5 || packet.Y != 88.25 {
		t.Fatalf("expected coords (144.5,88.25), got (%v,%v)", packet.X, packet.Y)
	}
	gotText, ok := packet.Text.(string)
	if !ok || gotText != text {
		t.Fatalf("expected text %q, got %T %#v", text, packet.Text, packet.Text)
	}
}

func TestRemoteInputHandlerBuildingControlSelectReadMatchesOfficialClientLayout(t *testing.T) {
	pos := PackPoint2(40, 88)
	wire := NewWriter()
	if err := WriteBuilding(wire, BuildingBox{PosValue: pos}); err != nil {
		t.Fatalf("write building: %v", err)
	}

	var packet Remote_InputHandler_buildingControlSelect_92
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in buildingControlSelect payload, got %T", packet.Player)
	}
	if packet.Build == nil || packet.Build.Pos() != pos {
		t.Fatalf("expected build pos=%d, got %#v", pos, packet.Build)
	}
}

func TestRemoteInputHandlerUnitControlReadMatchesOfficialClientLayout(t *testing.T) {
	wire := NewWriter()
	if err := WriteUnit(wire, UnitBox{IDValue: 205}); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	var packet Remote_InputHandler_unitControl_94
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Player != nil {
		t.Fatalf("expected no implicit player in unitControl payload, got %T", packet.Player)
	}
	if packet.Unit == nil || packet.Unit.ID() != 205 {
		t.Fatalf("expected unit id=205, got %#v", packet.Unit)
	}
}

func TestRemoteConstructBlockConstructFinishMatchesOfficialLayout(t *testing.T) {
	pos := PackPoint2(175, 75)
	wire := NewWriter()
	if err := WriteTile(wire, TileBox{PosValue: pos}); err != nil {
		t.Fatalf("write tile: %v", err)
	}
	if err := WriteBlock(wire, BlockRef{BlkID: 257, BlkName: "conveyor"}); err != nil {
		t.Fatalf("write block: %v", err)
	}
	if err := WriteUnit(wire, UnitBox{IDValue: 207}); err != nil {
		t.Fatalf("write builder: %v", err)
	}
	if err := wire.WriteByte(2); err != nil {
		t.Fatalf("write rotation: %v", err)
	}
	if err := WriteTeam(wire, &Team{ID: 1}); err != nil {
		t.Fatalf("write team: %v", err)
	}
	if err := WriteObject(wire, Point2{X: 0, Y: 0}, nil); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var packet Remote_ConstructBlock_constructFinish_146
	if err := packet.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if packet.Tile == nil || packet.Tile.Pos() != pos {
		t.Fatalf("expected tile pos=%d, got %#v", pos, packet.Tile)
	}
	if packet.Block == nil || packet.Block.ID() != 257 {
		t.Fatalf("expected block id=257, got %#v", packet.Block)
	}
	if packet.Builder == nil || packet.Builder.ID() != 207 {
		t.Fatalf("expected builder unit id=207, got %#v", packet.Builder)
	}
	if packet.Rotation != 2 || packet.Team.ID != 1 {
		t.Fatalf("expected rotation=2 team=1, got rotation=%d team=%d", packet.Rotation, packet.Team.ID)
	}
	if _, ok := packet.Config.(Point2); !ok {
		t.Fatalf("expected Point2 config, got %T %#v", packet.Config, packet.Config)
	}
}

func TestRemoteMenusInfoPopupNoIDUsesStringLayout(t *testing.T) {
	packet := Remote_Menus_infoPopup_118{
		Message:  "reactor unstable",
		Duration: 2.5,
		Align:    1,
		Top:      2,
		Left:     3,
		Bottom:   4,
		Right:    5,
	}

	wire := NewWriter()
	if err := packet.Write(wire); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	reader := NewReader(wire.Bytes())
	message, err := ReadString(reader)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if message == nil || *message != packet.Message {
		t.Fatalf("expected message %q, got %#v", packet.Message, message)
	}
	if _, err := reader.ReadFloat32(); err != nil {
		t.Fatalf("read duration: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := reader.ReadInt32(); err != nil {
			t.Fatalf("read bounds[%d]: %v", i, err)
		}
	}
	if reader.Remaining() != 0 {
		t.Fatalf("expected no trailing bytes, got %d", reader.Remaining())
	}

	var decoded Remote_Menus_infoPopup_118
	if err := decoded.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if decoded.Message != packet.Message || decoded.Duration != packet.Duration || decoded.Align != packet.Align {
		t.Fatalf("decoded packet mismatch: got %+v want %+v", decoded, packet)
	}
}

func TestRemoteMenusInfoPopupWithIDUsesStringLayout(t *testing.T) {
	packet := Remote_Menus_infoPopup_120{
		Message:  "reactor unstable",
		Id:       "reactor-warning",
		Duration: 2.5,
		Align:    1,
		Top:      2,
		Left:     3,
		Bottom:   4,
		Right:    5,
	}

	wire := NewWriter()
	if err := packet.Write(wire); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	reader := NewReader(wire.Bytes())
	message, err := ReadString(reader)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if message == nil || *message != packet.Message {
		t.Fatalf("expected message %q, got %#v", packet.Message, message)
	}
	id, err := ReadString(reader)
	if err != nil {
		t.Fatalf("read id: %v", err)
	}
	if id == nil || *id != packet.Id {
		t.Fatalf("expected id %q, got %#v", packet.Id, id)
	}
	if _, err := reader.ReadFloat32(); err != nil {
		t.Fatalf("read duration: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := reader.ReadInt32(); err != nil {
			t.Fatalf("read bounds[%d]: %v", i, err)
		}
	}
	if reader.Remaining() != 0 {
		t.Fatalf("expected no trailing bytes, got %d", reader.Remaining())
	}

	var decoded Remote_Menus_infoPopup_120
	if err := decoded.Read(NewReader(wire.Bytes()), len(wire.Bytes())); err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if decoded.Message != packet.Message || decoded.Id != packet.Id || decoded.Duration != packet.Duration {
		t.Fatalf("decoded packet mismatch: got %+v want %+v", decoded, packet)
	}
}
