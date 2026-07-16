#!/usr/bin/env sh
set -eu

BINARY=${1:-"$(command -v codex-auth-sync || true)"}
OS_NAME=$(uname -s)
if [ "$#" -ge 2 ]; then
  CONFIG=$2
elif [ "$OS_NAME" = "Darwin" ]; then
  CONFIG="$HOME/Library/Application Support/codex-auth-sync/config.json"
else
  CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/codex-auth-sync/config.json"
fi

if [ -z "$BINARY" ] || [ ! -x "$BINARY" ]; then
  echo "A valid codex-auth-sync binary path is required." >&2
  exit 1
fi

case "$OS_NAME" in
  Linux)
    if ! command -v systemctl >/dev/null 2>&1; then
      echo "No systemd user service manager was found; background service installation was skipped." >&2
      echo "In a container, run 'codex-auth-sync daemon --config $CONFIG' under the container entrypoint or a process supervisor." >&2
      echo "For manual synchronization, run 'codex-auth-sync now --config $CONFIG'." >&2
      exit 0
    fi
    UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
    UNIT="$UNIT_DIR/codex-auth-sync.service"
    mkdir -p "$UNIT_DIR"
    cat > "$UNIT" <<EOF
[Unit]
Description=Codex Auth Sync Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart="$BINARY" daemon --config "$CONFIG"
Restart=on-failure
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=default.target
EOF
    if ! systemctl --user daemon-reload >/dev/null 2>&1; then
      echo "The systemd user manager is unavailable; background service installation was skipped." >&2
      echo "In a container, run 'codex-auth-sync daemon --config $CONFIG' under the container entrypoint or a process supervisor." >&2
      echo "For manual synchronization, run 'codex-auth-sync now --config $CONFIG'." >&2
      exit 0
    fi
    if systemctl --user is-active --quiet codex-auth-sync.service; then
      systemctl --user restart codex-auth-sync.service
    else
      systemctl --user enable --now codex-auth-sync.service
    fi
    echo "Installed and started the systemd user service."
    ;;
  Darwin)
    PLIST_DIR="$HOME/Library/LaunchAgents"
    PLIST="$PLIST_DIR/io.codex-auth-sync.agent.plist"
    mkdir -p "$PLIST_DIR"
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>io.codex-auth-sync.agent</string>
  <key>ProgramArguments</key>
  <array><string>$BINARY</string><string>daemon</string><string>--config</string><string>$CONFIG</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$HOME/Library/Logs/codex-auth-sync.log</string>
  <key>StandardErrorPath</key><string>$HOME/Library/Logs/codex-auth-sync.error.log</string>
</dict>
</plist>
EOF
    launchctl bootout "gui/$(id -u)/io.codex-auth-sync.agent" >/dev/null 2>&1 || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST"
    echo "Installed and started the launchd user agent."
    ;;
  *)
    echo "This installer supports Linux and macOS. Use install-service.ps1 on Windows." >&2
    exit 1
    ;;
esac
