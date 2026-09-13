# Level 1 Connect And Sync Report

## Target

- Go server: `/server/go-Mindustry-main`
- Official reference: `/server/Mindustry-158`
- Scope: server-side only, focused on Mindustry build 158 connection admission, world-stream handoff, connect confirmation, and initial sync gating.

## Reference Checked

- `/server/Mindustry-158/core/src/mindustry/core/NetServer.java`
  - `ConnectPacket` handling
  - player creation and team assignment before world transfer
  - `sendWorldData(Player)` world stream flow
  - `connectConfirm(Player)` final connection confirmation
- `/server/Mindustry-158/core/src/mindustry/core/NetClient.java`
  - `WorldStream` load path
  - `worldDataBegin()` resync behavior
- `/server/Mindustry-158/core/src/mindustry/net/Packets.java`
  - `StreamBegin`
  - `StreamChunk`
  - `WorldStream`
  - `ConnectPacket`

## Changed Files

| Path | Change |
| --- | --- |
| `internal/net/server_sync_test.go` | Added/updated build 158 connection lifecycle coverage. The tests now lock the vanilla order: receive `ConnectPacket`, send `WorldStream`, wait for `connectConfirm`, then run post-connect sync. |
| `README.md` | Updated public documentation from build 157 to build 158 so deployers connect the correct official client version. |
| `internal/net/serializer.go` | Updated comments to describe build 158 official packet registration order. |
| `internal/net/server.go` | Updated stream chunk comment to avoid stale build 157 wording. |
| `cmd/mdt-server/main.go` | Updated stale build-authority comment to remove build 157 wording. |
| `md/build158-compat-matrix.md` | Added the active build 158 compatibility matrix and phase-1 connection/sync checklist. |
| `md/build157-compat-matrix.md` | Converted to archived historical context so it is not used as current implementation guidance. |
| `MD/level1-connect-sync-report.md` | Added this report. |

## New Directories

| Path | Purpose |
| --- | --- |
| `MD/` | User-facing work report directory for this optimization pass. |

## Optimization Result

- Confirmed the Go server already follows the important vanilla build 158 first-join shape:
  - validate `ConnectPacket`
  - assign connection identity and team state
  - send `WorldStream` through `StreamBegin` and `StreamChunk`
  - do not send `worldDataBegin` on first join
  - wait for client `connectConfirm`
  - only then mark the connection as fully connected and start post-connect synchronization
- Added regression coverage for this flow so future edits cannot accidentally start snapshots before official client confirmation.
- Kept `worldDataBegin` behavior aligned with vanilla resync/hot reload instead of first join.
- Cleaned stale build 157 documentation/comments that could cause wrong-client deployment.

## Verification

Command run with the valid local Go toolchain:

```bash
GOPROXY=https://goproxy.cn,direct /usr/lib/go-1.26/bin/go test ./internal/protocol ./internal/net -run 'TestBuild158ConnectLifecycleWaitsForOfficialConfirm|TestHandleConnectPacketSendsInitialWorldStreamWithoutWorldDataBegin|TestPacketRegistryUsesOfficial|TestCriticalSyncPacketsKeepOfficialPriorities|TestNewServerUsesConfiguredBuildVersion'
```

Result:

```text
ok  	mdt-server/internal/protocol	0.009s
ok  	mdt-server/internal/net	0.026s
```

## Environment Note

`/usr/bin/go` is `go1.18 gccgo`, which is too old for this repository because `go.mod` requires Go 1.22 and the code uses typed `sync/atomic` APIs. Use `/usr/lib/go-1.26/bin/go` for builds and tests on this machine.

## Remaining Work

- Run a live build 158 Mindustry client against the Go server and capture join, movement, build, break, and respawn behavior.
- Broaden tests from connection lifecycle to real play scenarios: block snapshots, entity snapshots, item/liquid/power sync, unit control, and build plan authority.
- Keep `/server/Mindustry-158` as the reference before changing packet fields, packet IDs, or sync order.
