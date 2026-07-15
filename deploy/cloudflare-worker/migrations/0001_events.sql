CREATE TABLE IF NOT EXISTS sync_events (
  revision INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL UNIQUE,
  cluster_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  ciphertext TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS sync_events_cluster_revision
  ON sync_events (cluster_id, revision);
