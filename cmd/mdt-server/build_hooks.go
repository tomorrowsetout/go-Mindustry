package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/buildsvc"
	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

// buildHookState owns the bookkeeping shared between the inbound build-plan
// hooks and the world event loop goroutine.
type buildHookState struct {
	cfg               *config.Config
	detailLog         *detailedLogWriter
	wld               *world.World
	unitCmds          *unitCommandService
	service           *buildsvc.Service
	gameTPS           int
	logSnapshots      func() bool
	logPlace          func() bool
	logFinish         func() bool
	logBreakStart     func() bool
	logBreakDone      func() bool
	fileLogPlace      func() bool
	fileLogFinish     func() bool
	fileLogBreakStart func() bool
	fileLogBreakDone  func() bool

	snapshotLogMu sync.Mutex
	snapshotLogBy map[int32]snapshotLogKey
	ownerActorMu  sync.RWMutex
	ownerActorBy  map[int32]string
	// planPresence tracks whether this owner's last clientSnapshot carried
	// build plans, so Q-cancel (empty queue) can clear server pending state.
	planPresenceMu sync.Mutex
	planPresenceBy map[int32]planPresenceState
}

type planPresenceState struct {
	hadPlans  bool
	lastSeen  time.Time
}

type snapshotLogKey struct {
	count    int
	breaking bool
	x        int32
	y        int32
	blockID  int16
}

func newBuildHookState(
	cfg *config.Config,
	detailLog *detailedLogWriter,
	wld *world.World,
	unitCmds *unitCommandService,
	service *buildsvc.Service,
	gameTPS int,
) *buildHookState {
	return &buildHookState{
		cfg:       cfg,
		detailLog: detailLog,
		wld:       wld,
		unitCmds:  unitCmds,
		service:   service,
		gameTPS:   gameTPS,
		logSnapshots: func() bool {
			return cfg.Building.Enabled && cfg.Development.BuildSnapshotLogsEnabled
		},
		logPlace: func() bool {
			return cfg.Building.Enabled && cfg.Development.BuildPlaceLogsEnabled
		},
		logFinish: func() bool {
			return cfg.Building.Enabled && cfg.Development.BuildFinishLogsEnabled
		},
		logBreakStart: func() bool {
			return cfg.Building.Enabled && cfg.Development.BuildBreakStartLogsEnabled
		},
		logBreakDone: func() bool {
			return cfg.Building.Enabled && cfg.Development.BuildBreakDoneLogsEnabled
		},
		fileLogPlace: func() bool {
			return cfg.Sundries.BuildPlaceLogsEnabled
		},
		fileLogFinish: func() bool {
			return cfg.Sundries.BuildFinishLogsEnabled
		},
		fileLogBreakStart: func() bool {
			return cfg.Sundries.BuildBreakStartLogsEnabled
		},
		fileLogBreakDone: func() bool {
			return cfg.Sundries.BuildBreakDoneLogsEnabled
		},
		snapshotLogBy: make(map[int32]snapshotLogKey),
		ownerActorBy:  make(map[int32]string),
		planPresenceBy: make(map[int32]planPresenceState),
	}
}

// maybeReconcileEmptyPlanSnapshot clears server pending plans when the official
// client has emptied its builder queue (Q / clearBuilding). Empty snapshots are
// otherwise no-ops to survive connect-time omissions, so only cancel shortly
// after this owner actually had plans.
func (bs *buildHookState) maybeReconcileEmptyPlanSnapshot(owner int32, team world.TeamID) {
	if owner == 0 || bs.wld == nil {
		return
	}
	now := time.Now()
	bs.planPresenceMu.Lock()
	st := bs.planPresenceBy[owner]
	bs.planPresenceMu.Unlock()
	if !st.hadPlans {
		return
	}
	if now.Sub(st.lastSeen) > 5*time.Second {
		return
	}
	if !bs.wld.HasPendingPlansForOwner(owner) {
		return
	}
	_ = bs.wld.ApplyBuildPlanSnapshotForOwner(owner, team, nil)
	if bs.logSnapshots() {
		fmt.Printf("[buildtrace] reconcile empty client queue owner=%d team=%d cancelled-pending\n", owner, team)
	}
}

func (bs *buildHookState) notePlanPresence(owner int32, hadPlans bool) {
	if owner == 0 {
		return
	}
	bs.planPresenceMu.Lock()
	st := bs.planPresenceBy[owner]
	st.hadPlans = hadPlans
	if hadPlans {
		st.lastSeen = time.Now()
	}
	bs.planPresenceBy[owner] = st
	bs.planPresenceMu.Unlock()
}

func (bs *buildHookState) buildActor(owner int32, team world.TeamID) string {
	if owner != 0 {
		bs.ownerActorMu.RLock()
		actor := strings.TrimSpace(bs.ownerActorBy[owner])
		bs.ownerActorMu.RUnlock()
		if actor != "" {
			return actor
		}
	}
	return fmt.Sprintf("team-%d", team)
}

func (bs *buildHookState) rememberBuildOwner(c *netserver.Conn, owner int32) {
	if c == nil || owner == 0 {
		return
	}
	bs.ownerActorMu.Lock()
	bs.ownerActorBy[owner] = displayPlayerName(c)
	bs.ownerActorMu.Unlock()
}

// bindBuildPlanHooks wires the inbound build-plan related hooks.
func (bs *buildHookState) bindBuildPlanHooks(srv *netserver.Server) {
	cfg := bs.cfg
	wld := bs.wld
	buildService := bs.service

	srv.OnBuildPlanSnapshot = func(c *netserver.Conn, plans []*protocol.BuildPlan) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		team := resolveConnTeam(c, wld)
		syncBuilderStateFromConnSnapshot(wld, c, owner, team, plans, false)
		bs.rememberBuildOwner(c, owner)
		if len(plans) == 0 {
			// Reconcile while presence still shows prior plans; then mark empty.
			bs.maybeReconcileEmptyPlanSnapshot(owner, team)
			bs.notePlanPresence(owner, false)
			key := snapshotLogKey{count: 0}
			bs.snapshotLogMu.Lock()
			prev, ok := bs.snapshotLogBy[c.PlayerID()]
			changed := !ok || prev != key
			if changed {
				bs.snapshotLogBy[c.PlayerID()] = key
			}
			bs.snapshotLogMu.Unlock()
			if changed && bs.logSnapshots() && !cfg.Building.Translated {
				fmt.Printf("[buildtrace] recv snapshot player=%d remote=%s count=0\n", c.PlayerID(), c.RemoteAddr().String())
			}
		} else {
			bs.notePlanPresence(owner, true)
			first := plans[0]
			blockID := int16(0)
			if first != nil && !first.Breaking && first.Block != nil {
				blockID = first.Block.ID()
			}
			if first != nil {
				key := snapshotLogKey{
					count:    len(plans),
					breaking: first.Breaking,
					x:        first.X,
					y:        first.Y,
					blockID:  blockID,
				}
				bs.snapshotLogMu.Lock()
				prev, ok := bs.snapshotLogBy[c.PlayerID()]
				changed := !ok || prev != key
				if changed {
					bs.snapshotLogBy[c.PlayerID()] = key
				}
				bs.snapshotLogMu.Unlock()
				if changed && bs.logSnapshots() {
					if cfg.Building.Translated {
						action := "建造"
						if first.Breaking {
							action = "拆除"
						}
						fmt.Printf("[建筑] 玩家=%s 快照队列=%d 首项=(x%d-y%d) 动作=%s block=%d(%s) team=%d\n",
							displayPlayerName(c), len(plans), first.X, first.Y, action, blockID, blockDisplayName(wld, blockID), team)
					} else {
						fmt.Printf("[buildtrace] recv snapshot player=%d remote=%s count=%d first_break=%v first_xy=(%d,%d) first_block=%d\n",
							c.PlayerID(), c.RemoteAddr().String(), len(plans), first.Breaking, first.X, first.Y, blockID)
					}
				}
			}
		}
		buildService.SyncPlans(owner, team, plans)
	}
	srv.OnDeletePlans = func(c *netserver.Conn, positions []int32) {
		owner := resolveBuildOwner(c)
		if c != nil && len(positions) > 0 && bs.logSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] recv deletePlans player=%d remote=%s count=%d\n", c.PlayerID(), c.RemoteAddr().String(), len(positions))
		}
		buildService.CancelPositions(owner, positions)
		wld.CancelBuildPlansPackedForOwner(owner, positions)
	}
	srv.OnRemoveQueueBlock = func(c *netserver.Conn, x, y int32, breaking bool) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		if cfg.Building.Translated {
			action := "取消建造"
			if breaking {
				action = "取消拆除"
			}
			fmt.Printf("[建筑] 玩家=%s (x%d-y%d) %s\n", displayPlayerName(c), x, y, action)
		}
		if bs.logSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] recv removeQueue player=%d remote=%s xy=(%d,%d) breaking=%v\n", c.PlayerID(), c.RemoteAddr().String(), x, y, breaking)
		}
		buildService.CancelPositions(owner, []int32{protocol.PackPoint2(x, y)})
		wld.CancelBuildAtForOwner(owner, x, y, breaking)
	}
	srv.OnOfficialUnitClear = func(c *netserver.Conn) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		// Official Q/unitClear empties the client builder queue; mirror on server
		// so stale pendingBreaks do not fire when the player next mines/builds.
		buildService.ClearOwner(owner)
		wld.ClearBuilderState(owner)
		wld.CancelBuildPlansByOwner(owner)
		if bs.logSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] unitClear cancel plans player=%d owner=%d\n", c.PlayerID(), owner)
		}
	}
	srv.OnCommandUnits = func(c *netserver.Conn, unitIDs []int32, buildTarget any, unitTarget any, posTarget any, queueCommand bool, _ bool) {
		bs.unitCmds.applyCommandUnits(c, wld, unitIDs, buildTarget, unitTarget, posTarget, queueCommand)
	}
	srv.OnSetUnitCommand = func(c *netserver.Conn, unitIDs []int32, command *protocol.UnitCommand) {
		bs.unitCmds.applySetUnitCommand(c, wld, unitIDs, command)
	}
	srv.OnSetUnitStance = func(c *netserver.Conn, unitIDs []int32, stance protocol.UnitStance, enable bool) {
		bs.unitCmds.applySetUnitStance(c, wld, unitIDs, stance, enable)
	}
}

// runEventLoop drains world entity events and broadcasts the corresponding
// protocol packets. It mirrors the original inline goroutine in main.
func (bs *buildHookState) runEventLoop(srv *netserver.Server) {
	cfg := bs.cfg
	wld := bs.wld
	unitCommands := bs.unitCmds
	detailLog := bs.detailLog

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				fmt.Printf("[net] event-loop panic err=%v\n", rec)
			}
		}()
		t := time.NewTicker(time.Second / time.Duration(bs.gameTPS))
		defer t.Stop()
		nextBlockSnapshotSync := time.Now().Add(2 * time.Second)
		nextPlanPreviewSync := time.Now()
		eventBuf := make([]world.EntityEvent, 0, 1024)
		for range t.C {
			now := time.Now()
			if !now.Before(nextPlanPreviewSync) {
				srv.BroadcastStoredClientPlanPreviewsAt(now)
				nextPlanPreviewSync = now.Add(500 * time.Millisecond)
			}
			eventBuf = wld.DrainEntityEventsInto(eventBuf)
			evs := eventBuf
			groupedExplosionBuilds := classifyReactorExplosionBuilds(wld, evs)
			buildHealth := make([]int32, 0, len(evs)*2)
			blockItemSync := make(map[int32]struct{})
			itemTurretAmmoSync := make(map[int32]struct{})
			for i := range evs {
				ev := evs[i]
				switch ev.Kind {
				case world.EntityEventRemoved:
					unitCommands.remove(ev.Entity.ID)
					broadcastUnitDestroy(srv, ev.Entity.ID)
					if ev.Entity.Health <= 0 {
						if _, ok := srv.PlayerUnitIDSet()[ev.Entity.ID]; ok {
							fmt.Printf("[net] world removed player-unit=%d hp=%.2f pos=(%.1f,%.1f) team=%d\n",
								ev.Entity.ID, ev.Entity.Health, ev.Entity.X, ev.Entity.Y, ev.Entity.Team)
						}
						srv.MarkUnitDead(ev.Entity.ID, "world-removed")
					} else {
						if _, ok := srv.PlayerUnitIDSet()[ev.Entity.ID]; ok {
							fmt.Printf("[net] ignored unit removal conn-unit=%d source=world-removed-positive-health hp=%.2f pos=(%.1f,%.1f)\n",
								ev.Entity.ID, ev.Entity.Health, ev.Entity.X, ev.Entity.Y)
						}
					}
				case world.EntityEventBuildPlaced:
					x, y := unpackTilePos(ev.BuildPos)
					if bs.fileLogPlace() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=placed x=%d y=%d block_id=%d block=%s team=%d rot=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot))
					}
					if bs.logPlace() {
						if cfg.Building.Translated {
							actor := bs.buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 建造了 block=%d(%s) team=%d rot=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot)
						} else {
							fmt.Printf("[buildtrace] placed xy=(%d,%d) block=%d team=%d rot=%d\n", x, y, ev.BuildBlock, ev.BuildTeam, ev.BuildRot)
						}
					}
					broadcastBuildBeginPlace(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam), ev.BuildConfig)
				case world.EntityEventBuildConstructed:
					x, y := unpackTilePos(ev.BuildPos)
					if bs.fileLogFinish() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=constructed x=%d y=%d block_id=%d block=%s team=%d rot=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot))
					}
					if bs.logFinish() {
						if cfg.Building.Translated {
							actor := bs.buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 完成建造 block=%d(%s) team=%d rot=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot)
						} else {
							fmt.Printf("[buildtrace] constructed xy=(%d,%d) block=%d team=%d rot=%d\n", x, y, ev.BuildBlock, ev.BuildTeam, ev.BuildRot)
						}
					}
					broadcastBuildConstructedState(srv, wld, ev)
				case world.EntityEventBuildConfig:
					if cfgValue, ok := wld.BuildingConfigPacked(ev.BuildPos); ok {
						srv.BroadcastTileConfig(ev.BuildPos, cfgValue, nil)
					} else if ev.BuildConfig != nil {
						srv.BroadcastTileConfig(ev.BuildPos, ev.BuildConfig, nil)
					}
					broadcastRelatedBlockSnapshots(srv, wld, ev.BuildPos)
				case world.EntityEventBuildDeconstructing:
					x, y := unpackTilePos(ev.BuildPos)
					if bs.fileLogBreakStart() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=deconstructing x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if bs.logBreakStart() {
						if cfg.Building.Translated {
							actor := bs.buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 正在拆除 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] deconstructing xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDeconstructBegin(srv, ev.BuildPos, byte(ev.BuildTeam))
				case world.EntityEventBuildCancelled:
					x, y := unpackTilePos(ev.BuildPos)
					if bs.logBreakDone() {
						if cfg.Building.Translated {
							actor := bs.buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 取消了建造 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] cancelled xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDestroyed(srv, ev.BuildPos, ev.BuildBlock)
				case world.EntityEventBuildDestroyed:
					x, y := unpackTilePos(ev.BuildPos)
					if bs.fileLogBreakDone() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=destroyed x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if bs.logBreakDone() && groupedExplosionBuilds[i] == nil {
						if cfg.Building.Translated {
							if ev.BuildOwner != 0 {
								actor := bs.buildActor(ev.BuildOwner, ev.BuildTeam)
								fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 拆除了 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
							} else {
								fmt.Printf("[建筑] (x%d-y%d) 被摧毁了 block=%d(%s) team=%d\n", x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
							}
						} else {
							fmt.Printf("[buildtrace] destroyed xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDestroyedState(srv, ev)
				case world.EntityEventBuildHealth:
					buildHealth = append(buildHealth, ev.BuildPos, int32(math.Float32bits(ev.BuildHP)))
				case world.EntityEventBlockItemSync:
					blockItemSync[ev.BuildPos] = struct{}{}
				case world.EntityEventItemTurretAmmoSync:
					itemTurretAmmoSync[ev.BuildPos] = struct{}{}
				case world.EntityEventTransferItemToUnit:
					amount := ev.ItemAmount
					if amount <= 0 {
						amount = 1
					}
					for n := int32(0); n < amount; n++ {
						broadcastTransferItemToUnit(srv, int16(ev.ItemID), ev.TransferX, ev.TransferY, ev.UnitID)
					}
				case world.EntityEventTransferItemToBuild:
					broadcastTransferItemTo(srv, ev.UnitID, int16(ev.ItemID), ev.ItemAmount, ev.TransferX, ev.TransferY, ev.BuildPos)
				case world.EntityEventBulletFired:
					broadcastBulletCreate(srv, ev.Bullet)
				case world.EntityEventEffect:
					if effectID, ok := lookupEffectID(ev.EffectName); ok {
						broadcastEffectReliable(srv, effectID, ev.EffectX, ev.EffectY, ev.EffectRot)
					}
				}
			}
			if len(buildHealth) > 0 {
				// Send all health deltas in small chunks; do not trim tail,
				// otherwise construct/deconstruct progress appears to "jump".
				const maxInts = 256 // 128 buildings per packet
				for i := 0; i < len(buildHealth); i += maxInts {
					end := i + maxInts
					if end > len(buildHealth) {
						end = len(buildHealth)
					}
					broadcastBuildHealthUpdate(srv, buildHealth[i:end])
				}
			}
			if len(blockItemSync) > 0 {
				positions := make([]int32, 0, len(blockItemSync))
				for packed := range blockItemSync {
					positions = append(positions, packed)
				}
				sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
				broadcastItemBlockSnapshotsForPacked(srv, wld, positions)
			}
			if len(itemTurretAmmoSync) > 0 {
				positions := make([]int32, 0, len(itemTurretAmmoSync))
				for packed := range itemTurretAmmoSync {
					positions = append(positions, packed)
				}
				sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
				broadcastItemTurretAmmoSnapshotsForPacked(srv, wld, positions)
			}
			if !now.Before(nextBlockSnapshotSync) {
				broadcastBlockSnapshots(srv, wld)
				nextBlockSnapshotSync = now.Add(2 * time.Second)
			}
			if bs.logBreakDone() {
				logGroupedReactorExplosions(wld, evs, groupedExplosionBuilds)
			}
		}
	}()
}
