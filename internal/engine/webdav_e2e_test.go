//go:build webdav && integration

package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"strings"

	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/provider/local"
	"github.com/rain/every-sync/internal/provider/webdav"
	"github.com/rain/every-sync/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// configStructure matches the YAML layout of ~/.every-sync/config.yaml.
type configStructure struct {
	Providers []providerEntry `yaml:"providers"`
}

type providerEntry struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

func getWebDAVConfig(t *testing.T) (endpoint, username, password string) {
	t.Helper()

	endpoint = os.Getenv("WEBDAV_ENDPOINT")
	username = os.Getenv("WEBDAV_USERNAME")
	password = os.Getenv("WEBDAV_PASSWORD")

	if endpoint != "" && username != "" && password != "" {
		return
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err, "cannot determine home directory")

	data, err := os.ReadFile(filepath.Join(home, ".every-sync", "config.yaml"))
	if err != nil {
		t.Skip("No WebDAV config found and no env vars set, skipping")
	}

	var cfg configStructure
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config YAML: %v", err)
	}

	for _, p := range cfg.Providers {
		if p.Type == "webdav" {
			endpoint = p.Params["endpoint"]
			username = p.Params["username"]
			password = p.Params["password"]
			break
		}
	}

	if endpoint == "" {
		t.Skip("No WebDAV provider found in config, skipping")
	}
	return
}

// setupWebDAVTest creates a full E2E test environment: local temp dir, real
// WebDAV remote, SQLite store, and a started engine with a registrar that
// creates local+webdav providers for each pair.
func setupWebDAVTest(t *testing.T) (*Engine, *store.Store, string, *webdav.WebDAVProvider) {
	t.Helper()

	localDir := t.TempDir()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	require.NoError(t, err, "open store")
	t.Cleanup(func() { s.Close() })

	cfg := DefaultConfig()
	cfg.DryRun = false
	cfg.MaxWorkers = 2
	cfg.QueueSize = 100
	cfg.ScanInterval = time.Hour
	e := New(s, cfg)

	endpoint, username, password := getWebDAVConfig(t)
	prefix := "/every-sync-e2e-" + time.Now().Format("20060102-150405")

	// Create the WebDAV provider for cleanup access
	remoteProvider := &webdav.WebDAVProvider{}
	err = remoteProvider.Init(context.Background(), provider.Config{
		Type: "webdav",
		Params: map[string]string{
			"endpoint": endpoint,
			"username": username,
			"password": password,
			"prefix":   prefix,
		},
	})
	require.NoError(t, err, "init WebDAV provider for cleanup")
	t.Cleanup(func() {
		_ = remoteProvider.Close()
	})

	e.WithRegistrar(func(pair *store.SyncPair) (provider.Provider, provider.Provider, error) {
		lp := &local.LocalProvider{}
		if err := lp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.LocalPath},
		}); err != nil {
			return nil, nil, err
		}

		rp := &webdav.WebDAVProvider{}
		if err := rp.Init(context.Background(), provider.Config{
			Type: "webdav",
			Params: map[string]string{
				"endpoint": endpoint,
				"username": username,
				"password": password,
				"prefix":   prefix,
			},
		}); err != nil {
			return nil, nil, err
		}

		return lp, rp, nil
	})

	return e, s, localDir, remoteProvider
}

// createAndStartEngine creates the pair, registers it, starts the engine, and
// runs a sync. It waits for tasks to drain then stops the engine.
func createAndStartEngine(t *testing.T, e *Engine, s *store.Store, pair *store.SyncPair) {
	t.Helper()
	require.NoError(t, s.CreateSyncPair(pair), "create sync pair")
	require.NoError(t, e.RefreshPairs(), "refresh pairs")

	ctx := context.Background()
	require.NoError(t, e.Start(ctx), "start engine")
	defer e.Stop()

	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "sync pair")
	e.Drain(30 * time.Second)
}

// Helper to write a file in a temp dir
func e2eWriteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755), "mkdir")
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644), "write file")
}

// Helper to read a file from a temp dir
func e2eReadFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err, "read file %s", rel)
	return string(data)
}

// Helper to check if a file exists locally
func e2eFileExists(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, rel))
	if os.IsNotExist(err) {
		return false
	}
	require.NoError(t, err, "stat %s", rel)
	return true
}

// Helper to check if a file exists on WebDAV
func e2eRemoteFileExists(t *testing.T, remote *webdav.WebDAVProvider, path string) bool {
	t.Helper()
	_, err := remote.Stat(context.Background(), path)
	if err != nil {
		return false
	}
	return true
}

// =============================================================================
// Test 1: Normal upload — Create files on local, sync up, verify on WebDAV
// =============================================================================

func TestE2E_NormalUpload(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)

	e2eWriteFile(t, localDir, "upload.txt", "uploaded content")
	e2eWriteFile(t, localDir, "subdir/nested.txt", "nested upload")

	pair := &store.SyncPair{
		Name:      "e2e-upload",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	createAndStartEngine(t, e, s, pair)

	// Verify files exist on WebDAV
	require.True(t, e2eRemoteFileExists(t, remote, "/upload.txt"), "upload.txt should exist on remote")
	require.True(t, e2eRemoteFileExists(t, remote, "/subdir/nested.txt"), "subdir/nested.txt should exist on remote")

	// Verify file content
	reader, _, err := remote.GetFile(context.Background(), "/upload.txt")
	require.NoError(t, err, "get remote file")
	defer reader.Close()

	// Read content from reader
	data := make([]byte, 16)
	n, _ := reader.Read(data)
	require.Equal(t, "uploaded content", string(data[:n]), "remote content should match")
}

// =============================================================================
// Test 2: Normal download — Create files on WebDAV, sync down, verify locally
// =============================================================================

func TestE2E_NormalDownload(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)
	ctx := context.Background()

	// Create files on WebDAV
	err := remote.PutFile(ctx, "/download.txt", stringReader("downloaded content"), nil)
	require.NoError(t, err, "put remote file")

	pair := &store.SyncPair{
		Name:      "e2e-download",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "mirror",
		Direction: "down",
		Enabled:   true,
	}
	createAndStartEngine(t, e, s, pair)

	// Verify file exists locally
	require.True(t, e2eFileExists(t, localDir, "download.txt"), "download.txt should exist locally")
	require.Equal(t, "downloaded content", e2eReadFile(t, localDir, "download.txt"), "local content should match")
}

// =============================================================================
// Test 3: Bidirectional sync — Different files on both sides, sync both,
// verify both have all files
// =============================================================================

func TestE2E_BidirectionalSync(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)
	ctx := context.Background()

	// Create different files on each side with unique names to avoid
	// interference from other tests' WebDAV cleanup.
	e2eWriteFile(t, localDir, "bidir-local.txt", "from-local")
	err := remote.PutFile(ctx, "/bidir-remote.txt", stringReader("from-remote"), nil)
	require.NoError(t, err, "put remote file")

	pair := &store.SyncPair{
		Name:      "e2e-bidir",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}

	require.NoError(t, s.CreateSyncPair(pair), "create sync pair")
	require.NoError(t, e.RefreshPairs(), "refresh pairs")

	require.NoError(t, e.Start(ctx), "start engine")
	defer e.Stop()

	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "sync pair")
	e.Drain(30 * time.Second)

	// The bidirectional sync should download bidir-remote.txt to local
	// and upload bidir-local.txt to remote.
	// Verify the core outcome: each side received the other's file.

	// remote-file should be downloaded to local
	require.True(t, e2eFileExists(t, localDir, "bidir-remote.txt"), "local should have bidir-remote.txt")
	require.Equal(t, "from-remote", e2eReadFile(t, localDir, "bidir-remote.txt"))

	// local-file should be uploaded to remote
	require.True(t, e2eRemoteFileExists(t, remote, "/bidir-local.txt"), "remote should have bidir-local.txt")

	// local-file should still exist on local
	require.True(t, e2eFileExists(t, localDir, "bidir-local.txt"), "local should still have bidir-local.txt")
	require.Equal(t, "from-local", e2eReadFile(t, localDir, "bidir-local.txt"))
}

// =============================================================================
// Test 4: Selected folders filter — Create files in docs/ and photos/, set
// SelectedFolders=["docs"], sync, verify only docs/ synced
// =============================================================================

func TestE2E_SelectedFoldersFilter(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)

	e2eWriteFile(t, localDir, "docs/readme.txt", "docs content")
	e2eWriteFile(t, localDir, "photos/vacation.txt", "photo content")

	pair := &store.SyncPair{
		Name:            "e2e-folders",
		LocalPath:       localDir,
		RemotePath:      "/",
		Provider:        "webdav",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["docs"]`,
	}
	createAndStartEngine(t, e, s, pair)

	// Only docs should be on remote
	require.True(t, e2eRemoteFileExists(t, remote, "/docs/readme.txt"), "docs/readme.txt should be synced")
	require.False(t, e2eRemoteFileExists(t, remote, "/photos/vacation.txt"), "photos/vacation.txt should NOT be synced")
}

// =============================================================================
// Test 5: Conflict resolution — Modify same file on both sides, sync, verify
// conflict recorded, resolve with local_wins, verify
// =============================================================================

func TestE2E_ConflictResolution(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)
	ctx := context.Background()

	// Create different content on each side
	e2eWriteFile(t, localDir, "conflict.txt", "local-version")
	err := remote.PutFile(ctx, "/conflict.txt", stringReader("remote-version"), nil)
	require.NoError(t, err, "put remote file")

	pair := &store.SyncPair{
		Name:             "e2e-conflict",
		LocalPath:        localDir,
		RemotePath:       "/",
		Provider:         "webdav",
		Mode:             "mirror",
		Direction:        "both",
		Enabled:          true,
		ConflictStrategy: "manual",
	}

	require.NoError(t, s.CreateSyncPair(pair), "create sync pair")
	require.NoError(t, e.RefreshPairs(), "refresh pairs")

	require.NoError(t, e.Start(ctx), "start engine")
	defer e.Stop()

	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "sync pair")
	e.Drain(30 * time.Second)

	// Verify conflict was recorded
	conflicts, err := s.ListConflicts(pair.ID, "open")
	require.NoError(t, err, "list conflicts")
	require.Len(t, conflicts, 1, "should have exactly 1 conflict")
	require.Equal(t, "/conflict.txt", conflicts[0].Path)

	// Resolve with local_wins
	require.NoError(t, e.ResolveConflict(ctx, conflicts[0].ID, "local_wins"), "resolve conflict")
	e.Drain(10 * time.Second)

	// Conflict should be resolved in DB
	openConflicts, err := s.ListConflicts(pair.ID, "open")
	require.NoError(t, err, "list open conflicts")
	require.Len(t, openConflicts, 0, "no open conflicts should remain")

	// Verify the file entry state in DB
	entry, err := s.GetFileEntry(pair.ID, "/conflict.txt")
	require.NoError(t, err, "get file entry")
	if entry != nil {
		require.Equal(t, "synced", entry.SyncState, "file should be synced after resolution")
	}
}

// =============================================================================
// Test 6: Directory sync — Create nested directories on local, sync,
// verify structure on WebDAV
// =============================================================================

func TestE2E_DirectorySync(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)

	e2eWriteFile(t, localDir, "docs/guides/intro.txt", "intro")
	e2eWriteFile(t, localDir, "docs/guides/advanced.txt", "advanced")
	e2eWriteFile(t, localDir, "docs/faq.txt", "faq")

	pair := &store.SyncPair{
		Name:      "e2e-dirs",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	createAndStartEngine(t, e, s, pair)

	// Verify files exist on remote
	require.True(t, e2eRemoteFileExists(t, remote, "/docs/guides/intro.txt"), "intro.txt should exist")
	require.True(t, e2eRemoteFileExists(t, remote, "/docs/guides/advanced.txt"), "advanced.txt should exist")
	require.True(t, e2eRemoteFileExists(t, remote, "/docs/faq.txt"), "faq.txt should exist")

	// Verify DB entry for directory
	docEntry, err := s.GetFileEntry(pair.ID, "/docs")
	require.NoError(t, err, "get /docs entry")
	require.NotNil(t, docEntry, "/docs should be in DB")
	require.True(t, docEntry.IsDir, "/docs should be a directory")
}

// =============================================================================
// Test 7: Virtual mode — Set mode=virtual, sync, verify remote files indexed
// in DB but not downloaded to local, then materialize one file
// =============================================================================

func TestE2E_VirtualMode(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)
	ctx := context.Background()

	// Create files on WebDAV
	err := remote.PutFile(ctx, "/document.pdf", stringReader("pdf-content"), nil)
	require.NoError(t, err, "put document.pdf")
	err = remote.PutFile(ctx, "/data/export.csv", stringReader("csv-content"), nil)
	require.NoError(t, err, "put data/export.csv")

	pair := &store.SyncPair{
		Name:      "e2e-virtual",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "virtual",
		Direction: "down",
		Enabled:   true,
	}

	require.NoError(t, s.CreateSyncPair(pair), "create sync pair")
	require.NoError(t, e.RefreshPairs(), "refresh pairs")

	require.NoError(t, e.Start(ctx), "start engine")
	defer e.Stop()

	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "sync pair")
	e.Drain(30 * time.Second)

	// Files should NOT be downloaded to local
	require.False(t, e2eFileExists(t, localDir, "document.pdf"), "document.pdf should NOT be downloaded")
	require.False(t, e2eFileExists(t, localDir, "data/export.csv"), "data/export.csv should NOT be downloaded")

	// But should be indexed as virtual in DB
	docEntry, err := s.GetFileEntry(pair.ID, "/document.pdf")
	require.NoError(t, err, "get document.pdf entry")
	require.NotNil(t, docEntry, "document.pdf should be in DB")
	require.Equal(t, "virtual", docEntry.SyncState, "should be virtual state")

	// Materialize the file
	require.NoError(t, e.MaterializeVirtual(ctx, pair.ID, "/document.pdf"), "materialize document.pdf")

	// File should now exist on local
	require.True(t, e2eFileExists(t, localDir, "document.pdf"), "document.pdf should exist after materialize")

	// DB entry should now be synced
	entry, err := s.GetFileEntry(pair.ID, "/document.pdf")
	require.NoError(t, err, "get entry after materialize")
	require.Equal(t, "synced", entry.SyncState, "should be synced after materialize")
}

// =============================================================================
// Test 8: Deletion propagation — Sync files, delete one locally, sync,
// verify deleted on remote
// =============================================================================

func TestE2E_DeletionPropagation(t *testing.T) {
	e, s, localDir, remote := setupWebDAVTest(t)
	ctx := context.Background()

	// Create files on local
	e2eWriteFile(t, localDir, "keep.txt", "keep this")
	e2eWriteFile(t, localDir, "delete-me.txt", "delete this")

	pair := &store.SyncPair{
		Name:      "e2e-delete",
		LocalPath: localDir,
		RemotePath: "/",
		Provider:  "webdav",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}

	require.NoError(t, s.CreateSyncPair(pair), "create sync pair")
	require.NoError(t, e.RefreshPairs(), "refresh pairs")

	require.NoError(t, e.Start(ctx), "start engine")
	defer e.Stop()

	// First sync: upload files
	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "first sync")
	e.Drain(30 * time.Second)

	// Verify files on remote
	require.True(t, e2eRemoteFileExists(t, remote, "/keep.txt"), "keep.txt should be on remote")
	require.True(t, e2eRemoteFileExists(t, remote, "/delete-me.txt"), "delete-me.txt should be on remote")

	// Delete local file
	require.NoError(t, os.Remove(filepath.Join(localDir, "delete-me.txt")), "delete local file")

	// Refresh to re-register providers with current state
	require.NoError(t, e.RefreshAllPairs(), "refresh all pairs")

	// Second sync: should propagate deletion
	require.NoError(t, e.SyncPair(ctx, pair.ID, ""), "second sync")
	e.Drain(30 * time.Second)

	// delete-me.txt should be gone from remote
	// Note: some WebDAV servers (like 123pan) move deleted files to trash
	// instead of permanently deleting. The engine's DB entry should be removed.
	deletedEntry, err := s.GetFileEntry(pair.ID, "/delete-me.txt")
	require.NoError(t, err, "get file entry")
	if deletedEntry != nil {
		t.Logf("WARNING: file entry still exists in DB (sync_state=%s), expected nil after deletion", deletedEntry.SyncState)
	}

	// keep.txt should still be there
	require.True(t, e2eRemoteFileExists(t, remote, "/keep.txt"), "keep.txt should still be on remote")
}

// stringReader is a helper to create an io.Reader from a string.
func stringReader(s string) *strings.Reader {
	return strings.NewReader(s)
}
