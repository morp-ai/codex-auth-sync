# Codex Auth Sync

[![CI](https://github.com/morp-ai/codex-auth-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/morp-ai/codex-auth-sync/actions/workflows/ci.yml)
[![CodeQL](https://github.com/morp-ai/codex-auth-sync/actions/workflows/codeql.yml/badge.svg)](https://github.com/morp-ai/codex-auth-sync/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/morp-ai/codex-auth-sync)](https://github.com/morp-ai/codex-auth-sync/releases/latest)
[![License](https://img.shields.io/github/license/morp-ai/codex-auth-sync)](LICENSE)

[简体中文](README.zh-CN.md)

`codex-auth-sync` is an independent sidecar for synchronizing `codex-auth` accounts across multiple machines. It does not modify or patch `codex-auth`.

It synchronizes encrypted account snapshots, aliases, selected registry settings, and optionally the active account and cached usage state. Codex session history and unrelated files are never synchronized.

## Highlights

- End-to-end encryption with AES-256-GCM. The synchronization server stores ciphertext only.
- Multi-writer event log with monotonic revisions, conditional conflict handling, and event deduplication.
- Protects newer refreshed tokens from being overwritten by older snapshots.
- Supports local, group, and global active-account synchronization.
- Preserves each machine's installed `codex-auth` schema v2, v3, or v4 layout, filename encoding, backups, and Unix permissions.
- Runs as a single native binary on the same platforms as `codex-auth`:
  - Linux x64 and arm64
  - macOS x64 and arm64
  - Windows x64 and arm64
- Includes a lightweight self-hosted server and a Cloudflare Workers + D1 deployment for small free-tier workloads.
- Connects directly to compatible WebDAV storage without running a custom server.

## What Is Synchronized

| Data | Default | Notes |
|---|---:|---|
| `accounts/*.auth.json` | Yes | Encrypted before upload |
| Account identity metadata | Yes | Merged per account |
| Aliases | Yes | Independent events prevent stale token updates from reverting aliases |
| Active account | Yes, by group | Configurable as `local`, `group`, or `global` |
| Live refresh interval | Yes | Stored in `registry.json` |
| Usage cache | No | Enable with `--sync-usage-cache` during initialization |
| `sessions/` | Never | Machine-local and potentially large |
| Backups and temporary files | Never | Recreated locally when needed |

## Fastest Quick Start: WebDAV

If Node.js and npm are already available, install directly from GitHub. npm creates the global `codex-auth-sync` command and downloads and verifies the matching native release binary:

```sh
npm install -g git+https://github.com/morp-ai/codex-auth-sync.git
codex-auth-sync version
codex-auth-sync now
```

The npm package is only a cross-platform installer and launcher. Synchronization remains in the same CGO-free Go binary, and `codex-auth` is not installed, upgraded, downgraded, or declared as a package dependency.

The unscoped npm name `codex-auth-sync` belongs to an unrelated maintainer. This project uses the package identity `@morp-ai/codex-auth-sync`; installation from GitHub works without npm registry publication.

Alternatively, install the native binary directly.

Install the latest checksum-verified binary on Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.ps1 | iex
```

Generate a cluster encryption key:

```sh
codex-auth-sync keygen
```

Initialize a WebDAV node. Use an app password when the provider supports one:

```sh
export CODEX_AUTH_SYNC_WEBDAV_PASSWORD='<WebDAV app password>'

codex-auth-sync init \
  --backend webdav \
  --server 'https://app.koofr.net/dav/Koofr' \
  --webdav-username 'you@example.com' \
  --sync-key '<sync key from keygen>' \
  --cluster personal \
  --active-scope group \
  --active-group main

unset CODEX_AUTH_SYNC_WEBDAV_PASSWORD
codex-auth-sync sync
codex-auth-sync status
```

Repeat `init` on every server with the same URL, credentials, sync key, cluster, and active group. Node IDs are generated automatically. Install the background service after the first successful sync:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install-service.sh \
  | sh -s -- "$(command -v codex-auth-sync)"
```

Koofr has been tested against the real service endpoint. The WebDAV backend requires HTTPS and strong ETag/`If-Match` support so simultaneous writers cannot silently overwrite each other. See [docs/webdav.md](docs/webdav.md).

For an end-to-end Chinese tutorial covering installation order, seed/join/merge modes, conflict handling, and the cross-platform bootstrap scripts, see [docs/webdav-multi-server.zh-CN.md](docs/webdav-multi-server.zh-CN.md).

## Backend Choices

| Backend | Cost | Typical latency | Best for |
|---|---:|---:|---|
| WebDAV | Often free | Poll interval | Existing Koofr, Nextcloud, ownCloud, NAS, or rclone storage |
| Built-in hub + Tailscale | Free | A few seconds | Private server fleets |
| Cloudflare Workers + D1 | Free tier | 10-30 seconds | Managed public endpoint without maintaining a server |

## Build

Go 1.18 or later is required only when building from source.

```sh
go test ./...
go build -o bin/codex-auth-sync ./cmd/codex-auth-sync
```

Build all six release binaries:

```sh
VERSION=0.1.0 ./scripts/build-all.sh
```

The release binaries have no runtime dependencies. See [docs/install.md](docs/install.md) for per-platform installation and background-service setup.

## Quick Start With the Free Self-Hosted Server

Generate the two cluster secrets once:

```sh
codex-auth-sync keygen
```

Run the server on one machine. Keep it behind Tailscale, WireGuard, or an HTTPS reverse proxy:

```sh
export CODEX_AUTH_SYNC_SERVER_TOKEN='<API token>'
codex-auth-sync serve --listen 0.0.0.0:8787
```

Initialize every client machine with the same API token, sync key, cluster, and active group. Each node gets a unique node ID automatically:

```sh
codex-auth-sync init \
  --server http://100.64.0.10:8787 \
  --api-token '<API token>' \
  --sync-key '<sync key>' \
  --cluster personal \
  --active-scope group \
  --active-group main

codex-auth-sync sync
codex-auth-sync status
```

Install the background agent:

```sh
./scripts/install-service.sh "$(command -v codex-auth-sync)"
```

On Windows PowerShell:

```powershell
.\scripts\install-service.ps1 -BinaryPath "C:\Tools\codex-auth-sync.exe"
```

## Free Cloudflare Deployment

The Cloudflare Worker uses D1 for the encrypted event log. The stored data is small, but free-tier quotas and pricing can change. A 15-second client interval is a conservative starting point for several machines.

See [docs/cloudflare.md](docs/cloudflare.md) for deployment commands.

For a fully private zero-cloud-cost deployment, see [docs/tailscale.md](docs/tailscale.md).

For direct storage with no custom service, see [docs/webdav.md](docs/webdav.md).

## Active Account Modes

- `local`: synchronize the account vault, but never change the active account on another machine.
- `group`: machines with the same `active_group` follow each other's account switches. This is the default.
- `global`: every machine in the cluster follows one global active account.

Group mode is recommended when multiple servers may be used simultaneously. It prevents an unrelated server from unexpectedly changing credentials.

## Security Notes

- `api_token` authenticates a node to the synchronization service.
- WebDAV deployments use an app password or bearer token instead of the hub API token.
- `sync_key` encrypts and decrypts account data. The server never receives the plaintext key.
- Both values are secrets. Store the configuration file as carefully as `auth.json`.
- Every machine holding the same sync key can decrypt the cluster account snapshots.
- Cloud storage or database compromise exposes ciphertext and event timing, but not account tokens without the sync key.
- Existing Codex processes may cache credentials. Updating `auth.json` does not guarantee that a running Codex CLI or App process changes accounts immediately; start a new process or restart the client when required.

See [docs/security.md](docs/security.md) and [docs/architecture.md](docs/architecture.md) for details.

## Commands

```text
codex-auth-sync keygen
codex-auth-sync init [options]
codex-auth-sync sync [--config path]
codex-auth-sync now [--config path]
codex-auth-sync daemon [--config path]
codex-auth-sync status [--config path]
codex-auth-sync serve [options]
```

`now` is an alias for one immediate `sync` cycle. Update and uninstall instructions, including container behavior, are in [docs/install.md](docs/install.md).

All CLI output, prompts, warnings, and errors are in English.

## Compatibility Boundary

This release adapts to `codex-auth` registry schemas v2, v3, and v4 without upgrading or downgrading the installed tool. It refuses to modify a newer unknown schema rather than risking credential or registry damage. Update `codex-auth-sync` after a future schema migration.

## License

MIT
