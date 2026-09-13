// Package harness provides the real-time client test framework for mdt-server.
//
// SimClient emulates an official Mindustry build-158 client over a real TCP
// connection. It drives the exact vanilla join/sync handshake (ConnectPacket
// -> WorldStream -> connectConfirm -> post-connect snapshots) so the Go server
// can be verified against the official wire protocol without a GUI client.
//
// Wire fidelity is guaranteed by reusing the server's own *net.Serializer:
// every byte the client writes and reads follows the same framing, byte order,
// and (optional) lz4 compression the official server uses.
package harness

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	mdtnet "mdt-server/internal/net"
	"mdt-server/internal/protocol"
)

// SimClient is a programmatic build-158 client.
type SimClient struct {
	conn   net.Conn
	addr   string
	serial *mdtnet.Serializer

	name   string
	uuid   string
	usid   string
	locale string

	mu       sync.Mutex
	closed   bool
	closeErr error

	trace *CompatTrace
	recv  chan recvItem
}

type recvItem struct {
	obj any
	id  byte
}

// NewSimClient builds a client targeting addr (e.g. "127.0.0.1:6567"). It uses
// a fresh build-158 registry/context constructed exactly as the server does in
// NewServer, so framing is byte-identical.
func NewSimClient(addr string) *SimClient {
	content := protocol.NewContentRegistry()
	reg := protocol.NewRegistry()
	return &SimClient{
		addr:   addr,
		serial: &mdtnet.Serializer{Registry: reg, Ctx: content.Context()},
		name:   "sim-bot",
		uuid:   fmt.Sprintf("sim-%d", time.Now().UnixNano()),
		usid:   "sim-usid",
		locale: "en",
		trace:  NewCompatTrace(),
		recv:   make(chan recvItem, 512),
	}
}

// Connect dials the server, starts the read loop, and performs the exact
// build-158 handshake: it acknowledges the server's RegisterTCP framework
// message, then sends the ConnectPacket. The server replies with the WorldStream.
func (c *SimClient) Connect() error {
	conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	go c.readLoop()

	// Official clients must echo the server's RegisterTCP before any
	// application traffic is accepted. Wait for it, then reply with the same ID.
	rt, ok := c.WaitFor(func(o any) bool {
		_, ok := o.(*protocol.RegisterTCP)
		return ok
	}, 5*time.Second)
	if !ok {
		return fmt.Errorf("harness: server did not send RegisterTCP on accept")
	}
	if err := c.Send(&protocol.RegisterTCP{ConnectionID: rt.(*protocol.RegisterTCP).ConnectionID}); err != nil {
		return err
	}

	cp := &protocol.ConnectPacket{
		Version:     159,
		VersionType: "official",
		Name:        c.name,
		Locale:      c.locale,
		UUID:        c.uuid,
		USID:        c.usid,
		Mobile:      false,
		Color:       0,
	}
	return c.Send(cp)
}

// Send frames and writes a single packet to the server, using the exact wire
// envelope the server expects: a 2-byte big-endian total length followed by the
// Serializer's WriteObject output.
func (c *SimClient) Send(obj any) error {
	var b bytes.Buffer
	if err := c.serial.WriteObject(&b, obj); err != nil {
		return err
	}
	payload := b.Bytes()
	lenbuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenbuf, uint16(len(payload)))
	if _, err := c.conn.Write(lenbuf); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

// Confirm sends the connectConfirm packet that unlocks post-connect sync.
func (c *SimClient) Confirm() error {
	return c.Send(&protocol.Remote_NetServer_connectConfirm_50{})
}

// SendClientSnapshot sends a minimal player clientSnapshot (position only).
func (c *SimClient) SendClientSnapshot(x, y float32) error {
	return c.Send(&protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 1,
		UnitID:     0,
		X:          x,
		Y:          y,
		Rotation:   0,
	})
}

// --- Inbound-gap coverage senders: mirror the vanilla client for the 14
//     previously-unhandled client->server packets so the harness can lock
//     their wire routing and server-side hooks. ---

// SendTileTap emulates a player tapping a tile (build/inspect input).
func (c *SimClient) SendTileTap(tilePos int32) error {
	return c.Send(&protocol.Remote_InputHandler_tileTap_91{Tile: protocol.TileBox{PosValue: tilePos}})
}

// SendTextInput emulates a menu text-input response from the client.
func (c *SimClient) SendTextInput(id int32, title, message string, numeric, allowEmpty bool) error {
	return c.Send(&protocol.Remote_Menus_textInput_111{
		TextInputId: id, Title: title, Message: message,
		TextLength: int32(len(message)), Numeric: numeric, AllowEmpty: allowEmpty,
	})
}

// SendCopyToClipboard emulates the client asking the server to copy text.
func (c *SimClient) SendCopyToClipboard(text string) error {
	return c.Send(&protocol.Remote_Menus_copyToClipboard_129{Text: text})
}

// SendDebugStatus emulates the client reporting its debug/perf status.
// `sent` (snapshots sent count) is tracked server-side and not part of the wire
// packet, so it is accepted for API symmetry but not transmitted.
func (c *SimClient) SendDebugStatus(value, lastSnapshot, sent int32) error {
	_ = sent
	return c.Send(&protocol.Remote_NetServer_debugStatusClient_37{
		Value: value, LastClientSnapshot: lastSnapshot,
	})
}

// SendClientPlanSnapshotReceived emulates the client acking a build plan group.
func (c *SimClient) SendClientPlanSnapshotReceived(groupID int32) error {
	return c.Send(&protocol.Remote_NetServer_clientPlanSnapshotReceived_47{GroupId: groupID})
}

// SendServerRelay emulates the client relaying a server packet (e.g. logic).
func (c *SimClient) SendServerRelay(typ, contents string) error {
	return c.Send(&protocol.Remote_NetServer_serverPacketReliable_39{Type: typ, Contents: contents})
}

// Trace returns the packet-trace recorder (full ordered history).
func (c *SimClient) Trace() *CompatTrace { return c.trace }

// Close tears down the connection.
func (c *SimClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// WaitFor blocks until an object matching pred is received or timeout elapses.
func (c *SimClient) WaitFor(pred func(obj any) bool, timeout time.Duration) (any, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case item, ok := <-c.recv:
			if !ok {
				return nil, false
			}
			if pred(item.obj) {
				return item.obj, true
			}
		case <-deadline:
			return nil, false
		}
	}
}

// WaitWorldStream waits for the full WorldStream (StreamBegin + all chunks) and
// returns the reassembled world payload. This proves the stream round-trips.
func (c *SimClient) WaitWorldStream(timeout time.Duration) ([]byte, bool) {
	deadline := time.After(timeout)
	var begin *protocol.StreamBegin
	var total int32
	var got int32
	var buf []byte
	for {
		select {
		case item, ok := <-c.recv:
			if !ok {
				return nil, false
			}
			switch v := item.obj.(type) {
			case *protocol.StreamBegin:
				begin = v
				total = v.Total
			case *protocol.StreamChunk:
				if begin != nil && v.ID == begin.ID {
					buf = append(buf, v.Data...)
					got += int32(len(v.Data))
					if total > 0 && got >= total {
						return buf, true
					}
				}
			}
		case <-deadline:
			return nil, false
		}
	}
}

func (c *SimClient) readLoop() {
	defer close(c.recv)
	for {
		obj, id, err := c.readOne()
		if err != nil {
			c.mu.Lock()
			if c.closeErr == nil {
				c.closeErr = err
			}
			c.mu.Unlock()
			return
		}
		c.trace.Record(id, obj)
		select {
		case c.recv <- recvItem{obj: obj, id: id}:
		default:
		}
	}
}

// readOne reads exactly one framed object from the socket and decodes it with
// the server's own Serializer so wire fidelity is guaranteed. It mirrors
// internal/net/server.go Conn.ReadObject exactly:
//   - the wire is [2-byte BE length n][WriteObject output of n bytes]
//   - WriteObject output is either a framework message (0xFE + id + body) or a
//     regular packet (packet-id + uint16 length + compression byte + payload)
func (c *SimClient) readOne() (any, byte, error) {
	for {
		lenbuf := make([]byte, 2)
		if _, err := io.ReadFull(c.conn, lenbuf); err != nil {
			return nil, 0, err
		}
		n := binary.BigEndian.Uint16(lenbuf)
		if n == 0 {
			continue // skip zero-length (keepalive) frames
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return nil, 0, err
		}
		obj, err := c.serial.ReadObject(bytes.NewReader(payload))
		if err != nil {
			return nil, 0, err
		}
		id := byte(0)
		if payload[0] == 0xFE {
			if len(payload) > 1 {
				id = payload[1]
			}
		} else {
			id = payload[0]
		}
		return obj, id, nil
	}
}
