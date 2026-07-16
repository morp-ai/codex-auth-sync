# Changelog

## v0.1.7

- Install root-owned native clients in `/usr/local/bin` so `codex-auth-sync now` works without a path prefix.
- Add safe update-only behavior to the WebDAV bootstrap when an existing configuration is detected.
- Preserve WebDAV credentials, sync keys, clusters, state, accounts, and installed `codex-auth` versions during repeat bootstrap runs.
- Require explicit `--reconfigure` or `-Reconfigure` before an existing configuration can be replaced.
- Remove the optional npm distribution layer in favor of the simpler native installation path.

## v0.1.3

- Preserve installed `codex-auth` versions and use the official stable npm package only when it is missing.
- Add schema-preserving adapters for codex-auth registry schemas v2, v3, and v4.
- Synchronize common account semantics across mixed-version servers without writing newer fields into older registries.

## v0.1.2

- Add idempotent Linux/macOS and Windows WebDAV bootstrap scripts.
- Install Codex CLI, `codex-auth`, the synchronization agent, and its background service in one workflow.
- Add automatic, seed, join, merge, and safe local-authoritative deployment roles with local backups.
- Document multi-server installation order, conflict behavior, credential rotation, and authoritative cluster replacement.

## v0.1.1

- Parse bounded HTTP list limits directly as native integers, resolving CodeQL integer-conversion findings.
- Update GitHub Actions dependencies and repository security automation.

## v0.1.0

- Initial encrypted multi-machine synchronization agent.
- Built-in self-hosted HTTP hub with persistent event storage.
- Cloudflare Workers + D1 free-tier backend.
- Direct WebDAV backend with strong ETag and conditional-write conflict protection.
- Account, alias, active selection, settings, deletion, and optional usage-cache synchronization.
- Linux, macOS, and Windows builds for amd64 and arm64.
- Linux systemd, macOS launchd, and Windows scheduled-task installers.
