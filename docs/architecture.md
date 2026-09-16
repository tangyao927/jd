# jd 架构说明

## 调用链

```text
shell function jd
    ├── 管理命令 ────────────────> jd binary
    └── 跳转查询 -> jd binary -> absolute target
                                  |
                                  └── shell builtin cd / Set-Location
```

二进制集中实现参数解析、目录匹配、索引、配置、状态和 TUI。shell 代码只负责转发管理命令、执行内建目录切换和记录目录变化。

## 启动依赖

CLI 采用按需初始化，确保基础命令不会被用户状态阻断：

- `help`、`--help`、`version`、`--version` 和 `completion` 不读取配置、不打开数据库；
- `init` 只读取配置，用于生成是否跟踪普通 `cd` 的 shell 脚本；
- 导航、书签、扫描、历史和 `doctor` 首次访问状态时才打开 SQLite；
- 数据库打开失败会包含实际路径，`doctor` 额外给出备份和恢复提示。

## 模块边界

- `internal/cli`：公共命令、JSON 输出、退出码和组件编排。
- `internal/nav`：直接路径、匹配分类、frecency 和确定性排序。
- `internal/store`：SQLite schema、事务、并发写入及数据生命周期。
- `internal/scan`：受深度、忽略项和符号链接策略约束的目录遍历。
- `internal/shell`：zsh、bash、fish、PowerShell 初始化代码。
- `internal/ui`：无外部选择器依赖的交互式候选列表。
- `internal/config`：TOML 默认值、校验、持久化和平台目录解析。

## 状态模型

SQLite 使用 WAL、外键和 5 秒 busy timeout。主要实体包括：

- `directories`：规范化绝对路径、访问次数、首次/最后访问、失效时间及来源标记；
- `pins`：大小写不敏感且唯一的书签名；
- `roots`：扫描深度、隐藏目录和符号链接选项；
- `root_entries`：扫描根与目录的多对多成员关系；
- `meta`：schema 版本。

移除扫描根时只清除纯扫描来源；仍有访问历史或书签的目录会保留。失效目录从候选中隐藏，但在保留期内不会自动删除。

## 导航优先级

1. 现存的直接路径；
2. 精确书签；
3. 精确目录名；
4. 路径段前缀；
5. 按序字符子序列。

同级使用书签来源、frecency、与当前目录的共同祖先深度、路径长度和字典序排序。普通查询存在多个候选时必须交互选择；非交互环境返回歧义错误，除非调用方显式使用 `--first`。

## 公共契约

退出码：

| Code | Meaning |
|---:|---|
| 0 | 成功 |
| 1 | 运行或存储错误 |
| 2 | 参数或配置错误 |
| 3 | 无匹配 |
| 4 | 非交互环境中存在多个候选 |
| 130 | 用户取消 |

`jd query --format json` 返回 `schema_version: 1`、查询词和有序候选。候选公开 `rank`、`path`、`display`、`match`、`sources`；排序内部数值和数据库字段不属于公共 API。

`jd --version` 与 `jd version` 输出相同的 `jd <version>`。源码构建默认版本为 `dev`，Release 由 GoReleaser 通过链接参数注入去除 `v` 前缀的语义化版本。

`--` 只终止选项解析。shell 包装层仅在分隔符之前识别帮助和管理参数，因此 `jd -- --help` 可以查询并跳转到名为 `--help` 的目录。

## 发布边界

GoReleaser 为 macOS、Linux、Windows 的 amd64/arm64 生成六个平台归档和 SHA-256 校验文件。标签工作流负责发布；普通运行时不包含更新或联网逻辑。

安装器默认下载并校验 Release，也支持指定版本、源码构建和本地二进制。任何下载或校验失败都发生在替换现有二进制之前。卸载继续默认保留配置和历史。

## 安全边界

- 不联网、不上传、不执行目录内命令；
- 路径不经过 shell 求值，POSIX 使用 `cd --`，PowerShell 使用 `-LiteralPath`；
- 配置通过临时文件和原子替换保存；
- 安装器只编辑带明确起止标记的 profile 区块；
- 卸载保留用户数据，purge 需要单独确认并拒绝根目录、HOME 等危险目标。

## 后续演进

以下事项不属于 v0.1.0 发布加固范围，需要基准或 schema 需求驱动：

- 大索引查询目前会检查所有候选目录，后续可按匹配结果延迟检查文件系统；
- 扫描目前遇到不可读目录会失败，后续可引入带告警的部分成功结果；
- schema 版本目前只支持 v1，首次结构变更前需要引入顺序迁移；
- TUI 和多 shell 行为继续依赖跨平台 CI 扩充覆盖。
