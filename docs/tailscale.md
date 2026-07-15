# Tailscale Deployment

Tailscale plus the built-in server is the simplest free deployment when all machines can join the same private tailnet.

## Server

Install and connect Tailscale, then find the private IPv4 address:

```sh
tailscale up
tailscale ip -4
```

Start the sync service on that address:

```sh
export CODEX_AUTH_SYNC_SERVER_TOKEN='<API token>'
codex-auth-sync serve \
  --listen '<tailscale-ip>:8787' \
  --data "$HOME/.config/codex-auth-sync/server-events.json"
```

Traffic remains inside the encrypted Tailscale network. Do not also expose this port through a public firewall rule.

## Clients

```sh
codex-auth-sync init \
  --server 'http://<tailscale-ip>:8787' \
  --api-token '<API token>' \
  --sync-key '<sync key>' \
  --cluster personal \
  --active-scope group \
  --active-group main
```

Run one manual synchronization, verify status, and then install the background service:

```sh
codex-auth-sync sync
codex-auth-sync status
./scripts/install-service.sh "$(command -v codex-auth-sync)"
```

On Linux servers that must keep a user service running after logout, an administrator may enable user lingering:

```sh
sudo loginctl enable-linger "$USER"
```
