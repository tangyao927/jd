# jd

[![CI](https://github.com/tangyao927/jd/actions/workflows/ci.yml/badge.svg)](https://github.com/tangyao927/jd/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tangyao927/jd?display_name=tag)](https://github.com/tangyao927/jd/releases)

`jd` 是一个跨平台的目录跳转工具。它根据访问历史、书签和扫描索引查找目录，并在有多个结果时提供交互选择。

- 支持 macOS、Linux 和 Windows（amd64、arm64）
- 支持 zsh、bash、fish、Windows PowerShell 5.1 和 PowerShell 7
- 使用本地 SQLite 保存状态，无遥测和后台服务
- 安装后无需 Go 或其他运行时
- 提供适合脚本使用的 JSON 查询接口

## 安装

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.sh | sh
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/tangyao927/jd/main/scripts/install.ps1 | iex
```

通过上述网络命令运行安装器时，会下载最新的 GitHub Release，校验 SHA-256 后安装到用户目录，并更新当前 shell 的 profile。安装完成后重新打开终端，或手动加载 profile。

如果还没有可用的 Release，或需要安装当前修改后的代码，可以从源码安装（需要 Go 1.24 或更高版本）。在源码 checkout 内直接运行安装器时，默认构建当前源码；`--source` / `-Source` 是对应的显式写法：

```sh
git clone https://github.com/tangyao927/jd.git
cd jd
./scripts/install.sh
# 等价：./scripts/install.sh --source
```

```powershell
git clone https://github.com/tangyao927/jd.git
Set-Location jd
.\scripts\install.ps1
# 等价：.\scripts\install.ps1 -Source
```

可选参数：

```sh
./scripts/install.sh --version latest # 在源码目录内也强制安装最新 Release
./scripts/install.sh --version v0.1.0  # 安装指定版本
./scripts/install.sh --source          # 显式构建当前源码
./scripts/install.sh --bind jdir       # 使用其他命令名
./scripts/install.sh --dry-run         # 只显示将要执行的操作
```

PowerShell 对应参数为 `-Version`、`-Source`、`-Bind` 和 `-DryRun`。

## 使用

### 书签

```sh
jd pin api ~/work/api  # 将目录保存为 api
jd api                 # 跳转到书签目录

jd pin work            # 省略目录时使用当前目录
jd pins                # 查看书签
jd unpin work          # 删除书签
```

### 扫描目录

```sh
jd root add ~/work
jd api
jd work api
```

刷新或管理扫描根：

```sh
jd scan
jd root list
jd root remove ~/work
```

### 查询目录

```sh
jd api          # 单个结果直接跳转，多个结果打开选择器
jd wapi         # 可匹配 /work/api
jd --first api  # 直接使用排序第一的结果
```

目录名与子命令重名时使用 `jump`：

```sh
jd jump config
```

## 常用命令

| 命令 | 作用 |
|---|---|
| `jd [query...]` | 查询并跳转 |
| `jd jump [query...]` | 将参数直接作为目录查询 |
| `jd pin <name> [path]` | 创建或更新书签 |
| `jd pins` / `jd unpin <name>` | 查看或删除书签 |
| `jd root add <path>` | 添加并扫描目录根 |
| `jd root list` / `jd root remove <path>` | 查看或删除扫描根 |
| `jd scan` | 刷新扫描索引 |
| `jd history list` | 查看访问历史 |
| `jd history forget [path]` | 删除目录的访问记录 |
| `jd history clean` | 清理过期的失效目录 |
| `jd query [terms...]` | 查询但不跳转 |
| `jd config ...` | 查看或修改配置 |
| `jd doctor` | 检查配置和本地状态 |
| `jd --help` / `jd --version` | 查看帮助或版本 |

## 扫描与匹配

```sh
jd root add ~/large --max-depth 2
jd root add ~/dotfiles --include-hidden
jd root add ~/links --follow-symlinks
```

默认扫描深度为 4，不跟随子目录符号链接，并忽略隐藏目录、`.git`、`node_modules`、`vendor`、`target`、`dist` 和 `.cache`。

匹配时优先使用直接路径和精确书签，其次是精确目录名、路径段前缀和字符子序列。同级结果按访问情况和当前位置等因素排序。

## 配置与数据

```sh
jd config path
jd config show
jd config get max_results
jd config set max_results 50
jd config set scan.max_depth 6
jd config set track_shell_cd false
```

| 平台 | 配置文件 | 数据库 |
|---|---|---|
| macOS / Linux | `$XDG_CONFIG_HOME/jd/config.toml` 或 `~/.config/jd/config.toml` | `$XDG_DATA_HOME/jd/state.db` 或 `~/.local/share/jd/state.db` |
| Windows | `%APPDATA%\jd\config.toml` | `%LOCALAPPDATA%\jd\state.db` |

可通过 `JD_CONFIG_DIR` 和 `JD_DATA_DIR` 覆盖默认位置。修改 `track_shell_cd` 后需要重新加载 shell profile。

## 脚本调用

`query` 子命令只输出候选结果，不执行跳转：

```sh
jd query api --format table
jd query api --format json
```

JSON 响应包含 `schema_version`、查询词和排序后的候选列表。退出码如下：

| Code | 含义 |
|---:|---|
| 0 | 成功 |
| 1 | 运行、配置或存储错误 |
| 2 | 参数错误 |
| 3 | 无匹配 |
| 4 | 非交互环境中存在多个候选 |
| 130 | 用户取消选择 |

## 更新与卸载

Release 用户重新运行网络安装命令即可更新到最新已发布版本。修改本地源码后，在源码目录重新运行安装器即可构建并替换已安装的二进制：

```sh
./scripts/install.sh
```

```powershell
.\scripts\install.ps1
```

配置、书签和历史不会被覆盖。普通二进制逻辑更新后无需重新加载 shell；如果 shell 初始化或补全逻辑发生变化，则重新打开终端或加载 profile。

从源码目录或下载后的脚本执行卸载：

```sh
./scripts/install.sh --uninstall
```

```powershell
.\scripts\install.ps1 -Uninstall
```

卸载默认保留用户数据。永久删除配置和历史需要显式确认：

```sh
./scripts/install.sh --uninstall --purge --yes
```

```powershell
.\scripts\install.ps1 -Uninstall -Purge -Yes
```

## 隐私

`jd` 日常运行不联网、不上传数据，也不会执行目标目录中的命令。安装 Release 或从源码获取依赖时可能需要网络。

实现细节和公共接口约定见 [架构说明](docs/architecture.md)。
