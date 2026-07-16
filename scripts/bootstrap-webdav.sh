#!/usr/bin/env sh
set -eu

REPOSITORY=${CODEX_AUTH_SYNC_REPOSITORY:-morp-ai/codex-auth-sync}
SERVER_URL=${CODEX_AUTH_SYNC_SERVER_URL:-https://app.koofr.net/dav/Koofr}
WEBDAV_USERNAME=${CODEX_AUTH_SYNC_WEBDAV_USERNAME:-}
WEBDAV_PASSWORD=${CODEX_AUTH_SYNC_WEBDAV_PASSWORD:-}
SYNC_KEY=${CODEX_AUTH_SYNC_KEY:-}
CLUSTER_ID=${CODEX_AUTH_SYNC_CLUSTER_ID:-personal}
ACTIVE_SCOPE=${CODEX_AUTH_SYNC_ACTIVE_SCOPE:-group}
ACTIVE_GROUP=${CODEX_AUTH_SYNC_ACTIVE_GROUP:-main}
SCAN_INTERVAL=${CODEX_AUTH_SYNC_SCAN_INTERVAL:-5}
ROLE=${CODEX_AUTH_SYNC_ROLE:-auto}
REPLACEMENT_CLUSTER=${CODEX_AUTH_SYNC_REPLACEMENT_CLUSTER:-}
INSTALL_CODEX=1
INSTALL_SERVICE=1
RECONFIGURE=0

usage() {
  cat <<'EOF'
Install or update codex-auth-sync and configure WebDAV synchronization.

Usage:
  bootstrap-webdav.sh [options]

An existing installation is updated without changing its configuration.
If no configuration exists, setup starts.

Options:
  --server URL          WebDAV base URL
  --username USER       WebDAV username
  --password PASSWORD   WebDAV password or app password (visible in shell history)
  --sync-key KEY        Shared base64 synchronization key
  --cluster ID          Shared cluster ID (default: personal)
  --active-scope SCOPE  local, group, or global (default: group)
  --active-group GROUP  Shared active-account group (default: main)
  --interval SECONDS    Synchronization interval (default: 5)
  --role ROLE           auto, seed, join, merge, or local (default: auto)
  --replacement-cluster ID
                        New empty cluster used by local-authoritative mode
  --skip-codex-install  Do not install Node.js, Codex CLI, or codex-auth
  --no-service          Do not install a background service (useful in containers)
  --reconfigure         Apply configuration options even when a config already exists
  --help                Show this help

Secrets can be supplied through CODEX_AUTH_SYNC_WEBDAV_PASSWORD and
CODEX_AUTH_SYNC_KEY. Otherwise they are read from the controlling terminal.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) SERVER_URL=$2; shift 2 ;;
    --username) WEBDAV_USERNAME=$2; shift 2 ;;
    --password) WEBDAV_PASSWORD=$2; shift 2 ;;
    --sync-key) SYNC_KEY=$2; shift 2 ;;
    --cluster) CLUSTER_ID=$2; shift 2 ;;
    --active-scope) ACTIVE_SCOPE=$2; shift 2 ;;
    --active-group) ACTIVE_GROUP=$2; shift 2 ;;
    --interval) SCAN_INTERVAL=$2; shift 2 ;;
    --role) ROLE=$2; shift 2 ;;
    --replacement-cluster) REPLACEMENT_CLUSTER=$2; shift 2 ;;
    --skip-codex-install) INSTALL_CODEX=0; shift ;;
    --no-service) INSTALL_SERVICE=0; shift ;;
    --reconfigure) RECONFIGURE=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$ROLE" in
  auto|seed|join|merge|local) ;;
  *) echo "Role must be auto, seed, join, merge, or local." >&2; exit 2 ;;
esac
case "$ACTIVE_SCOPE" in
  local|group|global) ;;
  *) echo "Active scope must be local, group, or global." >&2; exit 2 ;;
esac
case "$SCAN_INTERVAL" in
  ''|*[!0-9]*) echo "Interval must be an integer." >&2; exit 2 ;;
esac
if [ "$SCAN_INTERVAL" -lt 1 ] || [ "$SCAN_INTERVAL" -gt 3600 ]; then
  echo "Interval must be between 1 and 3600 seconds." >&2
  exit 2
fi

if [ -t 0 ]; then
  TTY_DEVICE=/dev/stdin
elif [ -r /dev/tty ]; then
  TTY_DEVICE=/dev/tty
else
  TTY_DEVICE=
fi

prompt_value() {
  prompt=$1
  if [ -z "$TTY_DEVICE" ]; then
    echo "$prompt must be supplied through an option or environment variable." >&2
    exit 2
  fi
  printf '%s: ' "$prompt" > "$TTY_DEVICE"
  IFS= read -r answer < "$TTY_DEVICE"
  printf '%s' "$answer"
}

prompt_secret() {
  prompt=$1
  if [ -z "$TTY_DEVICE" ]; then
    echo "$prompt must be supplied through an environment variable." >&2
    exit 2
  fi
  printf '%s: ' "$prompt" > "$TTY_DEVICE"
  stty -echo < "$TTY_DEVICE"
  IFS= read -r answer < "$TTY_DEVICE" || {
    stty echo < "$TTY_DEVICE"
    exit 1
  }
  stty echo < "$TTY_DEVICE"
  printf '\n' > "$TTY_DEVICE"
  printf '%s' "$answer"
}

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required." >&2
  exit 1
fi

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "Administrator privileges are required to install Node.js." >&2
    exit 1
  fi
}

install_node() {
  if command -v npm >/dev/null 2>&1; then
    return
  fi
  echo "Node.js and npm were not found; installing them with the system package manager."
  case "$(uname -s)" in
    Darwin)
      if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew is required to install Node.js automatically on macOS." >&2
        echo "Install Homebrew from https://brew.sh and run this script again." >&2
        exit 1
      fi
      brew install node
      ;;
    Linux)
      if command -v apt-get >/dev/null 2>&1; then
        run_as_root apt-get update
        run_as_root apt-get install -y nodejs npm
      elif command -v dnf >/dev/null 2>&1; then
        run_as_root dnf install -y nodejs npm
      elif command -v yum >/dev/null 2>&1; then
        run_as_root yum install -y nodejs npm
      elif command -v zypper >/dev/null 2>&1; then
        run_as_root zypper --non-interactive install nodejs npm
      elif command -v pacman >/dev/null 2>&1; then
        run_as_root pacman -Sy --noconfirm --needed nodejs npm
      elif command -v apk >/dev/null 2>&1; then
        run_as_root apk add nodejs npm
      else
        echo "No supported package manager was found. Install Node.js LTS and npm, then run this script again." >&2
        exit 1
      fi
      ;;
    *)
      echo "This script supports Linux and macOS. Use bootstrap-webdav.ps1 on Windows." >&2
      exit 1
      ;;
  esac
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is still unavailable after installing Node.js." >&2
    exit 1
  fi
}

install_codex_tools() {
  packages=
  if command -v codex >/dev/null 2>&1; then
    echo "Existing OpenAI Codex CLI installation detected; keeping it."
  else
    packages="$packages @openai/codex"
  fi
  if command -v codex-auth >/dev/null 2>&1; then
    echo "Existing codex-auth installation detected; keeping it."
  else
    packages="$packages @loongphy/codex-auth"
  fi
  if [ -z "$packages" ]; then
    return
  fi
  install_node
  echo "Installing missing Codex tools:$packages"
  if [ "$(id -u)" -eq 0 ]; then
    # Word splitting is intentional because packages is an internal fixed package list.
    # shellcheck disable=SC2086
    npm install --global $packages
  else
    mkdir -p "$HOME/.local"
    # Word splitting is intentional because packages is an internal fixed package list.
    # shellcheck disable=SC2086
    npm install --global --prefix "$HOME/.local" $packages
  fi
}

case "$(uname -s)" in
  Darwin)
    CONFIG_DIR="$HOME/Library/Application Support/codex-auth-sync"
    ;;
  Linux)
    CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/codex-auth-sync"
    ;;
  *)
    echo "This script supports Linux and macOS. Use bootstrap-webdav.ps1 on Windows." >&2
    exit 1
    ;;
esac
CONFIG_PATH="$CONFIG_DIR/config.json"
STATE_PATH="$CONFIG_DIR/state.json"
CODEX_HOME_PATH=${CODEX_HOME:-"$HOME/.codex"}
BACKUP_ROOT="$HOME/.codex-auth-sync/backups"
if [ -n "${CODEX_AUTH_SYNC_INSTALL_DIR:-}" ]; then
  INSTALL_DIR=$CODEX_AUTH_SYNC_INSTALL_DIR
elif [ "$(id -u)" -eq 0 ]; then
  INSTALL_DIR=/usr/local/bin
else
  INSTALL_DIR="$HOME/.local/bin"
fi
SYNC_BINARY="$INSTALL_DIR/codex-auth-sync"

PATH="$INSTALL_DIR:$HOME/.local/bin:$PATH"
export PATH

temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
curl -fsSL "https://raw.githubusercontent.com/$REPOSITORY/main/scripts/install.sh" -o "$temp_dir/install.sh"
CODEX_AUTH_SYNC_INSTALL_DIR="$INSTALL_DIR" sh "$temp_dir/install.sh"
if [ ! -x "$SYNC_BINARY" ]; then
  SYNC_BINARY=$(command -v codex-auth-sync || true)
fi
if [ -z "$SYNC_BINARY" ] || [ ! -x "$SYNC_BINARY" ]; then
  echo "codex-auth-sync installation failed." >&2
  exit 1
fi

if [ "$RECONFIGURE" -eq 0 ] && [ -f "$CONFIG_PATH" ]; then
  curl -fsSL "https://raw.githubusercontent.com/$REPOSITORY/main/scripts/install-service.sh" -o "$temp_dir/install-service.sh"
  sh "$temp_dir/install-service.sh" "$SYNC_BINARY" "$CONFIG_PATH"
  "$SYNC_BINARY" version
  echo "Update complete. Existing configuration, synchronization state, codex-auth, and account data were preserved."
  echo "Run this script with '--reconfigure' only when configuration changes are intended."
  echo "Run 'codex-auth-sync now' to synchronize immediately."
  exit 0
fi

if [ -z "$WEBDAV_USERNAME" ]; then
  WEBDAV_USERNAME=$(prompt_value "WebDAV username")
fi
if [ -z "$WEBDAV_PASSWORD" ]; then
  WEBDAV_PASSWORD=$(prompt_secret "WebDAV app password")
fi
if [ -z "$WEBDAV_USERNAME" ] || [ -z "$WEBDAV_PASSWORD" ]; then
  echo "WebDAV username and app password are required." >&2
  exit 2
fi

if [ "$INSTALL_CODEX" -eq 1 ]; then
  install_codex_tools
fi

hash_cluster() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$CLUSTER_ID" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$CLUSTER_ID" | shasum -a 256 | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    printf '%s' "$CLUSTER_ID" | openssl dgst -sha256 | awk '{print $NF}'
  else
    echo "A SHA-256 utility is required." >&2
    exit 1
  fi
}

probe_remote() {
  cluster_hash=$(hash_cluster)
  state_url=$(printf '%s/%s%s.json' "${SERVER_URL%/}" 'codex-auth-sync-v1-' "$cluster_hash")
  netrc="$temp_dir/webdav.netrc"
  umask 077
  printf 'machine %s\nlogin %s\npassword %s\n' \
    "$(printf '%s' "$SERVER_URL" | sed -E 's#^[A-Za-z]+://([^/:]+).*#\1#')" \
    "$WEBDAV_USERNAME" "$WEBDAV_PASSWORD" > "$netrc"
  http_status=$(curl -sS --netrc-file "$netrc" -o /dev/null -w '%{http_code}' "$state_url" || true)
  rm -f "$netrc"
  case "$http_status" in
    2??) REMOTE_EXISTS=1 ;;
    404) REMOTE_EXISTS=0 ;;
    401|403) echo "WebDAV authentication failed with HTTP $http_status." >&2; exit 1 ;;
    *) echo "WebDAV state probe failed with HTTP ${http_status:-unknown}." >&2; exit 1 ;;
  esac
}

local_account_count() {
  if [ ! -d "$CODEX_HOME_PATH/accounts" ]; then
    printf '0'
    return
  fi
  find "$CODEX_HOME_PATH/accounts" -type f -name '*.auth.json' 2>/dev/null | wc -l | tr -d ' '
}

probe_remote
LOCAL_ACCOUNTS=$(local_account_count)

if [ "$ROLE" = "auto" ] && [ "$REMOTE_EXISTS" -eq 0 ] && [ "$LOCAL_ACCOUNTS" -eq 0 ] && [ "$INSTALL_CODEX" -eq 1 ]; then
  if ! command -v codex-auth >/dev/null 2>&1; then
    echo "codex-auth is unavailable after installation." >&2
    exit 1
  fi
  if [ -n "$TTY_DEVICE" ]; then
    start_login=$(prompt_value "No local accounts were found. Start 'codex-auth login' now? [Y/n]")
    case "$start_login" in
      ''|y|Y|yes|YES)
        codex-auth login
        LOCAL_ACCOUNTS=$(local_account_count)
        ;;
    esac
  fi
fi

if [ "$ROLE" = "auto" ]; then
  if [ "$REMOTE_EXISTS" -eq 0 ] && [ "$LOCAL_ACCOUNTS" -gt 0 ]; then
    ROLE='seed'
  elif [ "$REMOTE_EXISTS" -eq 1 ] && [ "$LOCAL_ACCOUNTS" -eq 0 ]; then
    ROLE='join'
  elif [ "$REMOTE_EXISTS" -eq 0 ]; then
    echo "The remote cluster is empty and no local codex-auth accounts were found." >&2
    echo "Run 'codex-auth login' on the first server, then run this script again." >&2
    exit 1
  elif [ -n "$TTY_DEVICE" ]; then
    echo "Both local accounts and remote synchronization data exist."
    echo "Choose 'merge' for a union, 'cloud' to use the cloud as authoritative, or 'local' to create a new local-authoritative cluster."
    selected=$(prompt_value "Deployment role [merge/cloud/local]")
    case "$selected" in
      ''|merge) ROLE='merge' ;;
      cloud|join) ROLE='join' ;;
      local) ROLE='local' ;;
      *) echo "Role must be merge, cloud, or local." >&2; exit 2 ;;
    esac
  else
    echo "Both local and remote data exist. Specify --role merge, --role join, or --role local." >&2
    exit 2
  fi
fi

if [ "$ROLE" = "local" ]; then
  if [ "$LOCAL_ACCOUNTS" -eq 0 ]; then
    echo "Local-authoritative mode requires at least one local codex-auth account." >&2
    exit 1
  fi
  if [ -z "$REPLACEMENT_CLUSTER" ]; then
    REPLACEMENT_CLUSTER=$(prompt_value "New replacement cluster ID")
  fi
  if [ -z "$REPLACEMENT_CLUSTER" ] || [ "$REPLACEMENT_CLUSTER" = "$CLUSTER_ID" ]; then
    echo "Local-authoritative mode requires a different, previously unused replacement cluster ID." >&2
    exit 2
  fi
  CLUSTER_ID=$REPLACEMENT_CLUSTER
  probe_remote
  if [ "$REMOTE_EXISTS" -eq 1 ]; then
    echo "The replacement cluster already contains data. Choose a new replacement cluster ID." >&2
    exit 1
  fi
  ROLE='seed'
  echo "Using new local-authoritative cluster: $CLUSTER_ID"
fi

if [ "$ROLE" = "seed" ]; then
  if [ "$REMOTE_EXISTS" -eq 1 ]; then
    echo "Seed mode requires an empty remote cluster. Choose a new cluster ID or use merge mode." >&2
    exit 1
  fi
  if [ "$LOCAL_ACCOUNTS" -eq 0 ]; then
    echo "Seed mode requires at least one local codex-auth account." >&2
    echo "Run 'codex-auth login' and then run this script again." >&2
    exit 1
  fi
elif [ "$ROLE" = "join" ] && [ "$REMOTE_EXISTS" -eq 0 ]; then
  echo "Join mode requires an existing remote cluster." >&2
  exit 1
fi

if [ -z "$SYNC_KEY" ]; then
  if [ "$ROLE" = "seed" ]; then
    key_json=$($SYNC_BINARY keygen --json)
    SYNC_KEY=$(printf '%s' "$key_json" | sed -n 's/.*"sync_key":"\([^"]*\)".*/\1/p')
    if [ -z "$SYNC_KEY" ]; then
      echo "Failed to generate a synchronization key." >&2
      exit 1
    fi
    echo "Generated synchronization key. Store it in a password manager; every server needs the same value:"
    printf '%s\n' "$SYNC_KEY"
  else
    SYNC_KEY=$(prompt_secret "Shared synchronization key")
  fi
fi

timestamp=$(date -u '+%Y%m%dT%H%M%SZ')
mkdir -p "$BACKUP_ROOT"
chmod 700 "$HOME/.codex-auth-sync" "$BACKUP_ROOT" 2>/dev/null || true

stop_existing_service() {
  case "$(uname -s)" in
    Linux)
      if command -v systemctl >/dev/null 2>&1; then
        systemctl --user stop codex-auth-sync.service >/dev/null 2>&1 || true
      fi
      ;;
    Darwin)
      launchctl bootout "gui/$(id -u)/io.codex-auth-sync.agent" >/dev/null 2>&1 || true
      ;;
  esac
}

backup_existing_sync_config() {
  if [ -e "$CONFIG_PATH" ] || [ -e "$STATE_PATH" ]; then
    destination="$BACKUP_ROOT/sync-config-$timestamp"
    mkdir -p "$destination"
    [ ! -e "$CONFIG_PATH" ] || cp -p "$CONFIG_PATH" "$destination/config.json"
    [ ! -e "$STATE_PATH" ] || cp -p "$STATE_PATH" "$destination/state.json"
    echo "Previous synchronization configuration backed up to $destination"
  fi
}

reset_local_for_join() {
  if [ -e "$CODEX_HOME_PATH/auth.json" ] || [ -e "$CODEX_HOME_PATH/accounts" ]; then
    destination="$BACKUP_ROOT/codex-auth-$timestamp"
    mkdir -p "$destination"
    [ ! -e "$CODEX_HOME_PATH/auth.json" ] || cp -p "$CODEX_HOME_PATH/auth.json" "$destination/auth.json"
    [ ! -e "$CODEX_HOME_PATH/accounts" ] || cp -Rp "$CODEX_HOME_PATH/accounts" "$destination/accounts"
    rm -f "$CODEX_HOME_PATH/auth.json"
    rm -rf "$CODEX_HOME_PATH/accounts"
    echo "Local codex-auth data backed up to $destination"
  fi
}

stop_existing_service
backup_existing_sync_config
if [ "$ROLE" = "join" ]; then
  reset_local_for_join
fi
rm -f "$STATE_PATH"

export CODEX_AUTH_SYNC_WEBDAV_PASSWORD="$WEBDAV_PASSWORD"
export CODEX_AUTH_SYNC_KEY="$SYNC_KEY"
"$SYNC_BINARY" init \
  --force \
  --backend webdav \
  --server "$SERVER_URL" \
  --webdav-username "$WEBDAV_USERNAME" \
  --cluster "$CLUSTER_ID" \
  --active-scope "$ACTIVE_SCOPE" \
  --active-group "$ACTIVE_GROUP" \
  --interval "$SCAN_INTERVAL"
unset CODEX_AUTH_SYNC_WEBDAV_PASSWORD CODEX_AUTH_SYNC_KEY

"$SYNC_BINARY" sync
"$SYNC_BINARY" status

if [ "$INSTALL_SERVICE" -eq 1 ]; then
  curl -fsSL "https://raw.githubusercontent.com/$REPOSITORY/main/scripts/install-service.sh" -o "$temp_dir/install-service.sh"
  sh "$temp_dir/install-service.sh" "$SYNC_BINARY" "$CONFIG_PATH"
else
  echo "Background service installation skipped."
  echo "Run 'codex-auth-sync now' whenever an immediate synchronization is needed."
fi

if [ "$(uname -s)" = "Linux" ] && command -v loginctl >/dev/null 2>&1; then
  current_user=$(id -un)
  linger=$(loginctl show-user "$current_user" -p Linger --value 2>/dev/null || true)
  if [ "$linger" != "yes" ]; then
    if [ "$(id -u)" -eq 0 ]; then
      loginctl enable-linger "$current_user" >/dev/null 2>&1 || true
    elif command -v sudo >/dev/null 2>&1; then
      sudo loginctl enable-linger "$current_user" >/dev/null 2>&1 || true
    fi
    linger=$(loginctl show-user "$current_user" -p Linger --value 2>/dev/null || true)
    if [ "$linger" != "yes" ]; then
      echo "Warning: systemd user lingering is not enabled; the service may stop after logout." >&2
      echo "An administrator can run: loginctl enable-linger $current_user" >&2
    fi
  fi
fi

echo "Bootstrap complete."
echo "Role: $ROLE"
echo "Cluster: $CLUSTER_ID"
echo "Run 'codex-auth list' to view synchronized accounts."
echo "Run 'codex-auth-sync now' to synchronize immediately."
echo "Run 'codex-auth-sync status' to verify synchronization health."
