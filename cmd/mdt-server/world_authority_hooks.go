package main

import (
	netserver "mdt-server/internal/net"
	"mdt-server/internal/world"
)

func buildInfoToControlled(info world.BuildingInfo) netserver.ControlledBuildInfo {
	return netserver.ControlledBuildInfo{
		Pos:    info.Pos,
		X:      float32(info.X*8 + 4),
		Y:      float32(info.Y*8 + 4),
		TeamID: byte(info.Team),
	}
}

func bindWorldAuthorityHooks(srv *netserver.Server, wld *world.World) {
	if srv == nil || wld == nil {
		return
	}

	srv.ClaimControlledBuildFn = func(playerID int32, buildPos int32) (netserver.ControlledBuildInfo, bool) {
		info, ok := wld.ClaimControlledBuildingPacked(playerID, buildPos)
		if !ok {
			return netserver.ControlledBuildInfo{}, false
		}
		return buildInfoToControlled(info), true
	}
	srv.ControlledBuildInfoFn = func(playerID int32, buildPos int32) (netserver.ControlledBuildInfo, bool) {
		info, ok := wld.ControlledBuildingInfoPacked(playerID, buildPos)
		if !ok {
			return netserver.ControlledBuildInfo{}, false
		}
		return buildInfoToControlled(info), true
	}
	srv.ReleaseControlledBuildFn = func(playerID int32, buildPos int32) bool {
		if buildPos != 0 && wld.ReleaseControlledBuildingPacked(playerID, buildPos) {
			return true
		}
		return wld.ReleaseControlledBuildingByPlayer(playerID)
	}
	srv.SetControlledBuildInputFn = func(playerID int32, buildPos int32, aimX, aimY float32, shooting bool) bool {
		return wld.SetControlledBuildingInputPacked(playerID, buildPos, aimX, aimY, shooting)
	}

	// clientSnapshot writes client motion/position back into the authoritative world model.
	// This mirrors vanilla NetServer.clientSnapshot behavior; without it, entitySnapshot will
	// keep snapping units back to stale positions.
	srv.SetUnitMotionFn = func(unitID int32, vx, vy, rotVel float32) bool {
		_, ok := wld.SetEntityMotion(unitID, vx, vy, rotVel)
		return ok
	}
	srv.SetUnitPositionFn = func(unitID int32, x, y, rotation float32) bool {
		_, ok := wld.SetEntityPosition(unitID, x, y, rotation)
		return ok
	}
	srv.SetUnitRuntimeStateFn = func(unitID int32, state netserver.UnitRuntimeState) bool {
		_, ok := wld.SetEntityRuntimeState(unitID, state.Shooting, state.Boosting, state.UpdateBuilding, state.MineTilePos, state.Plans)
		return ok
	}
	srv.SetUnitStackFn = func(unitID int32, itemID int16, amount int32) bool {
		_, ok := wld.SetEntityStack(unitID, world.ItemID(itemID), amount)
		return ok
	}
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		_, ok := wld.SetEntityPlayerController(unitID, playerID)
		return ok
	}

	srv.OnRequestUnitPayload = func(c *netserver.Conn, targetID int32) {
		if c == nil || c.UnitID() == 0 || targetID == 0 {
			return
		}
		_, _ = wld.RequestUnitPayload(c.UnitID(), targetID)
	}
	srv.OnRequestBuildPayload = func(c *netserver.Conn, buildPos int32) {
		if c == nil || c.UnitID() == 0 || buildPos < 0 {
			return
		}
		_, _ = wld.RequestBuildPayloadPacked(c.UnitID(), buildPos)
	}
	srv.OnRequestDropPayload = func(c *netserver.Conn, x, y float32) {
		if c == nil || c.UnitID() == 0 {
			return
		}
		_, _ = wld.RequestDropPayload(c.UnitID(), x, y)
	}
	srv.OnRequestBlockSnapshot = func(c *netserver.Conn, pos int32) {
		if c == nil {
			return
		}
		if info, ok := wld.BuildingInfoPacked(pos); ok && info.Team != resolveConnTeam(c, wld) {
			return
		}
		sendRequestedBlockSnapshotToConn(srv, c, wld, pos)
	}
	srv.OnRequestItem = func(c *netserver.Conn, pos int32, itemID int16, amount int32) {
		if c == nil || c.UnitID() == 0 || amount <= 0 {
			return
		}
		result, ok := wld.RequestItemFromBuildingPacked(c.UnitID(), pos, world.ItemID(itemID), amount)
		if !ok || result.Amount <= 0 {
			return
		}
		broadcastTakeItems(srv, pos, itemID, result.Amount, result.UnitID)
	}
	srv.OnTransferInventory = func(c *netserver.Conn, pos int32) {
		if c == nil || c.UnitID() == 0 {
			return
		}
		result, ok := wld.TransferUnitInventoryToBuildingPacked(c.UnitID(), pos)
		if !ok || result.Amount <= 0 {
			return
		}
		broadcastTransferItemTo(srv, result.UnitID, int16(result.Item), result.Amount, result.UnitX, result.UnitY, pos)
	}
	srv.OnDropItem = func(c *netserver.Conn, angle float32) {
		if c == nil || c.UnitID() == 0 || c.PlayerID() == 0 {
			return
		}
		if _, ok := wld.DropUnitItems(c.UnitID()); !ok {
			return
		}
		broadcastDropItem(srv, c.PlayerID(), angle)
	}
	srv.OnUnitEnteredPayload = func(c *netserver.Conn, unitID, buildPos int32) {
		if unitID == 0 || buildPos < 0 {
			return
		}
		if !wld.EnterUnitPayloadPacked(buildPos, unitID) {
			return
		}
		if c != nil && c.UnitID() == unitID {
			srv.ConsumeConnUnit(c, unitID)
		}
	}
}
