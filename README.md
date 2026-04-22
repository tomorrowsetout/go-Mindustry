# mdt-server 使用教程

## 项目简介

`mdt-server` 是一个使用 Go 语言实现的 Mindustry 服务器，目前支持官方 `build 157`。

项目除了基础开服能力，还内置了 HTTP API，方便按需接入你自己的前端或管理工具。

## 主要特性

- 仅支持 Mindustry `build 157`
- 使用 Go 编写，方便二次开发与部署
- 内置 HTTP API，可按需对接前端
- 支持基础管理与持久化能力
- 配置已拆分为 `configs/*.toml`，比单文件配置更容易维护

## 系统要求

- Go `1.22` 或更高版本
- Windows / Linux / macOS

## 编译

```bash
# Windows
go build -o bin/mdt-server.exe ./cmd/mdt-server

# Linux / macOS
go build -o bin/mdt-server ./cmd/mdt-server
```

## 快速开始

```bash
# Windows
.\bin\mdt-server.exe

# Linux / macOS
./bin/mdt-server
```

常用启动示例：

```bash
# 使用默认配置启动
.\bin\mdt-server.exe

# 指定配置文件
.\bin\mdt-server.exe -config configs/config.toml

# 指定地图文件启动
.\bin\mdt-server.exe -world assets/worlds/22908.msav

# 按地图名启动
.\bin\mdt-server.exe -world 22908

# 随机地图
.\bin\mdt-server.exe -world random

# 自定义监听地址
.\bin\mdt-server.exe -addr 0.0.0.0:6567

# 输出版本号
.\bin\mdt-server.exe -version
```

## 启动参数

- `-config`：配置文件路径，默认 `configs/config.toml`
- `-addr`：Mindustry 协议监听地址，默认 `0.0.0.0:6567`
- `-build`：客户端版本，当前只支持 `157`
- `-world`：地图来源，支持 `random`、地图名、`.msav` 文件路径
- `-version`：输出版本信息并退出
- `-record-video`：录制实时对局视频
- `-video-dir`：视频输出目录，默认 `data/video`

## 配置说明

当前项目以 `configs/*.toml` 为准，不再推荐把主要说明写成 `config.json` 式配置。

建议优先关注这些文件：

- `configs/config.toml`：主配置入口
- `configs/api.toml`：HTTP API 开关、监听地址、密钥
- `configs/server.toml`：服务器名称、简介、虚拟人数
- `configs/core.toml`：核心数、TPS、内存参数
- `configs/misc.toml`：数据目录、脚本、mods 等杂项配置
- `configs/sync.toml`：同步策略与同步参数
- `configs/personalization.toml`：显示文案、前后缀、公告显示
- `configs/join_popup.toml`：入服公告与帮助弹窗
- `configs/status_bar.toml`：状态栏配置
- `configs/map_vote.toml`：投票换图配置
- `configs/sundries.toml`：日志输出相关配置
- `configs/tracepoints.toml`：追踪日志配置

配置文件总览可以查看 `configs/configs.md`。

## HTTP API

项目内置 HTTP API，默认监听 `0.0.0.0:8090`，配置文件位于 `configs/api.toml`。

如果你只想开服，这部分可以先不管；如果后续要接前端，再根据 `configs/api.toml` 开关和密钥配置使用即可。


## 控制台命令

服务端控制台支持管理命令，完整帮助可以直接输入 `help all` 查看。

## 目录结构

```text
mdt-server/
├── assets/                  # 地图与内置资源
│   └── worlds/
├── bin/                     # 编译输出目录
├── cmd/
│   ├── mdt-headless-probe/  # 探测工具
│   └── mdt-server/          # 主程序入口
├── config/                  # 兼容或辅助配置资源
├── configs/                 # 运行配置文件
├── data/                    # 运行时数据、快照、状态、vanilla 数据
├── internal/                # 内部实现
│   ├── api/                 # HTTP API
│   ├── bootstrap/           # 启动与工作区初始化
│   ├── buildsvc/            # 原版数据生成与构建辅助
│   ├── config/              # 配置加载
│   ├── core/                # 多核与核心逻辑
│   ├── entity/              # 实体定义
│   ├── net/                 # 网络层
│   ├── persist/             # 持久化
│   ├── protocol/            # 协议实现
│   ├── sim/                 # Tick 与仿真
│   ├── storage/             # 数据存储
│   ├── vanilla/             # 原版资料与 profiles
│   ├── video/               # 对局视频录制
│   ├── world/               # 世界逻辑
│   └── worldstream/         # MSAV 与世界流处理
├── logs/                    # 日志目录
├── mods/                    # 扩展脚本目录
├── tools/                   # 辅助工具
├── go.mod
├── Makefile
└── README.md
```

## 开发提示

- 地图参数支持 `random`、地图名和 `.msav` 路径
- 随机地图会优先从 `assets/worlds` 中选择
- 当前服务端只接受 `build 157`
- 如果后续要接前端，可以再去调整 `configs/api.toml`

## 许可证

本项目使用 `GPL-3.0` 许可证，详情请参阅 `LICENSE`。

## 社区与支持

- 项目地址：https://github.com/tomorrowsetout/go-Mindustry
- 问题反馈：请提交 Issue
- 欢迎提交 PR

## 贡献者

<a href="https://github.com/tomorrowsetout/go-Mindustry/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=tomorrowsetout/go-Mindustry" />
</a>

## Star History

[![Star History Chart](https://api.star-history.com/image?repos=tomorrowsetout/go-Mindustry&type=date&legend=bottom-right)](https://www.star-history.com/?repos=tomorrowsetout%2Fgo-Mindustry&type=date&legend=bottom-right)
