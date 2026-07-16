#!/usr/bin/env sh
set -eu

REPOSITORY_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

preserve_home="$TEST_ROOT/preserve-home"
mkdir -p "$preserve_home/.local/bin" "$preserve_home/.config/codex-auth-sync" \
  "$preserve_home/.codex" "$preserve_home/.codex-auth-sync/backups"
touch "$preserve_home/.local/bin/codex-auth-sync"
touch "$preserve_home/.config/codex-auth-sync/config.json"
touch "$preserve_home/.codex/auth.json"
touch "$preserve_home/.codex-auth-sync/backups/backup.json"
HOME="$preserve_home" XDG_CONFIG_HOME="$preserve_home/.config" \
  sh "$REPOSITORY_ROOT/scripts/uninstall.sh" \
    --binary "$preserve_home/.local/bin/codex-auth-sync"
test ! -e "$preserve_home/.local/bin/codex-auth-sync"
test -e "$preserve_home/.config/codex-auth-sync/config.json"
test -e "$preserve_home/.codex-auth-sync/backups/backup.json"
test -e "$preserve_home/.codex/auth.json"

purge_home="$TEST_ROOT/purge-home"
mkdir -p "$purge_home/.local/bin" "$purge_home/.config/codex-auth-sync" \
  "$purge_home/.codex" "$purge_home/.codex-auth-sync/backups"
touch "$purge_home/.local/bin/codex-auth-sync"
touch "$purge_home/.config/codex-auth-sync/config.json"
touch "$purge_home/.codex/auth.json"
HOME="$purge_home" XDG_CONFIG_HOME="$purge_home/.config" \
  sh "$REPOSITORY_ROOT/scripts/uninstall.sh" \
    --binary "$purge_home/.local/bin/codex-auth-sync" --purge
test ! -e "$purge_home/.local/bin/codex-auth-sync"
test ! -e "$purge_home/.config/codex-auth-sync"
test ! -e "$purge_home/.codex-auth-sync"
test -e "$purge_home/.codex/auth.json"

echo "Lifecycle uninstall tests passed."
