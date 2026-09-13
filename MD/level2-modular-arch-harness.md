# Level 2 — Modular Architecture & Real-time Client Test Framework

## Target

- Go server: `/server/go-Mindustry-main`（`mdt-server`，build 158）
- Official reference: `/server/Mindustry-158`
- Scope: 本次会话在 Level 1（连接/同步）之上新增两块地基——**模块化架构**与**真实 socket 客户端测试框架**，并补齐客户端→服务端 inbound 包的真实缺口，让"完整还原"有可验证的地基。

## Reference Checked

- `/server/go-Mindustry-main/internal/net/serializer.go`
  - `Serializer.WriteObject` / `ReadObject` 帧格式、压缩、framework 特例（id 31/36/72/81/142）
- `/server/go-Mindustry-main/internal/net/server.go`
  - `Conn.ReadObject`（外层 2 字节长度前缀）、`Conn.sendNowLocked`（写外层前缀）
  - `RegisterTCP` 握手、`handlePacket` 的 14 个新增 case
- `/server/go-Mindustry-main/internal/protocol/remote_packets.go`
  - 14 个此前未处理的客户端→服务端包结构定义

## Changed / New Files

| Path | Change |
| --- | --- |
| `internal/core/module.go` | **新增**。`Module` 生命周期接口、`LifecycleState`、`Deps`、`Container`（拓扑排序启动/关闭、依赖环检测、`Run`）。 |
| `internal/core/contracts.go` | **新增**。各子系统窄契约：`NetModule` / `WorldModule` / `SimModule` / `LogicModule` / `PersistModule` / `APIModule`（可替换边界，未来可换语言实现）。 |
| `internal/core/container_test.go` | **新增**。容器生命周期 + 依赖环检测测试。 |
| `internal/net/module_adapter.go` | **新增**。`NetModuleAdapter` 证明现有 `*net.Server` 已满足 `core.NetModule` 契约（含编译期断言）。 |
| `internal/net/harness/simclient.go` | **新增**。`SimClient`：真实 TCP 上模拟 build-158 客户端，复用服务端 `*net.Serializer`；含 14 个 inbound 包 `Send*` 辅助方法。 |
| `internal/net/harness/compat.go` | **新增**。`CompatTrace`：按线序记录收包，提供 `Order/Count/Seen/AssertOrder` 断言面。 |
| `internal/net/harness/simclient_test.go` | **新增**。`TestSimClientVanillaJoinAndSync`：真实服务端跑通完整 vanilla 握手。 |
| `internal/net/harness/inbound_hooks_test.go` | **新增**。`TestSimClientInboundPacketHooks`：校验 14 个 inbound 包路由到 hook 不崩溃。 |
| `internal/net/server.go` | 新增 9 个 nil-safe hook 字段 + 14 个 `handlePacket` case（见下）；新增 `BoundAddr()` 便于测试发现监听地址。 |
| `MD/wire-protocol.md` | **新增**。线协议权威规范（两层帧、压缩、framework、握手顺序、读端特例）。 |
| `MD/architecture.md` | **新增**。模块化架构 + 测试框架说明。 |
| `MD/build158-compat-matrix.md` | 更新：新增架构/测试框架行，状态前移。 |

## Optimization Result

### 模块化架构地基
- 引入 `Module` 生命周期契约 + `Container`（拓扑排序、依赖环检测、优雅关闭）。
- 每个子系统用窄接口（`NetModule` 等）切分可替换边界——owner 的多语言取向在此落地：未来某模块用 Rust/C 重写只需保持同一接口。
- `NetModuleAdapter` 证明现有 `*net.Server` 今天即可被 `Container` 托管、未来可无缝替换。

### 实时客户端测试框架
- `SimClient` 在真实 TCP 上模拟官方客户端，**复用服务端自身 Serializer**，保证字节级线保真。
- `CompatTrace` 锁死服务端→客户端包顺序，作为防回归断言面。
- `TestSimClientVanillaJoinAndSync` 启动真实服务端，跑通 `RegisterTCP → ConnectPacket → WorldStream → connectConfirm → 状态/实体快照`。

### 关闭真实 inbound 缺口
- 之前统计 `handlePacket` 处理 107/154 个 `Remote_*` 包、剩 47 个。精确核对后：约 33 个是**服务端→客户端出站包**（本来就不进 `handlePacket`）；**真正缺失的是客户端→服务端、完全未处理的 inbound 包，精确 14 个**。
- 这 14 个现已全部补齐（9 个 nil-safe hook 字段：`OnTileTap` / `OnTextInput` / `OnCopyToClipboard` / `OnAdminRequest` / `OnClientLogicData` / `OnClientPlanSnapshotReceived` / `OnDebugStatus` / `OnServerRelay` / `OnServerRelayBinary`）。未设置 hook 时仅记 dev 日志、不崩溃、也不静默丢弃。

### 关键线协议纠错
- 修复了导致早期框架收包 trace 全空的致命 bug：真实线帧是 `[2字节大端总长度 n][WriteObject 输出]`，`WriteObject` 内 framework 消息（`0xFE+id+body`）**无**长度前缀。早期 `SimClient` 收发两侧都漏掉外层 2 字节长度，现改为与 `Conn.ReadObject` 逐字节一致。
- 握手顺序对齐原版：accept 后服务端发 `RegisterTCP`，客户端**必须回显同 connectionID** 后才能发 `ConnectPacket`（已由 `SimClient.Connect` 实现并测试锁死）。

## Verification

```bash
# 全量构建
go build -buildvcs=false ./...

# 测试框架
go test -buildvcs=false -run TestSimClient ./internal/net/harness/...
go test -buildvcs=false ./internal/core/...

# 既有 net 包测试（确认无回归）
go test -buildvcs=false ./internal/net/
```

近期结果：

```text
ok  	mdt-server/internal/net/harness	3.16s   # TestSimClientVanillaJoinAndSync
ok  	mdt-server/internal/net/harness	0.04s   # TestSimClientInboundPacketHooks
ok  	mdt-server/internal/net        	3.335s  # 既有测试无回归
ok  	mdt-server/internal/core        	0.0s    # 容器测试
```

## Remaining Work

1. 实现 14 个 hook 的真实游戏逻辑（依赖 world/unit/logic 模拟进度）：`OnTileTap` 真正改世界、`OnClientLogicData` 喂逻辑解释器、`OnServerRelay` 转发给其他玩家等。
2. 把含 `Entity` 的包（`adminRequest` / `clientLogicData` / relay 变体）也纳入 `SimClient` 对拍测试。
3. 拆分巨型文件：`world.go`（14.7k）、`server.go`（7k）、`remote_packets.go`（6.3k）等，提升可读性。
4. 用 `SimClient` + `CompatTrace` 扩展逐包级线兼容断言，对拍原版更多场景（建造/破坏/单位控制/物品同步）。
