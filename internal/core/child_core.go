package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"mdt-server/internal/config"
)

type ipcStatsResponse struct {
	Received  int64 `json:"received"`
	Processed int64 `json:"processed"`
	Dropped   int64 `json:"dropped"`
	QueueSize int64 `json:"queue_size"`
	LatencyMs int64 `json:"latency_ms"`
}

type ipcWorldCacheRequest struct {
	Path string `json:"path"`
}

type ipcInvalidateWorldRequest struct {
	Path string `json:"path"`
}

type ipcWorldCacheResponse struct {
	Data      []byte `json:"data"`
	CorePosX  int32  `json:"core_pos_x"`
	CorePosY  int32  `json:"core_pos_y"`
	CorePosOK bool   `json:"core_pos_ok"`
	Level     string `json:"level"`
}

type ipcAllowConnectionRequest struct {
	IP   string `json:"ip"`
	UUID string `json:"uuid"`
}

type ipcAllowPacketRequest struct {
	IP     string `json:"ip"`
	ConnID int32  `json:"conn_id"`
	UUID   string `json:"uuid"`
	Packet string `json:"packet"`
}

type ipcRecordConnectionRequest struct {
	ConnID int32  `json:"conn_id"`
	IP     string `json:"ip"`
	UUID   string `json:"uuid"`
}

type ipcPlayerShardRequest struct {
	UUID string `json:"uuid"`
	IP   string `json:"ip"`
}

type ipcCoreShardRequest struct {
	Key string `json:"key"`
}

type ipcPolicyResponse struct {
	Allowed     bool `json:"allowed"`
	PlayerShard int  `json:"player_shard"`
	CoreShard   int  `json:"core_shard"`
}

type ipcCore2PersistenceRequest struct {
	Action    string `json:"action"`
	Path      string `json:"path"`
	StateData []byte `json:"state_data"`
	WorldData []byte `json:"world_data"`
}

type ipcCore2PersistenceResponse struct {
	StateData []byte `json:"state_data"`
	WorldData []byte `json:"world_data"`
}

type ipcCore2WorldStreamRequest struct {
	Action        string            `json:"action"`
	Path          string            `json:"path"`
	PlayerID      int32             `json:"player_id"`
	Tags          map[string]string `json:"tags"`
	ModelData     []byte            `json:"model_data"`
	Wave          int32             `json:"wave"`
	WaveTimeTicks float32           `json:"wave_time_ticks"`
	Tick          float64           `json:"tick"`
	Rules         string            `json:"rules"`
}

type ipcCore2WorldStreamResponse struct {
	Data []byte `json:"data"`
}

func RunChildCore(role, endpoint string, parentPID int, persistCfg ...config.PersistConfig) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return fmt.Errorf("child core role is empty")
	}
	ln, err := listenIPC(endpoint)
	if err != nil {
		return err
	}
	defer ln.Close()

	stopParentWatch := make(chan struct{})
	defer close(stopParentWatch)
	if parentPID > 0 {
		go watchParentProcess(parentPID, stopParentWatch)
	}
	core2PersistCfg := config.PersistConfig{}
	if len(persistCfg) > 0 {
		core2PersistCfg = persistCfg[0]
	}

	switch role {
	case "core2":
		c2 := NewCore2(Config{Name: "core2-child", MessageBuf: 128, WorkerCount: 1})
		c2.SetPersistConfig(core2PersistCfg)
		c2.Start()
		defer c2.Stop()
		return serveChildCore(role, ln, func(method string, payload json.RawMessage) (any, error) {
			switch method {
			case "ping":
				return map[string]any{"ok": true, "role": role}, nil
			case "stats":
				received, processed, dropped, queueSize, latency := c2.Stats()
				return ipcStatsResponse{Received: received, Processed: processed, Dropped: dropped, QueueSize: queueSize, LatencyMs: latency}, nil
			case "core2.save_state", "core2.load_state", "core2.save_world", "core2.load_world":
				var req ipcCore2PersistenceRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				ch := make(chan PersistenceResult, 1)
				msg := &PersistenceMessage{
					Action:     req.Action,
					Path:       req.Path,
					StateData:  req.StateData,
					WorldData:  req.WorldData,
					ResultChan: ch,
				}
				if !c2.Send(msg) {
					return nil, fmt.Errorf("core2 queue full for %s", req.Action)
				}
				res := <-ch
				if res.Error != nil {
					return nil, res.Error
				}
				return ipcCore2PersistenceResponse{
					StateData: res.StateData,
					WorldData: res.WorldData,
				}, nil
			case "core2.load_model", "core2.save_snapshot", "core2.rewrite_player":
				var req ipcCore2WorldStreamRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				if !c2.Send(&WorldStreamMessage{
					Action:    req.Action,
					Path:      req.Path,
					PlayerID:  req.PlayerID,
					Tags:      req.Tags,
					ModelData: req.ModelData,
				}) {
					return nil, fmt.Errorf("core2 queue full for %s", req.Action)
				}
				return map[string]any{"ok": true}, nil
			case "core2.rewrite_worldstream":
				var req ipcCore2WorldStreamRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				patched, err := rewriteWorldStreamPayload(req.ModelData, req.PlayerID, req.Rules, req.Wave, req.WaveTimeTicks, req.Tick)
				if err != nil {
					return nil, err
				}
				return ipcCore2WorldStreamResponse{Data: patched}, nil
			case "shutdown":
				return map[string]any{"ok": true}, io.EOF
			default:
				return nil, fmt.Errorf("unsupported core2 ipc method: %s", method)
			}
		})
	case "core3":
		c3 := NewCore3(Config{Name: "core3-child", MessageBuf: 32, WorkerCount: 1})
		c3.SetCacheBaseModel(false)
		c3.SetCopyOnRead(false)
		c3.Start()
		defer c3.Stop()
		return serveChildCore(role, ln, func(method string, payload json.RawMessage) (any, error) {
			switch method {
			case "ping":
				return map[string]any{"ok": true, "role": role}, nil
			case "stats":
				received, processed, dropped, queueSize, latency := c3.Stats()
				return ipcStatsResponse{Received: received, Processed: processed, Dropped: dropped, QueueSize: queueSize, LatencyMs: latency}, nil
			case "core3.get_world":
				var req ipcWorldCacheRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				res, err := c3.GetWorldCache(req.Path)
				if err != nil {
					return nil, err
				}
				return ipcWorldCacheResponse{
					Data:      res.Data,
					CorePosX:  res.CorePos.X,
					CorePosY:  res.CorePos.Y,
					CorePosOK: res.CorePosOK,
					Level:     res.Level,
				}, nil
			case "core3.invalidate_world":
				var req ipcInvalidateWorldRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true}, c3.InvalidateWorldCache(req.Path)
			case "shutdown":
				return map[string]any{"ok": true}, io.EOF
			default:
				return nil, fmt.Errorf("unsupported core3 ipc method: %s", method)
			}
		})
	case "core4":
		c4 := NewCore4(Config{Name: "core4-child", MessageBuf: 64, WorkerCount: 1})
		c4.Start()
		defer c4.Stop()
		return serveChildCore(role, ln, func(method string, payload json.RawMessage) (any, error) {
			switch method {
			case "ping":
				return map[string]any{"ok": true, "role": role}, nil
			case "stats":
				received, processed, dropped, queueSize, latency := c4.Stats()
				return ipcStatsResponse{Received: received, Processed: processed, Dropped: dropped, QueueSize: queueSize, LatencyMs: latency}, nil
			case "core4.allow_connection":
				var req ipcAllowConnectionRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				res, err := c4.AllowConnection(req.IP, req.UUID)
				if err != nil {
					return nil, err
				}
				return ipcPolicyResponse{Allowed: res.Allowed, PlayerShard: res.PlayerShard, CoreShard: res.CoreShard}, nil
			case "core4.allow_packet":
				var req ipcAllowPacketRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				res, err := c4.AllowPacket(req.IP, req.ConnID, req.UUID, req.Packet)
				if err != nil {
					return nil, err
				}
				return ipcPolicyResponse{Allowed: res.Allowed, PlayerShard: res.PlayerShard, CoreShard: res.CoreShard}, nil
			case "core4.record_open":
				var req ipcRecordConnectionRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				c4.RecordConnectionOpen(req.ConnID, req.IP, req.UUID)
				return map[string]any{"ok": true}, nil
			case "core4.record_close":
				var req ipcRecordConnectionRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				c4.RecordConnectionClose(req.ConnID)
				return map[string]any{"ok": true}, nil
			case "core4.player_shard":
				var req ipcPlayerShardRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				res, err := c4.PlayerShard(req.UUID, req.IP)
				if err != nil {
					return nil, err
				}
				return ipcPolicyResponse{Allowed: res.Allowed, PlayerShard: res.PlayerShard, CoreShard: res.CoreShard}, nil
			case "core4.core_shard":
				var req ipcCoreShardRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return nil, err
				}
				res, err := c4.CoreShard(req.Key)
				if err != nil {
					return nil, err
				}
				return ipcPolicyResponse{Allowed: res.Allowed, PlayerShard: res.PlayerShard, CoreShard: res.CoreShard}, nil
			case "shutdown":
				return map[string]any{"ok": true}, io.EOF
			default:
				return nil, fmt.Errorf("unsupported core4 ipc method: %s", method)
			}
		})
	default:
		return fmt.Errorf("unsupported child core role: %s", role)
	}
}

func serveChildCore(role string, ln net.Listener, handler func(method string, payload json.RawMessage) (any, error)) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		env, err := readIPCEnvelope(conn)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if env.Type != "request" {
			if err := writeIPCEnvelope(conn, ipcEnvelope{ID: env.ID, Type: "response", Error: fmt.Sprintf("invalid ipc envelope type for %s: %s", role, env.Type)}); err != nil {
				return err
			}
			continue
		}
		resp, callErr := handler(env.Method, env.Payload)
		reply := ipcEnvelope{ID: env.ID, Type: "response"}
		if callErr != nil && callErr != io.EOF {
			reply.Error = callErr.Error()
		}
		if resp != nil {
			raw, err := json.Marshal(resp)
			if err != nil {
				reply.Error = err.Error()
			} else {
				reply.Payload = raw
			}
		}
		if err := writeIPCEnvelope(conn, reply); err != nil {
			return err
		}
		if callErr == io.EOF {
			return nil
		}
	}
}

func watchParentProcess(parentPID int, stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if parentPID <= 0 {
				return
			}
			if !parentProcessAlive(parentPID) {
				os.Exit(0)
			}
		}
	}
}
