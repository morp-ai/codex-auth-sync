# Architecture

## Components

Each machine runs one `codex-auth-sync daemon`. The daemon reads the local `codex-auth` state, produces encrypted events for meaningful changes, pulls newer remote events, and applies them using local backups and atomic writes.

The synchronization backend is intentionally untrusted. The built-in and Cloudflare services assign monotonically increasing revisions. WebDAV clients assign revisions while holding an ETag compare-and-swap update. All backends store ciphertext and cannot decrypt an account snapshot.

## Event Types

- `account_upsert`: account identity record plus its encrypted auth snapshot.
- `account_delete`: a tombstone preventing an old node from silently restoring a deleted account.
- `alias_set`: alias-only update, separated from auth refreshes.
- `account_status`: optional cached usage state.
- `active_set`: the desired active account for a group or the global scope.
- `settings_set`: synchronized registry settings such as the live refresh interval.

The account key is not sent to the server as plaintext. The visible entity identifier is an HMAC-SHA-256 value derived from the cluster sync key.

## Local Reconciliation

The agent polls local state at a configurable interval. Polling is used instead of platform-specific filesystem notification APIs so the same binary behavior works on Linux, macOS, and Windows, including atomic rename events.

Before writing `registry.json`, the agent verifies that the file has not changed since it was read. If `codex-auth` modifies it concurrently, the agent reloads, merges, and retries. Sensitive files use same-directory temporary files, `fsync`, atomic replacement where available, and private Unix modes.

## Registry Schema Adapters

The agent preserves the registry schema already used on each machine:

- schema v2 keeps the legacy `version`, `active_email`, email-keyed snapshot filenames, and legacy account record layout;
- schema v3 keeps `schema_version: 3`, `auto_switch`, `api`, and the v3 active-account fields;
- schema v4 keeps `previous_active_account_key`, `interval_seconds`, and the v4 layout.

For schema v2 ChatGPT accounts, the canonical cross-machine account key is derived from the authenticated `id_token` claims and `tokens.account_id`, while adapter-only identity fields are removed again before the v2 registry is written. Records whose legacy snapshot cannot provide a verified identity remain untouched locally and are not published.

Only settings understood by the local schema are applied. In particular, the live refresh interval is synchronized among schema v4 nodes and ignored by v2/v3 nodes. Unknown newer schemas are rejected before any write.

## Conflict Rules

- Server revisions define event order.
- Duplicate event IDs are idempotent.
- Accounts from different nodes are merged by `account_key`.
- Deletions are explicit events.
- Alias changes are independent from auth snapshot changes.
- A snapshot with an older `last_refresh` or JWT expiry cannot replace a newer local snapshot.
- Usage cache updates apply only when enabled and newer than the local cached timestamp.
- The most recent `active_set` event for the configured group wins.

## Bootstrap

A new node first replays all remote events, then publishes any local-only accounts. This produces a union without treating missing first-run accounts as deletions. After initialization, removing a previously observed account creates a deletion event.

## Files Not Synchronized

The agent does not upload Codex sessions, rollout logs, backups, temporary files, or lock files. These remain local to avoid privacy leaks, unnecessary storage, and cross-machine conflicts.
