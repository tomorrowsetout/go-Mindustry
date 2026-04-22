json                     # JSON 数据文件目录，存放公开连接 UUID、玩家身份等静态信息
api.toml                 # API 配置，主要放监听地址与密钥
config.toml              # 主配置文件，只放全局控制项与 authority_sync.strategy
core.toml                # 核心配置，控制双核、TPS 与内存参数
development.toml         # 开发调试配置，控制台与调试日志开关
misc.toml                # 杂项配置，数据目录、mods、持久化、脚本等
personalization.toml     # 个性化显示配置
release.toml             # 内置资源释放控制，改成 false 后重启可强制重新释放
server.toml              # 服务器基础信息配置，名称、简介、虚拟玩家数
sundries.toml            # 附加日志配置，控制 logs 文件输出
sync.toml                # 快照/网络同步与权威同步参数
join_popup.toml          # 入服公告菜单配置
status_bar.toml          # 状态栏配置
map_vote.toml            # 投票换图配置
player.toml              # 玩家相关配置预留文件

authority_sync.strategy  # 主配置中的单选开关，只能填 official / static / dynamic 其中一个
所有配置文件现已统一使用 .toml，旧的 .ini 配置文件已转换并移除
