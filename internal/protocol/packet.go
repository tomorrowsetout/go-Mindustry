package protocol

import (
	"io"
	"reflect"
)

const (
	PriorityLow    = 0
	PriorityNormal = 1
	PriorityHigh   = 2
)

type Packet interface {
	Read(r *Reader, length int) error
	Write(w *Writer) error
	Priority() int
}

// PacketFactory creates new packet instances by ID.
type PacketFactory func() Packet

// PacketRegistry mirrors mindustry.net.Net registration order.
type PacketRegistry struct {
	factories []PacketFactory
}

func NewRegistry(buildVersion int) *PacketRegistry {
	r := &PacketRegistry{}
	r.Register(func() Packet { return &StreamBegin{} })
	r.Register(func() Packet { return &StreamChunk{} })
	r.Register(func() Packet { return &WorldStream{} })
	r.Register(func() Packet { return &ConnectPacket{} })
	// generated packets from @Remote (Call.registerPackets)
	initRemotePackets(r)
	return r
}

func (r *PacketRegistry) Register(f PacketFactory) {
	r.factories = append(r.factories, f)
}

func (r *PacketRegistry) NewPacket(id byte) (Packet, error) {
	idx := int(id)
	if idx < 0 || idx >= len(r.factories) {
		return nil, io.ErrUnexpectedEOF
	}
	return r.factories[idx](), nil
}

func (r *PacketRegistry) PacketID(p Packet) (byte, bool) {
	for i, f := range r.factories {
		if f() == nil {
			continue
		}
		if sameType(p, f()) {
			return byte(i), true
		}
	}
	return 0, false
}

func (r *PacketRegistry) Count() int {
	return len(r.factories)
}

func sameType(a, b Packet) bool {
	return a != nil && b != nil && (reflect.TypeOf(a) == reflect.TypeOf(b))
}
