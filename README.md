# jd

[![CI](https://github.com/tangyao927/jd/actions/workflows/ci.yml/badge.svg)](https://github.com/tangyao927/jd/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tangyao927/jd?display_name=tag)](https://github.com/tangyao927/jd/releases)

**A local-first, cross-platform directory jumper that learns where you work.**

`jd` 通过访问历史、目录书签和项目扫描索引，快速定位并跳转到目标目录。它是一个原生可执行文件，运行时不联网、无遥测、无后台进程，所有状态都保存在本机。

- macOS、Linux、Windows（amd64、arm64）
- zsh、bash、fish、Windows PowerShell 5.1、PowerShell 7
- 智能匹配、frecency 排序、内置交互选择器
- SQLite 本地状态库和稳定的 JSON 查询接口
- 安装后不需要 Go 或其他运行时

## 一分钟安装

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.sh | sh
```

重新打开终端，或立即加载对应配置：

```sh
source ~/.zshrc                    # zsh
source ~/.bashrc                   # bash
source ~/.config/fish/config.fish  # fish
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.ps1 | iex
. $PROFILE
```

安装器从最新 GitHub Release 下载当前平台的预编译包并校验 SHA-256，然后安装到用户目录并幂等更新 shell profile。它不会使用管理员权限；首次修改已有 profile 前会创建带时间戳的备份。

如果 [Releases](https://github.com/tangyao927/jd/releases) 尚无版本，请使用下方的源码安装。

### 指定版本或源码安装

```sh
# 安装指定 Release
./scripts/install.sh --version v0.1.0

# 从当前源码构建，需要 Go 1.24+
git clone https://github.com/tangyao927/jd.git
cd jd
./scripts/install.sh --source
```

```powershell
# 已下载 scripts/install.ps1 时
.\scripts\install.ps1 -Version v0.1.0
.\scripts\install.ps1 -Source
```

安装前可检查目标，不下载、不写文件：

```sh
./scripts/install.sh --dry-run
```

```powershell
.\scripts\install.ps1 -DryRun
```

## 30 秒上手

```sh
# 给任意现存目录创建快捷名称
jd pin aivo ~/.config/aivo

# 以后直接跳转
jd aivo

# 将当前目录保存为 work
jd pin work

# 查看和删除书签
jd pins
jd unpin work
```

扫描常用项目目录后，可以按名称或路径片段查询：

```sh
jd root add ~/work
jd api
jd work api

# 字符按序匹配：wapi 可匹配 /work/api
jd wapi
```

一个结果会直接跳转；多个结果会在交互式终端打开内置选择器。脚本或 CI 中可显式选择排序第一项：

```sh
jd --first api
```

## 命令参考

| 命令 | 作用 |
|---|---|
| `jd [query...]` | 查询并跳转 |
| `jd jump [query...]` | 强制把与子命令同名的词当作查询 |
| `jd pin <name> [path]` | 创建或更新目录书签，省略 path 时使用当前目录 |
| `jd unpin <name>` | 删除书签 |
| `jd pins` | 列出书签 |
| `jd root add <path>` | 添加并立即扫描项目根 |
| `jd root remove <path>` | 移除扫描根 |
| `jd root list` | 列出扫描根及选项 |
| `jd scan` | 刷新全部扫描根 |
| `jd history list` | 查看访问历史 |
| `jd history forget [path]` | 忘记目录访问记录 |
| `jd history clean` | 标记并清理过期失效目录 |
| `jd query [terms...]` | 只查询，不改变目录 |
| `jd config ...` | 查看或更新配置 |
| `jd doctor` | 检查配置、数据库和 shell 支持 |
| `jd --help` | 查看帮助；子命令同样支持 `--help` |
| `jd --version` / `jd version` | 查看版本 |

目录名与子命令冲突时使用 `jump`：

```sh
jd jump config
```

`--` 表示结束选项解析，后续内容始终作为查询词，不代表执行或转发外部命令：

```sh
jd -- -project
jd -- --help
```

PowerShell 调用安装生成的 `jd` 包装函数时，会把未加引号的 `--` 作为自身的参数分隔符，因此需要将传给包装函数的分隔符写成字符串字面量：

```powershell
jd '--' --help
```

## 扫描与匹配

```sh
jd root add ~/work
jd root add ~/large --max-depth 2
jd root add ~/dotfiles --include-hidden
jd root add ~/links --follow-symlinks
```

默认扫描深度为 4，不跟随子目录符号链接，并忽略 `.git`、`node_modules`、`vendor`、`target`、`dist`、`.cache` 和隐藏目录。

匹配优先级：

1. 现存的直接路径；
2. 精确书签；
3. 精确目录名；
4. 路径段前缀；
5. 按序字符子序列。

同级结果按书签来源、访问频率与时间、当前目录邻近度、路径长度和字典序稳定排序。

## 自动化接口

```sh
jd query api --format table
jd query api --format json
```

JSON 输出是版本化公共接口：

```json
{
  "schema_version": 1,
  "query": ["api"],
  "candidates": [
    {
      "rank": 1,
      "path": "/Users/me/work/api",
      "display": "~/work/api",
      "match": "exact_name",
      "sources": ["pin", "history"]
    }
  ]
}
```

退出码：

| Code | 含义 |
|---:|---|
| 0 | 成功 |
| 1 | 运行、配置或存储错误 |
| 2 | 参数错误 |
| 3 | 无匹配 |
| 4 | 非交互环境中存在多个候选 |
| 130 | 用户取消选择 |

内部 SQLite schema、frecency 数值和排序权重不是公共 API。

## 配置与数据

```sh
jd config path
jd config show
jd config get max_results
jd config set max_results 50
jd config set scan.max_depth 6
jd config set track_shell_cd false
```

默认配置：

```toml
version = 1
max_results = 100
stale_days = 30
track_shell_cd = true

[scan]
max_depth = 4
include_hidden = false
follow_symlinks = false
ignore = [".git", "node_modules", "vendor", "target", "dist", ".cache"]
```

| 平台 | 配置 | 数据库 |
|---|---|---|
| macOS / Linux | `$XDG_CONFIG_HOME/jd/config.toml` 或 `~/.config/jd/config.toml` | `$XDG_DATA_HOME/jd/state.db` 或 `~/.local/share/jd/state.db` |
| Windows | `%APPDATA%\jd\config.toml` | `%LOCALAPPDATA%\jd\state.db` |

`JD_CONFIG_DIR` 和 `JD_DATA_DIR` 可以覆盖默认位置。修改 `track_shell_cd` 后需要重新加载 shell profile。

## 为什么需要 shell 初始化

普通子进程不能改变父 shell 的当前目录。安装器写入的 shell 函数会调用 `jd` 二进制获取绝对路径，再使用 shell 内建的 `cd --` 或 PowerShell `Set-Location -LiteralPath` 完成跳转。

二进制输出的目标路径只作为数据处理，不会作为 shell 代码求值。手动集成：

```sh
eval "$(/absolute/path/to/jd init zsh)"
eval "$(/absolute/path/to/jd completion zsh)"
```

## 故障排查

### `jd` 只打印路径，没有跳转

说明当前终端调用的是二进制，而不是 shell 函数：

```sh
type jd
```

zsh/bash/fish 应显示 function。重新打开终端，或加载 profile：

```sh
source ~/.zshrc
```

PowerShell 使用：

```powershell
Get-Command jd
. $PROFILE
```

### 多个匹配但没有选择器

非交互环境不会猜测目标，会返回退出码 4。使用更具体的查询、书签名或 `--first`。

### 数据库异常

```sh
jd doctor
```

`doctor` 会报告数据库路径和恢复建议，不会自动删除状态文件。先备份文件，再决定移动或重建。

### 命令名冲突

安装时换一个 shell 函数名：

```sh
./scripts/install.sh --bind jdir
```

```powershell
.\scripts\install.ps1 -Bind jdir
```

## 更新与卸载

重新运行安装器即可升级到最新 Release；配置、书签和历史会保留。

```sh
curl -fsSL https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.sh | sh
./scripts/install.sh --uninstall
```

```powershell
irm https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.ps1 | iex
.\scripts\install.ps1 -Uninstall
```

卸载默认保留用户数据。永久删除配置与历史必须显式确认：

```sh
./scripts/install.sh --uninstall --purge --yes
```

```powershell
.\scripts\install.ps1 -Uninstall -Purge -Yes
```

## 隐私与安全边界

- `jd` 运行时不联网、不上传数据、不执行目录内命令；
- 无遥测、无后台服务，配置和历史仅保存在本机；
- Release 安装器是唯一需要联网的组件，并在替换现有二进制前验证 SHA-256；
- SQLite 使用 WAL、外键和 busy timeout，配置通过临时文件原子替换；
- purge 拒绝根目录和 HOME 等危险目标。

## 开发与发布

需要 Go 1.24 或更高版本：

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go mod verify
test -z "$(gofmt -l cmd internal)"
```

验证发布制品：

```sh
goreleaser check
goreleaser release --snapshot --clean
```

推送符合 `vX.Y.Z` 的标签后，Release workflow 会运行测试并发布 macOS、Linux、Windows 的 amd64/arm64 归档和 `checksums.txt`。标签和 GitHub Release 不由安装器自动创建。

详细模块边界、状态模型和公共契约见 [架构说明](docs/architecture.md)。
