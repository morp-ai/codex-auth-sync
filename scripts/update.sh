#!/usr/bin/env sh
set -eu

REPOSITORY=${CODEX_AUTH_SYNC_REPOSITORY:-morp-ai/codex-auth-sync}
VERSION=${CODEX_AUTH_SYNC_VERSION:-latest}
BINARY=${CODEX_AUTH_SYNC_BINARY:-"$(command -v codex-auth-sync || true)"}

if [ -z "$BINARY" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    BINARY=/usr/local/bin/codex-auth-sync
  else
    BINARY="$HOME/.local/bin/codex-auth-sync"
  fi
fi
binary_dir=$(dirname "$BINARY")
if [ -d "$binary_dir" ]; then
  INSTALL_DIR=$(CDPATH='' cd -- "$binary_dir" && pwd)
else
  INSTALL_DIR=$binary_dir
fi

service=none
case "$(uname -s)" in
  Linux)
    unit="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/codex-auth-sync.service"
    if [ -f "$unit" ] && command -v systemctl >/dev/null 2>&1 && \
      systemctl --user show-environment >/dev/null 2>&1; then
      service=systemd
      systemctl --user stop codex-auth-sync.service >/dev/null 2>&1 || true
    fi
    ;;
  Darwin)
    if [ -f "$HOME/Library/LaunchAgents/io.codex-auth-sync.agent.plist" ]; then
      service=launchd
      launchctl bootout "gui/$(id -u)/io.codex-auth-sync.agent" >/dev/null 2>&1 || true
    fi
    ;;
  *)
    echo "This updater supports Linux and macOS. Use update.ps1 on Windows." >&2
    exit 1
    ;;
esac

temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
curl -fsSL "https://raw.githubusercontent.com/$REPOSITORY/main/scripts/install.sh" -o "$temp_dir/install.sh"
CODEX_AUTH_SYNC_REPOSITORY="$REPOSITORY" \
CODEX_AUTH_SYNC_VERSION="$VERSION" \
CODEX_AUTH_SYNC_INSTALL_DIR="$INSTALL_DIR" \
  sh "$temp_dir/install.sh"

case "$service" in
  systemd) systemctl --user start codex-auth-sync.service ;;
  launchd) launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/io.codex-auth-sync.agent.plist" ;;
  none)
    echo "No managed background service was detected. Restart any manually managed daemon to use the new binary."
    ;;
esac

"$BINARY" version
echo "Update complete. codex-auth and its account data were not modified."
