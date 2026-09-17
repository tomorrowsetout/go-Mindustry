# go-Mindustry Demo版1.4.0

目标客户端：**官方 Mindustry 160.3**（不再兼容 158/159）。

## 新增 / 变更

### 协议与客户端兼容
- 仅接受 **build 160**
- 按官方 `Call.registerPackets` 重建线包 ID（connectConfirm=34、entitySnapshot=49、blockSnapshot=14、stateSnapshot=141 等）
- 注册 `TextureStream`（framework id 6）
- Join WorldStream 补齐 `entityMapping` + team blocks + world entity count
- TypeIO 与官方对齐：`byte[]`/String 用 short 长度与 nullable 格式；`Unit` 使用 `writeUnit`
- C→S 包省略注入的 Player（tileConfig、dropItem、聊天、tileTap、建造拆除等）
- 单位 `WriteSync` 按 `UnitEntity` 字段序；空 status 列表避免客户端 NPE

### 性能
- CPU：`runtime.cores=0` 自动用满逻辑核；双核 IO worker 随核数扩展；启动设置 `GOMAXPROCS`
- 实体空间索引：`internal/nativespatial`（CSR 网格，cgo 可选，默认纯 Go）
- 世界 tick 热路径：实体索引只建一次、钻头 profile 缓存、发电枚举与共享燃料表
- CurrentMap 基准：约 **7.4ms → 3.6ms**（约 50%）

### 修复
- 进服窗口 payload 解码 EOF 不再踢线
- 官方 160.3 线 ID / TypeIO 回归测试

### 已知问题
- 部分地图 block snapshot 坐标/content ID 仍不一致（传送带 257≠355 等，跳过同步、不断线）
- 少量双方向 TypeIO 布局测试仍为历史遗留失败

## 兼容说明
- 合并了原 `main` 历史（含 IYanHua 的 Mod/工作区相关 PR），以 merge commit 保留双方提交记录
- 版本标记：`Center/version.toml` = `1.4.0 - demo` / `Mindustry 160.3`
