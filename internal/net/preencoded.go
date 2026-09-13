package net

import (
	"mdt-server/internal/protocol"
)

// preEncodedPacket carries an already-serialized packet payload shared across
// connections. Broadcast paths serialize the object once and fan out the
// resulting bytes, avoiding N redundant WriteObject passes for N recipients.
// The original object is retained for tracing hooks (onSend) and per-type
// send statistics.
//
// The payload is the inner frame produced by Serializer.WriteObject (packet
// id + uint16 length + compression flag + body); the TCP writer prepends the
// outer 2-byte length, the UDP writer sends the bytes as one datagram — both
// identical to the per-connection serialization they replace.
type preEncodedPacket struct {
	obj     any
	payload []byte
}

// Priority implements protocol.Packet so async send queues keep the original
// packet's priority instead of demoting the wrapper to normal.
func (p *preEncodedPacket) Priority() int {
	if pkt, ok := p.obj.(protocol.Packet); ok {
		return pkt.Priority()
	}
	return protocol.PriorityNormal
}

// unwrapPreEncoded returns the original object behind a pre-encoded wrapper.
func unwrapPreEncoded(obj any) any {
	if pre, ok := obj.(*preEncodedPacket); ok {
		return pre.obj
	}
	return obj
}

// frameIDs extracts the packet/framework id from a serialized inner frame.
func frameIDs(payload []byte) (packetID, frameworkID int) {
	packetID, frameworkID = -1, -1
	if len(payload) >= 1 {
		if payload[0] == 0xFE {
			if len(payload) >= 2 {
				frameworkID = int(payload[1])
			}
		} else {
			packetID = int(payload[0])
		}
	}
	return packetID, frameworkID
}

// preEncode serializes an object once with the server serializer. Returns
// ok=false when serialization fails; callers then fall back to per-connection
// encoding.
func (s *Server) preEncode(obj any) (*preEncodedPacket, bool) {
	if s == nil || s.Serial == nil {
		return nil, false
	}
	buf := getSendBuffer()
	defer putSendBuffer(buf)
	if err := s.Serial.WriteObject(buf, obj); err != nil {
		return nil, false
	}
	payload := append(make([]byte, 0, buf.Len()), buf.Bytes()...)
	return &preEncodedPacket{obj: obj, payload: payload}, true
}
