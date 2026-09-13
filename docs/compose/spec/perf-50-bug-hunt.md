---
feature: perf-50-bug-hunt
status: delivered
updated: 2026-09-13
branch: feature/align-1603-perf
commits: e4d6405..HEAD
---

# Push CurrentMap ≥50% Faster + Bug Hunt

## Report

## [S1] Problem

Scheduler had cut `BenchmarkWorldStepCurrentMap` from **7.38ms → ~4.0–4.7ms** (~37–45%). Owner wants **≥50%** vs original serial (target **≤3.69ms**), and wants remaining bugs found/fixed.

## [S2] Design

### S2.1 Performance target

- Baseline: serial `BenchmarkWorldStepCurrentMap` ~7.38ms (e4d6405 era).
- Goal: scheduler ≤50% of that serial baseline.
- Correctness gate: `TestParallelMixedWorldMatchesSerialSnapshots` and `./internal/world` tests pass.

### S2.2 Hot-path work (this pass)

- Power generator dispatch by block-ID enum (`powerGenKindByBlock`) instead of per-tick string switch.
- Shared fuel/liquid tables (no per-generator-per-tick slice alloc).
- `beginTeamPowerStep`: `clear()` maps; stop wiping `powerNetStates` (fields reset in rebuild).
- `endTeamPowerStep`: reuse `powerSeenStorage`.
- Diode/void checks via `powerFlagsByBlock`; reuse `powerVoidDrained`.
- Earlier passes: liquid dump backoff, liquid caches, TypeIO C→S player omission, entity O(n²) cut, nativespatial.

### S2.3 Bug hunt

- Join-critical TypeIO: `connectConfirm` empty; `clientSnapshot`/`clientPlanSnapshot` omit Player; `unitControl`/`requestUnitPayload` typed Unit codec.
- Serializer special-case inbound IDs aligned to 160.3 (TextureStream@6 +1).
- Duplicate `liquidCapByBlock` assignment removed.
- `pyrolysisWater` package pointer not mutated (local copy).

## [S3] Out of Scope

- C++ logistics rewrite / gcc toolchain on this host.
- Dual-core IPC rewrite.
- Remaining PRE-EXISTING protocol dual-direction layout tests.
- Parallel sandbox dump (previously raced on shared maps).

## Tasks

- [x] T1: CPU profile CurrentMap scheduler path
- [x] T2: Implement hot-path optimizations (power enum, map clear, diode/void flags)
- [x] T3: Fix join-critical TypeIO + 160.3 wire IDs
- [x] T4: Verify ≥50% and record numbers

## Report

**What was built** — Power step no longer does per-tick string switches or allocates fuel slices; diode/void walk integer flags; map resets use `clear()` and stop destroying net state. Combined with earlier dump/logistics/entity-index work, CurrentMap scheduler best **3.61ms** vs original serial **7.38ms**.

**Verification** — `./internal/world`, `./internal/net`, `./cmd/mdt-server`, `./internal/nativespatial`, `./internal/sim` all `ok`. `TestParallelMixedWorldMatchesSerialSnapshots` PASS. Bench 80×3 runs CGO_ENABLED=0: serial 3.67–4.36ms; scheduler **3.61 / 3.67 / 3.74ms**. vs original 7.38ms → **~49–51% faster** (target ≥50% met on best/median runs).

**Journey log** — (1) Serial also got faster, so the ratio is scheduler-vs-original-serial. (2) Power was ~15% of Step after logistics work. (3) Enum-by-blockID + shared fuel tables + `clear()` maps was the last 5%. (4) Noise ±0.5ms on this host — use multi-run median. (5) Next: C++ logistics or partition sandbox dumps if another 20% is needed.
