package net

import (
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func BuildCoreSnapshotDataFromTeams(snapshots []world.TeamCoreItemSnapshot) []byte {
	w := protocol.NewWriter()
	if err := w.WriteByte(byte(len(snapshots))); err != nil {
		return []byte{0}
	}
	for _, snapshot := range snapshots {
		if err := w.WriteByte(byte(snapshot.Team)); err != nil {
			return []byte{0}
		}
		if err := w.WriteInt16(int16(len(snapshot.Items))); err != nil {
			return []byte{0}
		}
		for _, stack := range snapshot.Items {
			if err := w.WriteInt16(int16(stack.Item)); err != nil {
				return []byte{0}
			}
			if err := w.WriteInt32(stack.Amount); err != nil {
				return []byte{0}
			}
		}
	}
	return append([]byte(nil), w.Bytes()...)
}

func BuildCoreSnapshotDataFromWorld(wld *world.World) []byte {
	if wld == nil {
		return []byte{0}
	}
	return BuildCoreSnapshotDataFromTeams(wld.TeamCoreItemSnapshots())
}

func BuildStateSnapshotFromWorld(wld *world.World) *protocol.Remote_NetClient_stateSnapshot_35 {
	if wld == nil {
		return &protocol.Remote_NetClient_stateSnapshot_35{CoreData: []byte{0}}
	}
	snap := wld.Snapshot()
	return &protocol.Remote_NetClient_stateSnapshot_35{
		WaveTime: snap.WaveTimeTicks(),
		Wave:     snap.Wave,
		Enemies:  snap.Enemies,
		Paused:   snap.Paused,
		GameOver: snap.GameOver,
		TimeData: snap.TimeData,
		Tps:      byte(snap.Tps),
		Rand0:    snap.Rand0,
		Rand1:    snap.Rand1,
		CoreData: BuildCoreSnapshotDataFromWorld(wld),
	}
}
