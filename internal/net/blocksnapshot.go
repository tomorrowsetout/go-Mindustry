package net

import (
	"fmt"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

const MaxBlockSnapshotPayloadBytes = 800

const maxBlockSnapshotPayloadBytes = MaxBlockSnapshotPayloadBytes

func encodeBlockSnapshotEntry(snap world.BlockSyncSnapshot) ([]byte, bool) {
	if snap.BlockID <= 0 || len(snap.Data) == 0 {
		return nil, false
	}
	writer := protocol.NewWriter()
	_ = writer.WriteInt32(snap.Pos)
	_ = writer.WriteInt16(snap.BlockID)
	_ = writer.WriteBytes(snap.Data)
	return append([]byte(nil), writer.Bytes()...), true
}

func BuildIsolatedBlockSnapshotPackets(snaps []world.BlockSyncSnapshot) []*protocol.Remote_NetClient_blockSnapshot_34 {
	if len(snaps) == 0 {
		return nil
	}
	packets := make([]*protocol.Remote_NetClient_blockSnapshot_34, 0, len(snaps))
	for _, snap := range snaps {
		entry, ok := encodeBlockSnapshotEntry(snap)
		if !ok {
			continue
		}
		packets = append(packets, &protocol.Remote_NetClient_blockSnapshot_34{
			Amount: 1,
			Data:   entry,
		})
	}
	return packets
}

func BuildBlockSnapshotPackets(snaps []world.BlockSyncSnapshot) []*protocol.Remote_NetClient_blockSnapshot_34 {
	if len(snaps) == 0 {
		return nil
	}
	packets := make([]*protocol.Remote_NetClient_blockSnapshot_34, 0, (len(snaps)/12)+1)
	writer := protocol.NewWriter()
	amount := int16(0)
	flush := func() {
		if amount <= 0 {
			return
		}
		packets = append(packets, &protocol.Remote_NetClient_blockSnapshot_34{
			Amount: amount,
			Data:   append([]byte(nil), writer.Bytes()...),
		})
		writer = protocol.NewWriter()
		amount = 0
	}
	for _, snap := range snaps {
		entry, ok := encodeBlockSnapshotEntry(snap)
		if !ok {
			continue
		}
		if len(entry) > maxBlockSnapshotPayloadBytes {
			flush()
			packets = append(packets, &protocol.Remote_NetClient_blockSnapshot_34{
				Amount: 1,
				Data:   entry,
			})
			continue
		}
		if amount > 0 && len(writer.Bytes())+len(entry) > maxBlockSnapshotPayloadBytes {
			flush()
		}
		_ = writer.WriteBytes(entry)
		amount++
	}
	flush()
	return packets
}

func (s *Server) SyncBlockSnapshotsToConn(c *Conn, snaps []world.BlockSyncSnapshot) error {
	if s == nil || c == nil {
		return fmt.Errorf("invalid connection")
	}
	for _, packet := range BuildBlockSnapshotPackets(snaps) {
		if packet == nil {
			continue
		}
		if err := s.sendUnreliable(c, packet); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) SyncIsolatedBlockSnapshotsToConn(c *Conn, snaps []world.BlockSyncSnapshot) error {
	if s == nil || c == nil {
		return fmt.Errorf("invalid connection")
	}
	for _, packet := range BuildIsolatedBlockSnapshotPackets(snaps) {
		if packet == nil {
			continue
		}
		if err := s.sendUnreliable(c, packet); err != nil {
			return err
		}
	}
	return nil
}
