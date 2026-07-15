#!/usr/bin/env sh
set -eu

REPOSITORY=${CODEX_AUTH_SYNC_REPOSITORY:-morp-ai/codex-auth-sync}
VERSION=${CODEX_AUTH_SYNC_VERSION:-latest}
INSTALL_DIR=${CODEX_AUTH_SYNC_INSTALL_DIR:-"$HOME/.local/bin"}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *)
    echo "Unsupported operating system. Use install.ps1 on Windows." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)
    echo "Unsupported CPU architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

asset="codex-auth-sync-$os-$arch"
if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/$REPOSITORY/releases/latest/download"
else
  base_url="https://github.com/$REPOSITORY/releases/download/$VERSION"
fi

temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM

echo "Downloading $asset"
curl -fsSL "$base_url/$asset" -o "$temp_dir/$asset"
curl -fsSL "$base_url/checksums.txt" -o "$temp_dir/checksums.txt"

expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$temp_dir/checksums.txt")
if [ -z "$expected" ]; then
  echo "The release checksum file does not contain $asset." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temp_dir/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$temp_dir/$asset" | awk '{print $1}')
fi
if [ "$actual" != "$expected" ]; then
  echo "Checksum verification failed for $asset." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "$temp_dir/$asset" "$INSTALL_DIR/codex-auth-sync"
echo "Installed codex-auth-sync to $INSTALL_DIR/codex-auth-sync"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add $INSTALL_DIR to PATH before running codex-auth-sync." ;;
esac
