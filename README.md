<div align="center">
  <a href="https://github.com/MonthZifang/YUEYUEDAO-TECH">
    <img src="./md/logo.png" alt="月月岛科技 Logo" width="170" />
  </a>

  <p><strong>月月岛科技维护 GO-Mindustry Build 158 服务端</strong></p>

  <p>
    <a href="https://github.com/MonthZifang/YUEYUEDAO-TECH"><strong>查看月月岛科技详情</strong></a>
  </p>
</div>

## 项目简介

`mdt-server` 是一个使用 Go 语言实现的 Mindustry 服务端项目，当前面向官方 `build 158` 客户端版本。

项目提供基础开服能力，并内置 HTTP API，便于后续接入前端面板、自动化管理工具或其他扩展服务。

## 主要特性

- 支持 Mindustry `build 158`
- 使用 Go 编写，便于编译、部署和二次开发
- 内置 HTTP API，可按需接入外部管理工具
- 支持地图文件、地图名和随机地图启动
- 支持基础管理、数据存储与运行状态持久化
- 配置拆分在 `configs/*.toml` 中，便于维护和调整
- 支持实时对局视频录制输出

## 环境要求

- Go `1.22` 或更高版本
- Windows / Linux / macOS
- Mindustry 客户端版本：`build 158`

## 编译

```bash
# Windows
go build -o bin/mdt-server.exe ./cmd/mdt-server

# Linux / macOS
go build -o bin/mdt-server ./cmd/mdt-server
```

## 快速启动

```bash
# Windows
.\bin\mdt-server.exe

# Linux / macOS
./bin/mdt-server
```

常用启动方式：

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

# 查看版本号
.\bin\mdt-server.exe -version
```

## 启动参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `-config` | 配置文件路径 | `configs/config.toml` |
| `-addr` | Mindustry 协议监听地址 | `0.0.0.0:6567` |
| `-build` | 客户端版本 | `158` |
| `-world` | 地图来源，支持 `random`、地图名、`.msav` 文件路径 | 配置决定 |
| `-version` | 输出版本信息并退出 | 无 |
| `-record-video` | 录制实时对局视频 | 关闭 |
| `-video-dir` | 视频输出目录 | `data/video` |

## 配置文件

项目以 `configs/*.toml` 作为主要配置来源。建议优先查看以下文件：

| 文件 | 用途 |
| --- | --- |
| `configs/config.toml` | 主配置入口 |
| `configs/api.toml` | HTTP API 开关、监听地址、密钥 |
| `configs/server.toml` | 服务器名称、简介、虚拟人数 |
| `configs/core.toml` | 核心数、TPS、内存参数 |
| `configs/misc.toml` | 数据目录、脚本、mods 等杂项配置 |
| `configs/sync.toml` | 同步策略与同步参数 |
| `configs/personalization.toml` | 显示文案、前后缀、公告显示 |
| `configs/join_popup.toml` | 入服公告与帮助弹窗 |
| `configs/status_bar.toml` | 状态栏配置 |
| `configs/map_vote.toml` | 投票换图配置 |
| `configs/sundries.toml` | 日志输出相关配置 |
| `configs/tracepoints.toml` | 追踪日志配置 |

完整配置说明可查看 `configs/configs.md`。

## HTTP API

服务端内置 HTTP API，默认监听 `0.0.0.0:8090`，相关配置位于 `configs/api.toml`。

如果只需要基础开服，可以暂时忽略 API 配置；如果需要接入前端面板或外部管理工具，再根据 `configs/api.toml` 配置开关、监听地址和访问密钥。

## 控制台命令

服务端控制台支持管理命令。启动服务端后，可以在控制台输入：

```text
help all
```

查看完整命令帮助。

## 目录结构

```text
mdt-server/
├── assets/                  # 地图与内置资源
│   ├── logo.png             # 月月岛科技 Logo
│   └── worlds/              # 地图文件
├── bin/                     # 编译输出目录
├── cmd/                     # 程序入口与辅助工具
├── configs/                 # 运行配置文件
├── data/                    # 运行时数据、快照、状态、vanilla 数据
├── internal/                # 内部实现
├── mods/                    # 扩展脚本目录
├── go.mod
├── Makefile
└── README.md
```

## 开发提示

- 地图参数支持 `random`、地图名和 `.msav` 文件路径
- 随机地图会优先从 `assets/worlds` 中选择
- 当前服务端只接受 Mindustry `build 158`
- HTTP API 相关能力建议从 `configs/api.toml` 开始配置
- 扩展脚本和 mods 相关内容可以放在 `mods` 目录中维护

## 开发者文档

以下文档位于 `MD/` 目录，供参与还原 / 兼容工作的开发者参考：

| 文档 | 内容 |
| --- | --- |
| `MD/build158-compat-matrix.md` | build 158 兼容矩阵与各阶段进度（Level 1 / Level 2） |
| `MD/wire-protocol.md` | **线协议权威规范**：两层帧格式、lz4 压缩、framework 消息、握手顺序、读端特例 |
| `MD/architecture.md` | 模块化架构（`internal/core`）+ 实时客户端测试框架（`internal/net/harness`）说明 |
| `MD/level1-connect-sync-report.md` | Level 1 报告：连接准入 / 世界流 / connectConfirm / 同步门控 |
| `MD/level2-modular-arch-harness.md` | Level 2 报告：模块化架构、测试框架、14 个 inbound 包补齐、线协议 bug 修复 |

> 改动任何包的线 ID / 字段顺序 / 同步顺序前，请以 `/server/Mindustry-158` 为准，并先读 `MD/wire-protocol.md`。

## 月月岛科技

本项目由月月岛科技相关项目维护。更多信息请查看：

[https://github.com/MonthZifang/YUEYUEDAO-TECH](https://github.com/MonthZifang/YUEYUEDAO-TECH)

## 许可证

本项目使用 `GPL-3.0` 许可证，详情请参阅 `LICENSE`。



＃ 定档 2026.5.3

由于这几天心情不好 外加前一阵子写的新版本 还有我自己购入的新服务器 在调试的过程中不小心将C盘进行格式化了 可以这么理解 现在最新版的源码丢失了 无备份 只能从git上重新拉取然后构建编写
总结就是心情不好 导致粗心大意
