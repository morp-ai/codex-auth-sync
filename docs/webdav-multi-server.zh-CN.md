# WebDAV 多服务器一键部署教程

本文以 Koofr WebDAV 为例，从零部署 Codex CLI、`codex-auth` 和 `codex-auth-sync`。同一个安装脚本也适用于满足强 ETag 和条件写入要求的其他 WebDAV 服务。

## 先理解安装顺序

`codex-auth` 是账号登录和切换 CLI，不是常驻服务。`codex-auth-sync` 才是需要后台自启动的同步守护进程。

推荐顺序：

1. 在选定的第一台服务器安装 Codex CLI 和 `codex-auth`。
2. 在第一台服务器执行 `codex-auth login`，添加并检查所有希望作为初始云端状态的账号。
3. 在第一台服务器运行一键脚本。脚本将其识别为 `seed`，生成共享加密密钥并首次上传。
4. 保存脚本显示的共享加密密钥。其他服务器必须使用完全相同的密钥、cluster 和 active group。
5. 在其他服务器运行同一个脚本，输入相同的 WebDAV 应用密码和共享加密密钥。
6. 新服务器没有本地账号时，脚本自动使用 `join`，从云端恢复账号并安装后台服务，不需要重新逐个登录。

登录过程本身需要浏览器或设备授权，不能安全地放进无人值守脚本。除第一次建立账号库之外，后续服务器不需要重复登录。

## 部署前准备

在 Koofr 中为 WebDAV 单独生成应用密码，不要使用主账户密码。准备：

- WebDAV URL，例如 `https://app.koofr.net/dav/Koofr`
- Koofr 账户邮箱
- 新生成的应用密码
- 一个固定 cluster 名称，例如 `personal-v1`

应用密码和同步密钥不要写进 shell 历史、聊天记录、Git 仓库或公开脚本。一键脚本默认使用隐藏输入，并将最终配置保存为仅当前用户可读的配置文件。

如果必须用于无人值守部署，脚本也接受 `--password` 和 `--sync-key`。这两个参数会出现在 shell 历史、进程参数和部分审计日志中，安全性低于隐藏输入或受保护的环境变量；只应在隔离的部署系统中使用。

## Linux 和 macOS 一键命令

在每台服务器执行同一条命令：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- \
      --server 'https://app.koofr.net/dav/Koofr' \
      --username 'you@example.com' \
      --cluster 'personal-v1' \
      --active-scope group \
      --active-group main \
      --interval 5
```

root 用户会把同步工具安装到 `/usr/local/bin/codex-auth-sync`。一键命令完成后可直接运行：

```sh
codex-auth-sync now
codex-auth-sync status
```

不再需要 `/root/.local/bin/codex-auth-sync` 路径前缀。

需要把两个秘密显式写入命令时，可以使用以下形式（请替换为新生成的值，不要使用已经公开过的密码或密钥）：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- \
      --server 'https://app.koofr.net/dav/Koofr' \
      --username 'you@example.com' \
      --password '<new WebDAV app password>' \
      --sync-key '<shared synchronization key>' \
      --cluster 'personal-v1' \
      --active-scope group \
      --active-group main \
      --interval 5
```

脚本会：

1. 分别检查 OpenAI Codex CLI 和 `codex-auth`；已有安装及版本会原样保留，只通过官方稳定 npm 命令安装缺失项。
2. 需要安装 npm 包但缺少 npm 时，才尝试通过系统包管理器安装 Node.js 和 npm。
3. 下载与 CPU/系统匹配的 `codex-auth-sync`，并校验 SHA-256。
4. 隐藏读取 WebDAV 应用密码。
5. 检查本地账号和目标 WebDAV cluster，自动选择安全角色；两边都有数据时让用户选择合并、云端权威或本地权威。
6. 初始化、执行首次同步并验证状态。
7. 安装 systemd user service 或 macOS LaunchAgent。
8. Linux 上尽可能启用 user lingering，使用户退出 SSH 后服务仍能运行。

如果第一台服务器尚未登录账号，脚本会询问是否立即启动：

```sh
codex-auth login
codex-auth list
```

完成登录后，脚本会继续 seed。也可以拒绝自动登录，稍后手动登录并重新执行同一条一键命令。

## Windows PowerShell 一键命令

在普通 PowerShell 中运行：

```powershell
$script = irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.ps1
& ([scriptblock]::Create($script)) `
  -Server 'https://app.koofr.net/dav/Koofr' `
  -Username 'you@example.com' `
  -Cluster 'personal-v1' `
  -ActiveScope group `
  -ActiveGroup main `
  -Interval 5
```

脚本在缺少 Node.js 时使用 `winget` 安装 Node.js LTS，并注册 `CodexAuthSync` 计划任务。该任务在 Windows 用户登录时启动。

## 五种部署角色

| 角色 | 行为 | 适用情况 |
|---|---|---|
| `auto` | 根据本地账号和云端 cluster 自动判断；两边都有数据时要求选择 | 默认，推荐 |
| `seed` | 要求云端 cluster 为空；把本地账号作为第一份状态上传 | 第一台已有完整账号的服务器 |
| `join` | 先备份本地 `auth.json` 和 `accounts/`，再以云端为准恢复 | 新服务器，或希望云端覆盖本地的服务器 |
| `merge` | 保留本地账号，与云端做并集合并 | 两边账号都需要保留 |
| `local` | 要求一个新的空 replacement cluster，并把本地作为权威 seed | 希望本地覆盖云端状态，同时保留旧云端历史 |

可以显式指定角色：

```sh
# 第一台权威服务器
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- --server 'https://app.koofr.net/dav/Koofr' --username 'you@example.com' \
      --cluster 'personal-v2' --role seed

# 后续服务器：备份本地后严格使用云端
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- --server 'https://app.koofr.net/dav/Koofr' --username 'you@example.com' \
      --cluster 'personal-v2' --role join

# 两边都有数据时，以本地为权威并安全切换到一个新 cluster
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- --server 'https://app.koofr.net/dav/Koofr' --username 'you@example.com' \
      --cluster 'personal-v1' --role local --replacement-cluster 'personal-v2'
```

## 共享加密密钥

第一次 `seed` 时，如果没有提供同步密钥，脚本会生成一个 32 字节 base64 密钥并显示一次。立即保存到密码管理器。

后续服务器会隐藏提示输入该密钥。也可以用于自动化环境：

```sh
export CODEX_AUTH_SYNC_WEBDAV_PASSWORD='<new Koofr app password>'
export CODEX_AUTH_SYNC_KEY='<shared sync key>'

curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh \
  | sh -s -- --server 'https://app.koofr.net/dav/Koofr' --username 'you@example.com' \
      --cluster 'personal-v1' --role join

unset CODEX_AUTH_SYNC_WEBDAV_PASSWORD CODEX_AUTH_SYNC_KEY
```

环境变量适合受保护的部署系统或临时终端，不要把带有真实密钥的命令保存到脚本或 shell 历史中。

## 冲突如何处理

正常同步不会简单复制整个 `auth.json`：

- 不同服务器上存在不同账号时，默认形成账号并集。
- 同一账号的认证快照冲突时，优先保留 `last_refresh` 或 Token 到期信息更新的一份，避免旧 Token 覆盖新 Token。
- 同时上传时，WebDAV 强 ETag、`If-Match` 和重试保证写入被串行化，不会静默覆盖整个远端文件。
- 别名和活动账号按远端事件顺序收敛；活动账号默认只在同一个 active group 内传播。
- 删除已经被同步代理观察过的账号会产生删除事件，并传播到其他服务器。

首次接入时，默认行为是先拉取远端，再上传本地差异。因此不要把“普通 merge”理解为强制本地覆盖云端。

## 让某台服务器强制成为新的权威源

最安全的做法是切换到一个全新的 cluster，而不是直接删除或改写旧的加密事件文件：

1. 暂停其他服务器上的同步服务。
2. 在权威服务器确认 `codex-auth list` 只包含需要保留的账号。
3. 选择一个从未使用过的新 cluster，例如从 `personal-v1` 改为 `personal-v2`。
4. 在权威服务器用 `--role local --cluster personal-v1 --replacement-cluster personal-v2` 运行一键脚本；也可以直接对新 cluster 使用 `--role seed`。
5. 在其他服务器用相同密钥和新 cluster，以 `--role join` 运行脚本。
6. 确认所有服务器正常后，旧 cluster 可留作加密历史备份，或在 WebDAV 管理界面中手动删除。

`join` 会先将原有本地认证文件备份到：

```text
~/.codex-auth-sync/backups/codex-auth-<UTC时间>/
```

这样可以实现“本地权威源覆盖新云端，再同步到其他服务器”，同时保留可恢复的本地备份。

## 日常使用和自启动

日常操作仍使用：

```sh
codex-auth login
codex-auth list
codex-auth switch
codex-auth remove
```

同步守护进程会在几秒内发现这些修改并传播。已经运行的 Codex CLI、App 或扩展可能缓存认证状态；切换同步完成后，应新开 Codex 进程，必要时重启客户端。

也可以随时手动立即同步：

```sh
codex-auth-sync now
```

`now` 与原有的 `sync` 完全等价。手动传播一次切换时，先在发生切换的服务器 A 执行 `codex-auth-sync now`，使本地变化上传；再在服务器 B 执行 `codex-auth-sync now`，拉取 A 的变化。只在 B 执行无法取得尚未上传的 A 本地变化。

### Docker 或没有 systemd 的 Linux

Docker 容器通常没有 `systemctl`。首次同步仍然有效；脚本会明确跳过后台服务，而不是把整个部署显示为失败。若只需要手动同步，可在一键命令末尾增加：

```text
--no-service
```

然后在需要时运行 `codex-auth-sync now`。若需要持续自动同步，应把下面的进程交给容器的 entrypoint、Docker Compose、Kubernetes 或容器内的进程管理器管理：

```sh
codex-auth-sync daemon --config /root/.config/codex-auth-sync/config.json
```

必须把 `/root/.codex` 和 `/root/.config/codex-auth-sync` 挂载为持久卷，否则重建容器会丢失本地账户文件、节点配置和同步进度。同一个 `/root/.codex` 只能运行一个同步 daemon。

检查 Linux 服务：

```sh
systemctl --user status codex-auth-sync.service
journalctl --user -u codex-auth-sync.service -n 100 --no-pager
codex-auth-sync status
```

检查 macOS：

```sh
launchctl print "gui/$(id -u)/io.codex-auth-sync.agent"
tail -n 100 "$HOME/Library/Logs/codex-auth-sync.log"
codex-auth-sync status
```

检查 Windows：

```powershell
Get-ScheduledTask -TaskName CodexAuthSync
codex-auth-sync status
```

## 更新同步外挂

更新只替换 `codex-auth-sync`，不会升级、降级或修改 `codex-auth`：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh | sh
```

检测到本地已有配置时，一键脚本自动进入“仅更新”模式：下载并校验最新版本、修复全局短命令、重启已存在的后台服务，但不询问或改写 WebDAV 密码、同步密钥、cluster、同步状态和账户数据。即使重复粘贴第一次带完整参数的一键命令，也仍然只更新。只有明确追加 `--reconfigure` 时才会重新配置。

注意，下面的命令只会把脚本内容输出到终端，并不会执行：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/bootstrap-webdav.sh
```

必须保留末尾的 `| sh` 才会执行更新。

Windows PowerShell：

```powershell
$script = irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/update.ps1
& ([scriptblock]::Create($script))
```

在 systemd、launchd 或 Windows 计划任务节点上，更新脚本会停止并重新启动已检测到的后台服务。在 Docker/手动模式下，更新后需要重启由 entrypoint 或 supervisor 管理的 daemon；下一次执行 `codex-auth-sync now` 会直接使用新版本。

## 卸载

默认卸载只删除同步外挂和其后台服务，保留本地同步配置、备份、`~/.codex` 账户数据与 WebDAV 远端密文：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/uninstall.sh | sh
```

连同本地同步配置、状态、日志和备份一起删除：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/uninstall.sh \
  | sh -s -- --purge
```

Windows PowerShell 使用 `uninstall.ps1`，并可追加 `-Purge`。任何卸载模式都不会删除 `codex-auth`、`~/.codex` 或 WebDAV 远端集群；远端密文需要在确认所有节点退役后从 WebDAV 管理界面手动删除。

## 修改或轮换 WebDAV 应用密码

在 Koofr 生成新应用密码后，在每台服务器重新运行同一条一键命令，输入新密码。脚本会备份旧同步配置、重建本机配置、执行验证并重新启动服务。cluster 和同步密钥必须保持不变，否则会连接到不同数据集或无法解密原有事件。

## 避免安装冲突

- 同一用户只运行一个 `codex-auth-sync` 后台实例。
- 所有工具必须使用同一个 `CODEX_HOME`；默认是 `~/.codex`。
- 首次 seed 或 join 时不要同时运行 `codex-auth login`、`switch`、`remove`。
- `codex-auth` 不需要也不应该注册后台自启动服务。
- 同步外挂会识别并保留本机 `codex-auth` registry schema；不同服务器可以运行 schema v2、v3 或 v4 的版本，外挂不会替用户升级或降级原工具。
- 若已有旧同步配置，一键脚本会先备份配置和状态，再重新初始化。
- 不同用途的服务器可以使用不同 active group；账号仍共享，但活动账号不会跨 group 强制切换。
