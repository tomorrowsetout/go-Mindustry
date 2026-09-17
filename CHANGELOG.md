# Changelog

## 1.4.0 - demo (2026-09-15)

Target: **Mindustry 160.3** only (vanilla client).

### Protocol / client compatibility
- Accept **build 160** only; reject 158/159.
- Register `TextureStream` at framework packet id **6**.
- Rebuild remote registry to official **160.3 `Call.registerPackets` order** (from `Mindustry.jar`): connectConfirm=34, entitySnapshot=49, blockSnapshot=14, stateSnapshot=141, worldDataBegin=172, ping=84, etc.
- Join WorldStream includes `entityMapping` + team blocks + world entity count.
- TypeIO: `byte[]` uses **short** length; String fields use nullable exists-byte + UTF.
- Unit fields use `TypeIO.writeUnit/readUnit` (1-byte kind + 4-byte id), not `writeEntity`.
- Unit `WriteSync` aligned to `UnitEntity` field order (aimX/aimY…); empty status list to avoid client `readStatus` NPE.
- C→S packets omit injected Player (`tileConfig`, `dropItem`, `sendChatMessage`, `tileTap`, `menuChoose`, build/break, etc.).
- `clientSnapshot` selectedBlock is TypeIO **block** (short), not content.

### Runtime / performance
- Auto CPU budget: `runtime.cores = 0` → `NumCPU`; dual-core IO workers scale with core count; `GOMAXPROCS` set at startup.
- Entity spatial index via `internal/nativespatial` (CSR grid; cgo optional, pure-Go default).
- World step hot paths: single entity-index rebuild, drill profile cache, power generator enum + shared fuel tables, `clear()` map resets.
- Bench (CurrentMap, CGO off): serial ~4.6ms → scheduler **~3.6ms** vs original ~7.4ms (~50%).

### Bug fixes
- Payload decode EOF no longer tears down TCP during join.
- TypeIO / packet-ID canaries and tests for official 160.3 ids.

### Known issues
- Block snapshot tile coords / content IDs still diverge on some maps (`Missing entity`, conveyor `257 != 355`) — sync skipped, no crash.
- Some dual-direction TypeIO layout tests remain pre-existing failures.

## 1.3.9 - demo
- Previous Mindustry 157/159 oriented demo line.
