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
	officialPacketRegistry   = protocol.NewRegistry()
)

// Serializer implements the packet framing expected by the official protocol.
type Serializer struct {
	Registry *protocol.PacketRegistry
	Ctx      *protocol.TypeIOContext
}

// officialPacketID uses the canonical 157 packet registration order.
func officialPacketID(p protocol.Packet) (byte, bool) {
	return officialPacketRegistry.PacketID(p)
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

	tryRead := func(pid byte) (any, bool) {
		p, nerr := s.Registry.NewPacket(pid)
		if nerr != nil {
			return nil, false
		}
		if rerr := p.Read(protocol.NewReaderWithContext(payload, s.Ctx), int(length)); rerr != nil {
			return nil, false
		}
		return p, true
	}

	if obj, ok := tryRead(id); ok {
		return obj, nil
	}

	// A few official 157 client->server packets omit the injected player/entity
	// parameter on the wire even though the generated packet struct keeps it.
	switch id {
	case 31:
		return &protocol.Remote_NetServer_connectConfirm_50{}, nil
	case 36:
		return &protocol.Remote_NetServer_requestDebugStatus_36{Player: nil}, nil
	case 72:
		return readClientPingWithoutPlayer(payload)
	case 81:
		return readRequestBlockSnapshotWithoutPlayer(payload)
	case 142:
		return &protocol.Remote_InputHandler_unitClear_95{}, nil
	default:
		return nil, fmt.Errorf("unknown packet id: %d", id)
	}
}

// WriteObject writes a framed object to buf.
func (s *Serializer) WriteObject(buf *bytes.Buffer, obj any) error {
	switch v := obj.(type) {
	case protocol.FrameworkMessage:
		buf.WriteByte(0xFE)
		return writeFramework(buf, v)
	case protocol.Packet:
		id, ok := officialPacketID(v)
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
		useCompression := shouldCompressPacket(v, len(payload))
		if useCompression {
			if compressed, ok := tryCompressPayload(payload); ok {
				if err := writeUint16(buf, uint16(len(payload))); err != nil {
					return err
				}
				buf.WriteByte(1)
				_, err := buf.Write(compressed)
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

func shouldCompressPacket(p protocol.Packet, payloadLen int) bool {
	if payloadLen < 36 || isStreamChunk(p) {
		return false
	}
	switch p.(type) {
	case *protocol.Remote_NetClient_entitySnapshot_32,
		*protocol.Remote_NetClient_hiddenSnapshot_33,
		*protocol.Remote_NetClient_stateSnapshot_35,
		*protocol.Remote_NetClient_blockSnapshot_34:
		// Snapshot packets are latency-sensitive, and large snapshot payloads have
		// triggered lz4 crashes in practice. Favor stability over a few saved bytes.
		return false
	default:
		return true
	}
}

func tryCompressPayload(payload []byte) (compressed []byte, ok bool) {
	defer func() {
		if recover() != nil {
			compressed = nil
			ok = false
		}
	}()
	bound := lz4.CompressBlockBound(len(payload))
	if bound <= 0 {
		return nil, false
	}
	// The upstream encoder has panicked on exact-bound buffers for a few large
	// snapshot-shaped payloads. Keep a little slack instead of trusting the
	// minimal bound exactly.
	dst := make([]byte, bound+16)
	n, err := lz4.CompressBlock(payload, dst, nil)
	if err != nil || n <= 0 || n >= len(payload) {
		return nil, false
	}
	return dst[:n], true
}

func readClientPingWithoutPlayer(payload []byte) (*protocol.Remote_NetClient_ping_18, error) {
	r := bytes.NewReader(payload)
	value, err := readInt64(r)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("unexpected ping payload length: %d", len(payload))
	}
	return &protocol.Remote_NetClient_ping_18{Player: nil, Time: value}, nil
}

func readRequestBlockSnapshotWithoutPlayer(payload []byte) (*protocol.Remote_NetServer_requestBlockSnapshot_45, error) {
	r := bytes.NewReader(payload)
	pos, err := readInt32(r)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("unexpected requestBlockSnapshot payload length: %d", len(payload))
	}
	return &protocol.Remote_NetServer_requestBlockSnapshot_45{Player: nil, Pos: pos}, nil
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

func readInt64(r *bytes.Reader) (int64, error) {
	var v int64
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
