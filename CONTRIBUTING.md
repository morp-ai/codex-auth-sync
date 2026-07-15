# Contributing

Contributions are welcome. Keep the sidecar independent from `codex-auth`, preserve end-to-end encryption, and maintain CGO-free builds for all six supported targets.

Before opening a pull request, run:

```sh
go test -race ./...
go vet ./...
node --check deploy/cloudflare-worker/src/index.js
shellcheck scripts/*.sh
```

Do not commit real account files or credentials. All CLI output, prompts, warnings, and errors must remain English-only.
