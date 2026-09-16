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

## 安全边界

- 不联网、不上传、不执行目录内命令；
- 路径不经过 shell 求值，POSIX 使用 `cd --`，PowerShell 使用 `-LiteralPath`；
- 配置通过临时文件和原子替换保存；
- 安装器只编辑带明确起止标记的 profile 区块；
- 卸载保留用户数据，purge 需要单独确认并拒绝根目录、HOME 等危险目标。
