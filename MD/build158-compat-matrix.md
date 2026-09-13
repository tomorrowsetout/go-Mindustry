# Build 158 Compatibility Matrix

This file records the current phase-1 compatibility target against the official Mindustry build 158 server in `/server/Mindustry-158`.

- Official Java root: `/server/Mindustry-158`
- Main Go server repo: `/server/go-Mindustry-main`
- Compatibility rule: unmodified build 158 clients must interoperate with the Go server without client-side patches.
- Validation rule: connection and sync work is only considered stable after packet-id checks, focused Go tests, and live-client verification all pass.

## Status Scale

| Status | Meaning |
| --- | --- |
| `done` | Implemented and covered by direct tests or stable runtime behavior. |
| `partial` | Implemented baseline exists, but behavior or coverage is still incomplete. |
| `planned` | Not yet implemented as a compatibility-complete slice. |

## Matrix

| Java truth source | Go package(s) | Verification scenario | Status | Notes |
| --- | --- | --- | --- | --- |
| `mindustry.core.NetServer` connect/admission flow | `internal/net`, `internal/protocol` | `ConnectPacket -> admission -> WorldStream -> connectConfirm` | `partial` | Go now targets build 158 and keeps initial join aligned with vanilla: stream world first, wait for client confirm before post-connect sync. |
| `mindustry.core.NetClient` connect lifecycle | `internal/net`, `internal/worldstream`, `internal/world` | Client loads streamed world, then calls `connectConfirm` | `partial` | Initial stream and confirm gate are covered by focused tests; live-client verification is still required. |
| `mindustry.net.Packets`, `mindustry.io.TypeIO` | `internal/protocol`, `internal/net` | Packet IDs, field order, priorities, TCP framing | `partial` | Critical sync packet IDs are locked by tests. |
| `WorldStream`, `StreamBegin`, `StreamChunk` | `internal/protocol`, `internal/worldstream`, `internal/net` | Streamed map transfer and resync stream | `partial` | Initial connect sends only the world stream, while resync/hot reload sends `worldDataBegin` before the stream. |
| `stateSnapshot`, `entitySnapshot`, `hiddenSnapshot`, `blockSnapshot` | `internal/protocol`, `internal/net`, `internal/world` | Realtime sync after confirm | `partial` | Baseline snapshot loop exists; parity still needs broader play scenarios. |
| `BuildPlan`, `clientSnapshot`, block requests | `internal/protocol`, `internal/net`, `internal/world` | Place, break, preview, rotate, configure | `partial` | Client input packets are decoded and routed into server-authoritative hooks. |

## Level 2 Additions (this session)

| Java truth source | Go package(s) | Verification scenario | Status | Notes |
| --- | --- | --- | --- | --- |
| — (architecture) | `internal/core` | `Module` lifecycle, `Container` topo sort + cycle detect | `done` | 模块化地基：`Module` 契约 + `Container` + 各子系统窄接口（`NetModule` 等可替换边界）。 |
| — (test framework) | `internal/net/harness` | 真实 TCP 模拟 build-158 客户端跑通完整握手 | `done` | `SimClient` 复用服务端 `Serializer`；`CompatTrace` 锁死线序；`TestSimClientVanillaJoinAndSync` PASS。 |
| Client→server inbound packets (14) | `internal/net` | 14 个此前未处理的 C→S 包路由到 hook | `partial` | 已补齐 14 个 `handlePacket` case + 9 个 nil-safe hook 字段；hook 目前为占位接收，真实游戏逻辑待接入。 |
| `mindustry.net` framing (outer 2-byte len) | `internal/net`, `internal/protocol` | 线帧 `[2B len][WriteObject]` + framework `0xFE` 无长度前缀 | `done` | 见 `MD/wire-protocol.md`；早期测试桩漏外层长度导致 EOF 的 bug 已修复。 |

## Immediate Phase-1 Focus

1. Keep build 158 packet IDs, field order, nullability, and sync priorities stable.
2. Preserve vanilla join order: accept `ConnectPacket`, assign player state, send `WorldStream`, wait for `connectConfirm`, then start snapshots and respawn.
3. Use `/server/Mindustry-158` as the reference for every connection or sync behavior change.
