#!/usr/bin/env sh
set -eu

REPOSITORY_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

mkdir -p "$TEST_ROOT/bin" "$TEST_ROOT/fixtures"

cat > "$TEST_ROOT/fixtures/codex-auth-sync" <<'EOF'
#!/usr/bin/env sh
set -eu
case "${1:-}" in
  keygen)
    printf '%s\n' '{"api_token":"unused","sync_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}'
    ;;
  init)
    directory=${XDG_CONFIG_HOME:-"$HOME/.config"}/codex-auth-sync
    mkdir -p "$directory"
    printf '%s\n' '{"version":1}' > "$directory/config.json"
    printf '%s\n' 'Configuration written.'
    ;;
  sync)
    printf '%s\n' 'Synchronization complete: pulled=0 pushed=3'
    ;;
  status)
    printf '%s\n' 'Server: mock (online)'
    ;;
  *)
    printf 'Unexpected fake command: %s\n' "${1:-}" >&2
    exit 1
    ;;
esac
EOF

cat > "$TEST_ROOT/fixtures/install.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p "$CODEX_AUTH_SYNC_INSTALL_DIR"
cp "$TEST_FIXTURE/codex-auth-sync" "$CODEX_AUTH_SYNC_INSTALL_DIR/codex-auth-sync"
chmod 755 "$CODEX_AUTH_SYNC_INSTALL_DIR/codex-auth-sync"
EOF

cat > "$TEST_ROOT/fixtures/install-service.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
touch "$HOME/service-installed"
EOF

cat > "$TEST_ROOT/bin/curl" <<'EOF'
#!/usr/bin/env sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    -w|--netrc-file) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
case "$url" in
  */scripts/install.sh) cp "$TEST_FIXTURE/install.sh" "$output" ;;
  */scripts/install-service.sh) cp "$TEST_FIXTURE/install-service.sh" "$output" ;;
  */codex-auth-sync-v1-*.json) printf '%s' "$TEST_REMOTE_STATUS" ;;
  *) printf 'Unexpected curl URL: %s\n' "$url" >&2; exit 1 ;;
esac
EOF

chmod 755 "$TEST_ROOT/fixtures/codex-auth-sync" "$TEST_ROOT/bin/curl"

run_bootstrap() {
  home=$1
  remote_status=$2
  role=$3
  cluster=$4
  HOME="$home" \
  XDG_CONFIG_HOME="$home/.config" \
  PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
  TEST_FIXTURE="$TEST_ROOT/fixtures" \
  TEST_REMOTE_STATUS="$remote_status" \
  CODEX_AUTH_SYNC_WEBDAV_PASSWORD=test-password \
  CODEX_AUTH_SYNC_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA \
  CODEX_AUTH_SYNC_INSTALL_DIR="$home/.local/bin" \
  sh "$REPOSITORY_ROOT/scripts/bootstrap-webdav.sh" \
    --skip-codex-install \
    --server https://webdav.example.test/root \
    --username test@example.com \
    --cluster "$cluster" \
    --role "$role"
}

seed_home="$TEST_ROOT/seed-home"
mkdir -p "$seed_home/.codex/accounts"
printf '%s\n' '{"tokens":{"access_token":"test"}}' > "$seed_home/.codex/accounts/seed.auth.json"
run_bootstrap "$seed_home" 404 seed test-seed
test -f "$seed_home/.codex/accounts/seed.auth.json"
test -f "$seed_home/.config/codex-auth-sync/config.json"
test -f "$seed_home/service-installed"

join_home="$TEST_ROOT/join-home"
mkdir -p "$join_home/.codex/accounts" "$join_home/.config/codex-auth-sync"
printf '%s\n' '{"tokens":{"access_token":"old"}}' > "$join_home/.codex/auth.json"
printf '%s\n' '{"tokens":{"access_token":"old"}}' > "$join_home/.codex/accounts/local.auth.json"
printf '%s\n' '{"schema_version":4,"accounts":[]}' > "$join_home/.codex/accounts/registry.json"
printf '%s\n' '{"old":true}' > "$join_home/.config/codex-auth-sync/config.json"
printf '%s\n' '{"old":true}' > "$join_home/.config/codex-auth-sync/state.json"
run_bootstrap "$join_home" 200 join test-join
test ! -e "$join_home/.codex/auth.json"
test ! -e "$join_home/.codex/accounts"
test -f "$join_home/.config/codex-auth-sync/config.json"
test -f "$join_home/service-installed"
test "$(find "$join_home/.codex-auth-sync/backups" -name local.auth.json | wc -l | tr -d ' ')" = 1
test "$(find "$join_home/.codex-auth-sync/backups" -name state.json | wc -l | tr -d ' ')" = 1

local_home="$TEST_ROOT/local-home"
mkdir -p "$local_home/.codex/accounts"
printf '%s\n' '{"tokens":{"access_token":"authoritative"}}' > "$local_home/.codex/accounts/local.auth.json"
HOME="$local_home" \
XDG_CONFIG_HOME="$local_home/.config" \
PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
TEST_FIXTURE="$TEST_ROOT/fixtures" \
TEST_REMOTE_STATUS=404 \
CODEX_AUTH_SYNC_WEBDAV_PASSWORD=test-password \
CODEX_AUTH_SYNC_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA \
CODEX_AUTH_SYNC_INSTALL_DIR="$local_home/.local/bin" \
sh "$REPOSITORY_ROOT/scripts/bootstrap-webdav.sh" \
  --skip-codex-install \
  --server https://webdav.example.test/root \
  --username test@example.com \
  --cluster old-cluster \
  --role local \
  --replacement-cluster replacement-cluster
test -f "$local_home/.codex/accounts/local.auth.json"
test -f "$local_home/.config/codex-auth-sync/config.json"
test -f "$local_home/service-installed"

echo "WebDAV bootstrap seed, join, and local-authoritative tests passed."
