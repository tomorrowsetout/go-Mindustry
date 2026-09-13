package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/protocol"
)

const core2IPCTimeout = 5 * time.Second
const policyIPCTimeout = 250 * time.Millisecond

var spawnChildCoreProcessFn = spawnChildCoreProcess
var closeChildCoreProcessFn = func(child *childCoreProcess) error {
	if child == nil {
		return nil
	}
	return child.Close()
}

type childCoreProcess struct {
	Role     string
	Endpoint string
	Cmd      *exec.Cmd
	Client   *ipcClient

	exitCh    chan error
	closeOnce sync.Once
	closing   atomic.Bool
}

func (p *childCoreProcess) Close() error {
	if p == nil {
		return nil
	}
	var closeErr error
	p.closeOnce.Do(func() {
		p.closing.Store(true)
		if p.Client != nil {
			_ = p.Client.Call("shutdown", nil, nil)
			_ = p.Client.Close()
		}
		if p.Cmd == nil || p.Cmd.Process == nil {
			return
		}
		waitCh := p.Exited()
		select {
		case err, ok := <-waitCh:
			if ok {
				closeErr = err
			}
		case <-time.After(2 * time.Second):
			_ = p.Cmd.Process.Kill()
			if err, ok := <-waitCh; ok {
				closeErr = err
			}
		}
	})
	return closeErr
}

func (p *childCoreProcess) beginWait() {
	if p == nil || p.exitCh != nil || p.Cmd == nil {
		return
	}
	p.exitCh = make(chan error, 1)
	go func() {
		err := p.Cmd.Wait()
		p.exitCh <- err
		close(p.exitCh)
	}()
}

func (p *childCoreProcess) Exited() <-chan error {
	if p == nil {
		ch := make(chan error)
		close(ch)
		return ch
	}
	if p.exitCh == nil {
		p.beginWait()
	}
	return p.exitCh
}

func (p *childCoreProcess) Closing() bool {
	if p == nil {
		return true
	}
	return p.closing.Load()
}

func spawnChildCoreProcess(exePath, role string, extraArgs ...string) (*childCoreProcess, error) {
	if strings.TrimSpace(exePath) == "" {
		return nil, fmt.Errorf("empty executable path")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return nil, fmt.Errorf("empty child role")
	}
	endpoint := normalizeIPCEndpoint(fmt.Sprintf("mdt-server-%s-%d", role, time.Now().UnixNano()))
	args := []string{
		"--core-role=" + role,
		"--ipc-endpoint=" + endpoint,
		"--parent-pid=" + strconv.Itoa(os.Getpid()),
	}
	args = append(args, extraArgs...)
	cmd := exec.Command(exePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var ipcConnErr error
	for attempt := 0; attempt < 40; attempt++ {
		c, err := dialIPC(endpoint, 250*time.Millisecond)
		if err == nil {
			client := newIPCClient(c)
			var pong map[string]any
			if err := client.Call("ping", nil, &pong); err == nil {
				child := &childCoreProcess{
					Role:     role,
					Endpoint: endpoint,
					Cmd:      cmd,
					Client:   client,
				}
				child.beginWait()
				return child, nil
			}
			_ = c.Close()
			ipcConnErr = err
		} else {
			ipcConnErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if ipcConnErr != nil {
		return nil, ipcConnErr
	}
	return nil, fmt.Errorf("failed to connect child core %s", role)
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(exe), nil
}

type remoteCore2Client struct {
	client *ipcClient
}

func (r *remoteCore2Client) persistence(req ipcCore2PersistenceRequest) (PersistenceResult, error) {
	if r == nil || r.client == nil {
		return PersistenceResult{}, fmt.Errorf("remote core2 client not ready")
	}
	var resp ipcCore2PersistenceResponse
	if err := r.client.CallWithTimeout("core2."+req.Action, req, &resp, core2IPCTimeout); err != nil {
		return PersistenceResult{}, err
	}
	return PersistenceResult{
		StateData: resp.StateData,
		WorldData: resp.WorldData,
	}, nil
}

func (r *remoteCore2Client) worldStream(req ipcCore2WorldStreamRequest) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("remote core2 client not ready")
	}
	var resp map[string]any
	return r.client.CallWithTimeout("core2."+req.Action, req, &resp, core2IPCTimeout)
}

func (r *remoteCore2Client) rewriteWorldStream(req ipcCore2WorldStreamRequest) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("remote core2 client not ready")
	}
	var resp ipcCore2WorldStreamResponse
	if err := r.client.CallWithTimeout("core2.rewrite_worldstream", req, &resp, core2IPCTimeout); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (r *remoteCore2Client) stats() (int64, int64, int64, int64, int64, error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, 0, 0, fmt.Errorf("remote core2 client not ready")
	}
	var resp ipcStatsResponse
	if err := r.client.Call("stats", nil, &resp); err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return resp.Received, resp.Processed, resp.Dropped, resp.QueueSize, resp.LatencyMs, nil
}

type remoteCore3Client struct {
	client *ipcClient
}

func (r *remoteCore3Client) getWorld(path string) (SnapshotResult, error) {
	if r == nil || r.client == nil {
		return SnapshotResult{}, fmt.Errorf("remote core3 client not ready")
	}
	var resp ipcWorldCacheResponse
	if err := r.client.CallWithTimeout("core3.get_world", ipcWorldCacheRequest{Path: path}, &resp, core2IPCTimeout); err != nil {
		return SnapshotResult{}, err
	}
	return SnapshotResult{
		Data:      resp.Data,
		CorePos:   protocol.Point2{X: resp.CorePosX, Y: resp.CorePosY},
		CorePosOK: resp.CorePosOK,
		Level:     resp.Level,
	}, nil
}

func (r *remoteCore3Client) invalidateWorld(path string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("remote core3 client not ready")
	}
	var resp map[string]any
	return r.client.CallWithTimeout("core3.invalidate_world", ipcInvalidateWorldRequest{Path: path}, &resp, core2IPCTimeout)
}

func (r *remoteCore3Client) stats() (int64, int64, int64, int64, int64, error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, 0, 0, fmt.Errorf("remote core3 client not ready")
	}
	var resp ipcStatsResponse
	if err := r.client.CallWithTimeout("stats", nil, &resp, core2IPCTimeout); err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return resp.Received, resp.Processed, resp.Dropped, resp.QueueSize, resp.LatencyMs, nil
}

type remoteCore4Client struct {
	client *ipcClient
}

func (r *remoteCore4Client) allowConnection(ip, uuid string) (PolicyResult, error) {
	var resp ipcPolicyResponse
	if err := r.client.CallWithTimeout("core4.allow_connection", ipcAllowConnectionRequest{IP: ip, UUID: uuid}, &resp, policyIPCTimeout); err != nil {
		return PolicyResult{}, err
	}
	return PolicyResult{Allowed: resp.Allowed, PlayerShard: resp.PlayerShard, CoreShard: resp.CoreShard}, nil
}

func (r *remoteCore4Client) allowPacket(ip string, connID int32, uuid, packet string) (PolicyResult, error) {
	var resp ipcPolicyResponse
	if err := r.client.CallWithTimeout("core4.allow_packet", ipcAllowPacketRequest{IP: ip, ConnID: connID, UUID: uuid, Packet: packet}, &resp, policyIPCTimeout); err != nil {
		return PolicyResult{}, err
	}
	return PolicyResult{Allowed: resp.Allowed, PlayerShard: resp.PlayerShard, CoreShard: resp.CoreShard}, nil
}

func (r *remoteCore4Client) recordOpen(connID int32, ip, uuid string) error {
	var resp map[string]any
	return r.client.CallWithTimeout("core4.record_open", ipcRecordConnectionRequest{ConnID: connID, IP: ip, UUID: uuid}, &resp, policyIPCTimeout)
}

func (r *remoteCore4Client) recordClose(connID int32) error {
	var resp map[string]any
	return r.client.CallWithTimeout("core4.record_close", ipcRecordConnectionRequest{ConnID: connID}, &resp, policyIPCTimeout)
}

func (r *remoteCore4Client) playerShard(uuid, ip string) (PolicyResult, error) {
	var resp ipcPolicyResponse
	if err := r.client.CallWithTimeout("core4.player_shard", ipcPlayerShardRequest{UUID: uuid, IP: ip}, &resp, policyIPCTimeout); err != nil {
		return PolicyResult{}, err
	}
	return PolicyResult{Allowed: resp.Allowed, PlayerShard: resp.PlayerShard, CoreShard: resp.CoreShard}, nil
}

func (r *remoteCore4Client) coreShard(key string) (PolicyResult, error) {
	var resp ipcPolicyResponse
	if err := r.client.CallWithTimeout("core4.core_shard", ipcCoreShardRequest{Key: key}, &resp, policyIPCTimeout); err != nil {
		return PolicyResult{}, err
	}
	return PolicyResult{Allowed: resp.Allowed, PlayerShard: resp.PlayerShard, CoreShard: resp.CoreShard}, nil
}

func (r *remoteCore4Client) stats() (int64, int64, int64, int64, int64, error) {
	var resp ipcStatsResponse
	if err := r.client.Call("stats", nil, &resp); err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return resp.Received, resp.Processed, resp.Dropped, resp.QueueSize, resp.LatencyMs, nil
}

type coreSupervisor struct {
	mu               sync.Mutex
	children         map[string]*childCoreProcess
	closed           bool
	unexpectedExitFn func(role string, err error)
}

func newCoreSupervisor() *coreSupervisor {
	return &coreSupervisor{children: map[string]*childCoreProcess{}}
}

func (s *coreSupervisor) setUnexpectedExitHandler(fn func(role string, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unexpectedExitFn = fn
}

func (s *coreSupervisor) add(role string, child *childCoreProcess) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = closeChildCoreProcessFn(child)
		return
	}
	var previous *childCoreProcess
	if existing, ok := s.children[role]; ok && existing != nil && existing != child {
		previous = existing
	}
	s.children[role] = child
	s.mu.Unlock()
	if previous != nil {
		_ = closeChildCoreProcessFn(previous)
	}
	go s.watchChild(role, child)
}

func (s *coreSupervisor) watchChild(role string, child *childCoreProcess) {
	if s == nil || child == nil {
		return
	}
	var exitErr error
	if err, ok := <-child.Exited(); ok {
		exitErr = err
	}
	s.mu.Lock()
	if existing, ok := s.children[role]; ok && existing == child {
		delete(s.children, role)
	}
	closed := s.closed || child.Closing()
	handler := s.unexpectedExitFn
	s.mu.Unlock()
	if !closed && handler != nil {
		handler(role, exitErr)
	}
}

func (s *coreSupervisor) closeAll() {
	s.mu.Lock()
	s.closed = true
	children := make([]*childCoreProcess, 0, len(s.children))
	for _, child := range s.children {
		children = append(children, child)
	}
	s.children = map[string]*childCoreProcess{}
	s.mu.Unlock()
	for _, child := range children {
		_ = child.Close()
	}
}
