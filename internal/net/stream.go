package net

import (
	"bytes"
	"errors"

	"mdt-server/internal/protocol"
)

var ErrStreamIncomplete = errors.New("stream_incomplete")

// StreamBuilder accumulates StreamChunk payloads until complete.
type StreamBuilder struct {
	ID       int32
	Total    int32
	Type     byte
	Buffer   *bytes.Buffer
	Registry *protocol.PacketRegistry
}

func NewStreamBuilder(begin *protocol.StreamBegin, reg *protocol.PacketRegistry) *StreamBuilder {
	return &StreamBuilder{
		ID:       begin.ID,
		Total:    begin.Total,
		Type:     begin.Type,
		Buffer:   &bytes.Buffer{},
		Registry: reg,
	}
}

func (b *StreamBuilder) Add(chunk *protocol.StreamChunk) {
	b.Buffer.Write(chunk.Data)
}

func (b *StreamBuilder) Done() bool {
	return int32(b.Buffer.Len()) >= b.Total
}

func (b *StreamBuilder) Build() (protocol.Packet, error) {
	if !b.Done() {
		return nil, ErrStreamIncomplete
	}
	p, err := b.Registry.NewPacket(b.Type)
	if err != nil {
		return nil, err
	}
	if err := p.Read(protocol.NewReader(b.Buffer.Bytes()), int(b.Total)); err != nil {
		return nil, err
	}
	return p, nil
}
