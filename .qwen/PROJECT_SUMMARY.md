# Project Summary

## Overall Goal
将 Mindustry 服务器项目重构为双核心高性能架构，并清理无用文件后上传到 GitHub 仓库。

## Key Knowledge
- **项目位置**: `C:\Users\43551\Desktop\mdt-server`
- **Go 版本**: 1.22
- **Mindustry Build**: 155
- **核心架构**: 双核心系统
  - Core1 (Game Loop): 主线程，60 TPS 实时游戏逻辑（单核心）
  - Core2 (IO Core): 并发 goroutine，处理所有 IO 任务（4 workers）
- **技术栈**: Go 语言，API 服务，Mod 系统（支持 Java/JS/Node.js）
- **构建命令**: `go build -o bin/mdt-server.exe ./cmd/mdt-server`
- **Makefile 支持**: build, clean, test, dev, run, build-windows, build-linux, build-macos
- **远程仓库**: https://github.com/tomorrowsetout/go-Mindustry

## Architecture Details

### Core1 (Game Loop)
```
运行在主线程，单独占用一个核心
- World.Tick() - 60 TPS 实时游戏逻辑
- Sim.Step() - 物理仿真
- EntityBroadcast - 实体同步广播
- WaveUpdate - 波次更新
- PlayerUpdate - 玩家更新
- BuildQueueProcess - 建筑队列处理
- LogicCompile/Run - 逻辑编译/执行
```

### Core2 (IO Core)
```
处理所有 IO 密集型任务（4 workers）
├─ PacketMessage - 网络收发包
├─ ConnectionMessage - 连接事件
├─ PersistenceMessage - 存档/读档
├─ ModMessage - Mod 加载/卸载
├─ StorageMessage - 事件记录
└─ WorldStreamMessage - MSAV 文件读写
```

### 消息类型
```go
// Core1 消息
MessageGameTick, MessageWaveUpdate, MessageEntityBroadcast
MessagePlayerUpdate, MessageBuildQueueProcess
MessageLogicCompile, MessageLogicRun

// Core2 消息
MessagePacketIncoming, MessagePacketOutgoing
MessageConnectionOpen, MessageConnectionClose
MessageSaveState, MessageLoadState
MessageSaveWorld, MessageLoadWorld
MessageModLoad, MessageModUnload, MessageModStart, MessageModStop
MessageStorageRecord, MessageStorageFlush
MessageWorldStreamLoad, MessageWorldStreamSave
```

## Protocol Fixes Applied
- IntSeq 长度类型: ReadInt16 → ReadInt32
- CRC32 校验: ConnectPacket 添加 CRC32
- String 长度验证: 65KB 限制
- modCount 限制: 128

## Recent Actions
1. 重构双核心架构：`internal/core/` (core.go, messages.go, server_core.go)
2. 网络核心分离：`internal/net/` (buscore.go, netcore.go)
3. 清理无用文件：11 个 Python 脚本、2 个 diff 文件、2 个临时目录
4. 创建 Makefile 构建脚本
5. 创建 README.md 使用教程
6. 编译输出重定向到 `bin/` 目录
7. 上传到 GitHub 仓库

## Current Plan
- [DONE] 修复所有协议 bug (IntSeq, CRC32, ContentType, String validation)
- [DONE] 创建 Core/IOCore 架构
- [DONE] 实现 Core1 (Game Loop) for game logic
- [DONE] 实现 Core2 (IO Core) for network, persistence, mods, worldstream
- [DONE] 清理无用文件和临时目录
- [DONE] 创建 Makefile 和 README.md
- [DONE] 编译输出重定向到 bin 目录
- [DONE] 上传到 GitHub 仓库
- [TODO] 实际实现 IOCore handlers (Persistence/Mods/WorldStream)
- [TODO] 将 ServerCore 集成到 cmd/mdt-server/main.go
- [TODO] 与实际 Mindustry 客户端测试

---

## Summary Metadata
**Update time**: 2026-03-04T12:03:03.403Z 
