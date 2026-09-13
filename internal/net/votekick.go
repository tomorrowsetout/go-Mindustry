package net

import (
	"fmt"
	"sync"
	"time"
)

// VoteKickSession 投票踢人会话
type VoteKickSession struct {
	mu           sync.RWMutex
	target       *Conn
	initiator    *Conn
	votes        map[string]int // UUID -> vote (-1: no, 1: yes)
	task         *time.Timer
	startTime    time.Time
	kickDuration time.Duration
	voteDuration time.Duration
	voteCooldown time.Duration
	srv          *Server
	canceled     bool
}

const (
	// 默认投票时长（秒）
	defaultVoteDuration = 30 * time.Second
	// 默认踢人时长（秒）
	defaultKickDuration = 60 * time.Second
	// 默认投票冷却时间（秒）
	defaultVoteCooldown = 5 * time.Minute
)

// NewVoteKickSession 创建新的投票踢人会话
func NewVoteKickSession(target, initiator *Conn, srv *Server) *VoteKickSession {
	return &VoteKickSession{
		target:       target,
		initiator:    initiator,
		votes:        make(map[string]int),
		kickDuration: defaultKickDuration,
		voteDuration: defaultVoteDuration,
		voteCooldown: defaultVoteCooldown,
		srv:          srv,
		startTime:    time.Now(),
	}
}

// Start 启动投票会话
func (v *VoteKickSession) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.canceled {
		return fmt.Errorf("vote session has been canceled")
	}

	if v.target == nil {
		return fmt.Errorf("target cannot be nil")
	}

	if v.target.isAdmin() {
		return fmt.Errorf("cannot kick admin player")
	}

	if v.target == v.initiator {
		return fmt.Errorf("cannot vote to kick yourself")
	}

	if v.initiator != nil && v.target.TeamID() != v.initiator.TeamID() {
		return fmt.Errorf("can only kick players on the same team")
	}

	// 检查冷却时间
	if v.srv.recentKickUntil != nil {
		if cooldown, ok := v.srv.recentKickUntil[v.initiator.uuid]; ok && cooldown.After(time.Now()) {
			remaining := cooldown.Sub(time.Now())
			return fmt.Errorf("must wait %v before starting another vote", remaining.Round(time.Second))
		}
	}

	// 启动投票定时器
	v.task = time.AfterFunc(v.voteDuration, v.onVoteComplete)

	// 广播投票开始消息
	v.srv.BroadcastChat(fmt.Sprintf("[accent]Vote to kick [orange]%s[accent] started! Use [lightgray]/vote y[accent] or [lightgray]/vote n[accent].",
		v.target.playerName()))

	return nil
}

// Vote 处理投票
func (v *VoteKickSession) Vote(voter *Conn, vote int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.canceled {
		return fmt.Errorf("vote session has been canceled")
	}

	if voter == nil {
		return fmt.Errorf("voter cannot be nil")
	}

	if v.target == nil {
		return fmt.Errorf("target cannot be nil")
	}

	if v.target == voter {
		return fmt.Errorf("cannot vote on your own trial")
	}

	if voter.TeamID() != v.target.TeamID() {
		return fmt.Errorf("cannot vote for other teams")
	}

	if vote != 1 && vote != -1 {
		return fmt.Errorf("vote must be either 'y' (yes) or 'n' (no)")
	}

	// 检查是否已经投过票
	uuid := voter.uuid
	ip := voter.remoteIP()

	if existingVote, ok := v.votes[uuid]; ok && existingVote == vote {
		return fmt.Errorf("you have already voted '%d'", vote)
	}
	if existingVote, ok := v.votes[ip]; ok && existingVote == vote {
		return fmt.Errorf("you have already voted '%d'", vote)
	}

	// 记录投票
	v.votes[uuid] = vote
	v.votes[ip] = vote

	// 设置冷却时间
	if v.srv.recentKickUntil == nil {
		v.srv.recentKickUntil = make(map[string]time.Time)
	}
	v.srv.recentKickUntil[uuid] = time.Now().Add(v.voteCooldown)
	v.srv.recentKickUntil[ip] = time.Now().Add(v.voteCooldown)

	// 检查投票结果
	v.checkVoteResult()

	return nil
}

// Cancel 取消投票会话
func (v *VoteKickSession) Cancel() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.canceled {
		return
	}

	v.canceled = true
	if v.task != nil {
		v.task.Stop()
	}

	v.srv.BroadcastChat("[accent]Vote canceled by admin.")
}

// onVoteComplete 投票完成时的处理
func (v *VoteKickSession) onVoteComplete() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.canceled {
		return
	}

	// 检查投票结果
	yesVotes := 0
	noVotes := 0
	for _, vote := range v.votes {
		if vote == 1 {
			yesVotes++
		} else if vote == -1 {
			noVotes++
		}
	}

	// 判断是否通过
	requiredVotes := v.votesRequired()
	if yesVotes >= requiredVotes {
		// 投票通过，踢人
		reason := fmt.Sprintf("voted out by %d players", yesVotes)
		v.srv.KickPlayer(v.target, reason)
		v.srv.BroadcastChat(fmt.Sprintf("[accent]Vote passed: [orange]%s[accent] has been kicked (%d yes, %d no)",
			v.target.playerName(), yesVotes, noVotes))
	} else {
		// 投票未通过
		v.srv.BroadcastChat(fmt.Sprintf("[accent]Vote failed: [orange]%s[accent] stays (%d yes, %d no, required %d)",
			v.target.playerName(), yesVotes, noVotes, requiredVotes))
	}

	// 设置冷却时间
	if v.initiator != nil && v.srv.recentKickUntil != nil {
		v.srv.recentKickUntil[v.initiator.uuid] = time.Now().Add(v.voteCooldown)
		v.srv.recentKickUntil[v.initiator.remoteIP()] = time.Now().Add(v.voteCooldown)
	}
}

// checkVoteResult 检查投票结果
func (v *VoteKickSession) checkVoteResult() {
	if v.target == nil || v.srv == nil {
		return
	}

	yesVotes := 0
	noVotes := 0
	for _, vote := range v.votes {
		if vote == 1 {
			yesVotes++
		} else if vote == -1 {
			noVotes++
		}
	}

	// 如果已经达到必需的票数，立即执行
	requiredVotes := v.votesRequired()
	if yesVotes >= requiredVotes {
		if v.task != nil {
			v.task.Stop()
		}

		reason := fmt.Sprintf("voted out by %d players", yesVotes)
		v.srv.KickPlayer(v.target, reason)
		v.srv.BroadcastChat(fmt.Sprintf("[accent]Vote passed: [orange]%s[accent] has been kicked (%d yes, %d no)",
			v.target.playerName(), yesVotes, noVotes))
	}
}

// votesRequired 计算需要的票数
func (v *VoteKickSession) votesRequired() int {
	if v == nil || v.srv == nil {
		return 2
	}
	playerCount := v.srv.activePlayerCount()
	return 2 + playerCount/4 // 至少需要2票，每4个玩家多需要1票
}

// GetStatus 获取投票状态
func (v *VoteKickSession) GetStatus() (targetName string, yesVotes, noVotes, requiredVotes int, elapsed time.Duration, remaining time.Duration) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	targetName = ""
	if v.target != nil {
		targetName = v.target.playerName()
	}

	yesVotes = 0
	noVotes = 0
	for _, vote := range v.votes {
		if vote == 1 {
			yesVotes++
		} else if vote == -1 {
			noVotes++
		}
	}

	requiredVotes = v.votesRequired()
	elapsed = time.Since(v.startTime)
	remaining = v.voteDuration - elapsed
	if remaining < 0 {
		remaining = 0
	}

	return
}

// VoteKickManager 投票踢人管理器
type VoteKickManager struct {
	mu      sync.RWMutex
	current *VoteKickSession
	srv     *Server
}

// NewVoteKickManager 创建投票踢人管理器
func NewVoteKickManager(srv *Server) *VoteKickManager {
	return &VoteKickManager{
		srv: srv,
	}
}

// StartVote 启动新的投票
func (m *VoteKickManager) StartVote(target, initiator *Conn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果有正在进行的投票，拒绝
	if m.current != nil {
		return fmt.Errorf("a vote is already in progress")
	}

	session := NewVoteKickSession(target, initiator, m.srv)
	if err := session.Start(); err != nil {
		return err
	}

	m.current = session

	// 启动清理任务
	time.AfterFunc(session.voteDuration, func() {
		m.mu.Lock()
		if m.current == session {
			m.current = nil
		}
		m.mu.Unlock()
	})

	return nil
}

// Vote 投票
func (m *VoteKickManager) Vote(voter *Conn, vote int) error {
	m.mu.RLock()
	session := m.current
	m.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("no vote is currently in progress")
	}

	return session.Vote(voter, vote)
}

// Cancel 取消当前投票
func (m *VoteKickManager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
		m.current.Cancel()
		m.current = nil
	}
}

// GetStatus 获取当前投票状态
func (m *VoteKickManager) GetStatus() (targetName string, yesVotes, noVotes, requiredVotes int, elapsed, remaining time.Duration, active bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.current == nil {
		return "", 0, 0, 0, 0, 0, false
	}

	targetName, yesVotes, noVotes, requiredVotes, elapsed, remaining = m.current.GetStatus()
	return targetName, yesVotes, noVotes, requiredVotes, elapsed, remaining, true
}
