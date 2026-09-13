package main

import (
	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func bindWorldSnapshotHooks(
	srv *netserver.Server,
	wld *world.World,
	unitCommands *unitCommandService,
	playerUnitTypeFn func() int16,
	currentTraceCfg func() config.TracepointsConfig,
	logTrace func(category, point string, fields map[string]any),
) {
	if srv == nil || wld == nil {
		return
	}

	if playerUnitTypeFn != nil {
		srv.PlayerUnitTypeFn = playerUnitTypeFn
	}

	srv.StateSnapshotFn = func() *protocol.Remote_NetClient_stateSnapshot_35 {
		snap := wld.Snapshot()
		if currentTraceCfg != nil {
			tc := currentTraceCfg()
			if tc.Enabled && tc.StateBuildEnabled && logTrace != nil {
				logTrace("state_build", "build_state_snapshot", map[string]any{
					"wave":      snap.Wave,
					"wave_time": snap.WaveTime,
					"tick":      snap.Tick,
					"time_data": snap.TimeData,
					"tps":       snap.Tps,
				})
			}
		}
		return netserver.BuildStateSnapshotFromWorld(wld)
	}

	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		entities := wld.EntitySyncSnapshots(srv.Content, srv.PlayerUnitIDSet())
		if unitCommands != nil {
			entities = unitCommands.overlay(entities)
		}
		return entities, nil
	}
}
