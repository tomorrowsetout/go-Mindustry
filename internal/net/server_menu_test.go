package net

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func readFramedPacketForMenuTest(t *testing.T, conn net.Conn) (byte, []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		t.Fatalf("read packet length failed: %v", err)
	}
	size := int(lenBuf[0])<<8 | int(lenBuf[1])
	if size <= 0 {
		t.Fatalf("invalid framed packet length %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read framed packet failed: %v", err)
	}
	r := bytes.NewReader(payload)
	packetID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet id failed: %v", err)
	}
	frameLen := make([]byte, 2)
	if _, err := io.ReadFull(r, frameLen); err != nil {
		t.Fatalf("read packet payload length failed: %v", err)
	}
	frameSize := int(frameLen[0])<<8 | int(frameLen[1])
	compressed, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read compression flag failed: %v", err)
	}
	if compressed != 0 {
		t.Fatalf("unexpected compressed packet: %d", compressed)
	}
	data := make([]byte, frameSize)
	if _, err := io.ReadFull(r, data); err != nil {
		t.Fatalf("read packet payload failed: %v", err)
	}
	return packetID, data
}

func TestHandlePacketInvokesOnMenuChoose(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
	conn := &Conn{}

	called := 0
	var gotMenuID, gotOption int32
	srv.OnMenuChoose = func(c *Conn, menuID, option int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotMenuID = menuID
		gotOption = option
	}

	srv.handlePacket(conn, &protocol.Remote_Menus_menuChoose_105{
		MenuId: 77,
		Option: 2,
	}, true)

	if called != 1 {
		t.Fatalf("expected OnMenuChoose to be called once, got %d", called)
	}
	if gotMenuID != 77 || gotOption != 2 {
		t.Fatalf("expected menu choice (77,2), got (%d,%d)", gotMenuID, gotOption)
	}
}

func TestSendMenuEmitsMenuPacket(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.hasConnected = true

	srv.SendMenu(conn, 99, "Title", "Message", [][]string{{"公告", "链接"}, {"帮助", "关闭"}})

	packetID, payload := readFramedPacketForMenuTest(t, clientSide)
	if packetID == 0 {
		t.Fatal("expected non-zero packet id for menu packet")
	}
	packet := &protocol.Remote_Menus_menu_62{}
	if err := packet.Read(protocol.NewReader(payload), 0); err != nil {
		t.Fatalf("decode menu packet failed: %v", err)
	}
	if packet.MenuId != 99 {
		t.Fatalf("expected menu id 99, got %d", packet.MenuId)
	}
	if packet.Title != "Title" || packet.Message != "Message" {
		t.Fatalf("unexpected menu title/message: %+v", packet)
	}
	if len(packet.Options) != 2 || len(packet.Options[0]) != 2 || packet.Options[1][0] != "帮助" {
		t.Fatalf("unexpected menu options: %#v", packet.Options)
	}
}
