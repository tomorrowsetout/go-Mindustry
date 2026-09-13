---
feature: align-1603-perf
status: in-progress
updated: 2026-09-13
branch: feature/align-1603-perf
commits: 
---

# Align Go Server to Mindustry 160.3 + Hot-Path C++ Acceleration

## Report

## [S1] Problem

`go-Mindustry-main` targeted **build 159**. The owner has Java sources for **Mindustry 160.3** and wants:

1. **160.3 only** client support.
2. **Faster** ticks using all logical cores (host is 10C/20T).
3. **C++/cgo** on key hot paths.

## [S2] Design

### S2.1 Compatibility target

- `supportedMindustryBuild` = **160**.
- Java 160.3 `Net.java` registers `TextureStream` as framework packet id **6**; Go registry inserts `TextureStream` there, shifting all `@Remote` IDs by +1.
- Seven new 160.3 remotes (`fillTileBlocks/Floors/Overlays`, `menuBuilder*`) inserted in alphabetical factory slots to keep later wire IDs aligned.
- ConnectPacket field layout unchanged vs 159.7 (already matched).

### S2.2 Multi-core runtime budget

- `runtime.cores = 0` → auto `NumCPU`, never above host logical CPUs.
- Dual-core IO workers scale 1/2/3 at ~2/8/16+ cores.
- Sim workers = `cores - 1 - ioWorkers` (min 1).
- Parent sets `GOMAXPROCS(totalCores)` at startup; `bin/configs/core.toml` also defaults to auto.
- Console `scheduler status` prints cores_cfg/effective, NumCPU, GOMAXPROCS, dual_core.

### S2.3 World tick hot-path reductions

- `stepEntities`: one `idToIndex` + spatial build after movement; rebuild spatial only if abilities moved entities.
- Drill/burst/beam: profile resolved once in parallel sample; sequential apply reuses it.

### S2.4 nativespatial (C++ CSR grid + pure-Go fallback)

- Package `internal/nativespatial`.
- C++ uniform grid (cell 64) with CSR layout behind `//go:build cgo`.
- Pure-Go cell-overlap fallback is the default (Windows hosts without gcc).
- `entitySpatialIndex` uses the grid API (`QueryRadius` / cell overlap), preserving `forEachInRange` semantics.

### S2.5 Dual-core visibility

- Startup log and `scheduler status` expose full NumCPU vs configured cores.

## [S3] Out of Scope

- Full Java entities/world/logic port.
- Client port.
- Dual-core IPC framing rewrite.
- Parallel item/liquid logistics walks.
- Build 158/159 clients.
- Full TypeIO for NodeBuilder/MenuResult (stubs keep IDs only).
- Official 159 wire-table reordering of remotes (pre-existing factory-order gap remains).

## Tasks

- [x] T1: Confirm 160.3 build integer (160) and TextureStream id 6 + 7 new remotes
- [x] T2: Switch server default build / flag / tests to 160 only
- [x] T3: Align dual `configs/core.toml` + GOMAXPROCS ownership
- [x] T4: Entity-index / drill-profile hot-path reductions in worktree
- [x] T5: Implement `internal/nativespatial` + wire into entity spatial queries
- [x] T6: Benchmark before/after (world package benches)
- [x] T7: Console status shows cores, GOMAXPROCS, dual-core

## Report

**What was built** — The Go server now defaults to Mindustry **build 160** only, registers `TextureStream` at framework packet id 6, and inserts the seven new 160.3 `@Remote` slots so later wire IDs stay aligned. Runtime core budget auto-scales to `NumCPU` (capped), dual-core IO workers grow with core count, and the process sets `GOMAXPROCS` once. World tick hot paths stop rebuilding entity indexes three times per step and stop double-resolving drill profiles. A new `internal/nativespatial` package provides a CSR uniform-grid spatial index (optional C++ via cgo, pure-Go fallback by default on this Windows host) and is wired into `entitySpatialIndex`.

**Verification** — `CGO_ENABLED=0 go test ./internal/nativespatial ./internal/world ./internal/sim ./cmd/mdt-server` → all `ok`. Protocol/net layout tests that fail also fail on baseline (`PRE-EXISTING`). Bench (`internal/world`, 30 iters): `BenchmarkWorldStepCurrentMap` 7.38ms → with scheduler 4.68ms; `BenchmarkWorldStepOfficialCompareMap` 786µs → with scheduler 687µs.

**Journey log** — (1) Full 800-file Java port is not a first feature; existing Go base + Java as oracle is. (2) Factory remote order in Go does not match historical official 159 wire IDs (pre-existing; tests already failed). (3) Windows host has no gcc; cgo module is opt-in behind `//go:build cgo`. (4) Child dual-core IPC was left alone this pass. (5) Next: regenerate remote registry from 160.3 Java, full TypeIO for menuBuilder, then C++ logistics if benches show need.
