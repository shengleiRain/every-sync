package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_Migration(t *testing.T) {
	s := openTestStore(t)

	// Verify tables exist
	tables := []string{"file_entries", "sync_pairs", "change_events", "schema_migrations", "providers", "file_versions", "conflicts", "sync_stats"}
	for _, table := range tables {
		var count int
		err := s.DB().QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s not found", table)
		}
	}
}

func TestStore_SyncPairCRUD(t *testing.T) {
	s := openTestStore(t)

	// Create
	pair := &SyncPair{
		Name:       "test",
		LocalPath:  "/tmp/local",
		RemotePath: "/remote",
		Provider:   "webdav",
		Mode:       "mirror",
		Direction:  "both",
		Enabled:    true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pair.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Get by ID
	got, err := s.GetSyncPair(pair.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "test" {
		t.Fatalf("name: got %q, want %q", got.Name, "test")
	}

	// Get by name
	got2, err := s.GetSyncPairByName("test")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got2.ID != pair.ID {
		t.Fatal("ID mismatch")
	}

	// List
	pairs, err := s.ListSyncPairs()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}

	// Update
	got.Name = "updated"
	got.Provider = "alist"
	got.Direction = "up"
	if err := s.UpdateSyncPair(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := s.GetSyncPair(pair.ID)
	if updated.Name != "updated" {
		t.Fatalf("name: got %q, want %q", updated.Name, "updated")
	}
	if updated.Provider != "alist" {
		t.Fatalf("provider: got %q, want %q", updated.Provider, "alist")
	}
	if updated.Direction != "up" {
		t.Fatalf("direction: got %q, want %q", updated.Direction, "up")
	}

	// Delete
	if err := s.DeleteSyncPair(pair.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deleted, _ := s.GetSyncPair(pair.ID)
	if deleted != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestStore_SyncPairUniqueName(t *testing.T) {
	s := openTestStore(t)

	p1 := &SyncPair{Name: "dup", LocalPath: "/a", RemotePath: "/b", Direction: "both"}
	p2 := &SyncPair{Name: "dup", LocalPath: "/c", RemotePath: "/d", Direction: "both"}

	if err := s.CreateSyncPair(p1); err != nil {
		t.Fatalf("Create p1: %v", err)
	}
	if err := s.CreateSyncPair(p2); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestStore_FileEntryCRUD(t *testing.T) {
	s := openTestStore(t)

	pair := &SyncPair{Name: "files", LocalPath: "/l", RemotePath: "/r", Direction: "both"}
	s.CreateSyncPair(pair)

	now := time.Now()

	// Upsert (create)
	entry := &FileEntry{
		Path:        "/test.txt",
		SyncPairID:  pair.ID,
		LocalHash:   "abc123",
		RemoteHash:  "abc123",
		LocalMTime:  &now,
		RemoteMTime: &now,
		LocalSize:   100,
		RemoteSize:  100,
		SyncState:   "synced",
	}
	if err := s.UpsertFileEntry(entry); err != nil {
		t.Fatalf("Upsert create: %v", err)
	}

	// Get
	got, err := s.GetFileEntry(pair.ID, "/test.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LocalHash != "abc123" {
		t.Fatalf("hash: got %q, want %q", got.LocalHash, "abc123")
	}

	// Upsert (update)
	got.LocalHash = "def456"
	got.SyncState = "pending"
	if err := s.UpsertFileEntry(got); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	updated, _ := s.GetFileEntry(pair.ID, "/test.txt")
	if updated.LocalHash != "def456" {
		t.Fatalf("hash after update: got %q", updated.LocalHash)
	}
	if updated.Version != 2 {
		t.Fatalf("version: got %d, want 2", updated.Version)
	}

	// List by pair
	entries, err := s.ListFileEntriesByPair(pair.ID)
	if err != nil {
		t.Fatalf("ListByPair: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Update state
	s.UpdateFileEntryState(updated.ID, "synced")
	got2, _ := s.GetFileEntry(pair.ID, "/test.txt")
	if got2.SyncState != "synced" {
		t.Fatalf("state: got %q", got2.SyncState)
	}

	// List by state
	pending, err := s.ListFileEntriesByState(pair.ID, "pending")
	if err != nil {
		t.Fatalf("ListByState: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending, got %d", len(pending))
	}

	// Delete
	s.DeleteFileEntry(updated.ID)
	deleted, _ := s.GetFileEntry(pair.ID, "/test.txt")
	if deleted != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestStore_ChangeEvents(t *testing.T) {
	s := openTestStore(t)

	pair := &SyncPair{Name: "events", LocalPath: "/l", RemotePath: "/r", Direction: "both"}
	s.CreateSyncPair(pair)

	entry := &FileEntry{Path: "/ev.txt", SyncPairID: pair.ID, SyncState: "pending"}
	s.UpsertFileEntry(entry)
	got, _ := s.GetFileEntry(pair.ID, "/ev.txt")

	// Create event
	ce := &ChangeEvent{
		FileID:    got.ID,
		Source:    "local",
		EventType: "modify",
		Path:      "/ev.txt",
	}
	if err := s.CreateChangeEvent(ce); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// List unprocessed
	events, err := s.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Source != "local" {
		t.Fatalf("source: got %q", events[0].Source)
	}

	// Mark processed
	s.MarkEventProcessed(events[0].ID)
	events2, _ := s.ListUnprocessedEvents()
	if len(events2) != 0 {
		t.Fatalf("expected 0 unprocessed, got %d", len(events2))
	}
}
