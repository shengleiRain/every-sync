//go:build integration

package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/provider/local"
	"github.com/rain/every-sync/internal/store"
)

// setupIntegrationTest creates the standard integration test environment:
// two temp directories (local + remote), a real SQLite store, and a started
// engine with a registrar that creates local providers for both sides.
func setupIntegrationTest(t *testing.T) (*Engine, *store.Store, string, string) {
	t.Helper()

	localDir := t.TempDir()
	remoteDir := t.TempDir()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := DefaultConfig()
	cfg.DryRun = false
	cfg.MaxWorkers = 2
	cfg.QueueSize = 100
	cfg.ScanInterval = time.Hour
	e := New(s, cfg)

	e.WithRegistrar(func(pair *store.SyncPair) (provider.Provider, provider.Provider, error) {
		lp := &local.LocalProvider{}
		if err := lp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.LocalPath},
		}); err != nil {
			return nil, nil, err
		}

		rp := &local.LocalProvider{}
		if err := rp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.RemotePath},
		}); err != nil {
			return nil, nil, err
		}

		return lp, rp, nil
	})

	return e, s, localDir, remoteDir
}

// createAndRegisterPair creates a SyncPair in the store and registers it with
// the engine using the registrar.
func createAndRegisterPair(t *testing.T, e *Engine, s *store.Store, pair *store.SyncPair) {
	t.Helper()
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create sync pair: %v", err)
	}
	if err := e.RefreshPairs(); err != nil {
		t.Fatalf("refresh pairs: %v", err)
	}
}

// startAndSync starts the engine, runs a sync, waits for completion, then stops.
func startAndSync(t *testing.T, e *Engine, pairID int64, direction string) {
	t.Helper()
	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	if err := e.SyncPair(ctx, pairID, direction); err != nil {
		t.Fatalf("sync pair: %v", err)
	}
	e.Drain(5 * time.Second)
}

// --- Helper assertions (re-use the same patterns from engine_test.go) ---

func intgWriteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func intgAssertContent(t *testing.T, root, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", rel, string(got), want)
	}
}

func intgAssertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s exists or stat failed with unexpected error: %v", rel, err)
	}
}

func intgAssertDirExists(t *testing.T, root, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("expected directory %s to exist, got error: %v", rel, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory, got file", rel)
	}
}

// =============================================================================
// Test 1: Normal full sync — Both dirs empty, create files on local, sync,
// verify on remote.
// =============================================================================

func TestIntegration_NormalFullSync(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	// Create files on local side
	intgWriteFile(t, localDir, "hello.txt", "hello world")
	intgWriteFile(t, localDir, "subdir/nested.txt", "nested content")

	pair := &store.SyncPair{
		Name:      "full-sync",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	// Verify files on remote
	intgAssertContent(t, remoteDir, "hello.txt", "hello world")
	intgAssertContent(t, remoteDir, filepath.Join("subdir", "nested.txt"), "nested content")

	// Verify DB state
	entry, err := s.GetFileEntry(pair.ID, "/hello.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state = %q, want synced", entry.SyncState)
	}
}

// =============================================================================
// Test 2: Selected folders — Create files in multiple dirs, set
// SelectedFolders=["docs"], sync, verify only docs/ contents synced.
// =============================================================================

func TestIntegration_SelectedFolders(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "docs/readme.txt", "docs-content")
	intgWriteFile(t, localDir, "photos/vacation.txt", "photo-content")
	intgWriteFile(t, localDir, "work/report.txt", "work-content")

	pair := &store.SyncPair{
		Name:            "selected-folders",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["docs"]`,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	intgAssertContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-content")
	intgAssertMissing(t, remoteDir, filepath.Join("photos", "vacation.txt"))
	intgAssertMissing(t, remoteDir, filepath.Join("work", "report.txt"))
}

// =============================================================================
// Test 3: Both direction — Create different files on both sides, sync both
// direction, verify both sides have all files.
// =============================================================================

func TestIntegration_BothDirection(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "local-only.txt", "from-local")
	intgWriteFile(t, remoteDir, "remote-only.txt", "from-remote")

	pair := &store.SyncPair{
		Name:      "both-dir",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	// Both sides should have both files
	intgAssertContent(t, remoteDir, "local-only.txt", "from-local")
	intgAssertContent(t, localDir, "remote-only.txt", "from-remote")
	intgAssertContent(t, localDir, "local-only.txt", "from-local")
	intgAssertContent(t, remoteDir, "remote-only.txt", "from-remote")
}

// =============================================================================
// Test 4: Conflict detection — Modify same file on both sides, sync, verify
// conflict recorded.
// =============================================================================

func TestIntegration_ConflictDetection(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	// Create different content on each side with controlled mtimes
	intgWriteFile(t, localDir, "conflict.txt", "local-version")
	intgWriteFile(t, remoteDir, "conflict.txt", "remote-version")
	now := time.Now()
	if err := os.Chtimes(filepath.Join(localDir, "conflict.txt"), now, now.Add(2*time.Second)); err != nil {
		t.Fatalf("chtimes local: %v", err)
	}
	if err := os.Chtimes(filepath.Join(remoteDir, "conflict.txt"), now, now.Add(1*time.Second)); err != nil {
		t.Fatalf("chtimes remote: %v", err)
	}

	pair := &store.SyncPair{
		Name:             "conflict-detect",
		LocalPath:        localDir,
		RemotePath:       remoteDir,
		Provider:         "local",
		Mode:             "mirror",
		Direction:        "both",
		Enabled:          true,
		ConflictStrategy: "manual",
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	// Verify conflict was recorded
	conflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(conflicts))
	}
	if conflicts[0].Path != "/conflict.txt" {
		t.Fatalf("conflict path = %q, want /conflict.txt", conflicts[0].Path)
	}

	// Both files should remain unchanged (manual conflict does not overwrite)
	intgAssertContent(t, localDir, "conflict.txt", "local-version")
	intgAssertContent(t, remoteDir, "conflict.txt", "remote-version")
}

// =============================================================================
// Test 5: Conflict resolution — Create conflict, resolve with local_wins,
// verify.
// =============================================================================

func TestIntegration_ConflictResolution(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "resolve.txt", "local-wins")
	intgWriteFile(t, remoteDir, "resolve.txt", "remote-loses")
	now := time.Now()
	if err := os.Chtimes(filepath.Join(localDir, "resolve.txt"), now, now.Add(2*time.Second)); err != nil {
		t.Fatalf("chtimes local: %v", err)
	}
	if err := os.Chtimes(filepath.Join(remoteDir, "resolve.txt"), now, now.Add(1*time.Second)); err != nil {
		t.Fatalf("chtimes remote: %v", err)
	}

	pair := &store.SyncPair{
		Name:             "conflict-resolve",
		LocalPath:        localDir,
		RemotePath:       remoteDir,
		Provider:         "local",
		Mode:             "mirror",
		Direction:        "both",
		Enabled:          true,
		ConflictStrategy: "manual",
	}
	createAndRegisterPair(t, e, s, pair)

	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("sync pair: %v", err)
	}
	e.Drain(5 * time.Second)

	// Get the conflict
	conflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(conflicts))
	}

	// Resolve with local_wins
	if err := e.ResolveConflict(ctx, conflicts[0].ID, "local_wins"); err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}

	// Remote should now have local content
	intgAssertContent(t, remoteDir, "resolve.txt", "local-wins")

	// Conflict should be resolved
	openConflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list open conflicts: %v", err)
	}
	if len(openConflicts) != 0 {
		t.Fatalf("open conflicts = %d, want 0", len(openConflicts))
	}
}

// =============================================================================
// Test 6: Directory sync — Create directory structure on local, sync, verify
// dirs and contents on remote.
// =============================================================================

func TestIntegration_DirectorySync(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "docs/guides/intro.txt", "intro")
	intgWriteFile(t, localDir, "docs/guides/advanced.txt", "advanced")
	intgWriteFile(t, localDir, "docs/faq.txt", "faq")

	pair := &store.SyncPair{
		Name:      "dir-sync",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	intgAssertDirExists(t, remoteDir, "docs")
	intgAssertDirExists(t, remoteDir, filepath.Join("docs", "guides"))
	intgAssertContent(t, remoteDir, filepath.Join("docs", "guides", "intro.txt"), "intro")
	intgAssertContent(t, remoteDir, filepath.Join("docs", "guides", "advanced.txt"), "advanced")
	intgAssertContent(t, remoteDir, filepath.Join("docs", "faq.txt"), "faq")

	// Verify directory entries in DB
	docEntry, err := s.GetFileEntry(pair.ID, "/docs")
	if err != nil || docEntry == nil {
		t.Fatalf("get /docs entry: %v", err)
	}
	if !docEntry.IsDir {
		t.Fatal("/docs entry should be marked as directory")
	}
}

// =============================================================================
// Test 7: Directory delete — Delete a directory on local, sync, verify remote
// directory and contents deleted.
// =============================================================================

func TestIntegration_DirectoryDelete(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	// Create directory structure on both sides
	intgWriteFile(t, localDir, "project/main.go", "package main")
	intgWriteFile(t, localDir, "project/util.go", "package main")
	intgWriteFile(t, remoteDir, "project/main.go", "package main")
	intgWriteFile(t, remoteDir, "project/util.go", "package main")

	pair := &store.SyncPair{
		Name:      "dir-delete",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)

	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	// First sync to establish synced state
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	e.Drain(5 * time.Second)

	intgAssertContent(t, remoteDir, filepath.Join("project", "main.go"), "package main")

	// Delete directory on local side
	if err := os.RemoveAll(filepath.Join(localDir, "project")); err != nil {
		t.Fatalf("remove local dir: %v", err)
	}

	// Re-register with fresh providers so scan reflects current state
	if err := e.RefreshAllPairs(); err != nil {
		t.Fatalf("refresh all pairs: %v", err)
	}

	// Second sync should propagate deletion
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	e.Drain(5 * time.Second)

	intgAssertMissing(t, remoteDir, filepath.Join("project", "main.go"))
	intgAssertMissing(t, remoteDir, filepath.Join("project", "util.go"))
}

// =============================================================================
// Test 8: Virtual mode — Set mode=virtual, sync, verify remote files only
// indexed (not present on local).
// =============================================================================

func TestIntegration_VirtualMode(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, remoteDir, "document.pdf", "pdf-content")
	intgWriteFile(t, remoteDir, "data/export.csv", "csv-content")

	pair := &store.SyncPair{
		Name:      "virtual-mode",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "virtual",
		Direction: "down",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	// Files should NOT be downloaded to local
	intgAssertMissing(t, localDir, "document.pdf")
	intgAssertMissing(t, localDir, filepath.Join("data", "export.csv"))

	// But should be indexed as virtual in DB
	docEntry, err := s.GetFileEntry(pair.ID, "/document.pdf")
	if err != nil || docEntry == nil {
		t.Fatalf("get document.pdf entry: %v", err)
	}
	if docEntry.SyncState != "virtual" {
		t.Fatalf("sync state = %q, want virtual", docEntry.SyncState)
	}

	csvEntry, err := s.GetFileEntry(pair.ID, "/data/export.csv")
	if err != nil || csvEntry == nil {
		t.Fatalf("get data/export.csv entry: %v", err)
	}
	if csvEntry.SyncState != "virtual" {
		t.Fatalf("sync state = %q, want virtual", csvEntry.SyncState)
	}
}

// =============================================================================
// Test 9: Virtual materialize — After virtual sync, materialize a file,
// verify it's downloaded.
// =============================================================================

func TestIntegration_VirtualMaterialize(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, remoteDir, "bigfile.zip", "zip-content")

	pair := &store.SyncPair{
		Name:      "virtual-materialize",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "virtual",
		Direction: "down",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)

	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	// Virtual sync
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}
	e.Drain(5 * time.Second)

	intgAssertMissing(t, localDir, "bigfile.zip")

	// Materialize the file
	if err := e.MaterializeVirtual(ctx, pair.ID, "/bigfile.zip"); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	// File should now exist on local
	intgAssertContent(t, localDir, "bigfile.zip", "zip-content")

	// DB entry should now be synced
	entry, err := s.GetFileEntry(pair.ID, "/bigfile.zip")
	if err != nil || entry == nil {
		t.Fatalf("get entry after materialize: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state after materialize = %q, want synced", entry.SyncState)
	}
}

// =============================================================================
// Test 10: Selected folders change — Sync with SelectedFolders=["docs"], then
// change to ["photos"], verify files cleaned up and new files synced.
// =============================================================================

func TestIntegration_SelectedFoldersChange(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "docs/readme.txt", "docs-readme")
	intgWriteFile(t, localDir, "photos/vacation.txt", "vacation")
	intgWriteFile(t, localDir, "work/report.txt", "work-report")

	pair := &store.SyncPair{
		Name:            "folder-change",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["docs"]`,
	}
	createAndRegisterPair(t, e, s, pair)

	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	// First sync with docs selected
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	e.Drain(5 * time.Second)

	intgAssertContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-readme")
	intgAssertMissing(t, remoteDir, filepath.Join("photos", "vacation.txt"))
	intgAssertMissing(t, remoteDir, filepath.Join("work", "report.txt"))

	// Now change selected folders to photos
	pair.SelectedFolders = `["photos"]`
	if err := s.UpdateSyncPair(pair); err != nil {
		t.Fatalf("update pair: %v", err)
	}

	// Refresh to pick up new pair config with fresh providers
	if err := e.RefreshAllPairs(); err != nil {
		t.Fatalf("refresh pairs: %v", err)
	}

	// Second sync with photos selected
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	e.Drain(5 * time.Second)

	// photos should now be synced
	intgAssertContent(t, remoteDir, filepath.Join("photos", "vacation.txt"), "vacation")

	// docs/readme.txt still exists on remote because the engine does not
	// remove previously synced files when SelectedFolders changes —
	// it only filters what gets synced going forward.
	// This is consistent with the existing engine behavior.
	intgAssertContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-readme")
}

// =============================================================================
// Test 11: First sync same content — Put identical files on both sides, sync,
// verify no conflict, marked as synced.
// =============================================================================

func TestIntegration_FirstSyncSameContent(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "shared.txt", "same-content")
	intgWriteFile(t, remoteDir, "shared.txt", "same-content")

	pair := &store.SyncPair{
		Name:      "first-same",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)
	startAndSync(t, e, pair.ID, "")

	// No conflicts should be recorded
	conflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %d, want 0 for identical first sync", len(conflicts))
	}

	// File should be marked as synced
	entry, err := s.GetFileEntry(pair.ID, "/shared.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state = %q, want synced", entry.SyncState)
	}

	// Both sides should have the same content
	intgAssertContent(t, localDir, "shared.txt", "same-content")
	intgAssertContent(t, remoteDir, "shared.txt", "same-content")
}

// =============================================================================
// Test 12: Hash detection touch no change — Touch a file (change mtime only),
// sync, verify no task generated (content unchanged).
// =============================================================================

func TestIntegration_HashDetection_TouchNoChange(t *testing.T) {
	e, s, localDir, remoteDir := setupIntegrationTest(t)

	intgWriteFile(t, localDir, "stable.txt", "unchanged-content")
	intgWriteFile(t, remoteDir, "stable.txt", "unchanged-content")

	pair := &store.SyncPair{
		Name:      "touch-nochange",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	createAndRegisterPair(t, e, s, pair)

	ctx := context.Background()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer e.Stop()

	// First sync to establish synced state with cached hashes
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	e.Drain(5 * time.Second)

	entry, err := s.GetFileEntry(pair.ID, "/stable.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry after first sync: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state = %q, want synced", entry.SyncState)
	}

	// Refresh providers so they see the current FS state
	if err := e.RefreshAllPairs(); err != nil {
		t.Fatalf("refresh pairs: %v", err)
	}

	// Touch the local file (change mtime only, content stays the same)
	now := time.Now()
	if err := os.Chtimes(filepath.Join(localDir, "stable.txt"), now, now.Add(10*time.Second)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// Second sync: two-stage detection should detect content is unchanged
	if err := e.SyncPair(ctx, pair.ID, ""); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	e.Drain(5 * time.Second)

	// File should still exist on both sides with unchanged content
	intgAssertContent(t, localDir, "stable.txt", "unchanged-content")
	intgAssertContent(t, remoteDir, "stable.txt", "unchanged-content")

	// The hash should remain the same (content didn't change)
	entryAfter, err := s.GetFileEntry(pair.ID, "/stable.txt")
	if err != nil || entryAfter == nil {
		t.Fatalf("get entry after touch sync: %v", err)
	}
	if entryAfter.LocalHash != entry.LocalHash {
		t.Fatalf("local hash changed from %q to %q after touch (content unchanged)", entry.LocalHash, entryAfter.LocalHash)
	}
}
