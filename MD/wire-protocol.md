# Build 158 Wire Protocol Reference

权威的 build 158 线协议规范，供所有连接 / 同步 / 包解析相关工作参考。

- 配套源码：`internal/net/serializer.go`（`Serializer.WriteObject` / `Serializer.ReadObject`）、`internal/net/server.go`（`Conn.ReadObject` / `Conn.sendNowLocked`）、`internal/protocol/framework.go`。
- 参考实现：`/server/Mindustry-158`（Java 原版 `mindustry.net`、`mindustry.io`）。
- 兼容红线：**不允许**改变任何包的线 ID、字段顺序、可空性、同步优先级，或帧格式。所有改动必须以 `/server/Mindustry-158` 为准。

---

## 1. 两层帧结构

线协议是**两层封装**：

```
[ 2 字节大端长度 n ]  [ WriteObject 输出（n 字节） ]
└──── 外层（Conn 层） ──┘└──────── 内层（Serializer 层） ────────┘
```

- **外层（Conn 层）**：仅一个 2 字节大端无符号长度 `n`，等于后续内层输出的字节数。读取时先读 `n`，再精确读 `n` 字节，然后交给 `Serializer.ReadObject` 解码。
  - 写：`conn.Conn.Write(lenbuf)` + `conn.Conn.Write(payload)`（`server.go:3131`）。
  - 读：`Conn.ReadObject` 读 2 字节 → `n` → 读 `n` 字节 → `serial.ReadObject`（`server.go:3022`）。
- **内层（Serializer 层）**：`WriteObject` 的输出，分两种形状（见第 2 节）。

> ⚠️ 常见错误：客户端 / 测试桩**两端都必须加外层 2 字节长度前缀**。早期 `SimClient` 漏掉这层，导致服务端读到 `unexpected EOF`、收包 trace 全空。正确做法是复用服务端自身的 `*net.Serializer`，逐字节镜像 `Conn.ReadObject`。

---

## 2. 内层对象形状

`Serializer.ReadObject` 先读第 1 字节决定形状：

### 2.1 Framework 消息（握手 / 心跳 / 发现）

第 1 字节为 `0xFE`（有符号字节即 `-2`）。**没有长度前缀，也没有压缩字节**：

```
0xFE  [ framework-id 字节 ]  [ body ]
```

- framework-id 取值（`internal/protocol/framework.go`）：

  | 常量 | 值 | body 内容 | 方向 |
  | --- | --- | --- | --- |
  | `FrameworkPing` | `0` | `int32 id` + `byte isReply` | 双向 |
  | `FrameworkDiscover` | `1` | 无 | C→S |
  | `FrameworkKeepAlive` | `2` | 无 | 双向 |
  | `FrameworkRegisterUD` | `3` | `int32 connectionID` | 双向 |
  | `FrameworkRegisterTC` | `4` | `int32 connectionID` | 双向 |

- 解码函数：`readFramework`（`serializer.go:210`）；编码：`writeFramework`（`serializer.go:247`）。

### 2.2 普通包（Regular Packet）

第 1 字节为包线 ID（非 `0xFE`）：

```
[ packet-id 字节 ]  [ uint16 大端 length ]  [ compression 字节 ]  [ payload ]
```

- `length`：payload 字节数（**不含**前面的 id / length / compression 三字节）。
- `compression`：`0` = 原始 payload；`1` = lz4 压缩 payload。其它值返回 `ErrCompressedUnsupported`。
- 解码：`serializer.go:32` 起；编码：`serializer.go:102` 起。

---

## 3. 压缩规则

- 仅在 `payloadLen >= 36` **且**不是 `StreamChunk` 时考虑压缩（`shouldCompressPacket`，`serializer.go:147`）。
- 以下快照包**即使很大也禁用压缩**（历史 lz4 崩溃教训，优先稳定）：
  - `Remote_NetClient_entitySnapshot_32`
  - `Remote_NetClient_hiddenSnapshot_33`
  - `Remote_NetClient_blockSnapshot_34`
  - `Remote_NetClient_stateSnapshot_35`
- 压缩实现 `tryCompressPayload` 带 recover 保护，压缩失败或体积未变小则回退原始（`serializer.go:164`）。

---

## 4. 读端特例（wire 省略注入参数）

部分官方 C→S 包在线上的结构体比生成代码少一个注入的 `Player` / `Entity` 参数，`ReadObject` 对以下 id 做特判跳过正常 `p.Read`：

| 包 id | 类型 | 说明 |
| --- | --- | --- |
| `31` | `Remote_NetServer_connectConfirm_50` | 线上无 player 参数 |
| `36` | `Remote_NetServer_requestDebugStatus_36` | player 为 nil |
| `72` | `Remote_NetClient_ping_18` | 线上仅 `int64 time`，无 player |
| `81` | `Remote_NetServer_requestBlockSnapshot_45` | 线上仅 `int32 pos`，无 player |
| `142` | `Remote_InputHandler_unitClear_95` | 线上无 player |

（`serializer.go:85` 的 switch）

---

## 5. 握手顺序（连接生命周期）

1. TCP accept 后，服务端立即向该连接发送 **`RegisterTCP`**（framework，`0xFE 04 <int32 connectionID>`）。
2. 客户端**必须回显相同 `connectionID`** 的 `RegisterTCP`，之后才能发送 `ConnectPacket`。
3. 客户端发送 `ConnectPacket`（普通包，含 `Version=158`、`VersionType="official"`、`Name`、`Locale`、`UUID`、`USID` 等）。
4. 服务端校验 → 分配玩家状态 → 发送 `WorldStream`（`StreamBegin` + `StreamChunk`）。**首次加入不发 `worldDataBegin`**。
5. 客户端加载世界后发送 `connectConfirm`（id `31`）。
6. 服务端收到 `connectConfirm` 后才标记连接完全建立，开始 `stateSnapshot` / `entitySnapshot` / `blockSnapshot` 等实时同步。
7. 无 UDP 注册时，`sendUnreliable` 自动回退 TCP（`server.go` `sendUnreliable`），故纯 TCP 客户端也能收到实时快照。

> 该顺序由 `TestSimClientVanillaJoinAndSync`（`internal/net/harness/simclient_test.go`）锁死，任何打乱顺序的改动都会让测试失败。

---

## 6. 复用与校验

- **服务端 ↔ 测试桩必须复用同一套 `*net.Serializer`**：构造方式与服务端 `NewServer` 内部一致（`protocol.NewContentRegistry()` + `protocol.NewRegistry()` + `protocol.NewReader/WriterWithContext(ctx)`）。
- 包线 ID 由 `officialPacketRegistry`（build 158 注册顺序）决定；`WriteObject` 优先用官方 ID，回退到 `s.Registry`。
- 调试：每个 `Conn` 记录 `lastRecvPacketID` / `lastRecvFrameworkID` 与 `bytesSent` / `sendCount`，供追踪（见 `configs/tracepoints.toml`）。

---

## 7. 已知非阻断项

- 服务端 `world handshake inspect` 会把 world payload 当作 zlib 解（真实世界是压缩的）；测试用裸字节时会打 `zlib: invalid header` 日志，不影响 `WorldStream` 收发，测试仍通过。
