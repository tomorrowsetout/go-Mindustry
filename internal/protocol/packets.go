package protocol

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
)

// Connect is a generic client connect event (server-side only in Java).
type Connect struct {
	AddressTCP string
}

func (p *Connect) Read(r *Reader, _ int) error  { return nil }
func (p *Connect) Write(w *Writer) error        { return nil }
func (p *Connect) Priority() int                { return PriorityHigh }

// Disconnect is a generic disconnect event.
type Disconnect struct {
	Reason string
}

func (p *Disconnect) Read(r *Reader, _ int) error { return nil }
func (p *Disconnect) Write(w *Writer) error       { return nil }
func (p *Disconnect) Priority() int               { return PriorityHigh }

// StreamBegin marks a stream.
type StreamBegin struct {
	ID    int32
	Total int32
	Type  byte
}

func (p *StreamBegin) Read(r *Reader, _ int) error {
	id, err := r.ReadInt32()
	if err != nil {
		return err
	}
	total, err := r.ReadInt32()
	if err != nil {
		return err
	}
	t, err := r.ReadByte()
	if err != nil {
		return err
	}
	p.ID = id
	p.Total = total
	p.Type = t
	return nil
}

func (p *StreamBegin) Write(w *Writer) error {
	if err := w.WriteInt32(p.ID); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Total); err != nil {
		return err
	}
	return w.WriteByte(p.Type)
}

func (p *StreamBegin) Priority() int { return PriorityHigh }

// StreamChunk is a chunk of a stream.
type StreamChunk struct {
	ID   int32
	Data []byte
}

func (p *StreamChunk) Read(r *Reader, _ int) error {
	id, err := r.ReadInt32()
	if err != nil {
		return err
	}
	l, err := r.ReadInt16()
	if err != nil {
		return err
	}
	data, err := r.ReadBytes(int(l))
	if err != nil {
		return err
	}
	p.ID = id
	p.Data = data
	return nil
}

func (p *StreamChunk) Write(w *Writer) error {
	if err := w.WriteInt32(p.ID); err != nil {
		return err
	}
	if err := w.WriteInt16(int16(len(p.Data))); err != nil {
		return err
	}
	return w.WriteBytes(p.Data)
}

func (p *StreamChunk) Priority() int { return PriorityHigh }

// WorldStream is a marker for a streaming packet payload.
type WorldStream struct{}

func (p *WorldStream) Read(r *Reader, _ int) error { return nil }
func (p *WorldStream) Write(w *Writer) error       { return nil }
func (p *WorldStream) Priority() int               { return PriorityNormal }

// ConnectPacket mirrors mindustry.net.Packets.ConnectPacket.
type ConnectPacket struct {
	Version     int32
	VersionType string
	Mods        []string
	Name        string
	Locale      string
	UUID        string
	USID        string
	Mobile      bool
	Color       int32
}

func (p *ConnectPacket) Read(r *Reader, _ int) error {
	version, err := r.ReadInt32()
	if err != nil {
		return err
	}
	versionType, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	name, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	locale, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	usid, err := r.ReadStringNullable()
	if err != nil {
		return err
	}
	uuidBytes, err := r.ReadBytes(16)
	if err != nil {
		return err
	}
	mobileByte, err := r.ReadByte()
	if err != nil {
		return err
	}
	color, err := r.ReadInt32()
	if err != nil {
		return err
	}
	modCount, err := r.ReadByte()
	if err != nil {
		return err
	}

	p.Version = version
	if versionType != nil {
		p.VersionType = *versionType
	}
	if name != nil {
		p.Name = *name
	}
	if locale != nil {
		p.Locale = *locale
	}
	if usid != nil {
		p.USID = *usid
	}
	p.UUID = base64.StdEncoding.EncodeToString(uuidBytes)
	p.Mobile = mobileByte == 1
	p.Color = color

	// BOUNDS CHECK: Limit mods count to prevent OOM
	if modCount > 128 {
		return fmt.Errorf("too many mods: %d (max 128)", modCount)
	}

	if modCount > 0 {
		p.Mods = make([]string, 0, modCount)
		for i := 0; i < int(modCount); i++ {
			ms, err := r.ReadStringNullable()
			if err != nil {
				return err
			}
			if ms != nil {
				p.Mods = append(p.Mods, *ms)
			} else {
				p.Mods = append(p.Mods, "")
			}
		}
	}
	return nil
}

func (p *ConnectPacket) Write(w *Writer) error {
	if err := w.WriteInt32(p.Version); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.VersionType); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Name); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.Locale); err != nil {
		return err
	}
	if err := w.WriteStringNullable(&p.USID); err != nil {
		return err
	}

	uuidBytes, err := base64.StdEncoding.DecodeString(p.UUID)
	if err != nil {
		uuidBytes = make([]byte, 16)
	}
	if len(uuidBytes) < 16 {
		padded := make([]byte, 16)
		copy(padded, uuidBytes)
		uuidBytes = padded
	}

	// Java writes 16 bytes UUID + 8 bytes CRC32, but only reads 16 bytes from client.
	// We write CRC32 to match Java's write behavior exactly.
	if err := w.WriteBytes(uuidBytes[:16]); err != nil {
		return err
	}
	crc := crc32.NewIEEE()
	_, _ = crc.Write(uuidBytes[:16])
	if err := w.WriteInt64(int64(crc.Sum32())); err != nil {
		return err
	}

	if err := w.WriteByte(boolToByte(p.Mobile)); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Color); err != nil {
		return err
	}

	if err := w.WriteByte(byte(len(p.Mods))); err != nil {
		return err
	}
	for _, m := range p.Mods {
		if err := w.WriteStringNullable(&m); err != nil {
			return err
		}
	}
	return nil
}

func (p *ConnectPacket) Priority() int { return PriorityHigh }

func boolToByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
