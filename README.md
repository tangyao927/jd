# jd

`jd` 是一个本地优先、跨平台的智能目录跳转工具。它能学习访问历史、维护目录书签、扫描项目根目录，并在多个结果之间提供内置交互选择器。

- macOS、Linux、Windows
- zsh、bash、fish、Windows PowerShell 5.1、PowerShell 7
- 单个原生可执行文件，安装后不需要 Go 或其他运行时
- 本地 SQLite 状态库，无网络、无遥测、无后台进程

## 安装

从当前源码目录安装需要 Go 1.24 或更高版本。安装完成后的日常使用不依赖 Go。

macOS / Linux：

```sh
./scripts/install.sh
```

指定 shell 或避免已有命令冲突：

```sh
./scripts/install.sh --shell zsh --bind jd
./scripts/install.sh --shell fish --bind jdir
```

Windows PowerShell：

```powershell
.\scripts\install.ps1
.\scripts\install.ps1 -Bind jdir
```

安装器会：

1. 在临时目录构建 `jd`；
2. 安装到用户级目录，不使用管理员权限；
3. 备份并幂等更新当前 shell 的 profile；
4. 在交互安装时列出常见项目目录，确认后才建立扫描索引。

安装后重新打开终端，或手动加载对应 profile。可先用 `--dry-run` 查看所有目标，不写入文件。

## 快速开始

```sh
# 直接路径
jd ./src

# 根据历史、书签和扫描索引搜索
jd api
jd work api src

# 模糊字符匹配：wapi 可匹配 /work/api
jd wapi

# 多个结果时打开内置选择器；脚本中可明确取第一项
jd api
jd --first api

# 目录名与子命令冲突时
jd jump config
```

### 书签

```sh
jd pin api                 # 把当前目录保存为 api
jd pin docs ~/work/docs
jd pins
jd api
jd unpin api
```

### 扫描根目录

```sh
jd root add ~/work
jd root add ~/large --max-depth 2
jd root list
jd scan                    # 刷新全部扫描根
jd root remove ~/work
```

扫描默认深度为 4，不跟随目录符号链接，并忽略 `.git`、`node_modules`、`vendor`、`target`、`dist`、`.cache` 和隐藏目录。

### 历史与自动化

```sh
jd history list
jd history forget          # 忘记当前目录
jd history forget ~/tmp
jd history clean           # 清理已失效超过保留期的目录

jd query api --format table
jd query api --format json
```

`query --format json` 是稳定的机器接口，返回带 `schema_version` 的候选列表。内部 SQLite schema 不是公共接口。

## 配置

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

Unix 遵循 XDG Config/Data 目录；Windows 分别使用 Roaming AppData 与 Local AppData。`JD_CONFIG_DIR`、`JD_DATA_DIR` 可覆盖位置。

## 为什么需要 shell 初始化

普通子进程不能改变父 shell 的当前目录。`jd init` 生成一个很薄的 shell 函数：二进制只负责选择并打印绝对路径，函数再通过 shell 内建的 `cd` 或 `Set-Location` 完成跳转。目标路径始终按数据处理，不会被 `eval`。

手动集成示例：

```sh
eval "$(/absolute/path/to/jd init zsh)"
eval "$(/absolute/path/to/jd completion zsh)"
```

## 诊断与卸载

```sh
jd doctor
jd version

./scripts/install.sh --uninstall
.\scripts\install.ps1 -Uninstall
```

卸载默认保留配置和历史。永久删除数据必须显式使用 `--purge --yes` 或 PowerShell 的 `-Purge -Yes`。

## 开发验证

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/jd
```

详细模块边界、状态模型和退出码见 [架构说明](docs/architecture.md)。
