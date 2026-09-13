package net

import (
	"net"
	"testing"
	"time"
)

type capturedUDPPacket struct {
	payload []byte
	addr    *net.UDPAddr
}

type udpCapture struct {
	ch chan capturedUDPPacket
}

func installUDPCapture(s *Server) *udpCapture {
	capture := &udpCapture{ch: make(chan capturedUDPPacket, 16)}
	s.udpWriteFn = func(payload []byte, addr *net.UDPAddr) (int, error) {
		cp := append([]byte(nil), payload...)
		capture.ch <- capturedUDPPacket{payload: cp, addr: addr}
		return len(payload), nil
	}
	return capture
}

func (c *udpCapture) read(t *testing.T) capturedUDPPacket {
	t.Helper()
	select {
	case packet := <-c.ch:
		return packet
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected udp packet")
		return capturedUDPPacket{}
	}
}

func (c *udpCapture) assertEmpty(t *testing.T) {
	t.Helper()
	select {
	case packet := <-c.ch:
		t.Fatalf("unexpected udp packet to %v payload=%v", packet.addr, packet.payload)
	case <-time.After(50 * time.Millisecond):
	}
}
