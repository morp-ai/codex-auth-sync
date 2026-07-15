# WebDAV Backend

The WebDAV backend stores one encrypted append-only event document per synchronization cluster. It requires no custom server process or database.

## Compatibility Requirements

The provider must support:

- HTTPS
- authenticated `GET`, `PUT`, and `OPTIONS`
- strong `ETag` response headers
- conditional writes with `If-Match` and `If-None-Match`
- HTTP `412 Precondition Failed` or `409 Conflict` when a conditional write loses a race

The client refuses to use an existing WebDAV state object without a strong ETag. This prevents silent last-writer-wins data loss.

## Verified Service

Koofr was tested with its standard endpoint:

```text
https://app.koofr.net/dav/Koofr
```

Use the Koofr account email as the username and a generated app password as the WebDAV password.

## Configuration

Avoid placing a password directly in shell history:

```sh
export CODEX_AUTH_SYNC_WEBDAV_PASSWORD='<app password>'

codex-auth-sync init \
  --backend webdav \
  --server 'https://app.koofr.net/dav/Koofr' \
  --webdav-username 'you@example.com' \
  --sync-key '<shared 32-byte base64 key>' \
  --cluster personal \
  --active-scope group \
  --active-group main

unset CODEX_AUTH_SYNC_WEBDAV_PASSWORD
```

Bearer-authenticated WebDAV services can use `--webdav-bearer-token` or the `CODEX_AUTH_SYNC_WEBDAV_BEARER_TOKEN` environment variable instead.

For a repeatable installation of Codex CLI, `codex-auth`, the synchronization agent, and its background service, see the [multi-server WebDAV bootstrap tutorial](webdav-multi-server.zh-CN.md).

## Storage Layout

The cluster ID is SHA-256 hashed into a filename similar to:

```text
codex-auth-sync-v1-<cluster-hash>.json
```

The object contains encrypted events and non-secret synchronization metadata. Account keys, emails, aliases, tokens, and usage values remain inside authenticated ciphertext.

## Concurrency

Every upload performs this compare-and-swap sequence:

1. Download the current object and strong ETag.
2. Append an event and assign the next revision.
3. Upload with `If-Match`.
4. Retry from the newest object after a `409` or `412` conflict.
5. Download again and verify that the event is present.

This is sufficient for a small number of servers and normal login/switch activity. High-frequency or very large fleets should use the built-in hub or Cloudflare D1.

## Other Likely Compatible Providers

Nextcloud, ownCloud, Synology WebDAV Server, Koofr, and `rclone serve webdav` commonly provide the required methods. Provider behavior can differ by version and configuration, so always run `codex-auth-sync sync` and `status` once before enabling the daemon.
