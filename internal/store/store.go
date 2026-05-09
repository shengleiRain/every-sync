package store

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	// Create migrations tracking table
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get applied versions
	applied := map[int]bool{}
	rows, err := s.db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	// Read and sort migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	type migration struct {
		version int
		content string
	}
	migrations := make([]migration, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{version: version, content: string(content)})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration tx: %w", err)
		}

		if _, err := tx.Exec(m.content); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}

	return nil
}

// FileEntry represents a file record in the database.
type FileEntry struct {
	ID          int64      `json:"id"`
	Path        string     `json:"path"`
	SyncPairID  int64      `json:"sync_pair_id"`
	LocalHash   string     `json:"local_hash"`
	RemoteHash  string     `json:"remote_hash"`
	LocalMTime  *time.Time `json:"local_mtime"`
	RemoteMTime *time.Time `json:"remote_mtime"`
	LocalSize   int64      `json:"local_size"`
	RemoteSize  int64      `json:"remote_size"`
	SyncState   string     `json:"sync_state"`
	Version     int        `json:"version"`
}

// SyncPair represents a sync pair configuration stored in the database.
type SyncPair struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	LocalPath        string    `json:"local_path"`
	RemotePath       string    `json:"remote_path"`
	Provider         string    `json:"provider"`
	Mode             string    `json:"mode"`
	Direction        string    `json:"direction"`
	Enabled          bool      `json:"enabled"`
	Schedule         string    `json:"schedule"`
	IncludePatterns  string    `json:"include_patterns"`
	ExcludePatterns  string    `json:"exclude_patterns"`
	ConflictStrategy string    `json:"conflict_strategy"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ChangeEvent represents a file change event in the database.
type ChangeEvent struct {
	ID        int64     `json:"id"`
	FileID    int64     `json:"file_id"`
	Source    string    `json:"source"`
	EventType string    `json:"event_type"`
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
	Processed bool      `json:"processed"`
}

// --- SyncPair CRUD ---

func (s *Store) CreateSyncPair(pair *SyncPair) error {
	if pair.ConflictStrategy == "" {
		pair.ConflictStrategy = "latest_wins"
	}
	result, err := s.db.Exec(
		`INSERT INTO sync_pairs (name, local_path, remote_path, provider, mode, direction, enabled, schedule, include_patterns, exclude_patterns, conflict_strategy)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pair.Name, pair.LocalPath, pair.RemotePath, pair.Provider, pair.Mode, pair.Direction, pair.Enabled, pair.Schedule,
		pair.IncludePatterns, pair.ExcludePatterns, pair.ConflictStrategy,
	)
	if err != nil {
		return fmt.Errorf("create sync pair: %w", err)
	}
	pair.ID, _ = result.LastInsertId()
	return nil
}

func (s *Store) GetSyncPair(id int64) (*SyncPair, error) {
	pair := &SyncPair{}
	err := s.db.QueryRow(
		`SELECT id, name, local_path, remote_path, provider, mode, direction, enabled, schedule, include_patterns, exclude_patterns, conflict_strategy, created_at, updated_at
		 FROM sync_pairs WHERE id = ?`, id,
	).Scan(&pair.ID, &pair.Name, &pair.LocalPath, &pair.RemotePath, &pair.Provider,
		&pair.Mode, &pair.Direction, &pair.Enabled, &pair.Schedule, &pair.IncludePatterns, &pair.ExcludePatterns,
		&pair.ConflictStrategy, &pair.CreatedAt, &pair.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return pair, nil
}

func (s *Store) GetSyncPairByName(name string) (*SyncPair, error) {
	pair := &SyncPair{}
	err := s.db.QueryRow(
		`SELECT id, name, local_path, remote_path, provider, mode, direction, enabled, schedule, include_patterns, exclude_patterns, conflict_strategy, created_at, updated_at
		 FROM sync_pairs WHERE name = ?`, name,
	).Scan(&pair.ID, &pair.Name, &pair.LocalPath, &pair.RemotePath, &pair.Provider,
		&pair.Mode, &pair.Direction, &pair.Enabled, &pair.Schedule, &pair.IncludePatterns, &pair.ExcludePatterns,
		&pair.ConflictStrategy, &pair.CreatedAt, &pair.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return pair, nil
}

func (s *Store) ListSyncPairs() ([]*SyncPair, error) {
	rows, err := s.db.Query(
		`SELECT id, name, local_path, remote_path, provider, mode, direction, enabled, schedule, include_patterns, exclude_patterns, conflict_strategy, created_at, updated_at
		 FROM sync_pairs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []*SyncPair
	for rows.Next() {
		pair := &SyncPair{}
		if err := rows.Scan(&pair.ID, &pair.Name, &pair.LocalPath, &pair.RemotePath, &pair.Provider,
			&pair.Mode, &pair.Direction, &pair.Enabled, &pair.Schedule, &pair.IncludePatterns, &pair.ExcludePatterns,
			&pair.ConflictStrategy, &pair.CreatedAt, &pair.UpdatedAt); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	return pairs, nil
}

func (s *Store) UpdateSyncPair(pair *SyncPair) error {
	if pair.ConflictStrategy == "" {
		pair.ConflictStrategy = "latest_wins"
	}
	_, err := s.db.Exec(
		`UPDATE sync_pairs SET name=?, local_path=?, remote_path=?, provider=?, mode=?, direction=?, enabled=?, schedule=?, include_patterns=?, exclude_patterns=?, conflict_strategy=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id = ?`,
		pair.Name, pair.LocalPath, pair.RemotePath, pair.Provider, pair.Mode, pair.Direction, pair.Enabled, pair.Schedule,
		pair.IncludePatterns, pair.ExcludePatterns, pair.ConflictStrategy, pair.ID)
	return err
}

func (s *Store) DeleteSyncPair(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	// Delete related file entries and change events
	tx.Exec("DELETE FROM change_events WHERE file_id IN (SELECT id FROM file_entries WHERE sync_pair_id = ?)", id)
	tx.Exec("DELETE FROM file_entries WHERE sync_pair_id = ?", id)
	tx.Exec("DELETE FROM sync_pairs WHERE id = ?", id)
	return tx.Commit()
}

// --- FileEntry CRUD ---

func (s *Store) UpsertFileEntry(entry *FileEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO file_entries (path, sync_pair_id, local_hash, remote_hash, local_mtime, remote_mtime, local_size, remote_size, sync_state, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		 ON CONFLICT(path, sync_pair_id) DO UPDATE SET
		   local_hash=excluded.local_hash,
		   remote_hash=excluded.remote_hash,
		   local_mtime=excluded.local_mtime,
		   remote_mtime=excluded.remote_mtime,
		   local_size=excluded.local_size,
		   remote_size=excluded.remote_size,
		   sync_state=excluded.sync_state,
		   version=version+1,
		   updated_at=CURRENT_TIMESTAMP`,
		entry.Path, entry.SyncPairID, entry.LocalHash, entry.RemoteHash,
		entry.LocalMTime, entry.RemoteMTime, entry.LocalSize, entry.RemoteSize, entry.SyncState,
	)
	return err
}

func (s *Store) GetFileEntry(pairID int64, path string) (*FileEntry, error) {
	entry := &FileEntry{}
	err := s.db.QueryRow(
		`SELECT id, path, sync_pair_id, local_hash, remote_hash, local_mtime, remote_mtime, local_size, remote_size, sync_state, version
		 FROM file_entries WHERE sync_pair_id = ? AND path = ?`, pairID, path,
	).Scan(&entry.ID, &entry.Path, &entry.SyncPairID, &entry.LocalHash, &entry.RemoteHash,
		&entry.LocalMTime, &entry.RemoteMTime, &entry.LocalSize, &entry.RemoteSize, &entry.SyncState, &entry.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *Store) ListFileEntriesByPair(pairID int64) ([]*FileEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, path, sync_pair_id, local_hash, remote_hash, local_mtime, remote_mtime, local_size, remote_size, sync_state, version
		 FROM file_entries WHERE sync_pair_id = ? ORDER BY path`, pairID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*FileEntry
	for rows.Next() {
		entry := &FileEntry{}
		if err := rows.Scan(&entry.ID, &entry.Path, &entry.SyncPairID, &entry.LocalHash, &entry.RemoteHash,
			&entry.LocalMTime, &entry.RemoteMTime, &entry.LocalSize, &entry.RemoteSize, &entry.SyncState, &entry.Version); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Store) ListFileEntriesByState(pairID int64, state string) ([]*FileEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, path, sync_pair_id, local_hash, remote_hash, local_mtime, remote_mtime, local_size, remote_size, sync_state, version
		 FROM file_entries WHERE sync_pair_id = ? AND sync_state = ? ORDER BY path`, pairID, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*FileEntry
	for rows.Next() {
		entry := &FileEntry{}
		if err := rows.Scan(&entry.ID, &entry.Path, &entry.SyncPairID, &entry.LocalHash, &entry.RemoteHash,
			&entry.LocalMTime, &entry.RemoteMTime, &entry.LocalSize, &entry.RemoteSize, &entry.SyncState, &entry.Version); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Store) UpdateFileEntryState(id int64, state string) error {
	_, err := s.db.Exec(
		`UPDATE file_entries SET sync_state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, state, id)
	return err
}

func (s *Store) DeleteFileEntry(id int64) error {
	_, err := s.db.Exec(`DELETE FROM file_entries WHERE id = ?`, id)
	return err
}

// --- ChangeEvents ---

func (s *Store) CreateChangeEvent(event *ChangeEvent) error {
	result, err := s.db.Exec(
		`INSERT INTO change_events (file_id, source, event_type, path) VALUES (?, ?, ?, ?)`,
		event.FileID, event.Source, event.EventType, event.Path)
	if err != nil {
		return err
	}
	event.ID, _ = result.LastInsertId()
	return nil
}

func (s *Store) ListUnprocessedEvents() ([]*ChangeEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, file_id, source, event_type, path, timestamp, processed
		 FROM change_events WHERE processed = 0 ORDER BY timestamp ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*ChangeEvent
	for rows.Next() {
		event := &ChangeEvent{}
		if err := rows.Scan(&event.ID, &event.FileID, &event.Source, &event.EventType, &event.Path, &event.Timestamp, &event.Processed); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) MarkEventProcessed(id int64) error {
	_, err := s.db.Exec(`UPDATE change_events SET processed = 1 WHERE id = ?`, id)
	return err
}

// FileVersion stores metadata for a previous local or remote file snapshot.
type FileVersion struct {
	ID         int64      `json:"id"`
	SyncPairID int64      `json:"sync_pair_id"`
	Path       string     `json:"path"`
	Source     string     `json:"source"`
	Size       int64      `json:"size"`
	ModTime    *time.Time `json:"mod_time,omitempty"`
	Hash       string     `json:"hash"`
	StoredPath string     `json:"stored_path"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Store) CreateFileVersion(version *FileVersion) error {
	result, err := s.db.Exec(
		`INSERT INTO file_versions (sync_pair_id, path, source, size, mod_time, hash, stored_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		version.SyncPairID, version.Path, version.Source, version.Size, version.ModTime, version.Hash, version.StoredPath,
	)
	if err != nil {
		return fmt.Errorf("create file version: %w", err)
	}
	version.ID, _ = result.LastInsertId()
	return nil
}

func (s *Store) ListFileVersions(pairID int64, filePath string) ([]*FileVersion, error) {
	query := `SELECT id, sync_pair_id, path, source, size, mod_time, hash, stored_path, created_at
		FROM file_versions WHERE sync_pair_id = ?`
	args := []interface{}{pairID}
	if filePath != "" {
		query += ` AND path = ?`
		args = append(args, filePath)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*FileVersion
	for rows.Next() {
		version := &FileVersion{}
		if err := rows.Scan(&version.ID, &version.SyncPairID, &version.Path, &version.Source, &version.Size,
			&version.ModTime, &version.Hash, &version.StoredPath, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

// ConflictRecord stores a manual or detected sync conflict.
type ConflictRecord struct {
	ID          int64      `json:"id"`
	SyncPairID  int64      `json:"sync_pair_id"`
	Path        string     `json:"path"`
	LocalMTime  *time.Time `json:"local_mtime,omitempty"`
	RemoteMTime *time.Time `json:"remote_mtime,omitempty"`
	LocalSize   int64      `json:"local_size"`
	RemoteSize  int64      `json:"remote_size"`
	Status      string     `json:"status"`
	Strategy    string     `json:"strategy"`
	Resolution  string     `json:"resolution"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *Store) UpsertOpenConflict(conflict *ConflictRecord) error {
	if conflict.Status == "" {
		conflict.Status = "open"
	}
	if conflict.Strategy == "" {
		conflict.Strategy = "manual"
	}
	result, err := s.db.Exec(
		`INSERT INTO conflicts (sync_pair_id, path, local_mtime, remote_mtime, local_size, remote_size, status, strategy, resolution)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(sync_pair_id, path) WHERE status = 'open' DO UPDATE SET
		   local_mtime=excluded.local_mtime,
		   remote_mtime=excluded.remote_mtime,
		   local_size=excluded.local_size,
		   remote_size=excluded.remote_size,
		   strategy=excluded.strategy,
		   updated_at=CURRENT_TIMESTAMP`,
		conflict.SyncPairID, conflict.Path, conflict.LocalMTime, conflict.RemoteMTime, conflict.LocalSize,
		conflict.RemoteSize, conflict.Status, conflict.Strategy, conflict.Resolution,
	)
	if err != nil {
		return fmt.Errorf("upsert conflict: %w", err)
	}
	conflict.ID, _ = result.LastInsertId()
	return nil
}

func (s *Store) GetConflict(id int64) (*ConflictRecord, error) {
	conflict := &ConflictRecord{}
	err := s.db.QueryRow(
		`SELECT id, sync_pair_id, path, local_mtime, remote_mtime, local_size, remote_size, status, strategy, resolution, created_at, updated_at
		 FROM conflicts WHERE id = ?`, id,
	).Scan(&conflict.ID, &conflict.SyncPairID, &conflict.Path, &conflict.LocalMTime, &conflict.RemoteMTime,
		&conflict.LocalSize, &conflict.RemoteSize, &conflict.Status, &conflict.Strategy, &conflict.Resolution,
		&conflict.CreatedAt, &conflict.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return conflict, nil
}

func (s *Store) ListConflicts(pairID int64, status string) ([]*ConflictRecord, error) {
	query := `SELECT id, sync_pair_id, path, local_mtime, remote_mtime, local_size, remote_size, status, strategy, resolution, created_at, updated_at
		FROM conflicts WHERE 1=1`
	var args []interface{}
	if pairID > 0 {
		query += ` AND sync_pair_id = ?`
		args = append(args, pairID)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, id DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []*ConflictRecord
	for rows.Next() {
		conflict := &ConflictRecord{}
		if err := rows.Scan(&conflict.ID, &conflict.SyncPairID, &conflict.Path, &conflict.LocalMTime, &conflict.RemoteMTime,
			&conflict.LocalSize, &conflict.RemoteSize, &conflict.Status, &conflict.Strategy, &conflict.Resolution,
			&conflict.CreatedAt, &conflict.UpdatedAt); err != nil {
			return nil, err
		}
		conflicts = append(conflicts, conflict)
	}
	return conflicts, nil
}

func (s *Store) ResolveConflict(id int64, resolution string) error {
	_, err := s.db.Exec(
		`UPDATE conflicts SET status = 'resolved', resolution = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		resolution, id,
	)
	return err
}

type SyncStats struct {
	UploadedBytes     int64     `json:"uploaded_bytes"`
	DownloadedBytes   int64     `json:"downloaded_bytes"`
	DeletedFiles      int64     `json:"deleted_files"`
	VirtualFiles      int64     `json:"virtual_files"`
	MaterializedFiles int64     `json:"materialized_files"`
	Conflicts         int64     `json:"conflicts"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (s *Store) AddSyncStats(uploadedBytes, downloadedBytes, deletedFiles, virtualFiles, materializedFiles, conflicts int64) error {
	_, err := s.db.Exec(
		`UPDATE sync_stats SET
		   uploaded_bytes = uploaded_bytes + ?,
		   downloaded_bytes = downloaded_bytes + ?,
		   deleted_files = deleted_files + ?,
		   virtual_files = virtual_files + ?,
		   materialized_files = materialized_files + ?,
		   conflicts = conflicts + ?,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = 1`,
		uploadedBytes, downloadedBytes, deletedFiles, virtualFiles, materializedFiles, conflicts,
	)
	return err
}

func (s *Store) GetSyncStats() (*SyncStats, error) {
	stats := &SyncStats{}
	err := s.db.QueryRow(
		`SELECT uploaded_bytes, downloaded_bytes, deleted_files, virtual_files, materialized_files, conflicts, updated_at
		 FROM sync_stats WHERE id = 1`,
	).Scan(&stats.UploadedBytes, &stats.DownloadedBytes, &stats.DeletedFiles, &stats.VirtualFiles,
		&stats.MaterializedFiles, &stats.Conflicts, &stats.UpdatedAt)
	if err == sql.ErrNoRows {
		return &SyncStats{}, nil
	}
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// --- Provider CRUD ---

// ProviderConfig represents a stored provider configuration.
type ProviderConfig struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Params    map[string]string `json:"params"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func (s *Store) CreateProviderConfig(pc *ProviderConfig) error {
	paramsJSON, err := json.Marshal(pc.Params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	result, err := s.db.Exec(
		`INSERT INTO providers (name, type, params) VALUES (?, ?, ?)`,
		pc.Name, pc.Type, string(paramsJSON))
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	pc.ID, _ = result.LastInsertId()
	return nil
}

func (s *Store) GetProviderConfig(id int64) (*ProviderConfig, error) {
	pc := &ProviderConfig{}
	var paramsJSON string
	err := s.db.QueryRow(
		`SELECT id, name, type, params, created_at, updated_at FROM providers WHERE id = ?`, id,
	).Scan(&pc.ID, &pc.Name, &pc.Type, &paramsJSON, &pc.CreatedAt, &pc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(paramsJSON), &pc.Params)
	return pc, nil
}

func (s *Store) GetProviderConfigByName(name string) (*ProviderConfig, error) {
	pc := &ProviderConfig{}
	var paramsJSON string
	err := s.db.QueryRow(
		`SELECT id, name, type, params, created_at, updated_at FROM providers WHERE name = ?`, name,
	).Scan(&pc.ID, &pc.Name, &pc.Type, &paramsJSON, &pc.CreatedAt, &pc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(paramsJSON), &pc.Params)
	return pc, nil
}

func (s *Store) GetProviderConfigByType(providerType string) (*ProviderConfig, error) {
	pc := &ProviderConfig{}
	var paramsJSON string
	err := s.db.QueryRow(
		`SELECT id, name, type, params, created_at, updated_at FROM providers WHERE type = ? LIMIT 1`, providerType,
	).Scan(&pc.ID, &pc.Name, &pc.Type, &paramsJSON, &pc.CreatedAt, &pc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(paramsJSON), &pc.Params)
	return pc, nil
}

func (s *Store) ListProviderConfigs() ([]*ProviderConfig, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, params, created_at, updated_at FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*ProviderConfig
	for rows.Next() {
		pc := &ProviderConfig{}
		var paramsJSON string
		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Type, &paramsJSON, &pc.CreatedAt, &pc.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(paramsJSON), &pc.Params)
		configs = append(configs, pc)
	}
	return configs, nil
}

func (s *Store) UpdateProviderConfig(pc *ProviderConfig) error {
	paramsJSON, err := json.Marshal(pc.Params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE providers SET name=?, type=?, params=?, updated_at=CURRENT_TIMESTAMP WHERE id = ?`,
		pc.Name, pc.Type, string(paramsJSON), pc.ID)
	return err
}

func (s *Store) DeleteProviderConfig(id int64) error {
	_, err := s.db.Exec(`DELETE FROM providers WHERE id = ?`, id)
	return err
}
