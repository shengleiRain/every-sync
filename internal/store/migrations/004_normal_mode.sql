-- 004_normal_mode.sql
-- Refactor: merge mirror/selective → normal, add hash/conflict fields

-- sync_pairs: new fields
ALTER TABLE sync_pairs ADD COLUMN selected_folders TEXT DEFAULT '[]';
ALTER TABLE sync_pairs ADD COLUMN scan_interval INTEGER DEFAULT 300;

-- file_entries: hash and directory tracking
ALTER TABLE file_entries ADD COLUMN remote_etag TEXT DEFAULT '';
ALTER TABLE file_entries ADD COLUMN is_dir INTEGER DEFAULT 0;

-- conflicts: conflict type and hash
ALTER TABLE conflicts ADD COLUMN conflict_type TEXT DEFAULT 'modify_modify';
ALTER TABLE conflicts ADD COLUMN local_hash TEXT DEFAULT '';
ALTER TABLE conflicts ADD COLUMN remote_hash TEXT DEFAULT '';
