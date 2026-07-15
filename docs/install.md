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
