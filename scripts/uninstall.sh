#!/usr/bin/env sh
set -eu

PURGE=0
BINARY=${CODEX_AUTH_SYNC_BINARY:-"$(command -v codex-auth-sync || true)"}

usage() {
  cat <<'EOF'
Uninstall codex-auth-sync without modifying codex-auth account data.

Usage:
  uninstall.sh [--binary PATH] [--purge]

Options:
  --binary PATH  Installed codex-auth-sync binary path
  --purge        Also remove local sync configuration, state, logs, and backups
  --help         Show this help

Remote WebDAV data and ~/.codex are never removed.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary) BINARY=$2; shift 2 ;;
    --purge) PURGE=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ -z "$BINARY" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    BINARY=/usr/local/bin/codex-auth-sync
  else
    BINARY="$HOME/.local/bin/codex-auth-sync"
  fi
fi

case "$(uname -s)" in
  Linux)
    UNIT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/codex-auth-sync.service"
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --user disable --now codex-auth-sync.service >/dev/null 2>&1 || true
    fi
    rm -f "$UNIT"
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --user daemon-reload >/dev/null 2>&1 || true
    fi
    CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/codex-auth-sync"
    ;;
  Darwin)
    PLIST="$HOME/Library/LaunchAgents/io.codex-auth-sync.agent.plist"
    launchctl bootout "gui/$(id -u)/io.codex-auth-sync.agent" >/dev/null 2>&1 || true
    rm -f "$PLIST"
    CONFIG_DIR="$HOME/Library/Application Support/codex-auth-sync"
    ;;
  *)
    echo "This uninstaller supports Linux and macOS. Use uninstall.ps1 on Windows." >&2
    exit 1
    ;;
esac

rm -f "$BINARY"
echo "Removed the background service and $BINARY."

if [ "$PURGE" -eq 1 ]; then
  rm -rf "$CONFIG_DIR" "$HOME/.codex-auth-sync"
  if [ "$(uname -s)" = "Darwin" ]; then
    rm -f "$HOME/Library/Logs/codex-auth-sync.log" "$HOME/Library/Logs/codex-auth-sync.error.log"
  fi
  echo "Removed local synchronization configuration, state, logs, and backups."
else
  echo "Local synchronization configuration and backups were preserved."
fi

echo "codex-auth, ~/.codex, and remote WebDAV data were not modified."
