# Installation

Release artifacts are produced in `dist/` by `scripts/build-all.sh`.

The fastest installation uses the release installer, which verifies `checksums.txt` before copying the binary:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/install.ps1 | iex
```

## npm installation

Machines that already have Node.js 16 or newer can install from GitHub in the same style as other Codex helper CLIs:

```sh
npm install -g git+https://github.com/morp-ai/codex-auth-sync.git
codex-auth-sync version
```

The package identity is `@morp-ai/codex-auth-sync`, but no npm registry login is needed for this GitHub installation. Its post-install step selects Linux, macOS, or Windows and x64 or arm64, downloads the native binary from the matching GitHub Release, verifies it against `checksums.txt`, and exposes `codex-auth-sync` through npm's global command directory.

The npm layer contains no account synchronization implementation and has no dependency on `codex-auth`; it only launches the verified Go binary. Existing configuration in the platform's normal `codex-auth-sync` configuration directory continues to work.

Update a GitHub/npm installation by running the same command again:

```sh
npm install -g git+https://github.com/morp-ai/codex-auth-sync.git
```

Uninstall only the npm-managed launcher and binary with:

```sh
npm uninstall -g @morp-ai/codex-auth-sync
```

This does not remove synchronization configuration, `~/.codex`, `codex-auth`, or remote WebDAV data.

## Linux

Choose `codex-auth-sync-linux-amd64` or `codex-auth-sync-linux-arm64`:

```sh
install -m 0755 codex-auth-sync-linux-amd64 "$HOME/.local/bin/codex-auth-sync"
codex-auth-sync version
```

## macOS

Choose `codex-auth-sync-darwin-amd64` for Intel or `codex-auth-sync-darwin-arm64` for Apple Silicon:

```sh
install -m 0755 codex-auth-sync-darwin-arm64 /usr/local/bin/codex-auth-sync
codex-auth-sync version
```

If Gatekeeper marks a manually transferred binary as quarantined, inspect it and remove the quarantine attribute only after verifying the checksum:

```sh
xattr -d com.apple.quarantine /usr/local/bin/codex-auth-sync
```

## Windows

Copy the matching `.exe` file into a private directory such as:

```text
%LOCALAPPDATA%\Programs\codex-auth-sync\codex-auth-sync.exe
```

Add that directory to the user `PATH`, initialize the configuration, and run `scripts/install-service.ps1` to register the background task.

## Codex Auth

The sidecar can restore the synchronized files without invoking `codex-auth`, but each machine still needs `codex-auth` if users want its `login`, `list`, `switch`, `remove`, and import/export commands:

```sh
npm install -g @loongphy/codex-auth
```

The sidecar adapts to the registry schema used by the installed `codex-auth` release and does not upgrade or downgrade it. Existing installations are preserved; missing installations use the official stable npm command above. Only this one-time binary installation is machine-specific. Account login state and synchronized configuration are pulled automatically afterward.

## Update

The updater downloads a release, verifies its published SHA-256 checksum, replaces only the `codex-auth-sync` binary, and restarts a managed service when one is detected. It does not update or modify `codex-auth`.

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/update.sh | sh
```

To install a specific release, set `CODEX_AUTH_SYNC_VERSION`, including the `v` prefix:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/update.sh \
  | CODEX_AUTH_SYNC_VERSION='v0.1.4' sh
```

Windows PowerShell:

```powershell
$script = irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/update.ps1
& ([scriptblock]::Create($script))
```

In a container without a managed service, the updater replaces the binary and asks you to restart any manually managed daemon. A later `codex-auth-sync now` invocation immediately uses the new binary.

## Uninstall

The default uninstaller removes the binary and managed background service while preserving synchronization configuration, local backups, `~/.codex`, and remote WebDAV data:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/uninstall.sh | sh
```

Add `--purge` to also delete local synchronization configuration, state, logs, and backups:

```sh
curl -fsSL https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/uninstall.sh \
  | sh -s -- --purge
```

Windows PowerShell:

```powershell
$script = irm https://raw.githubusercontent.com/morp-ai/codex-auth-sync/main/scripts/uninstall.ps1
& ([scriptblock]::Create($script))          # preserve local sync configuration
& ([scriptblock]::Create($script)) -Purge   # also remove local sync configuration and backups
```

Uninstalling never removes the remote encrypted cluster. Delete that object through the WebDAV provider only after every node has been retired and a backup is no longer needed.

## Containers and manual synchronization

Containers commonly do not run systemd. The service installer therefore skips service registration cleanly when no usable systemd user manager exists. Use either of these models:

- Manual: run `codex-auth-sync now` on the source node after a login or switch, then run it on follower nodes.
- Continuous: run `codex-auth-sync daemon` under the container entrypoint, Docker Compose, Kubernetes, or another process supervisor.

Persist both the Codex home and synchronization configuration across container recreation. For the root user these are normally `/root/.codex` and `/root/.config/codex-auth-sync`. Only one daemon should write a given local Codex home.
