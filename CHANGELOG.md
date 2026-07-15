# Changelog

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
