package core

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ipcEnvelope struct {
	ID      uint64          `json:"id"`
	Type    string          `json:"type"`
	Method  string          `json:"method,omitempty"`
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type ipcClient struct {
	conn net.Conn
	mu   sync.Mutex
	seq  atomic.Uint64
}

func newIPCClient(conn net.Conn) *ipcClient {
	return &ipcClient{conn: conn}
}

func (c *ipcClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ipcClient) Call(method string, req any, resp any) error {
	return c.call(method, req, resp, 0)
}

func (c *ipcClient) CallWithTimeout(method string, req any, resp any, timeout time.Duration) error {
	return c.call(method, req, resp, timeout)
}

func (c *ipcClient) call(method string, req any, resp any, timeout time.Duration) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("ipc client not connected")
	}
	var payload []byte
	if req != nil {
		raw, err := json.Marshal(req)
		if err != nil {
			return err
		}
		payload = raw
	}
	env := ipcEnvelope{
		ID:      c.seq.Add(1),
		Type:    "request",
		Method:  method,
		Payload: payload,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if timeout > 0 {
		if err := c.conn.SetDeadline(time.Now().Add(timeout)); err == nil {
			defer func() {
				_ = c.conn.SetDeadline(time.Time{})
			}()
		}
	}

	if err := writeIPCEnvelope(c.conn, env); err != nil {
		return err
	}
	reply, err := readIPCEnvelope(c.conn)
	if err != nil {
		return err
	}
	if reply.ID != env.ID {
		return fmt.Errorf("ipc response id mismatch: got=%d want=%d", reply.ID, env.ID)
	}
	if reply.Error != "" {
		return fmt.Errorf(reply.Error)
	}
	if resp != nil && len(reply.Payload) > 0 {
		if err := json.Unmarshal(reply.Payload, resp); err != nil {
			return err
		}
	}
	return nil
}

func readIPCEnvelope(r io.Reader) (ipcEnvelope, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return ipcEnvelope{}, err
	}
	if length == 0 {
		return ipcEnvelope{}, fmt.Errorf("ipc frame length is zero")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return ipcEnvelope{}, err
	}
	var env ipcEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return ipcEnvelope{}, err
	}
	return env, nil
}

func writeIPCEnvelope(w io.Writer, env ipcEnvelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
