.PHONY: test vet build build-all check

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o bin/codex-auth-sync ./cmd/codex-auth-sync

build-all:
	./scripts/build-all.sh

check: test vet
	node --check deploy/cloudflare-worker/src/index.js
