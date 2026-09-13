package protocol

import (
	"encoding/base64"
	"hash/crc32"
	"testing"
)

func TestConnectPacketWriteReadRoundTrip(t *testing.T) {
	reg := NewRegistry()
	ctx := NewContentRegistry().Context()
	ser := &struct {
		*PacketRegistry
	}{PacketRegistry: reg}
	_ = ser

	original := &ConnectPacket{
		Version:     157,
		VersionType: "official",
		Mods:        []string{"mod-a", "mod-b"},
		Name:        "probe",
		Locale:      "en_US",
		UUID:        "AAAAAAAAAAAAAAAAAAAAAA==",
		USID:        "usid-probe",
		Mobile:      true,
		Color:       0x11223344,
	}

	writer := NewWriterWithContext(ctx)
	if err := original.Write(writer); err != nil {
		t.Fatalf("write: %v", err)
	}

	var decoded ConnectPacket
	if err := decoded.Read(NewReaderWithContext(writer.Bytes(), ctx), len(writer.Bytes())); err != nil {
		t.Fatalf("read: %v", err)
	}

	if decoded.Version != original.Version ||
		decoded.VersionType != original.VersionType ||
		decoded.Name != original.Name ||
		decoded.Locale != original.Locale ||
		decoded.UUID != original.UUID ||
		decoded.USID != original.USID ||
		decoded.Mobile != original.Mobile ||
		decoded.Color != original.Color {
		t.Fatalf("decoded packet mismatch: got=%+v want=%+v", decoded, *original)
	}
	if len(decoded.Mods) != len(original.Mods) {
		t.Fatalf("decoded mods len mismatch: got=%d want=%d", len(decoded.Mods), len(original.Mods))
	}
	for i := range original.Mods {
		if decoded.Mods[i] != original.Mods[i] {
			t.Fatalf("decoded mod[%d] mismatch: got=%q want=%q", i, decoded.Mods[i], original.Mods[i])
		}
	}
}

func TestConnectPacketReadAcceptsLegacyLayoutWithoutCRC(t *testing.T) {
	uuidRaw, err := base64.StdEncoding.DecodeString("AAAAAAAAAAAAAAAAAAAAAA==")
	if err != nil {
		t.Fatalf("decode uuid: %v", err)
	}

	w := NewWriter()
	if err := w.WriteInt32(157); err != nil {
		t.Fatalf("write version: %v", err)
	}
	versionType := "official"
	name := "probe"
	locale := "en_US"
	usid := "usid-probe"
	if err := w.WriteStringNullable(&versionType); err != nil {
		t.Fatalf("write versionType: %v", err)
	}
	if err := w.WriteStringNullable(&name); err != nil {
		t.Fatalf("write name: %v", err)
	}
	if err := w.WriteStringNullable(&locale); err != nil {
		t.Fatalf("write locale: %v", err)
	}
	if err := w.WriteStringNullable(&usid); err != nil {
		t.Fatalf("write usid: %v", err)
	}
	if err := w.WriteBytes(uuidRaw[:16]); err != nil {
		t.Fatalf("write uuid: %v", err)
	}
	if err := w.WriteByte(1); err != nil {
		t.Fatalf("write mobile: %v", err)
	}
	if err := w.WriteInt32(0x11223344); err != nil {
		t.Fatalf("write color: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write mod count: %v", err)
	}

	var decoded ConnectPacket
	if err := decoded.Read(NewReaderWithContext(w.Bytes(), nil), len(w.Bytes())); err != nil {
		t.Fatalf("read legacy layout: %v", err)
	}
	if decoded.Version != 157 || decoded.VersionType != "official" || decoded.Name != "probe" || decoded.Locale != "en_US" || decoded.USID != "usid-probe" {
		t.Fatalf("decoded legacy packet mismatch: %+v", decoded)
	}
	if !decoded.Mobile || decoded.Color != 0x11223344 {
		t.Fatalf("decoded legacy mobile/color mismatch: %+v", decoded)
	}
}

func TestConnectPacketWriteKeepsOfficialUUIDAndCRCLayout(t *testing.T) {
	const rawUUID = "AQIDBAUGBwgJCgsMDQ4PEA=="
	packet := &ConnectPacket{
		Version:     157,
		VersionType: "official",
		Name:        "probe",
		Locale:      "en_US",
		UUID:        rawUUID,
		USID:        "usid-probe",
		Mobile:      true,
		Color:       0x11223344,
		Mods:        []string{"mod-a"},
	}

	writer := NewWriter()
	if err := packet.Write(writer); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	reader := NewReader(writer.Bytes())
	for _, label := range []string{"version", "versionType", "name", "locale", "usid"} {
		switch label {
		case "version":
			if _, err := reader.ReadInt32(); err != nil {
				t.Fatalf("read %s: %v", label, err)
			}
		default:
			if _, err := reader.ReadStringNullable(); err != nil {
				t.Fatalf("read %s: %v", label, err)
			}
		}
	}

	uuidBytes, err := reader.ReadBytes(16)
	if err != nil {
		t.Fatalf("read uuid bytes: %v", err)
	}
	crcWire, err := reader.ReadInt64()
	if err != nil {
		t.Fatalf("read crc slot: %v", err)
	}
	mobile, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("read mobile: %v", err)
	}
	color, err := reader.ReadInt32()
	if err != nil {
		t.Fatalf("read color: %v", err)
	}
	modCount, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("read mod count: %v", err)
	}

	wantUUID, err := base64.StdEncoding.DecodeString(rawUUID)
	if err != nil {
		t.Fatalf("decode expected uuid: %v", err)
	}
	if got, want := string(uuidBytes), string(wantUUID[:16]); got != want {
		t.Fatalf("uuid bytes mismatch: got=%v want=%v", uuidBytes, wantUUID[:16])
	}
	checksum := crc32.ChecksumIEEE(wantUUID[:16])
	if uint32(crcWire) != checksum {
		t.Fatalf("crc slot mismatch: got=%d want=%d", uint32(crcWire), checksum)
	}
	if mobile != 1 {
		t.Fatalf("expected mobile byte 1, got %d", mobile)
	}
	if color != 0x11223344 {
		t.Fatalf("expected color 0x11223344, got %#x", color)
	}
	if modCount != 1 {
		t.Fatalf("expected mod count 1, got %d", modCount)
	}
}
