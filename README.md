# mdt-server 使用教程

## 项目简介

mdt-server 是一个用 Go 语言重写的 Mindustry 服务器实现，支持 Mindustry build 155。

### 新增
# 我们提供了游戏API你可以将它连接到自己的前端
API可以控制游戏单位时间配置文件等等等等

### 系统要求

- Go 1.22 或更高版本
- Windows/Linux/macOS

#### 编译

```
# Windows
go build -o bin/mdt-server.exe ./cmd/mdt-server

# Linux/macOS
go build -o bin/mdt-server ./cmd/mdt-server
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

### 其他端点

- `GET /api/v1/status` - 获取服务器状态
- `POST /api/v1/summon` - 召唤单位
- `POST /api/v1/stop` - 关闭服务器

### 私聊命令

```
/status            显示服务器状态
/help              显示帮助信息
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

<a href="https://github.com/tomorrowsetout/go-Mindustry/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=tomorrowsetout/go-Mindustry" />
</a>



## Star History
[![Star History Chart](https://api.star-history.com/image?repos=tomorrowsetout/go-Mindustry&type=date&legend=bottom-right)](https://www.star-history.com/?repos=tomorrowsetout%2Fgo-Mindustry&type=date&legend=bottom-right)



