package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/protocol"
)

const policyIPCTimeout = 250 * time.Millisecond

type childCoreProcess struct {
	Role     string
	Endpoint string
	Cmd      *exec.Cmd
	Client   *ipcClient
}

func (p *childCoreProcess) Close() error {
	if p == nil {
		return nil
	}
	if p.Client != nil {
		_ = p.Client.Call("shutdown", nil, nil)
		_ = p.Client.Close()
	}
	if p.Cmd != nil && p.Cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- p.Cmd.Wait() }()
		select {
		case <-time.After(2 * time.Second):
			_ = p.Cmd.Process.Kill()
			<-done
		case <-done:
		}
	}
	return nil
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
				return &childCoreProcess{
					Role:     role,
					Endpoint: endpoint,
					Cmd:      cmd,
					Client:   client,
				}, nil
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

type remoteCore3Client struct {
	client *ipcClient
}

func (r *remoteCore3Client) getWorld(path string) (SnapshotResult, error) {
	if r == nil || r.client == nil {
		return SnapshotResult{}, fmt.Errorf("remote core3 client not ready")
	}
	var resp ipcWorldCacheResponse
	if err := r.client.Call("core3.get_world", ipcWorldCacheRequest{Path: path}, &resp); err != nil {
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
	return r.client.Call("core3.invalidate_world", ipcInvalidateWorldRequest{Path: path}, &resp)
}

func (r *remoteCore3Client) stats() (int64, int64, int64, int64, int64, error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, 0, 0, fmt.Errorf("remote core3 client not ready")
	}
	var resp ipcStatsResponse
	if err := r.client.Call("stats", nil, &resp); err != nil {
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
	mu       sync.Mutex
	children map[string]*childCoreProcess
}

func newCoreSupervisor() *coreSupervisor {
	return &coreSupervisor{children: map[string]*childCoreProcess{}}
}

func (s *coreSupervisor) add(role string, child *childCoreProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.children[role] = child
}

func (s *coreSupervisor) closeAll() {
	s.mu.Lock()
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
