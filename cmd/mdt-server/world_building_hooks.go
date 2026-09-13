package main

import (
	"strings"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func handleCoreBuildingControlSelectHook(srv *netserver.Server, wld *world.World, c *netserver.Conn, info world.BuildingInfo) {
	if srv == nil || wld == nil || c == nil || c.IsDead() || c.UnitID() == 0 {
		return
	}
	if !strings.HasPrefix(info.Name, "core-") || info.Team != resolveConnTeam(c, wld) {
		return
	}
	_ = srv.HandleCoreBuildingControlSelect(c, protocol.UnpackPoint2(info.Pos))
}

func handlePlayerPayloadBuildingControlSelectHook(srv *netserver.Server, wld *world.World, c *netserver.Conn, info world.BuildingInfo) {
	if srv == nil || wld == nil || c == nil || c.IsDead() || c.UnitID() == 0 {
		return
	}
	unitID := c.UnitID()
	if !wld.ControlSelectPayloadUnitPacked(info.Pos, unitID) {
		return
	}
	srv.ConsumeConnUnit(c, unitID)
	broadcastRelatedBlockSnapshots(srv, wld, info.Pos)
}

func handleUnitPayloadBuildingControlSelectHook(srv *netserver.Server, wld *world.World, c *netserver.Conn, info world.BuildingInfo, unitID int32) {
	if srv == nil || wld == nil || unitID == 0 {
		return
	}
	if !wld.ControlSelectPayloadUnitPacked(info.Pos, unitID) {
		return
	}
	if c != nil && c.UnitID() == unitID {
		srv.ConsumeConnUnit(c, unitID)
	}
	broadcastRelatedBlockSnapshots(srv, wld, info.Pos)
}

func bindWorldBuildingHooks(srv *netserver.Server, wld *world.World) {
	if srv == nil || wld == nil {
		return
	}

	srv.OnCommandBuilding = func(c *netserver.Conn, buildings []int32, target protocol.Vec2) {
		wld.CommandBuildingsPacked(buildings, target)
		for _, pos := range buildings {
			broadcastRelatedBlockSnapshots(srv, wld, pos)
		}
	}
	srv.OnTileConfig = func(c *netserver.Conn, pos int32, value any) {
		wld.ConfigureBuildingPacked(pos, value)
		if normalized, ok := wld.BuildingConfigPacked(pos); ok {
			srv.BroadcastTileConfig(pos, normalized, c)
		} else {
			srv.BroadcastTileConfig(pos, value, c)
		}
		broadcastRelatedBlockSnapshots(srv, wld, pos)
	}
	srv.OnRotateBlock = func(c *netserver.Conn, pos int32, direction bool) {
		res, ok := wld.RotateBuildingPacked(pos, direction)
		if !ok {
			return
		}
		broadcastSetTile(srv, pos, res.BlockID, res.Rotation, byte(res.Team))
		if effectID, ok := lookupEffectID("rotateblock"); ok {
			broadcastEffectReliable(srv, effectID, res.EffectX, res.EffectY, res.EffectRot)
		}
		broadcastRelatedBlockSnapshots(srv, wld, pos)
	}
	srv.OnBuildingControlSelect = func(c *netserver.Conn, pos int32) {
		if c == nil || !wld.CanControlSelectBuildingPacked(pos) {
			return
		}
		info, ok := wld.BuildingInfoPacked(pos)
		if !ok {
			return
		}
		if strings.HasPrefix(info.Name, "core-") {
			handleCoreBuildingControlSelectHook(srv, wld, c, info)
			return
		}
		handlePlayerPayloadBuildingControlSelectHook(srv, wld, c, info)
	}
	srv.OnUnitBuildingControlSelect = func(c *netserver.Conn, unitID, pos int32) {
		if unitID == 0 || !wld.CanControlSelectBuildingPacked(pos) {
			return
		}
		info, ok := wld.BuildingInfoPacked(pos)
		if !ok || strings.HasPrefix(info.Name, "core-") {
			return
		}
		handleUnitPayloadBuildingControlSelectHook(srv, wld, c, info, unitID)
	}
}
