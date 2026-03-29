package protocol

import "testing"

func TestRemoteInputHandlerTileConfigServerReadMatchesOfficialClientLayout(t *testing.T) {
	pos := PackPoint2(113, 298)

	wire := NewWriter()
	if err := WriteBuilding(wire, BuildingBox{PosValue: pos}); err != nil {
		t.Fatalf("write building: %v", err)
	}
	if err := WriteObject(wire, ItemRef{ItmID: 11}, nil); err != nil {
		t.Fatalf("write object: %v", err)
	}

	var packet Remote_InputHandler_tileConfig_127
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
	packet := Remote_InputHandler_tileConfig_127{
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

	var packet Remote_InputHandler_rotateBlock_83
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
