# Mindustry 155 联机兼容调试记录（2026-03-07）

## 用户现象
- 初期：进服后各种网络 IO/NPE/EOF（`UpdateMarkerCallPacket`、`UnitSpawnCallPacket`、`ClientBinaryPacketUnreliable` 等）。
- 中期：稳定连接后，出现建造无法完成/卡建造中。
- 当前：不再崩溃，玩家可连接并主动退出，但建造状态仍需继续收敛。

## 本次已完成的关键修改
1. 收发包模式拆分与兼容处理
- `recvCompatIDs/sendCompatIDs` 拆分。
- 当前运行策略为 `sendCompat=true, recvCompat=true`。

2. 高风险包临时规避
- 已临时跳过 `playerSpawn`（避免某些客户端映射错位崩溃）。
- 已禁用 `pingResponse` 回包（避免被错解成 marker 类包）。

3. 建造相关包映射与广播收敛
- `setTile` 走兼容 ID 路径（日志中为 `packet_id=135`）。
- `constructFinish(137)` 在最近版本中已移除（曾导致异常中断风险）。
- 建造完成广播当前为：
  - `removeTile(130)`
  - `setTile(135 -> Remote_Tile_setTile_131)`
  - `buildHealthUpdate(13)`（新增，强制客户端刷新构建状态）

4. 世界坐标修正
- 将建造事件位置从线性索引改为 `PackPoint2(x,y)` 语义，避免错误 tile 定位。

5. 实体快照防护
- 控制 entity snapshot 负载，禁用全图额外实体注入，降低解包错位风险。

## 最近日志结论
- 连接链路稳定：`world handshake sent` -> `connect confirm` 正常。
- 玩家退出为主动行为（非崩溃断开）。
- 建造时可见服务器连续广播：
  - `packet_id=130 type=*protocol.Remote_Tile_removeTile_130`
  - `packet_id=135 type=*protocol.Remote_Tile_setTile_131`
  - 以及后续已加入 `buildHealthUpdate` 刷新。

## 待继续验证
- 客户端是否仍“卡建造中”。
- 若仍卡，需要继续比对客户端对 `construct/remove/set/health` 顺序与字段细节。

---
该文件由当前会话自动保存于项目目录。
