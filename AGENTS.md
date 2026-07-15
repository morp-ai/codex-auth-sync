# Language

- All user-facing CLI output, prompts, help text, warnings, and error messages must be written in English only.

# Compatibility

- The client must continue to cross-compile without CGO for Linux, macOS, and Windows on amd64 and arm64.
- Do not write a `codex-auth` registry schema newer than the implementation explicitly supports.

# Validation

- Run `go test ./...`, `go vet ./...`, and `node --check deploy/cloudflare-worker/src/index.js` after code changes.
- Run `./scripts/build-all.sh` before release handoff.

# Security

- Never log plaintext account snapshots, API tokens, sync keys, access tokens, refresh tokens, or API keys.
- Remote payloads must remain end-to-end encrypted.
