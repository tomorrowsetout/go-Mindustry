---
feature: perf-50-bug-hunt
status: designed
updated: 2026-09-13
branch: feature/align-1603-perf
commits: 
---

# Push CurrentMap ≥50% Faster + Bug Hunt

## Report

## [S1] Problem

Scheduler already cut `BenchmarkWorldStepCurrentMap` from **7.38ms → 4.68ms** (~37%). Owner wants **≥50%** vs serial (target **≤3.69ms**), and wants remaining bugs found/fixed.

## [S2] Design

### S2.1 Performance target

- Baseline: serial `BenchmarkWorldStepCurrentMap` on this host (~7.38ms, 30 iters).
- Success: scheduler path **≤ 50% of serial** (i.e. ≥50% faster), same map, `CGO_ENABLED=0`.
- Correctness gate: `TestParallelMixedWorldMatchesSerialSnapshots` and `./internal/world` tests pass.

### S2.2 Planned hot-path work (profile-driven)

Prioritized candidates from prior audits; implement only what profile confirms:

1. **entityByID O(n)** → O(1) via `idToIndex` on World during stepEntities.
2. **TypeID profile cache** for unit AI kind / runtime profile / mounts (stop per-entity string maps).
3. **Building weapon profile dirty-flag** (stop full rebuild every tick).
4. **pendingBuilds scratch maps** reused on World.
5. **Block-kind enum for item logistics** (integer switch instead of string name).
6. **Reuse drill/pump sample buffers** on World.
7. **Spatial query buffer growth** already done; keep.

### S2.3 Bug hunt

- Remaining protocol TypeIO layout failures (PRE-EXISTING list).
- Fix highest-impact ones that block vanilla client join (connectConfirm/chat/clientSnapshot/plans).
- Add regression tests for each fixed bug.

## [S3] Out of Scope

- Full C++ logistics rewrite this pass (optional if pure Go hits target).
- Dual-core IPC rewrite.
- New gameplay features.

## Tasks

- [ ] T1: CPU profile CurrentMap scheduler path — acceptance: top ≥80% cost attributed (covers: S2.2)
- [ ] T2: Implement confirmed hot-path optimizations — acceptance: bench ≤50% of serial; world tests pass (covers: S2.1, S2.2)
- [ ] T3: Fix 1+ PRE-EXISTING TypeIO/join bugs with regression test — acceptance: previously failing layout test now passes (covers: S2.3)
- [ ] T4: Full verify + record numbers in Report — acceptance: commands + ns/tick written (covers: S2.1)
