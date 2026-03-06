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

func compatPacketID(p protocol.Packet) (byte, bool) {
	switch p.(type) {
	case *protocol.Remote_NetClient_entitySnapshot_32:
		return 43, true
	case *protocol.Remote_NetClient_hiddenSnapshot_33:
		return 46, true
	case *protocol.Remote_NetClient_pingResponse_19:
		return 66, true
	case *protocol.Remote_NetClient_kick_22:
		return 53, true
	case *protocol.Remote_NetClient_kick_21:
		return 54, true
	case *protocol.Remote_NetClient_playerDisconnect_31:
		return 67, true
	case *protocol.Remote_CoreBlock_playerSpawn_140:
		return 68, true
	case *protocol.Remote_NetClient_sendMessage_15:
		return 82, true
	case *protocol.Remote_NetClient_sendMessage_14:
		return 83, true
	case *protocol.Remote_NetClient_stateSnapshot_35:
		return 117, true
	case *protocol.Remote_Tile_buildHealthUpdate_135:
		return 13, true
	case *protocol.Remote_InputHandler_unitClear_91:
		return 133, true
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
	case 65: // PingCallPacket (custom 155.4, payload is int64 only)
		if len(payload) >= 8 {
			t := int64(binary.BigEndian.Uint64(payload[:8]))
			return &protocol.Remote_NetClient_ping_18{Time: t}, nil
		}
	case 81: // SendChatMessageCallPacket(message)
		if msg, err := readSendChatMessageCompat(payload, s.Ctx); err == nil {
			return msg, nil
		}
	case 133: // UnitClearCallPacket (client->server carries no fields)
		return &CompatUnitClearPacket{}, nil
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
