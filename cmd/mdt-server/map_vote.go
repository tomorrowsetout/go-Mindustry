package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/worldstream"
)

const (
	voteMapSelectMenuBaseID int32 = 910100
	voteMapPromptMenuID     int32 = 910200
	voteMapPageSize               = 4
)

const (
	mapVoteDecisionPending mapVoteDecision = iota
	mapVoteDecisionPassed
	mapVoteDecisionRejected
	mapVoteDecisionExpired
)

var errMapVoteActive = errors.New("已有换图投票进行中")

type mapVoteDecision int

type mapVoteResult struct {
	Decision  mapVoteDecision
	MapName   string
	WorldPath string
	StartedBy string
	Yes       int
	No        int
	Neutral   int
	Needed    int
}

type mapVoteSnapshot struct {
	MapName   string
	StartedBy string
	Yes       int
	No        int
	Neutral   int
	Needed    int
	ExpiresAt time.Time
}

type mapVoteSession struct {
	Token     uint64
	MapName   string
	WorldPath string
	StartedBy string
	Votes     map[string]int8
	ExpiresAt time.Time
	Timer     *time.Timer
}

type mapVoteRuntime struct {
	mu sync.Mutex

	listMaps     func() ([]string, error)
	resolveWorld func(string) (string, error)
	applyWorld   func(string) error
	notifyResult func(mapVoteResult)
	server       *netserver.Server
	duration     time.Duration
	nextToken    uint64
	active       *mapVoteSession
	statusToken  uint64
}

type mapVoteRuntimeConfig struct {
	Duration      time.Duration
	StatusRefresh time.Duration
	PopupDuration float32
	HomeLinkURL   string
	Align         string
	Top           int
	Left          int
	Bottom        int
	Right         int
}

var (
	runtimeMapVoteMu sync.RWMutex
	runtimeMapVote   *mapVoteRuntime
	runtimeMapVoteUI atomic.Value
)

func initMapVoteRuntimeConfig(cfg config.Config) {
	runtimeMapVoteUI.Store(mapVoteConfigFrom(cfg.MapVote))
}

func mapVoteConfigFrom(cfg config.MapVoteConfig) mapVoteRuntimeConfig {
	duration := time.Duration(cfg.DurationSec) * time.Second
	if duration <= 0 {
		duration = 15 * time.Second
	}
	refresh := time.Duration(cfg.StatusRefreshMs) * time.Millisecond
	if refresh <= 0 {
		refresh = 1500 * time.Millisecond
	}
	popupDuration := float32(cfg.PopupDurationMs) / 1000
	if popupDuration <= 0 {
		popupDuration = 1.8
	}
	return mapVoteRuntimeConfig{
		Duration:      duration,
		StatusRefresh: refresh,
		PopupDuration: popupDuration,
		HomeLinkURL:   strings.TrimSpace(cfg.HomeLinkURL),
		Align:         strings.TrimSpace(cfg.Align),
		Top:           cfg.Top,
		Left:          cfg.Left,
		Bottom:        cfg.Bottom,
		Right:         cfg.Right,
	}
}

func currentMapVoteConfig() mapVoteRuntimeConfig {
	if v := runtimeMapVoteUI.Load(); v != nil {
		if cfg, ok := v.(mapVoteRuntimeConfig); ok {
			return cfg
		}
	}
	return mapVoteConfigFrom(config.Default().MapVote)
}

func initMapVoteRuntime(listMaps func() ([]string, error), resolveWorld func(string) (string, error), applyWorld func(string) error, notifyResult func(mapVoteResult), srv *netserver.Server) {
	runtimeMapVoteMu.Lock()
	runtimeMapVote = &mapVoteRuntime{
		listMaps:     listMaps,
		resolveWorld: resolveWorld,
		applyWorld:   applyWorld,
		notifyResult: notifyResult,
		server:       srv,
	}
	runtimeMapVoteMu.Unlock()
}

func currentMapVoteRuntime() *mapVoteRuntime {
	runtimeMapVoteMu.RLock()
	defer runtimeMapVoteMu.RUnlock()
	return runtimeMapVote
}

func (rt *mapVoteRuntime) voteDuration() time.Duration {
	if rt != nil && rt.duration > 0 {
		return rt.duration
	}
	return currentMapVoteConfig().Duration
}

func (rt *mapVoteRuntime) activeToken() uint64 {
	if rt == nil {
		return 0
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.active == nil {
		return 0
	}
	return rt.active.Token
}

func (rt *mapVoteRuntime) currentTotalPlayers() int {
	if rt != nil && rt.server != nil {
		return len(rt.server.ListConnectedConns())
	}
	if rt == nil {
		return 1
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.active != nil {
		return max(len(rt.active.Votes), 1)
	}
	return 1
}

func (rt *mapVoteRuntime) startStatusLoop(token uint64) {
	if rt == nil || rt.server == nil || token == 0 {
		return
	}
	rt.mu.Lock()
	if rt.statusToken == token {
		rt.mu.Unlock()
		return
	}
	rt.statusToken = token
	rt.mu.Unlock()
	go rt.runStatusLoop(token)
}

func (rt *mapVoteRuntime) runStatusLoop(token uint64) {
	for {
		if !rt.broadcastStatus(token) {
			return
		}
		time.Sleep(currentMapVoteConfig().StatusRefresh)
	}
}

func (rt *mapVoteRuntime) broadcastActiveStatus() {
	if rt == nil {
		return
	}
	if token := rt.activeToken(); token != 0 {
		_ = rt.broadcastStatus(token)
	}
}

func (rt *mapVoteRuntime) broadcastStatus(token uint64) bool {
	if rt == nil || rt.server == nil || token == 0 {
		return false
	}
	totalPlayers := len(rt.server.ListConnectedConns())
	rt.mu.Lock()
	if rt.active == nil || rt.active.Token != token {
		if rt.statusToken == token {
			rt.statusToken = 0
		}
		rt.mu.Unlock()
		return false
	}
	if totalPlayers < 1 {
		totalPlayers = max(len(rt.active.Votes), 1)
	}
	snapshot := buildMapVoteSnapshot(rt.active, totalPlayers)
	rt.mu.Unlock()
	broadcastMapVoteStatus(rt.server, snapshot)
	return true
}

func broadcastMapVoteStatus(srv *netserver.Server, snapshot mapVoteSnapshot) {
	if srv == nil {
		return
	}
	remaining := time.Until(snapshot.ExpiresAt)
	if remaining < 0 {
		remaining = 0
	}
	lines := []string{
		"[accent]换图投票[]",
		fmt.Sprintf("地图: [white]%s[]", snapshot.MapName),
		fmt.Sprintf("发起: %s", snapshot.StartedBy),
		fmt.Sprintf("同意: [green]%d[]/[white]%d[]  反对: [scarlet]%d[]  中立: [lightgray]%d[]", snapshot.Yes, snapshot.Needed, snapshot.No, snapshot.Neutral),
		fmt.Sprintf("剩余: [white]%.1fs[]", remaining.Seconds()),
	}
	srv.BroadcastSetHudTextReliable(strings.Join(lines, "\n"))
}

func voteParticipantKey(c *netserver.Conn) string {
	if c == nil {
		return ""
	}
	if uuid := strings.ToLower(strings.TrimSpace(c.UUID())); uuid != "" {
		return uuid
	}
	return fmt.Sprintf("conn:%d", c.ConnID())
}

func voteParticipantName(srv *netserver.Server, c *netserver.Conn) string {
	if srv != nil && c != nil {
		if name := strings.TrimSpace(srv.PlayerDisplayName(c)); name != "" {
			return name
		}
	}
	if c != nil {
		if name := strings.TrimSpace(c.Name()); name != "" {
			return name
		}
	}
	return "玩家"
}

func neededVotes(totalPlayers int) int {
	if totalPlayers < 1 {
		totalPlayers = 1
	}
	return totalPlayers/2 + 1
}

func countMapVotes(votes map[string]int8) (yes, no int) {
	yes, no, _ = countMapVotesDetailed(votes)
	return yes, no
}

func countMapVotesDetailed(votes map[string]int8) (yes, no, neutral int) {
	for _, vote := range votes {
		switch vote {
		case 1:
			yes++
		case -1:
			no++
		case 0:
			neutral++
		}
	}
	return yes, no, neutral
}

func evaluateMapVote(yes, no, totalPlayers, voted int) mapVoteDecision {
	need := neededVotes(totalPlayers)
	if yes >= need {
		return mapVoteDecisionPassed
	}
	if no >= need {
		return mapVoteDecisionRejected
	}
	if voted >= max(totalPlayers, 1) {
		return mapVoteDecisionRejected
	}
	return mapVoteDecisionPending
}

func (rt *mapVoteRuntime) snapshot(totalPlayers int) (mapVoteSnapshot, bool) {
	if rt == nil {
		return mapVoteSnapshot{}, false
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.active == nil {
		return mapVoteSnapshot{}, false
	}
	return buildMapVoteSnapshot(rt.active, totalPlayers), true
}

func buildMapVoteSnapshot(active *mapVoteSession, totalPlayers int) mapVoteSnapshot {
	if active == nil {
		return mapVoteSnapshot{}
	}
	yes, no, neutral := countMapVotesDetailed(active.Votes)
	return mapVoteSnapshot{
		MapName:   active.MapName,
		StartedBy: active.StartedBy,
		Yes:       yes,
		No:        no,
		Neutral:   neutral,
		Needed:    neededVotes(totalPlayers),
		ExpiresAt: active.ExpiresAt,
	}
}

func finalizeMapVote(active *mapVoteSession, decision mapVoteDecision, totalPlayers int) mapVoteResult {
	yes, no, neutral := countMapVotesDetailed(active.Votes)
	return mapVoteResult{
		Decision:  decision,
		MapName:   active.MapName,
		WorldPath: active.WorldPath,
		StartedBy: active.StartedBy,
		Yes:       yes,
		No:        no,
		Neutral:   neutral,
		Needed:    neededVotes(totalPlayers),
	}
}

func (rt *mapVoteRuntime) beginVote(worldPath, mapName, starterKey, starterName string, totalPlayers int) (mapVoteSnapshot, mapVoteResult, error) {
	if rt == nil {
		return mapVoteSnapshot{}, mapVoteResult{}, errors.New("vote runtime is nil")
	}
	now := time.Now()
	duration := rt.voteDuration()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.active != nil {
		return buildMapVoteSnapshot(rt.active, totalPlayers), mapVoteResult{}, errMapVoteActive
	}
	rt.nextToken++
	session := &mapVoteSession{
		Token:     rt.nextToken,
		MapName:   mapName,
		WorldPath: worldPath,
		StartedBy: starterName,
		Votes: map[string]int8{
			starterKey: 1,
		},
		ExpiresAt: now.Add(duration),
	}
	yes, no, _ := countMapVotesDetailed(session.Votes)
	decision := evaluateMapVote(yes, no, totalPlayers, yes+no)
	if decision != mapVoteDecisionPending {
		return mapVoteSnapshot{}, finalizeMapVote(session, decision, totalPlayers), nil
	}
	session.Timer = time.AfterFunc(duration, func() {
		rt.expireVote(session.Token)
	})
	rt.active = session
	return buildMapVoteSnapshot(session, totalPlayers), mapVoteResult{Decision: mapVoteDecisionPending}, nil
}

func (rt *mapVoteRuntime) castVote(voterKey string, value int8, totalPlayers int) (mapVoteSnapshot, mapVoteResult, error) {
	if rt == nil {
		return mapVoteSnapshot{}, mapVoteResult{}, errors.New("vote runtime is nil")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.active == nil {
		return mapVoteSnapshot{}, mapVoteResult{}, errors.New("当前没有进行中的换图投票")
	}
	if value != 1 && value != -1 && value != 0 {
		return buildMapVoteSnapshot(rt.active, totalPlayers), mapVoteResult{}, errors.New("无效投票")
	}
	rt.active.Votes[voterKey] = value
	yes, no, _ := countMapVotesDetailed(rt.active.Votes)
	decision := evaluateMapVote(yes, no, totalPlayers, yes+no)
	if decision == mapVoteDecisionPending {
		return buildMapVoteSnapshot(rt.active, totalPlayers), mapVoteResult{Decision: mapVoteDecisionPending}, nil
	}
	finished := rt.active
	if finished.Timer != nil {
		finished.Timer.Stop()
	}
	rt.active = nil
	return mapVoteSnapshot{}, finalizeMapVote(finished, decision, totalPlayers), nil
}

func (rt *mapVoteRuntime) expireVote(token uint64) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	if rt.active == nil || rt.active.Token != token {
		rt.mu.Unlock()
		return
	}
	finished := rt.active
	rt.active = nil
	rt.mu.Unlock()
	result := finalizeMapVote(finished, mapVoteDecisionExpired, max(len(finished.Votes), 1))
	if rt.notifyResult != nil {
		rt.notifyResult(result)
		return
	}
	handleMapVoteResult(nil, rt, result)
}

func (rt *mapVoteRuntime) listPage(page int) ([]string, int, int, error) {
	if rt == nil || rt.listMaps == nil {
		return nil, 0, 0, errors.New("vote map list is unavailable")
	}
	maps, err := rt.listMaps()
	if err != nil {
		return nil, 0, 0, err
	}
	totalPages := 1
	if len(maps) > 0 {
		totalPages = (len(maps) + voteMapPageSize - 1) / voteMapPageSize
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * voteMapPageSize
	end := min(start+voteMapPageSize, len(maps))
	if start >= len(maps) {
		return nil, page, totalPages, nil
	}
	return maps[start:end], page, totalPages, nil
}

func showMapVoteMenu(srv *netserver.Server, c *netserver.Conn, page int) {
	if srv == nil || c == nil {
		return
	}
	rt := currentMapVoteRuntime()
	if rt == nil {
		srv.SendInfoMessage(c, "[scarlet]换图投票功能未初始化。[]")
		return
	}
	if _, ok := rt.snapshot(len(srv.ListConnectedConns())); ok {
		showActiveMapVoteMenu(srv, c)
		return
	}
	pageMaps, currentPage, totalPages, err := rt.listPage(page)
	if err != nil {
		srv.SendInfoMessage(c, fmt.Sprintf("[scarlet]读取地图列表失败: %s[]", err.Error()))
		return
	}
	if len(pageMaps) == 0 {
		srv.SendInfoMessage(c, "[scarlet]当前没有可投票的地图。[]")
		return
	}
	lines := []string{
		"[accent]投票换图[]",
		fmt.Sprintf("当前地图: [white]%s[]", currentStatusBarMapName(srv)),
		fmt.Sprintf("第 [white]%d/%d[] 页，选择一张地图发起投票。", currentPage+1, totalPages),
	}
	srv.SendMenu(c, voteMapSelectMenuBaseID+int32(currentPage), "[accent]投票换图[]", strings.Join(lines, "\n"), mapVoteSelectionOptions(pageMaps, currentPage, totalPages, currentMapVoteConfig().HomeLinkURL != ""))
}

func mapVoteSelectionOptions(pageMaps []string, page, totalPages int, hasLink bool) [][]string {
	options := make([][]string, 0, 4)
	for i := 0; i < len(pageMaps); i += 2 {
		end := min(i+2, len(pageMaps))
		row := make([]string, 0, end-i)
		for _, name := range pageMaps[i:end] {
			row = append(row, name)
		}
		options = append(options, row)
	}
	nav := make([]string, 0, 2)
	if page > 0 {
		nav = append(nav, "上一页")
	}
	if page+1 < totalPages {
		nav = append(nav, "下一页")
	}
	if len(nav) > 0 {
		options = append(options, nav)
	}
	if hasLink {
		options = append(options, []string{"打开链接", "关闭"})
	} else {
		options = append(options, []string{"关闭"})
	}
	return options
}

func showActiveMapVoteMenu(srv *netserver.Server, c *netserver.Conn) {
	if srv == nil || c == nil {
		return
	}
	rt := currentMapVoteRuntime()
	if rt == nil {
		return
	}
	snapshot, ok := rt.snapshot(len(srv.ListConnectedConns()))
	if !ok {
		srv.SendInfoMessage(c, "[scarlet]当前没有进行中的换图投票。[]")
		return
	}
	remaining := time.Until(snapshot.ExpiresAt)
	if remaining < 0 {
		remaining = 0
	}
	lines := []string{
		"[accent]换图投票[]",
		fmt.Sprintf("目标地图: [white]%s[]", snapshot.MapName),
		fmt.Sprintf("发起玩家: %s", snapshot.StartedBy),
		fmt.Sprintf("同意: [green]%d[]  反对: [scarlet]%d[]  中立: [lightgray]%d[]", snapshot.Yes, snapshot.No, snapshot.Neutral),
		fmt.Sprintf("通过需要: [white]%d[]  剩余: [white]%ds[]", snapshot.Needed, int(remaining/time.Second)),
	}
	srv.SendMenu(c, voteMapPromptMenuID, "[accent]换图投票[]", strings.Join(lines, "\n"), [][]string{
		{"同意", "反对", "中立", "关闭"},
	})
}

func showActiveMapVoteMenuToAll(srv *netserver.Server) {
	if srv == nil {
		return
	}
	for _, conn := range srv.ListConnectedConns() {
		showActiveMapVoteMenu(srv, conn)
	}
}

func handleMapVoteMenuChoice(srv *netserver.Server, c *netserver.Conn, menuID, option int32) bool {
	if srv == nil || c == nil {
		return false
	}
	switch {
	case menuID == voteMapPromptMenuID:
		switch option {
		case 0:
			castMapVote(srv, c, 1)
		case 1:
			castMapVote(srv, c, -1)
		case 2:
			castMapVote(srv, c, 0)
		case 3:
			srv.BroadcastChat(fmt.Sprintf("[accent]%s[] 关闭了投票窗口。", voteParticipantName(srv, c)))
			return true
		}
		return true
	case menuID >= voteMapSelectMenuBaseID && menuID < voteMapSelectMenuBaseID+100:
		rt := currentMapVoteRuntime()
		if rt == nil {
			srv.SendInfoMessage(c, "[scarlet]换图投票功能未初始化。[]")
			return true
		}
		linkURL := currentMapVoteConfig().HomeLinkURL
		hasLink := strings.TrimSpace(linkURL) != ""
		pageMaps, page, totalPages, err := rt.listPage(int(menuID - voteMapSelectMenuBaseID))
		if err != nil {
			srv.SendInfoMessage(c, fmt.Sprintf("[scarlet]读取地图列表失败: %s[]", err.Error()))
			return true
		}
		if option < int32(len(pageMaps)) {
			startMapVote(srv, c, pageMaps[option])
			return true
		}
		option -= int32(len(pageMaps))
		if page > 0 {
			if option == 0 {
				showMapVoteMenu(srv, c, page-1)
				return true
			}
			option--
		}
		if page+1 < totalPages {
			if option == 0 {
				showMapVoteMenu(srv, c, page+1)
				return true
			}
			option--
		}
		if hasLink {
			if option == 0 {
				srv.SendOpenURI(c, linkURL)
				return true
			}
			option--
		}
		return true
	default:
		return false
	}
}

func startMapVote(srv *netserver.Server, c *netserver.Conn, target string) {
	if srv == nil || c == nil {
		return
	}
	rt := currentMapVoteRuntime()
	if rt == nil {
		srv.SendInfoMessage(c, "[scarlet]换图投票功能未初始化。[]")
		return
	}
	if _, ok := rt.snapshot(len(srv.ListConnectedConns())); ok {
		showActiveMapVoteMenu(srv, c)
		return
	}
	if rt.resolveWorld == nil {
		srv.SendInfoMessage(c, "[scarlet]当前无法解析目标地图。[]")
		return
	}
	worldPath, err := rt.resolveWorld(strings.TrimSpace(target))
	if err != nil {
		srv.SendChat(c, fmt.Sprintf("[scarlet]地图无效: %s[]", err.Error()))
		return
	}
	mapName := worldstream.TrimMapName(filepath.Base(worldPath))
	snapshot, result, err := rt.beginVote(worldPath, mapName, voteParticipantKey(c), voteParticipantName(srv, c), len(srv.ListConnectedConns()))
	if err != nil {
		if errors.Is(err, errMapVoteActive) {
			srv.SendChat(c, "[scarlet]已有换图投票进行中。[]")
			showActiveMapVoteMenu(srv, c)
			return
		}
		srv.SendChat(c, fmt.Sprintf("[scarlet]发起投票失败: %s[]", err.Error()))
		return
	}
	if result.Decision == mapVoteDecisionPending {
		durationSec := int(currentMapVoteConfig().Duration / time.Second)
		srv.BroadcastChat(fmt.Sprintf("[accent]%s[] 发起了换图投票: [white]%s[] ([green]%d/%d[]，限时 [white]%ds[]，输入 [white]/vote[] 可投票)", voteParticipantName(srv, c), snapshot.MapName, snapshot.Yes, snapshot.Needed, durationSec))
		if token := rt.activeToken(); token != 0 {
			rt.startStatusLoop(token)
		}
		rt.broadcastActiveStatus()
		showActiveMapVoteMenuToAll(srv)
		return
	}
	handleMapVoteResult(srv, rt, result)
}

func castMapVote(srv *netserver.Server, c *netserver.Conn, vote int8) {
	if srv == nil || c == nil {
		return
	}
	rt := currentMapVoteRuntime()
	if rt == nil {
		return
	}
	_, result, err := rt.castVote(voteParticipantKey(c), vote, len(srv.ListConnectedConns()))
	if err != nil {
		srv.SendChat(c, fmt.Sprintf("[scarlet]%s[]", err.Error()))
		return
	}
	choiceLabel := mapVoteChoiceLabel(vote)
	srv.SendChat(c, fmt.Sprintf("[accent]你选择了%s。[]", choiceLabel))
	srv.BroadcastChat(fmt.Sprintf("[accent]%s[] 选择了%s。", voteParticipantName(srv, c), choiceLabel))
	if result.Decision == mapVoteDecisionPending {
		rt.broadcastActiveStatus()
		return
	}
	handleMapVoteResult(srv, rt, result)
}

func mapVoteChoiceLabel(vote int8) string {
	switch vote {
	case 1:
		return "[green]同意[]"
	case -1:
		return "[scarlet]反对[]"
	case 0:
		return "[lightgray]中立[]"
	default:
		return "[lightgray]未知[]"
	}
}

func handleMapVoteResult(srv *netserver.Server, rt *mapVoteRuntime, result mapVoteResult) {
	if rt == nil || result.Decision == mapVoteDecisionPending {
		return
	}
	if srv != nil {
		srv.BroadcastHideHudText()
	}
	switch result.Decision {
	case mapVoteDecisionPassed:
		if srv != nil {
			srv.BroadcastChat(fmt.Sprintf("[accent]换图投票通过[]: [white]%s[] ([green]%d/%d[] 反对 [scarlet]%d[] 中立 [lightgray]%d[])", result.MapName, result.Yes, result.Needed, result.No, result.Neutral))
		}
		if rt.applyWorld == nil {
			if srv != nil {
				srv.BroadcastChat("[scarlet]换图失败：切图回调未初始化。[]")
			}
			return
		}
		if err := rt.applyWorld(result.WorldPath); err != nil && srv != nil {
			srv.BroadcastChat(fmt.Sprintf("[scarlet]换图失败: %s[]", err.Error()))
		}
	case mapVoteDecisionRejected:
		if srv != nil {
			srv.BroadcastChat(fmt.Sprintf("[scarlet]换图投票未通过[]: [white]%s[] (同意 [green]%d[] / 反对 [scarlet]%d[] / 中立 [lightgray]%d[] / 需要 %d)", result.MapName, result.Yes, result.No, result.Neutral, result.Needed))
		}
	case mapVoteDecisionExpired:
		if srv != nil {
			srv.BroadcastChat(fmt.Sprintf("[scarlet]换图投票超时[]: [white]%s[] (同意 [green]%d[] / 反对 [scarlet]%d[] / 中立 [lightgray]%d[])", result.MapName, result.Yes, result.No, result.Neutral))
		}
	}
}
