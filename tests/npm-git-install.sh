#!/usr/bin/env sh
set -eu

REPOSITORY_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
TEST_VERSION=${CODEX_AUTH_SYNC_NPM_TEST_VERSION:-v0.1.5}
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

mkdir -p "$TEST_ROOT/home" "$TEST_ROOT/cache" "$TEST_ROOT/prefix"
HOME="$TEST_ROOT/home" \
npm_config_cache="$TEST_ROOT/cache" \
CODEX_AUTH_SYNC_VERSION="$TEST_VERSION" \
  npm install --global --prefix "$TEST_ROOT/prefix" "git+file://$REPOSITORY_ROOT"

CODEX_AUTH_SYNC_VERSION="$TEST_VERSION" "$TEST_ROOT/prefix/bin/codex-auth-sync" version
"$TEST_ROOT/prefix/bin/codex-auth-sync" help | grep -F 'now      Perform one synchronization cycle immediately' >/dev/null

HOME="$TEST_ROOT/home" npm_config_cache="$TEST_ROOT/cache" \
  npm uninstall --global --prefix "$TEST_ROOT/prefix" @morp-ai/codex-auth-sync
test ! -e "$TEST_ROOT/prefix/bin/codex-auth-sync"

echo "Git-based global npm install and uninstall tests passed."
