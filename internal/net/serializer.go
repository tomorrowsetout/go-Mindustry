package net

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/pierrec/lz4/v4"

	"mdt-server/internal/protocol"
)

var (
	ErrCompressedUnsupported = errors.New("lz4_compression_not_supported")
)

// Serializer mirrors ArcNetProvider.PacketSerializer framing.
type Serializer struct {
	Registry *protocol.PacketRegistry
	Ctx      *protocol.TypeIOContext
}

// CompatIgnoredPacket is returned for client packets that are intentionally
// ignored under custom-client compatibility mode.
type CompatIgnoredPacket struct {
	ID     byte
	Length int
	Payload []byte
}

// CompatUnitClearPacket represents UnitClearCallPacket from client (id=133),
// which carries no payload on client->server direction.
type CompatUnitClearPacket struct{}

// compatPacketID returns a re-mapped packet ID for compatibility with certain clients.
// Most server->client packets use their standard IDs without re-mapping.
// Only少数 exceptional cases need re-mapping.
func compatPacketID(p protocol.Packet) (byte, bool) {
	switch p.(type) {
	case *protocol.Remote_NetClient_stateSnapshot_35:
		return 117, true  // Alternative ID for state snapshot
	// NOTE: Removed incorrect mappings that confused client/server directions:
	// - entitySnapshot (43): client->server, not server->client
	// - hiddenSnapshot (46): client->server, not server->client
	// - pingResponse (66): should use standard ID, but ping uses different packet
	// - kick (53, 54): client->server packets, not server->client
	// - playerDisconnect (67): client->client, not server->client
	// - playerSpawn (68): server->client, uses standard ID 140
	// - sendMessage (82, 83): client->server packets, not server->client
	// - buildHealthUpdate (135): server->client, uses standard ID 135
	// - unitClear (91): client->server, uses standard ID 91
	}
	return 0, false
}

// ReadObject reads a single framed object from buf.
func (s *Serializer) ReadObject(buf *bytes.Reader) (any, error) {
	id, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if id == 0xFE { // -2 in signed byte
		return readFramework(buf)
	}

	length, err := readUint16(buf)
	if err != nil {
		return nil, err
	}
	comp, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	payload := make([]byte, length)
	if comp == 0 {
		if _, err := io.ReadFull(buf, payload); err != nil {
			return nil, err
		}
	} else if comp == 1 {
		// Java uses remaining bytes in the packet as compressed payload.
		compressed := make([]byte, buf.Len())
		if _, err := io.ReadFull(buf, compressed); err != nil {
			return nil, err
		}
		if _, err := lz4.UncompressBlock(compressed, payload); err != nil {
			return nil, err
		}
	} else {
		return nil, ErrCompressedUnsupported
	}

	// Compatibility path: user custom 155.4 client observed IDs.
	switch id {
	case 29: // ConnectConfirmCallPacket (custom 155.4)
		return &protocol.Remote_NetServer_connectConfirm_47{}, nil
	case 24: // ClientSnapshotCallPacket (custom 155.4)
		if snap, err := readClientSnapshotCompat(payload, s.Ctx); err == nil {
			return snap, nil
		}
	case 64: // SetLiquidCallPacket (client->server)
		return &protocol.Remote_InputHandler_setLiquid_64{}, nil
	case 65: // PingCallPacket (custom 155.4, payload is int64 only)
		if len(payload) >= 8 {
			t := int64(binary.BigEndian.Uint64(payload[:8]))
			return &protocol.Remote_NetClient_ping_18{Time: t}, nil
		}
	case 81: // SendChatMessageCallPacket(message)
		if msg, err := readSendChatMessageCompat(payload, s.Ctx); err == nil {
			return msg, nil
		}
	case 91: // UnitClearCallPacket (client->server, standard ID)
		return &protocol.Remote_InputHandler_unitClear_91{}, nil
	case 133: // UnitClearCallPacket (client->server, alternative ID for compat)
		return &CompatUnitClearPacket{}, nil
	case 128: // SetFloorCallPacket (server->client)
		return &protocol.Remote_Tile_setFloor_128{}, nil
	case 129: // SetOverlayCallPacket (server->client)
		return &protocol.Remote_Tile_setOverlay_129{}, nil
	case 130: // RemoveTileCallPacket (server->client)
		return &protocol.Remote_Tile_removeTile_130{}, nil
	case 131: // SetTileCallPacket (server->client)
		return &protocol.Remote_Tile_setTile_131{}, nil
	case 132: // SetTeamCallPacket (server->client)
		return &protocol.Remote_Tile_setTeam_132{}, nil
	case 134: // BuildDestroyedCallPacket (server->client)
		return &protocol.Remote_Tile_buildDestroyed_134{}, nil
	case 136: // DeconstructFinishCallPacket (server->client)
		return &protocol.Remote_ConstructBlock_deconstructFinish_136{}, nil
	case 138: // LandingPadLandedCallPacket (server->client)
		return &protocol.Remote_LandingPad_landingPadLanded_138{}, nil
	case 139: // AutoDoorToggleCallPacket (server->client)
		return &protocol.Remote_AutoDoor_autoDoorToggle_139{}, nil
	// Standard server→client packets that should be processed directly
	case 32: // EntitySnapshotCallPacket (server->client)
		return &protocol.Remote_NetClient_entitySnapshot_32{}, nil
	case 33: // HiddenSnapshotCallPacket (server->client)
		return &protocol.Remote_NetClient_hiddenSnapshot_33{}, nil
	case 35: // StateSnapshotCallPacket (server->client)
		return &protocol.Remote_NetClient_stateSnapshot_35{}, nil
	case 45: // ClientSnapshotCallPacket (server->client)
		return &protocol.Remote_NetServer_clientSnapshot_45{}, nil
	case 47: // ConnectConfirmCallPacket (server->client)
		return &protocol.Remote_NetServer_connectConfirm_47{}, nil
	case 117: // StateSnapshotCallPacket (server->client, alternative ID)
		return &protocol.Remote_NetClient_stateSnapshot_35{}, nil
	case 135: // BuildHealthUpdateCallPacket (server->client)
		return &protocol.Remote_Tile_buildHealthUpdate_135{}, nil
	case 137: // ConstructFinishCallPacket (server->client)
		return &protocol.Remote_ConstructBlock_constructFinish_137{}, nil
	case 140: // PlayerSpawnCallPacket (server->client)
		return &protocol.Remote_CoreBlock_playerSpawn_140{}, nil
	case 141: // AssemblerUnitSpawnedCallPacket (server->client)
		return &protocol.Remote_UnitAssembler_assemblerUnitSpawned_141{}, nil
	case 142: // AssemblerDroneSpawnedCallPacket (server->client)
		return &protocol.Remote_UnitAssembler_assemblerDroneSpawned_142{}, nil
	case 143: // UnitBlockSpawnCallPacket (server->client)
		return &protocol.Remote_UnitBlock_unitBlockSpawn_143{}, nil
	case 144: // UnitTetherBlockSpawnedCallPacket (server->client)
		return &protocol.Remote_UnitCargoLoader_unitTetherBlockSpawned_144{}, nil
	}
	// For this custom client, many other IDs are currently not aligned with
	// generated registry order. Ignore them instead of mis-parsing and forcing
	// disconnects.
	if id >= 4 {
		return &CompatIgnoredPacket{ID: id, Length: int(length), Payload: payload}, nil
	}

	p, err := s.Registry.NewPacket(id)
	if err != nil {
		return nil, err
	}
	if err := p.Read(protocol.NewReaderWithContext(payload, s.Ctx), int(length)); err != nil {
		// Compatibility fallback for older modified mappings.
		if id == 25 {
			if snap, ferr := readClientSnapshotCompat(payload, s.Ctx); ferr == nil {
				return snap, nil
			}
		}
		return nil, err
	}
	return p, nil
}

func readClientSnapshotCompat(payload []byte, ctx *protocol.TypeIOContext) (*protocol.Remote_NetServer_clientSnapshot_45, error) {
	r := protocol.NewReaderWithContext(payload, ctx)
	out := &protocol.Remote_NetServer_clientSnapshot_45{}
	var err error
	if out.SnapshotID, err = r.ReadInt32(); err != nil {
		return nil, err
	}
	if out.UnitID, err = r.ReadInt32(); err != nil {
		return nil, err
	}
	if out.Dead, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if out.X, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.Y, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.PointerX, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.PointerY, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.Rotation, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.BaseRotation, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.XVelocity, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.YVelocity, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.Mining, err = protocol.ReadTile(r, ctx); err != nil {
		return nil, err
	}
	if out.Boosting, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if out.Shooting, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if out.Chatting, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if out.Building, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if out.SelectedBlock, err = protocol.ReadBlock(r, ctx); err != nil {
		return nil, err
	}
	if out.SelectedRotation, err = r.ReadInt32(); err != nil {
		return nil, err
	}
	plans, err := protocol.ReadPlansQueue(r, ctx)
	if err != nil {
		return nil, err
	}
	out.Plans = plans
	if out.ViewX, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.ViewY, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.ViewWidth, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	if out.ViewHeight, err = r.ReadFloat32(); err != nil {
		return nil, err
	}
	return out, nil
}

// WriteObject writes a framed object to buf.
func (s *Serializer) WriteObject(buf *bytes.Buffer, obj any) error {
	switch v := obj.(type) {
	case protocol.FrameworkMessage:
		buf.WriteByte(0xFE)
		return writeFramework(buf, v)
	case protocol.Packet:
		id, ok := compatPacketID(v)
		if !ok {
			id, ok = s.Registry.PacketID(v)
		}
		if !ok {
			return fmt.Errorf("unknown packet type: %T", v)
		}
		buf.WriteByte(id)

		w := protocol.NewWriterWithContext(s.Ctx)
		if err := v.Write(w); err != nil {
			return err
		}
		payload := w.Bytes()

		// Compression: only when payload is large enough and not a stream chunk.
		useCompression := len(payload) >= 36 && !isStreamChunk(v)
		if useCompression {
			dst := make([]byte, lz4.CompressBlockBound(len(payload)))
			if n, err := lz4.CompressBlock(payload, dst, nil); err == nil && n > 0 && n < len(payload) {
				if err := writeUint16(buf, uint16(len(payload))); err != nil {
					return err
				}
				buf.WriteByte(1)
				_, err = buf.Write(dst[:n])
				return err
			}
		}

		if err := writeUint16(buf, uint16(len(payload))); err != nil {
			return err
		}
		buf.WriteByte(0)
		_, err := buf.Write(payload)
		return err
	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
}

func readSendChatMessageCompat(payload []byte, ctx *protocol.TypeIOContext) (*protocol.Remote_NetClient_sendChatMessage_16, error) {
	r := protocol.NewReaderWithContext(payload, ctx)
	msg, err := protocol.ReadString(r)
	if err != nil {
		return nil, err
	}
	out := &protocol.Remote_NetClient_sendChatMessage_16{}
	if msg != nil {
		out.Message = *msg
	}
	return out, nil
}

func readFramework(buf *bytes.Reader) (protocol.FrameworkMessage, error) {
	id, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	switch id {
	case protocol.FrameworkPing:
		i, err := readInt32(buf)
		if err != nil {
			return nil, err
		}
		reply, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		return &protocol.Ping{ID: i, IsReply: reply == 1}, nil
	case protocol.FrameworkDiscover:
		return &protocol.DiscoverHost{}, nil
	case protocol.FrameworkKeepAlive:
		return &protocol.KeepAlive{}, nil
	case protocol.FrameworkRegisterUD:
		i, err := readInt32(buf)
		if err != nil {
			return nil, err
		}
		return &protocol.RegisterUDP{ConnectionID: i}, nil
	case protocol.FrameworkRegisterTC:
		i, err := readInt32(buf)
		if err != nil {
			return nil, err
		}
		return &protocol.RegisterTCP{ConnectionID: i}, nil
	default:
		return nil, fmt.Errorf("unknown framework id: %d", id)
	}
}

func writeFramework(buf *bytes.Buffer, msg protocol.FrameworkMessage) error {
	id := msg.FrameworkID()
	buf.WriteByte(id)
	switch v := msg.(type) {
	case *protocol.Ping:
		if err := writeInt32(buf, v.ID); err != nil {
			return err
		}
		if v.IsReply {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	case *protocol.DiscoverHost:
	case *protocol.KeepAlive:
	case *protocol.RegisterUDP:
		if err := writeInt32(buf, v.ConnectionID); err != nil {
			return err
		}
	case *protocol.RegisterTCP:
		if err := writeInt32(buf, v.ConnectionID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown framework type: %T", msg)
	}
	return nil
}

func readUint16(r *bytes.Reader) (uint16, error) {
	var v uint16
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func writeUint16(w *bytes.Buffer, v uint16) error {
	return binary.Write(w, binary.BigEndian, v)
}

func readInt32(r *bytes.Reader) (int32, error) {
	var v int32
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func writeInt32(w *bytes.Buffer, v int32) error {
	return binary.Write(w, binary.BigEndian, v)
}

func isStreamChunk(p protocol.Packet) bool {
	_, ok := p.(*protocol.StreamChunk)
	return ok
}
