-- Migration 003: Phase 3 metadata

ALTER TABLE sync_pairs ADD COLUMN include_patterns TEXT DEFAULT '';
ALTER TABLE sync_pairs ADD COLUMN exclude_patterns TEXT DEFAULT '';
ALTER TABLE sync_pairs ADD COLUMN conflict_strategy TEXT DEFAULT 'latest_wins';

CREATE TABLE IF NOT EXISTS file_versions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_pair_id INTEGER NOT NULL REFERENCES sync_pairs(id) ON DELETE CASCADE,
    path         TEXT NOT NULL,
    source       TEXT NOT NULL,
    size         INTEGER DEFAULT 0,
    mod_time     DATETIME,
    hash         TEXT DEFAULT '',
    stored_path  TEXT DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_file_versions_pair_path ON file_versions(sync_pair_id, path);

CREATE TABLE IF NOT EXISTS conflicts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_pair_id  INTEGER NOT NULL REFERENCES sync_pairs(id) ON DELETE CASCADE,
    path          TEXT NOT NULL,
    local_mtime   DATETIME,
    remote_mtime  DATETIME,
    local_size    INTEGER DEFAULT 0,
    remote_size   INTEGER DEFAULT 0,
    status        TEXT DEFAULT 'open',
    strategy      TEXT DEFAULT 'manual',
    resolution    TEXT DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_conflicts_pair_status ON conflicts(sync_pair_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conflicts_open_path ON conflicts(sync_pair_id, path) WHERE status = 'open';

CREATE TABLE IF NOT EXISTS sync_stats (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    uploaded_bytes   INTEGER DEFAULT 0,
    downloaded_bytes INTEGER DEFAULT 0,
    deleted_files    INTEGER DEFAULT 0,
    virtual_files    INTEGER DEFAULT 0,
    materialized_files INTEGER DEFAULT 0,
    conflicts        INTEGER DEFAULT 0,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO sync_stats (id) VALUES (1);
