# Architecture & Test Framework

本文档说明 `mdt-server` 的两块地基性设计：

1. **模块化架构**（`internal/core`）：把服务端拆成可替换的子系统，每个模块用接口切分边界。未来的热点模块（例如用 Rust/C 写更快的模拟核心）可以整体替换而不动其余代码。
2. **实时客户端测试框架**（`internal/net/harness`）：在真实 TCP 上模拟官方 build-158 客户端，锁死线协议行为后再谈扩展。

---

## 1. 模块化架构（`internal/core`）

设计目标（来自项目 owner）：服务端核心网络保持 Go，但每个子系统暴露干净的接口，使其可整体替换——边界是接口，不是实现。

### 1.1 Module 生命周期契约

`internal/core/module.go` 定义 `Module` 接口：

```go
type Module interface {
    Name() string                       // 稳定唯一标识，如 "net" / "world"
    Dependencies() []string            // 必须在自身之前启动的模块名
    Init(deps Deps) error              // 准备；通过 deps 取兄弟模块引用
    Start() error                      // 启动运行时工作（监听、tick）；应快速返回
    Stop(ctx context.Context) error    // 优雅关闭
}
```

生命周期状态机（`LifecycleState`）：`registered → initializing → initialized → running → stopping → stopped`，失败为 `failed`。

### 1.2 Container（生命周期容器）

`Container` 拥有模块集合并驱动生命周期，**不含任何游戏逻辑**，便于复用（含未来 FFI 模块宿主）。

- `Register(m)`：注册模块（重名 panic，属编程错误）。
- `Init()`：按依赖拓扑排序依次 `Init`；`orderedStart()` 用 DFS 检测**依赖环**并返回错误。
- `Start()`：按拓扑序 `Start`。
- `Stop(ctx)`：按**逆拓扑序** `Stop`（依赖者先关）。
- `Run(ctx)`：便捷封装 `Init → Start → 等待 ctx 取消 → Stop`。
- `Get` / `Must` / `State` / `Names`：查询与诊断。

`Deps` 是 `Init` 阶段传入的只读视图：`Get(name)` / `Must(name)` 按名取已初始化模块，模块借此拿到它需要的兄弟接口（例如 net 模块取 world 模块）。

> 拓扑排序以 `sort.Strings` 做确定性 tie-break；未知依赖、依赖环都会在 `Init` 时返回明确错误。

### 1.3 子系统契约（`internal/core/contracts.go`）

每个子系统暴露**窄契约**；实现今天用 Go，明天可换语言，只要保持同一接口：

| 契约 | 关键方法 | 职责 |
| --- | --- | --- |
| `NetModule` | `Broadcast(obj any) error` | 线协议：监听、接入、向客户端广播服务端包 |
| `WorldModule` | `LoadMap(payload []byte) error` | 权威世界状态：瓦片、方块、队伍 |
| `SimModule` | `Step() error` | 每 tick 推进模拟：单位、电力、物品、波次 |
| `LogicModule` | `Run(source string) (string, error)` | 运行 Mindustry Logic 语言 |
| `PersistModule` | `Save(key, value)` / `Load(key)` | 持久化：封禁、段位、世界存档 |
| `APIModule` | `Route(pattern string, handler any) error` | 管理 / HTTP 面 |

> `core` 包**禁止 import** `net` / `world` 等具体子系统（避免循环依赖）。载荷用 `any`、地址用 `string`，契约与具体 web 框架解耦。

### 1.4 适配证明（`internal/net/module_adapter.go`）

`NetModuleAdapter` 把现有 `*net.Server` 包装成 `core.Module` + `core.NetModule`，**证明现有网络层今天就已满足可替换边界**：

```go
a := net.NewNetModule(srv)   // *Server → 可托管模块
// Name()="net"; Dependencies()=nil
// Start() 起后台 goroutine 跑 srv.Serve()
// Stop() 调 srv.Shutdown()
// Broadcast(obj) 转 srv.Broadcast(obj)
```

文件末尾有编译期断言 `var _ core.Module = (*NetModuleAdapter)(nil)`，确保契约不被悄悄破坏。

### 1.5 新增一个模块的步骤

1. 在 `internal/core/contracts.go` 若需要新契约，先加接口（窄、用 `any` 载荷）。
2. 实现 `Module`（及对应契约）；`Dependencies()` 声明它依赖的模块名。
3. 用 `Container.Register(module)` 注册；`Init(deps)` 里通过 `deps.Must("world")` 取兄弟引用。
4. 由 `Container.Run(ctx)` 统一驱动，无需改动其它模块。

---

## 2. 实时客户端测试框架（`internal/net/harness`）

为什么需要它：不能声称"完整还原"，却没有自动化客户端侧验证。早期测试用 `net.Pipe` + 直接 `handlePacket` 调用，**没有真实端到端 socket 测试**——这正是该框架要补的洞。

### 2.1 线保真度

`SimClient` 在**真实 TCP socket** 上模拟官方 build-158 客户端，关键点是**复用服务端自己的 `*net.Serializer`**，因此每个字节的封装、字节序、可选 lz4 压缩都和官方服务端一致。详见 `MD/wire-protocol.md`。

构造方式与服务端 `NewServer` 内部一致：

```go
content := protocol.NewContentRegistry()
reg := protocol.NewRegistry()
serial := &mdtnet.Serializer{Registry: reg, Ctx: content.Context()}
```

### 2.2 SimClient API

| 方法 | 说明 |
| --- | --- |
| `NewSimClient(addr string) *SimClient` | 构造，指向如 `"127.0.0.1:6567"` |
| `Connect() error` | 拨号 + 读循环 + **先回显服务端 `RegisterTCP` 再发 `ConnectPacket`** |
| `Send(obj any) error` | 按线封套写单个包（`[2字节大端长度][WriteObject 输出]`） |
| `Confirm() error` | 发 `connectConfirm`，解锁连接后同步 |
| `SendClientSnapshot(x,y float32) error` | 发最简玩家快照（仅位置） |
| `SendTileTap(tilePos int32) error` | 模拟玩家点瓦片（建造/查看输入） |
| `SendTextInput(...)` / `SendCopyToClipboard(...)` | 模拟菜单文本输入 / 复制 |
| `SendDebugStatus(...)` / `SendClientPlanSnapshotReceived(groupID)` | 调试状态 / 建造计划确认 |
| `SendServerRelay(typ, contents)` | 模拟客户端中继服务端包（如逻辑） |
| `WaitFor(pred, timeout) (any, bool)` | 阻塞等匹配对象 |
| `WaitWorldStream(timeout) ([]byte, bool)` | 等完整 `WorldStream`，返回重组的世界 payload |
| `Trace() *CompatTrace` | 取收包轨迹记录器 |

> 14 个 `Send*` 辅助方法对应本次补齐的客户端→服务端 inbound 包（详见 `MD/level2-modular-arch-harness.md`），让 harness 能锁死它们的线路由与 hook。

### 2.3 CompatTrace（防回归断言面）

记录客户端收到的**每一个包（按线序）**，用于断言 build-158 服务端→客户端顺序不被重排（官方客户端会拒绝乱序的关键同步包）：

- `Order() []int`：按接收顺序的线包 id 列表（`0xFE`=254 表示 framework）。
- `Count(id byte) int` / `Seen(id byte) bool`：计数 / 是否出现过。
- `AssertOrder(want ...int) string`：校验 `want` 按序出现（中间可夹其它包），返回首个不匹配。
- `String()`：紧凑摘要，如 `"0,1,126,46"`。

### 2.4 测试

| 测试 | 文件 | 断言 |
| --- | --- | --- |
| `TestSimClientVanillaJoinAndSync` | `simclient_test.go` | 真实服务端跑通 `RegisterTCP → ConnectPacket → WorldStream → connectConfirm → 状态/实体快照`，无回归 |
| `TestSimClientInboundPacketHooks` | `inbound_hooks_test.go` | 6 个代表性 inbound 包（tileTap / textInput / 复制 / debugStatus / plan 确认 / 中继）均路由到 hook 不崩溃 |

`internal/core/container_test.go` 验证容器生命周期与依赖环检测。

### 2.5 运行

```bash
go test -buildvcs=false -run TestSimClient ./internal/net/harness/...
go test -buildvcs=false ./internal/core/...
```

---

## 3. 设计取舍

- **模块边界用接口而非实现**：未来整块替换（含其它语言）不动其余代码，契合 owner 的多语言取向。
- **`handlePacket` 保持薄分发**：新包走既有 hook 字段（`OnTileTap` / `OnTextInput` / `OnAdminRequest` / `OnClientLogicData` / …），游戏逻辑在对应模块实现 hook，无需再动巨型 switch。
- **测试桩复用服务端 Serializer**：线保真度最高，且任何帧格式改动都会让 `TestSimClientVanillaJoinAndSync` 立刻失败。
