# Backend Options

## Direct WebDAV

WebDAV is the easiest option when an account already has compatible cloud or NAS storage. The backend has been tested against Koofr and uses strong ETag conditional writes to serialize multiple writers. See [webdav.md](webdav.md).

## Built-In Server Over Tailscale

This is the recommended free option for a small set of personal servers. Run `codex-auth-sync serve` on one always-available machine and expose port 8787 only on the private Tailscale network. It provides immediate persistence in a private JSON event store and requires no external database.

## Cloudflare Workers + D1

This is the recommended managed free-tier option. It removes the always-on server requirement and uses the same HTTP protocol as the built-in server. Use a longer polling interval to remain conservative with request quotas.

## Docker or Any Small VPS

The included Docker image runs the built-in service. Any existing server, home lab, NAS, or free compute instance can host it behind HTTPS.

## Platforms Not Recommended for Direct File Sync

Dropbox, Google Drive, OneDrive, raw Syncthing, Git, and private Git repositories should not directly synchronize `~/.codex`. They do not understand registry merges, token freshness, active-account events, or tombstones. Syncthing is safe for transporting an encrypted staging directory, but the synchronization agent already provides the required transport and reconciliation layer.
