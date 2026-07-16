# Codex Auth Sync

`codex-auth-sync` 是一个独立的 `codex-auth` 多服务器同步外挂，不修改原项目。它可以同步账号凭据快照、别名、活动账号、刷新间隔，以及可选的额度缓存。

主要特性：

- AES-256-GCM 端到端加密，后端只保存密文。
- 支持多服务器同时写入、删除标记、事件去重和冲突重试。
- 避免旧 `last_refresh` Token 覆盖新 Token。
- 支持 `local`、`group`、`global` 三种活动账号同步范围。
- 支持 Linux、macOS、Windows 的 x64/arm64。
- 支持 WebDAV、内置服务 + Tailscale、Cloudflare Workers + D1。

## 最快开始：WebDAV

Linux/macOS 安装：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.ps1 | iex
```

生成集群加密密钥：

```sh
codex-auth-sync keygen
```

配置 WebDAV。推荐使用服务商生成的应用密码：

```sh
export CODEX_AUTH_SYNC_WEBDAV_PASSWORD='<WebDAV 应用密码>'

codex-auth-sync init \
  --backend webdav \
  --server 'https://app.koofr.net/dav/Koofr' \
  --webdav-username 'you@example.com' \
  --sync-key '<keygen 输出的 sync key>' \
  --cluster personal \
  --active-scope group \
  --active-group main

unset CODEX_AUTH_SYNC_WEBDAV_PASSWORD
codex-auth-sync now
codex-auth-sync status
```

在其他服务器重复执行 `init`，使用相同的 WebDAV、sync key、cluster 和 active group 即可。每台服务器会自动生成独立 node ID。

安装后台服务：

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install-service.sh \
  | sh -s -- "$(command -v codex-auth-sync)"
```

Koofr 已经过真实服务测试。WebDAV 服务必须支持强 ETag、`If-Match` 和条件 PUT；不满足安全并发要求时，客户端会拒绝启用。

需要从零安装 Codex CLI、`codex-auth`、同步工具和后台服务，或需要区分首台种子服务器、云端覆盖和账号合并时，请使用[WebDAV 多服务器一键部署教程](docs/webdav-multi-server.zh-CN.md)。

`codex-auth-sync now` 会立即完成一次同步。在没有 systemd 的 Docker 容器里，可以选择手动执行 `now`，也可以把 `codex-auth-sync daemon` 放入容器入口或进程管理器。更新和卸载方法见[安装与维护文档](docs/install.md)。

## 同步内容

| 内容 | 默认行为 |
|---|---|
| `accounts/*.auth.json` | 加密同步 |
| 账号信息与别名 | 同步 |
| 活动账号 | 按 group 同步 |
| 刷新间隔 | 同步 |
| 额度缓存 | 默认关闭，可选同步 |
| `sessions/` | 永不同步 |
| 备份、临时文件 | 永不上传，在本地生成 |

## 后端选择

- WebDAV：已有 Koofr、Nextcloud、ownCloud、群晖或 NAS 时最简单。
- 内置服务 + Tailscale：完全私有、无云服务费用，适合多台自有服务器。
- Cloudflare Workers + D1：免维护的免费额度方案，建议使用 15 秒左右轮询间隔。

## 注意事项

- `sync_key`、WebDAV 应用密码和 Hub API Token 都是敏感信息。
- 拥有同一个 sync key 的服务器可以解密整个集群的账号凭据。
- 已经运行的 Codex 进程可能缓存认证信息，远程切换后可能需要启动新进程或重启客户端。
- 当前版本会原样适配 `codex-auth` registry schema v2、v3 和 v4，不升级或降级原工具；遇到未知新 schema 会拒绝写入。

完整技术文档与构建说明请查看 [English README](README.md)。
