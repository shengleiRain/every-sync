-- Migration 001: Initial schema

CREATE TABLE IF NOT EXISTS file_entries (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    path         TEXT NOT NULL,
    sync_pair_id INTEGER NOT NULL,
    local_hash   TEXT DEFAULT '',
    remote_hash  TEXT DEFAULT '',
    local_mtime  DATETIME,
    remote_mtime DATETIME,
    local_size   INTEGER DEFAULT 0,
    remote_size  INTEGER DEFAULT 0,
    sync_state   TEXT DEFAULT 'pending',
    version      INTEGER DEFAULT 1,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(path, sync_pair_id)
);

CREATE INDEX IF NOT EXISTS idx_file_entries_sync_pair ON file_entries(sync_pair_id);
CREATE INDEX IF NOT EXISTS idx_file_entries_state ON file_entries(sync_state);
CREATE INDEX IF NOT EXISTS idx_file_entries_hash ON file_entries(local_hash);

CREATE TABLE IF NOT EXISTS sync_pairs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    local_path  TEXT NOT NULL,
    remote_path TEXT NOT NULL,
    provider    TEXT DEFAULT 'webdav',
    mode        TEXT DEFAULT 'mirror',
    direction   TEXT DEFAULT 'both',
    enabled     INTEGER DEFAULT 1,
    schedule    TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS change_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id    INTEGER REFERENCES file_entries(id) ON DELETE CASCADE,
    source     TEXT NOT NULL,
    event_type TEXT NOT NULL,
    path       TEXT NOT NULL,
    timestamp  DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed  INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_change_events_processed ON change_events(processed);
CREATE INDEX IF NOT EXISTS idx_change_events_file ON change_events(file_id);
