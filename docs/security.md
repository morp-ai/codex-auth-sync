# Security Model

## Secrets

Two independent secrets are used:

1. The API token grants access to the event service.
2. The 32-byte sync key encrypts payloads and derives opaque entity identifiers.

The sync key never needs to be configured on the server. Account payloads use AES-256-GCM with a fresh random nonce and authenticated event metadata.

## Server Visibility

The server can observe event time, event kind, cluster ID, node ID, opaque entity ID, and ciphertext size. It cannot read email addresses, account IDs, OAuth tokens, API keys, aliases, usage values, or the selected account.

## Local Storage

On Unix-like systems, the agent creates its configuration and state files with mode `0600`, the configuration directory with `0700`, and preserves the private `codex-auth` account directory and snapshot permissions.

Windows does not implement POSIX mode bits. Protect the user profile and avoid placing the configuration in a shared directory.

## Recommended Deployment

- Prefer Tailscale or WireGuard plus the built-in server for the smallest trusted surface.
- If the server is public, use HTTPS and a long randomly generated API token.
- Keep the Cloudflare Worker token in a Worker secret, not in source control.
- Rotate both secrets if any client machine is lost or compromised.
- Use a separate cluster and key for unrelated teams.

## Threats Not Solved

- A compromised synchronized client can decrypt all accounts in its cluster.
- A malicious service can delete, reorder, or withhold ciphertext events. Authenticated encryption prevents silent payload modification, but not denial of service.
- Synchronizing the same OAuth refresh token to many concurrently active machines may still be constrained by upstream token rotation behavior.
- The agent cannot force an already running Codex process to reload `auth.json`.
