#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DIST=${DIST:-"$ROOT/dist"}
VERSION=${VERSION:-"0.1.0"}

mkdir -p "$DIST"
rm -f "$DIST"/codex-auth-sync-* "$DIST"/checksums.txt

build() {
  os=$1
  arch=$2
  suffix=$3
  output="$DIST/codex-auth-sync-$os-$arch$suffix"
  echo "Building $os/$arch"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$output" ./cmd/codex-auth-sync
}

cd "$ROOT"
build linux amd64 ""
build linux arm64 ""
build darwin amd64 ""
build darwin arm64 ""
build windows amd64 ".exe"
build windows arm64 ".exe"

cd "$DIST"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum codex-auth-sync-* > checksums.txt
else
  shasum -a 256 codex-auth-sync-* > checksums.txt
fi
echo "Release artifacts written to $DIST"
