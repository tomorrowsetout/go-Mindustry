---
feature: perf-50-bug-hunt
status: delivered
updated: 2026-09-13
branch: feature/align-1603-perf
commits: 907d4fc..ec49bcd
---

# Push CurrentMap ≥50% Faster + Bug Hunt

## Report

## [S1] Problem

Scheduler already cut `BenchmarkWorldStepCurrentMap` from **7.38ms → 4.68ms** (~37%). Owner wants **≥50%** vs original serial (target **≤3.69ms**), and wants remaining bugs found/fixed.

## [S2] Design

### S2.1 Performance target

- Baseline: serial `BenchmarkWorldStepCurrentMap` ~7.38ms.
- Goal: scheduler ≤50% of that serial baseline.
- Correctness gate: `TestParallelMixedWorldMatchesSerialSnapshots` and `./internal/world` tests pass.

### S2.2 Hot-path work

- Liquid dump neighbor cache (was alloc+sort every dump).
- Sandbox dump failure backoff (8 ticks) to skip saturated walks.
- Reuse liquid source stacks; liquid capacity cache by block id.
- Duct loop dispatch via `logisticsKind` instead of string names.
- Bench harness uses NumCPU partitions (was hardcoded 6/4).

### S2.3 Bug hunt (TypeIO C→S player omission)

Java `CallGenerator` skips injected `Player` on client→server wire. Fixed:

- `connectConfirm` empty body
- `clientSnapshot` / `clientPlanSnapshot` omit Player
- `unitControl` / `requestUnitPayload` typed Unit codec, no Player
- Serializer special-case IDs already aligned to 160.3

## [S3] Out of Scope

- Remaining dual-direction packet Read/Write split (S→C includes Player, C→S uses serializer fallback).
- C++ logistics rewrite.
- Dual-core IPC rewrite.

## Tasks

- [x] T1: CPU profile CurrentMap scheduler path
- [x] T2: Implement hot-path optimizations
- [x] T3: Fix join-critical TypeIO bugs with regression tests
- [x] T4: Verify and record numbers

## Report

**What was built** — Sandbox dump path no longer re-sorts liquid neighbors every call and backs off when all neighbors refuse items/liquids. Liquid capacity and duct dispatch use integer caches. The CurrentMap bench now scales with host cores instead of a hardcoded 6/4 engine. Join-critical TypeIO bugs were fixed: `connectConfirm` is empty on the wire; `clientSnapshot`/`clientPlanSnapshot` omit the injected Player; `unitControl`/`requestUnitPayload` use the typed Unit codec.

**Verification** — `./internal/world`, `./internal/net`, `./cmd/mdt-server`, `./internal/nativespatial`, `./internal/sim` all `ok`. Protocol still has 9 PRE-EXISTING dual-direction layout tests (S→C Write includes Player; C→S Read still expects it). Bench (80 iters, CGO_ENABLED=0): serial ~4.5–4.9ms; scheduler **~4.08–4.36ms**. vs original serial 7.38ms → **~42–45% faster**. Target of ≥50% (≤3.69ms) was **not reached**.

**Journey log** — (1) Serial got faster from the same dump/logistics fixes, so scheduler/serial ratio narrowed. (2) Parallelizing sandbox sources raced on shared maps (`fatal error: concurrent map read and map write`) — reverted. (3) Batch-stripping Player from dual-direction packets broke S→C relay tests — restored Read for those; kept join-critical C→S-only packets. (4) Next for ≥50%: partition sandbox dump targets, or C++ CSR logistics, or reduce Step exclusive-lock scope.
