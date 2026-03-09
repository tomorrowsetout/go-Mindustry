# mdt-server 使用教程

## 项目简介

mdt-server 是一个用 Go 语言重写的 Mindustry 服务器实现，支持 Mindustry build 155。

### 特性

- ✅ 高性能网络协议实现
- ✅ 完整的游戏逻辑支持
- ✅ Mod 系统（支持 Java/JS/Node.js）
- ✅ 事件日志记录
- ✅ Web API 管理接口
- ✅ 持久化存档支持
- ✅ 虚拟玩家支持
- ✅ OP 命令系统

## 快速开始

### 系统要求

- Go 1.22 或更高版本
- Windows/Linux/macOS

### 编译

#### 使用 Make（推荐）

```bash
# 构建服务器
make build

# 清理编译文件
make clean

# 测试
make test
```

#### 手动编译

```bash
# Windows
go build -o bin/mdt-server.exe ./cmd/mdt-server

# Linux/macOS
go build -o bin/mdt-server ./cmd/mdt-server
```

### 运行服务器

```bash
# 方式 1：使用 Make
make run

# 方式 2：直接运行
./bin/mdt-server.exe
```

### 命令行参数

```bash
-c, --config <path>     配置文件路径（默认：config.json）
-a, --addr <address>    监听地址（默认：0.0.0.0:6567）
-b, --build <version>   Mindustry 客户端版本（必须匹配，例如：155）
-w, --world <source>    世界源：random | <map-name> | <.msav 文件路径>
    --version           显示版本信息
```

### 运行示例

```bash
# 使用默认配置运行
./bin/mdt-server.exe

# 使用指定地图运行
./bin/mdt-server.exe -w assets/worlds/23315.msav -b 155

# 使用随机地图运行
./bin/mdt-server.exe -w random -b 155
```

## 配置文件

配置文件 `config.json` 包含以下主要设置：

### 基本配置

```json
{
  "runtime": {
    "serverName": "我的服务器",
    "serverDesc": "一个有趣的 Mindustry 服务器",
    "vanillaProfiles": "data/vanilla/profiles.json"
  },
  "net": {
    "udpRetryCount": 3,
    "udpRetryDelayMs": 500,
    "udpFallbackTCP": true,
    "syncEntityMs": 200,
    "syncStateMs": 1000
  }
}
```

### 启用持久化

```json
{
  "persist": {
    "enabled": true,
    "path": "data/state",
    "intervalSec": 30
  }
}
```

### 启用 API

```json
{
  "api": {
    "enabled": true,
    "bind": "127.0.0.1:8080",
    "keys": ["your-api-key-here"]
  }
}
```

## Web API

启用 API 后，可以使用以下端点：

### 获取状态

```bash
curl http://127.0.0.1:8080/api/v1/status
```

### 召唤单位

```bash
curl -X POST http://127.0.0.1:8080/api/v1/summon \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"typeId": 1, "x": 100, "y": 100, "team": 1}'
```

### 其他端点

- `GET /api/v1/status` - 获取服务器状态
- `POST /api/v1/summon` - 召唤单位
- `POST /api/v1/stop` - 关闭服务器

## 游戏内命令

### OP 命令

只有 OP 用户可以使用这些命令：

```
/stop              保存并关闭服务器
/summon <type>     召唤单位（需要 OP）
/despawn <id>      移除单位（需要 OP）
/umove <id> <vx> <vy>     设置单位运动（需要 OP）
/uteleport <id> <x> <y>   传送单位（需要 OP）
/ulife <id> <seconds>     设置单位寿命（需要 OP）
/ufollow <id> <target>    设置单位跟随（需要 OP）
/upatrol <id> <x1> <y1> <x2> <y2>  设置单位巡逻（需要 OP）
/ubehavior clear <id>     清除单位行为（需要 OP）
```

### 私聊命令

```
/status            显示服务器状态
/help              显示帮助信息
```

## Mod 系统

服务器支持多种语言的 Mod：

### Java Mod

```java
// mods/java/hello.java
public class HelloMod {
    public void onInit() {
        System.out.println("Hello from Java Mod!");
    }
}
```

### JavaScript Mod

```javascript
// mods/js/hello.js
function onInit() {
    console.log("Hello from JS Mod!");
}
```

### Node.js Mod

```javascript
// mods/node/hello.js
console.log("Hello from Node.js Mod!");
```

## 文件结构

```
mdt-server/
├── bin/                    # 编译输出目录
│   └── mdt-server.exe     # 服务器可执行文件
├── cmd/
│   └── mdt-server/        # 主程序入口
│       └── main.go
├── internal/              # 内部包
│   ├── api/              # API 服务
│   ├── config/           # 配置管理
│   ├── core/             # 核心架构
│   ├── entity/           # 实体系统
│   ├── mods/             # Mod 系统
│   ├── net/              # 网络层
│   ├── persist/          # 持久化
│   ├── protocol/         # 协议实现
│   ├── sim/              # 物理仿真
│   ├── storage/          # 事件存储
│   └── world/            # 世界管理
├── data/                  # 运行时数据
│   ├── events/           # 事件日志
│   ├── maps/             # 地图文件
│   ├── mods/             # Mod 文件
│   ├── snapshots/        # 快照文件
│   └── state/            # 游戏状态
├── assets/                # 静态资源
│   └── worlds/           # 地图文件
├── logs/                  # 日志文件
├── config.json            # 配置文件
├── Makefile               # 构建脚本
└── README.md              # 项目文档
```

## 许可证

本项目使用 GPL-3.0 许可证。详情请参阅 `LICENSE` 文件。

## 社区和支持

- 项目地址: https://github.com/tomorrowsetout/go-Mindustry
- 问题反馈: 请提交 issue

## 贡献

欢迎贡献！请按照以下步骤：

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request


