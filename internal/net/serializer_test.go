package net

import (
	"bytes"
	"encoding/binary"
	"testing"

	"mdt-server/internal/protocol"
)

func frameSerializerPacket(t *testing.T, packetID byte, payload []byte) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	if err := buf.WriteByte(packetID); err != nil {
		t.Fatalf("write packet id: %v", err)
	}
	if err := binary.Write(&buf, binary.BigEndian, uint16(len(payload))); err != nil {
		t.Fatalf("write payload length: %v", err)
	}
	if err := buf.WriteByte(0); err != nil {
		t.Fatalf("write compression flag: %v", err)
	}
	if _, err := buf.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func readSerializerHeader(t *testing.T, framed []byte) (byte, uint16, byte) {
	t.Helper()

	r := bytes.NewReader(framed)
	packetID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet id: %v", err)
	}
	var payloadLen uint16
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		t.Fatalf("read payload length: %v", err)
	}
	compressed, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read compression flag: %v", err)
	}
	return packetID, payloadLen, compressed
}

func TestCriticalPacketIDsMatchRegistryOfficial157(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	tests := []struct {
		name         string
		packet       protocol.Packet
		wantOfficial byte
	}{
		{
			name:         "connect confirm official alias",
			packet:       &protocol.Remote_NetServer_connectConfirm_50{},
			wantOfficial: 31,
		},
		{
			name:         "world data begin official",
			packet:       &protocol.Remote_NetClient_worldDataBegin_28{},
			wantOfficial: 157,
		},
		{
			name:         "unit clear official",
			packet:       &protocol.Remote_InputHandler_unitClear_95{},
			wantOfficial: 142,
		},
		{
			name:         "player spawn official",
			packet:       &protocol.Remote_CoreBlock_playerSpawn_149{},
			wantOfficial: 76,
		},
		{
			name:         "kick enum official",
			packet:       &protocol.Remote_NetClient_kick_21{Reason: protocol.KickReasonKick},
			wantOfficial: 58,
		},
		{
			name:         "kick string official",
			packet:       &protocol.Remote_NetClient_kick_22{Reason: "map changed"},
			wantOfficial: 59,
		},
		{
			name:         "ping response official",
			packet:       &protocol.Remote_NetClient_pingResponse_19{Time: 99},
			wantOfficial: 74,
		},
		{
			name:         "create marker official",
			packet:       &protocol.Remote_LExecutor_createMarker_100{},
			wantOfficial: 35,
		},
		{
			name:         "debug status unreliable official",
			packet:       &protocol.Remote_NetServer_debugStatusClientUnreliable_38{},
			wantOfficial: 38,
		},
		{
			name:         "update marker official",
			packet:       &protocol.Remote_LExecutor_updateMarker_102{},
			wantOfficial: 153,
		},
		{
			name:         "update marker text official",
			packet:       &protocol.Remote_LExecutor_updateMarkerText_103{},
			wantOfficial: 154,
		},
		{
			name:         "update marker texture official",
			packet:       &protocol.Remote_LExecutor_updateMarkerTexture_104{},
			wantOfficial: 155,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOfficial, ok := officialPacketID(tt.packet)
			if !ok {
				t.Fatalf("officialPacketID missing for %T", tt.packet)
			}
			if gotOfficial != tt.wantOfficial {
				t.Fatalf("officialPacketID(%T)=%d want %d", tt.packet, gotOfficial, tt.wantOfficial)
			}

			gotRegistry, ok := srv.Registry.PacketID(tt.packet)
			if !ok {
				t.Fatalf("registry packet id missing for %T", tt.packet)
			}
			if gotRegistry != tt.wantOfficial {
				t.Fatalf("registry packet id(%T)=%d want %d", tt.packet, gotRegistry, tt.wantOfficial)
			}
		})
	}
}

func TestSerializerDefaultReadWriteUseOfficialPacketIDs(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	var buf bytes.Buffer
	if err := srv.Serial.WriteObject(&buf, &protocol.Remote_NetClient_kick_22{Reason: "map changed"}); err != nil {
		t.Fatalf("WriteObject: %v", err)
	}
	packetID, payloadLen, compressed := readSerializerHeader(t, buf.Bytes())
	if packetID != 59 {
		t.Fatalf("default WriteObject packet id=%d want 59", packetID)
	}
	if payloadLen == 0 {
		t.Fatal("expected kick packet payload")
	}
	if compressed != 0 {
		t.Fatalf("expected uncompressed kick packet, got %d", compressed)
	}

	obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, 142, nil))
	if err != nil {
		t.Fatalf("ReadObject: %v", err)
	}
	if _, ok := obj.(*protocol.Remote_InputHandler_unitClear_95); !ok {
		t.Fatalf("default ReadObject decoded %T, want *protocol.Remote_InputHandler_unitClear_95", obj)
	}
}

func TestSerializerUpdateMarkerUsesMindustry157WireID(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	packet := &protocol.Remote_LExecutor_updateMarker_102{
		Id:      77,
		Control: protocol.LMarkerControl(3),
		P1:      1.25,
		P2:      2.5,
		P3:      3.75,
	}

	var buf bytes.Buffer
	if err := srv.Serial.WriteObject(&buf, packet); err != nil {
		t.Fatalf("WriteObject(updateMarker): %v", err)
	}

	packetID, payloadLen, compressed := readSerializerHeader(t, buf.Bytes())
	if packetID != 153 {
		t.Fatalf("updateMarker packet id=%d want 153", packetID)
	}
	if payloadLen == 0 {
		t.Fatal("expected updateMarker payload")
	}
	if compressed != 0 {
		t.Fatalf("expected uncompressed updateMarker packet, got %d", compressed)
	}

	obj, err := srv.Serial.ReadObject(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadObject(updateMarker): %v", err)
	}
	got, ok := obj.(*protocol.Remote_LExecutor_updateMarker_102)
	if !ok {
		t.Fatalf("decoded %T, want *protocol.Remote_LExecutor_updateMarker_102", obj)
	}
	if got.Id != packet.Id || got.Control != packet.Control || got.P1 != packet.P1 || got.P2 != packet.P2 || got.P3 != packet.P3 {
		t.Fatalf("decoded packet mismatch: got %+v want %+v", got, packet)
	}
}

func TestSerializerReadObjectOfficialZeroPayloadClientPackets(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	tests := []struct {
		name     string
		packetID byte
		check    func(t *testing.T, obj any)
	}{
		{
			name:     "official connect confirm",
			packetID: 31,
			check: func(t *testing.T, obj any) {
				t.Helper()
				if _, ok := obj.(*protocol.Remote_NetServer_connectConfirm_50); !ok {
					t.Fatalf("decoded %T, want *protocol.Remote_NetServer_connectConfirm_50", obj)
				}
			},
		},
		{
			name:     "official unit clear",
			packetID: 142,
			check: func(t *testing.T, obj any) {
				t.Helper()
				if _, ok := obj.(*protocol.Remote_InputHandler_unitClear_95); !ok {
					t.Fatalf("decoded %T, want *protocol.Remote_InputHandler_unitClear_95", obj)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, tt.packetID, nil))
			if err != nil {
				t.Fatalf("ReadObject: %v", err)
			}
			tt.check(t, obj)
		})
	}
}

func TestSerializerReadObjectOfficialInjectedPlayerFallbacks(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	t.Run("ping without player", func(t *testing.T) {
		var payload bytes.Buffer
		if err := binary.Write(&payload, binary.BigEndian, int64(12345)); err != nil {
			t.Fatalf("write ping payload: %v", err)
		}
		obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, 72, payload.Bytes()))
		if err != nil {
			t.Fatalf("ReadObject ping fallback: %v", err)
		}
		got, ok := obj.(*protocol.Remote_NetClient_ping_18)
		if !ok {
			t.Fatalf("decoded %T, want *protocol.Remote_NetClient_ping_18", obj)
		}
		if got.Player != nil {
			t.Fatalf("expected nil player, got %T", got.Player)
		}
		if got.Time != 12345 {
			t.Fatalf("expected time 12345, got %d", got.Time)
		}
	})

	t.Run("request block snapshot without player", func(t *testing.T) {
		var payload bytes.Buffer
		if err := binary.Write(&payload, binary.BigEndian, int32(0x00110022)); err != nil {
			t.Fatalf("write requestBlockSnapshot payload: %v", err)
		}
		obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, 81, payload.Bytes()))
		if err != nil {
			t.Fatalf("ReadObject requestBlockSnapshot fallback: %v", err)
		}
		got, ok := obj.(*protocol.Remote_NetServer_requestBlockSnapshot_45)
		if !ok {
			t.Fatalf("decoded %T, want *protocol.Remote_NetServer_requestBlockSnapshot_45", obj)
		}
		if got.Player != nil {
			t.Fatalf("expected nil player, got %T", got.Player)
		}
		if got.Pos != 0x00110022 {
			t.Fatalf("expected pos %#x, got %#x", int32(0x00110022), got.Pos)
		}
	})

	t.Run("request debug status without player", func(t *testing.T) {
		obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, 36, nil))
		if err != nil {
			t.Fatalf("ReadObject requestDebugStatus fallback: %v", err)
		}
		got, ok := obj.(*protocol.Remote_NetServer_requestDebugStatus_36)
		if !ok {
			t.Fatalf("decoded %T, want *protocol.Remote_NetServer_requestDebugStatus_36", obj)
		}
		if got.Player != nil {
			t.Fatalf("expected nil player, got %T", got.Player)
		}
	})
}

func TestSerializerLargeEntitySnapshotStaysUncompressed(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	packet := &protocol.Remote_NetClient_entitySnapshot_32{
		Amount: 1,
		Data:   bytes.Repeat([]byte{0xAB}, 5000),
	}

	var buf bytes.Buffer
	if err := srv.Serial.WriteObject(&buf, packet); err != nil {
		t.Fatalf("WriteObject large entity snapshot: %v", err)
	}

	packetID, payloadLen, compressed := readSerializerHeader(t, buf.Bytes())
	if packetID == 0 {
		t.Fatal("expected non-zero packet id for entity snapshot")
	}
	if payloadLen == 0 {
		t.Fatal("expected encoded entity snapshot payload")
	}
	if compressed != 0 {
		t.Fatalf("expected entity snapshot compression to stay disabled, got %d", compressed)
	}
}

func TestSerializerSnapshotPacketsStayUncompressed(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	tests := []struct {
		name   string
		packet protocol.Packet
	}{
		{
			name: "state snapshot",
			packet: &protocol.Remote_NetClient_stateSnapshot_35{
				WaveTime: 123,
				Wave:     7,
				CoreData: bytes.Repeat([]byte{0xCD}, 5000),
			},
		},
		{
			name: "block snapshot",
			packet: &protocol.Remote_NetClient_blockSnapshot_34{
				Amount: 32,
				Data:   bytes.Repeat([]byte{0xEF}, 5000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := srv.Serial.WriteObject(&buf, tt.packet); err != nil {
				t.Fatalf("WriteObject(%s): %v", tt.name, err)
			}
			packetID, payloadLen, compressed := readSerializerHeader(t, buf.Bytes())
			if packetID == 0 {
				t.Fatalf("expected non-zero packet id for %s", tt.name)
			}
			if payloadLen == 0 {
				t.Fatalf("expected encoded payload for %s", tt.name)
			}
			if compressed != 0 {
				t.Fatalf("expected %s compression to stay disabled, got %d", tt.name, compressed)
			}
		})
	}
}
