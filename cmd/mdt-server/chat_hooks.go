package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/storage"
	"mdt-server/internal/world"
)

// chatHookDeps carries the main-local dependencies of the player chat command
// hook. cfg is passed by pointer (console commands mutate it in place), and
// saveState/flushColdSnapshot are invoked through function values so the
// persistence layer can swap their implementations after the hook is bound.
type chatHookDeps struct {
	cfg               *config.Config
	state             *worldState
	detailLog         *detailedLogWriter
	shouldFileLogChat func() bool
	saveState         func()
	flushColdSnapshot func()
	saveOps           func()
	recorder          storage.Recorder
	// runPluginCommand dispatches a "/cmd ..." chat command to plugins that
	// registered it. It returns handled=true when a plugin consumed the
	// command. When nil, unknown commands fall through to the built-in
	// "invalid command" reply.
	runPluginCommand func(name string, c *netserver.Conn, args []string) (handled bool)
}

func bindChatHooks(srv *netserver.Server, wld *world.World, d chatHookDeps) {
	srv.OnChat = func(c *netserver.Conn, msg string) bool {
		if c != nil && strings.TrimSpace(msg) != "" && d.shouldFileLogChat() {
			d.detailLog.LogLine(fmt.Sprintf("%s [CHAT] from=%q player_id=%d uuid=%s ip=%s msg=%q",
				time.Now().Format(time.RFC3339Nano), c.Name(), c.PlayerID(), c.UUID(), c.RemoteAddr().String(), strings.TrimSpace(msg)))
		}
		trimmed := strings.TrimSpace(msg)
		switch trimmed {
		case "/help":
			sendChatHelp(srv, c, *d.cfg)
			return true
		case "/votemap":
			if c == nil {
				return true
			}
			showMapVoteMenu(srv, c, 0)
			return true
		case "/vote":
			if c == nil {
				return true
			}
			showActiveMapVoteMenu(srv, c)
			return true
		case "/status":
			srv.SendStatusTo(c)
			return true
		case "/sync":
			if c == nil {
				return true
			}
			syncCurrentRuntimeStateToConn(srv, c, wld, d.state.get())
			srv.SendChat(c, "[accent]已同步当前运行状态[]")
			return true
		}
		lowerTrimmed := strings.ToLower(trimmed)
		if strings.HasPrefix(lowerTrimmed, "/votemap ") {
			if c == nil {
				return true
			}
			startMapVote(srv, c, strings.TrimSpace(trimmed[len("/votemap "):]))
			return true
		}
		if strings.HasPrefix(lowerTrimmed, "/vote ") {
			if c == nil {
				return true
			}
			args := strings.Fields(lowerTrimmed)
			if len(args) < 2 {
				showActiveMapVoteMenu(srv, c)
				return true
			}
			switch args[1] {
			case "yes", "y", "1", "同意":
				castMapVote(srv, c, 1)
			case "no", "n", "0", "反对":
				castMapVote(srv, c, -1)
			case "neutral", "mid", "中立", "abstain":
				castMapVote(srv, c, 0)
			default:
				srv.SendChat(c, "[scarlet]用法: /vote yes|no|neutral[]")
			}
			return true
		}
		if strings.EqualFold(trimmed, "/stop") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			d.saveState()
			d.flushColdSnapshot()
			d.saveOps()
			srv.BroadcastChat("[accent]服务器正在保存并关闭...")
			go func() {
				time.Sleep(200 * time.Millisecond)
				_ = d.recorder.Close()
				os.Exit(0)
			}()
			return true
		}
		if strings.HasPrefix(trimmed, "/summon ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 2 {
				srv.SendChat(c, "[scarlet]用法: /summon <typeId|unitName> [x y] [count] [team] []")
				return true
			}
			typeID, typeName, ok := resolveUnitTypeArg(args[1], wld)
			if !ok {
				srv.SendChat(c, "[scarlet]typeId/unitName 无效[]")
				return true
			}
			px, py := c.SnapshotPos()
			x := float64(px)
			y := float64(py)
			team := world.TeamID(1)
			count := 1
			next := 2
			if len(args) >= 4 {
				if xv, err := strconv.ParseFloat(args[2], 32); err == nil {
					if yv, err2 := strconv.ParseFloat(args[3], 32); err2 == nil {
						x = xv
						y = yv
						next = 4
					}
				}
			}
			if len(args) > next {
				if n, err := strconv.ParseInt(args[next], 10, 32); err == nil {
					count = int(n)
					next++
				}
			}
			if len(args) > next {
				if t, err := strconv.ParseInt(args[next], 10, 8); err == nil {
					team = world.TeamID(t)
				}
			}
			if count < 1 {
				count = 1
			}
			if count > 500 {
				count = 500
			}
			success := 0
			var firstID int32
			for i := 0; i < count; i++ {
				sx := float32(x)
				sy := float32(y)
				if i > 0 {
					ring := float32((i-1)/12+1) * 12
					ang := float64(i) * 2 * math.Pi / 12
					sx += float32(math.Cos(ang)) * ring
					sy += float32(math.Sin(ang)) * ring
				}
				ent, err := wld.AddEntity(typeID, sx, sy, team)
				if err != nil {
					continue
				}
				if success == 0 {
					firstID = ent.ID
				}
				success++
			}
			if success == 0 {
				srv.SendChat(c, "[scarlet]召唤失败[]")
				return true
			}
			broadcastSummonVisible(srv, typeID, float32(x), float32(y), byte(team))
			d.saveState()
			srv.BroadcastChat(fmt.Sprintf("[accent]OP召唤单位[] firstId=%d count=%d type=%d(%s) x=%.1f y=%.1f team=%d", firstID, success, typeID, typeName, x, y, team))
			return true
		}
		if strings.HasPrefix(trimmed, "/despawn ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 2 {
				srv.SendChat(c, "[scarlet]用法: /despawn <entityId>[]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			if _, ok := wld.RemoveEntity(int32(id)); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			d.saveState()
			srv.BroadcastChat(fmt.Sprintf("[accent]OP移除单位[] id=%d", id))
			return true
		}
		if strings.EqualFold(trimmed, "/kill") {
			if c == nil {
				return true
			}
			if !srv.KillSelfUnit(c) {
				srv.SendChat(c, "[scarlet]当前没有可处理的单位[]")
				return true
			}
			srv.SendChat(c, "[accent]已执行 /kill：当前单位已清除[]")
			return true
		}
		if strings.HasPrefix(trimmed, "/umove ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 4 {
				srv.SendChat(c, "[scarlet]用法: /umove <entityId> <vx> <vy> [rotVel][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			vx, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]vx 无效[]")
				return true
			}
			vy, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]vy 无效[]")
				return true
			}
			rotVel := float32(0)
			if len(args) >= 5 {
				if rv, rerr := strconv.ParseFloat(args[4], 32); rerr == nil {
					rotVel = float32(rv)
				}
			}
			if _, ok := wld.SetEntityMotion(int32(id), float32(vx), float32(vy), rotVel); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位运动已设置[] id=%d vx=%.2f vy=%.2f rv=%.2f", id, vx, vy, rotVel))
			return true
		}
		if strings.HasPrefix(trimmed, "/uteleport ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 4 {
				srv.SendChat(c, "[scarlet]用法: /uteleport <entityId> <x> <y> [rotation][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			x, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x 无效[]")
				return true
			}
			y, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y 无效[]")
				return true
			}
			rot := float32(0)
			if len(args) >= 5 {
				if rv, rerr := strconv.ParseFloat(args[4], 32); rerr == nil {
					rot = float32(rv)
				}
			}
			if _, ok := wld.SetEntityPosition(int32(id), float32(x), float32(y), rot); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位传送完成[] id=%d x=%.1f y=%.1f rot=%.1f", id, x, y, rot))
			return true
		}
		if strings.HasPrefix(trimmed, "/ulife ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ulife <entityId> <seconds(<=0表示无限)>[]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			life, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]seconds 无效[]")
				return true
			}
			if _, ok := wld.SetEntityLife(int32(id), float32(life)); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位寿命已设置[] id=%d life=%.2fs", id, life))
			return true
		}
		if strings.HasPrefix(trimmed, "/ufollow ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ufollow <id> <targetId> [speed][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			targetID, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil || targetID <= 0 {
				srv.SendChat(c, "[scarlet]targetId 无效[]")
				return true
			}
			speed := float32(0)
			if len(args) >= 4 {
				if sp, serr := strconv.ParseFloat(args[3], 32); serr == nil {
					speed = float32(sp)
				}
			}
			if _, ok := wld.SetEntityFollow(int32(id), int32(targetID), speed); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位跟随已设置[] id=%d -> target=%d speed=%.2f", id, targetID, speed))
			return true
		}
		if strings.HasPrefix(trimmed, "/upatrol ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 6 {
				srv.SendChat(c, "[scarlet]用法: /upatrol <id> <x1> <y1> <x2> <y2> [speed][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			x1, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x1 无效[]")
				return true
			}
			y1, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y1 无效[]")
				return true
			}
			x2, err := strconv.ParseFloat(args[4], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x2 无效[]")
				return true
			}
			y2, err := strconv.ParseFloat(args[5], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y2 无效[]")
				return true
			}
			speed := float32(0)
			if len(args) >= 7 {
				if sp, serr := strconv.ParseFloat(args[6], 32); serr == nil {
					speed = float32(sp)
				}
			}
			if _, ok := wld.SetEntityPatrol(int32(id), float32(x1), float32(y1), float32(x2), float32(y2), speed); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位巡逻已设置[] id=%d A(%.1f,%.1f) B(%.1f,%.1f) speed=%.2f", id, x1, y1, x2, y2, speed))
			return true
		}
		if strings.HasPrefix(trimmed, "/ubehavior ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ubehavior clear <id>[]")
				return true
			}
			action := strings.ToLower(args[1])
			if action != "clear" {
				srv.SendChat(c, "[scarlet]仅支持: clear[]")
				return true
			}
			id, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			if _, ok := wld.ClearEntityBehavior(int32(id)); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			d.saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位行为已清除[] id=%d", id))
			return true
		}
		if strings.HasPrefix(trimmed, "/") {
			if d.runPluginCommand != nil {
				fields := strings.Fields(trimmed)
				cmdName := strings.TrimPrefix(fields[0], "/")
				if handled := d.runPluginCommand(cmdName, c, fields[1:]); handled {
					return true
				}
			}
			srv.SendChat(c, fmt.Sprintf("[scarlet]无效命令: %s[]", trimmed))
			return true
		}
		return false
	}
}
