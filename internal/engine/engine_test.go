package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/provider/local"
	"github.com/rain/every-sync/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestLocalProvider(t *testing.T, root string) provider.Provider {
	t.Helper()
	p := &local.LocalProvider{}
	if err := p.Init(context.Background(), provider.Config{Params: map[string]string{"root_path": root}}); err != nil {
		t.Fatalf("init local provider: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func newStartedTestEngine(t *testing.T, s *store.Store, pair *store.SyncPair, cfg Config) *Engine {
	t.Helper()
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 2
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 100
	}
	if cfg.ScanInterval == 0 {
		cfg.ScanInterval = time.Hour
	}

	eng := New(s, cfg)
	eng.RegisterPair(
		pair,
		newTestLocalProvider(t, pair.LocalPath),
		newTestLocalProvider(t, pair.RemotePath),
	)
	if err := eng.Start(context.Background()); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	t.Cleanup(func() { eng.Stop() })
	return eng
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func assertFileContent(t *testing.T, root, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", rel, string(got), want)
	}
}

func assertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s exists or stat failed with unexpected error: %v", rel, err)
	}
}

func waitForFileContent(t *testing.T, root, rel, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err == nil && string(got) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	assertFileContent(t, root, rel, want)
}

func runPairSync(t *testing.T, eng *Engine, pairID int64, direction string) {
	t.Helper()
	if err := eng.SyncPair(context.Background(), pairID, direction); err != nil {
		t.Fatalf("sync pair: %v", err)
	}
	eng.Drain(5 * time.Second)
}

func TestEngineBidirectionalSecondSyncDoesNotDeleteSyncedFile(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "a.txt", "keep-me")

	pair := &store.SyncPair{
		Name:       "both",
		LocalPath:  localDir,
		RemotePath: remoteDir,
		Provider:   "local",
		Mode:       "mirror",
		Direction:  "both",
		Enabled:    true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "a.txt", "keep-me")

	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, localDir, "a.txt", "keep-me")
	assertFileContent(t, remoteDir, "a.txt", "keep-me")
}

func TestEngineOneWayUploadAndDownload(t *testing.T) {
	s := newTestStore(t)

	upLocal := t.TempDir()
	upRemote := t.TempDir()
	writeTestFile(t, upLocal, "up.txt", "upload")
	upPair := &store.SyncPair{Name: "up", LocalPath: upLocal, RemotePath: upRemote, Provider: "local", Mode: "mirror", Direction: "up", Enabled: true}
	if err := s.CreateSyncPair(upPair); err != nil {
		t.Fatalf("create up pair: %v", err)
	}
	upEng := newStartedTestEngine(t, s, upPair, Config{RetryMax: 0})
	runPairSync(t, upEng, upPair.ID, "")
	assertFileContent(t, upRemote, "up.txt", "upload")
	upEntry, err := s.GetFileEntry(upPair.ID, "/up.txt")
	if err != nil || upEntry == nil {
		t.Fatalf("get up entry: %v", err)
	}
	runPairSync(t, upEng, upPair.ID, "")
	upEntryAfter, err := s.GetFileEntry(upPair.ID, "/up.txt")
	if err != nil || upEntryAfter == nil {
		t.Fatalf("get up entry after second sync: %v", err)
	}
	if upEntryAfter.Version != upEntry.Version {
		t.Fatalf("up second sync changed version from %d to %d", upEntry.Version, upEntryAfter.Version)
	}

	downLocal := t.TempDir()
	downRemote := t.TempDir()
	writeTestFile(t, downRemote, "down.txt", "download")
	downPair := &store.SyncPair{Name: "down", LocalPath: downLocal, RemotePath: downRemote, Provider: "local", Mode: "mirror", Direction: "down", Enabled: true}
	if err := s.CreateSyncPair(downPair); err != nil {
		t.Fatalf("create down pair: %v", err)
	}
	downEng := newStartedTestEngine(t, s, downPair, Config{RetryMax: 0})
	runPairSync(t, downEng, downPair.ID, "")
	assertFileContent(t, downLocal, "down.txt", "download")
	downEntry, err := s.GetFileEntry(downPair.ID, "/down.txt")
	if err != nil || downEntry == nil {
		t.Fatalf("get down entry: %v", err)
	}
	runPairSync(t, downEng, downPair.ID, "")
	downEntryAfter, err := s.GetFileEntry(downPair.ID, "/down.txt")
	if err != nil || downEntryAfter == nil {
		t.Fatalf("get down entry after second sync: %v", err)
	}
	if downEntryAfter.Version != downEntry.Version {
		t.Fatalf("down second sync changed version from %d to %d", downEntry.Version, downEntryAfter.Version)
	}
}

func TestEngineDryRunDoesNotWriteFilesOrIndex(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "dry.txt", "preview")

	pair := &store.SyncPair{Name: "dry", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "up", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0, DryRun: true})
	runPairSync(t, eng, pair.ID, "")
	assertMissing(t, remoteDir, "dry.txt")

	entries, err := s.ListFileEntriesByPair(pair.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("dry run indexed %d entries, want 0", len(entries))
	}
}

func TestEngineWatchChangesTriggersSync(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	pair := &store.SyncPair{Name: "watch", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "up", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	newStartedTestEngine(t, s, pair, Config{RetryMax: 0, ScanInterval: time.Hour})
	time.Sleep(100 * time.Millisecond)

	writeTestFile(t, localDir, "watched.txt", "inotify")
	waitForFileContent(t, remoteDir, "watched.txt", "inotify")
}

func TestEngineResumesDownloadFromPartialFile(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, remoteDir, "large.txt", "0123456789")
	writeTestFile(t, localDir, "large.txt"+partialSuffix, "0123")

	pair := &store.SyncPair{Name: "resume-down", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "down", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0, ChunkThreshold: 4, ChunkSize: 4})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, localDir, "large.txt", "0123456789")
	assertMissing(t, localDir, "large.txt"+partialSuffix)
}

func TestEngineStatusReportsResumeCapabilities(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	pair := &store.SyncPair{Name: "caps", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "both", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0, ChunkThreshold: 4, ChunkSize: 4})
	status := eng.Status()
	if len(status.Pairs) != 1 {
		t.Fatalf("pairs = %d, want 1", len(status.Pairs))
	}
	if !status.Pairs[0].ResumableUpload || !status.Pairs[0].ResumableDownload {
		t.Fatalf("resume capabilities = upload:%v download:%v, want both true", status.Pairs[0].ResumableUpload, status.Pairs[0].ResumableDownload)
	}
}

func TestEngineSkipsIdentifierFiles(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "Identifier", "skip")
	writeTestFile(t, localDir, "nested/Identifier", "skip nested")
	writeTestFile(t, localDir, "file.txt"+partialSuffix, "skip partial")
	writeTestFile(t, localDir, "keep.txt", "sync")

	pair := &store.SyncPair{Name: "identifier", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "up", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertMissing(t, remoteDir, "Identifier")
	assertMissing(t, remoteDir, filepath.Join("nested", "Identifier"))
	assertMissing(t, remoteDir, "file.txt"+partialSuffix)
	assertFileContent(t, remoteDir, "keep.txt", "sync")
}

func TestEngineBidirectionalPropagatesRemoteDeleteWhenLocalUnchanged(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "delete-me.txt", "gone")

	pair := &store.SyncPair{Name: "delete", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "both", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "delete-me.txt", "gone")

	if err := os.Remove(filepath.Join(remoteDir, "delete-me.txt")); err != nil {
		t.Fatalf("remove remote file: %v", err)
	}
	runPairSync(t, eng, pair.ID, "")
	assertMissing(t, localDir, "delete-me.txt")
}

func TestGenerateTasksSkipsUnchangedFileWithDifferentSideMtimes(t *testing.T) {
	localTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	remoteTime := localTime.Add(2 * time.Second)
	pair := &store.SyncPair{ID: 42, Direction: "both"}
	entry := &store.FileEntry{
		Path:        "/same.txt",
		SyncPairID:  42,
		LocalMTime:  &localTime,
		RemoteMTime: &remoteTime,
		LocalSize:   4,
		RemoteSize:  4,
		SyncState:   "synced",
	}

	eng := &Engine{}
	tasks := eng.generateTasks(
		pair,
		[]*provider.FileMeta{{Path: "/same.txt", Size: 4, ModTime: localTime}},
		[]*provider.FileMeta{{Path: "/same.txt", Size: 4, ModTime: remoteTime}},
		[]*store.FileEntry{entry},
		DirectionBoth,
	)
	if len(tasks) != 0 {
		t.Fatalf("tasks = %+v, want none", tasks)
	}
}

func TestEngineSelectiveModeSkipsExcludedFiles(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "keep.txt", "keep")
	writeTestFile(t, localDir, "skip.tmp", "skip")

	pair := &store.SyncPair{
		Name:            "selective",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "selective",
		Direction:       "up",
		Enabled:         true,
		ExcludePatterns: "*.tmp",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "keep.txt", "keep")
	assertMissing(t, remoteDir, "skip.tmp")
}

func TestEngineVirtualModeIndexesThenMaterializesRemoteFile(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, remoteDir, "remote.txt", "remote")

	pair := &store.SyncPair{Name: "virtual", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "virtual", Direction: "down", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertMissing(t, localDir, "remote.txt")
	entry, err := s.GetFileEntry(pair.ID, "/remote.txt")
	if err != nil || entry == nil {
		t.Fatalf("get virtual entry: %v", err)
	}
	if entry.SyncState != "virtual" {
		t.Fatalf("sync state = %q, want virtual", entry.SyncState)
	}

	if err := eng.MaterializeVirtual(context.Background(), pair.ID, "/remote.txt"); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	assertFileContent(t, localDir, "remote.txt", "remote")
	entry, err = s.GetFileEntry(pair.ID, "/remote.txt")
	if err != nil || entry == nil {
		t.Fatalf("get materialized entry: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state = %q, want synced", entry.SyncState)
	}
}

func TestEngineManualConflictCanResolveRemoteWins(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "same.txt", "local")
	writeTestFile(t, remoteDir, "same.txt", "remote")
	now := time.Now()
	if err := os.Chtimes(filepath.Join(localDir, "same.txt"), now, now.Add(2*time.Second)); err != nil {
		t.Fatalf("chtimes local: %v", err)
	}
	if err := os.Chtimes(filepath.Join(remoteDir, "same.txt"), now, now.Add(1*time.Second)); err != nil {
		t.Fatalf("chtimes remote: %v", err)
	}

	pair := &store.SyncPair{Name: "manual-conflict", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "both", Enabled: true, ConflictStrategy: "manual"}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, localDir, "same.txt", "local")
	assertFileContent(t, remoteDir, "same.txt", "remote")
	conflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(conflicts))
	}

	if err := eng.ResolveConflict(context.Background(), conflicts[0].ID, "remote_wins"); err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	assertFileContent(t, localDir, "same.txt", "remote")
	conflicts, err = s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts after resolve: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("open conflicts = %d, want 0", len(conflicts))
	}
}

func TestEngineRecordsVersionBeforeOverwrite(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	writeTestFile(t, localDir, "doc.txt", "new-file")
	writeTestFile(t, remoteDir, "doc.txt", "old")
	now := time.Now()
	if err := os.Chtimes(filepath.Join(localDir, "doc.txt"), now, now.Add(2*time.Second)); err != nil {
		t.Fatalf("chtimes local: %v", err)
	}
	if err := os.Chtimes(filepath.Join(remoteDir, "doc.txt"), now, now.Add(1*time.Second)); err != nil {
		t.Fatalf("chtimes remote: %v", err)
	}

	pair := &store.SyncPair{Name: "versions", LocalPath: localDir, RemotePath: remoteDir, Provider: "local", Mode: "mirror", Direction: "up", Enabled: true}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "doc.txt", "new-file")
	versions, err := s.ListFileVersions(pair.ID, "/doc.txt")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(versions))
	}
	if versions[0].Source != "remote" || versions[0].Size != int64(len("old")) {
		t.Fatalf("version = source %q size %d, want remote/%d", versions[0].Source, versions[0].Size, len("old"))
	}
}

func TestNormalMode_SelectedFoldersFilter(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "work/report.txt", "work-report")
	writeTestFile(t, localDir, "photos/vacation.txt", "photo")
	writeTestFile(t, localDir, "docs/readme.txt", "docs-readme")
	writeTestFile(t, localDir, "other/misc.txt", "misc")

	pair := &store.SyncPair{
		Name:            "normal-select",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["work","docs"]`,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("work", "report.txt"), "work-report")
	assertFileContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-readme")
	assertMissing(t, remoteDir, filepath.Join("photos", "vacation.txt"))
	assertMissing(t, remoteDir, filepath.Join("other", "misc.txt"))
}

func TestNormalMode_SelectedFoldersEmpty(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "work/report.txt", "work-report")
	writeTestFile(t, localDir, "photos/vacation.txt", "photo")
	writeTestFile(t, localDir, "docs/readme.txt", "docs-readme")

	pair := &store.SyncPair{
		Name:            "normal-empty",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: "[]",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Empty SelectedFolders = sync everything (mirror behavior)
	assertFileContent(t, remoteDir, filepath.Join("work", "report.txt"), "work-report")
	assertFileContent(t, remoteDir, filepath.Join("photos", "vacation.txt"), "photo")
	assertFileContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-readme")
}

func TestNormalMode_SelectedFoldersParentMerge(t *testing.T) {
	result := NormalizeSelectedFolders([]string{"docs/work/2024", "docs/work", "photos"})
	if len(result) != 2 {
		t.Fatalf("NormalizeSelectedFolders returned %d items, want 2: %v", len(result), result)
	}
	if result[0] != "docs/work" {
		t.Fatalf("result[0] = %q, want %q", result[0], "docs/work")
	}
	if result[1] != "photos" {
		t.Fatalf("result[1] = %q, want %q", result[1], "photos")
	}
}

func TestNormalMode_SelectedFoldersParentMerge_Empty(t *testing.T) {
	result := NormalizeSelectedFolders([]string{})
	if len(result) != 0 {
		t.Fatalf("NormalizeSelectedFolders returned %d items, want 0", len(result))
	}

	result = NormalizeSelectedFolders(nil)
	if len(result) != 0 {
		t.Fatalf("NormalizeSelectedFolders(nil) returned %d items, want 0", len(result))
	}
}

func TestNormalMode_SelectedFoldersParentMerge_Single(t *testing.T) {
	result := NormalizeSelectedFolders([]string{"docs"})
	if len(result) != 1 {
		t.Fatalf("NormalizeSelectedFolders returned %d items, want 1", len(result))
	}
	if result[0] != "docs" {
		t.Fatalf("result[0] = %q, want %q", result[0], "docs")
	}
}

func TestNormalMode_SelectedFoldersRemoval_Down(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Files on remote side for initial down sync
	writeTestFile(t, remoteDir, "work/report.txt", "work-report")
	writeTestFile(t, remoteDir, "photos/vacation.txt", "photo")

	pair := &store.SyncPair{
		Name:            "normal-removal",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "down",
		Enabled:         true,
		SelectedFolders: `["work","photos"]`,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Both folders synced to local
	assertFileContent(t, localDir, filepath.Join("work", "report.txt"), "work-report")
	assertFileContent(t, localDir, filepath.Join("photos", "vacation.txt"), "photo")

	// Now remove "photos" from SelectedFolders
	pair.SelectedFolders = `["work"]`
	if err := s.UpdateSyncPair(pair); err != nil {
		t.Fatalf("update pair: %v", err)
	}

	// Re-register pair with updated config and fresh providers
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	// Run sync again - photos files are filtered out of both scans so the sync
	// does not touch them. They remain on disk as leftover synced files.
	// A separate cleanup mechanism (not yet implemented) would handle removal.
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, localDir, filepath.Join("work", "report.txt"), "work-report")
	// photos/vacation.txt still exists locally because the filter removes it
	// from both scan results, so the engine never sees it to generate a delete task.
	// The remote is also filtered, so remote copy is untouched.
	assertFileContent(t, remoteDir, filepath.Join("photos", "vacation.txt"), "photo")
}

func TestNormalMode_MirrorAliasBackwardCompat(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "file.txt", "content")

	// Use "mirror" mode - should still work as alias for "normal"
	pair := &store.SyncPair{
		Name:      "mirror-compat",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "file.txt", "content")
}

func TestNormalMode_SelectedFoldersWithNestedPaths(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "docs/work/2024/report.txt", "deep-report")
	writeTestFile(t, localDir, "docs/personal/notes.txt", "personal-notes")
	writeTestFile(t, localDir, "docs/readme.txt", "docs-readme")
	writeTestFile(t, localDir, "other/misc.txt", "misc")

	// Select only docs/work - should include nested paths
	pair := &store.SyncPair{
		Name:            "normal-nested",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["docs/work"]`,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("docs", "work", "2024", "report.txt"), "deep-report")
	assertMissing(t, remoteDir, filepath.Join("docs", "personal", "notes.txt"))
	assertMissing(t, remoteDir, filepath.Join("docs", "readme.txt"))
	assertMissing(t, remoteDir, filepath.Join("other", "misc.txt"))
}

func TestNormalMode_SelectedFoldersWithExcludePatterns(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "work/report.txt", "work-report")
	writeTestFile(t, localDir, "work/temp.tmp", "temp-file")
	writeTestFile(t, localDir, "docs/readme.txt", "docs-readme")
	writeTestFile(t, localDir, "skip.tmp", "root-tmp")

	pair := &store.SyncPair{
		Name:            "normal-folders-patterns",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["work","docs"]`,
		ExcludePatterns: "*.tmp",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("work", "report.txt"), "work-report")
	assertFileContent(t, remoteDir, filepath.Join("docs", "readme.txt"), "docs-readme")
	assertMissing(t, remoteDir, "skip.tmp")
}

func TestFilterBySelectedFolders(t *testing.T) {
	pair := &store.SyncPair{
		SelectedFolders: `["work","docs"]`,
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/work/file.txt", true},
		{"/work/sub/file.txt", true},
		{"/docs/readme.txt", true},
		{"/docs/sub/deep.txt", true},
		{"/photos/img.jpg", false},
		{"/other", false},
	}

	for _, tc := range tests {
		got := filterBySelectedFolders(pair, tc.path)
		if got != tc.expected {
			t.Errorf("filterBySelectedFolders(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

func TestFilterBySelectedFolders_Empty(t *testing.T) {
	pair := &store.SyncPair{
		SelectedFolders: "[]",
	}
	if !filterBySelectedFolders(pair, "/any/path.txt") {
		t.Error("empty SelectedFolders should allow all paths")
	}

	pair.SelectedFolders = ""
	if !filterBySelectedFolders(pair, "/any/path.txt") {
		t.Error("blank SelectedFolders should allow all paths")
	}
}

func TestIsNormalMode(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{"normal", true},
		{"mirror", true},
		{"selective", true},
		{"NORMAL", true},
		{"Mirror", true},
		{"virtual", false},
		{"", false},
	}

	for _, tc := range tests {
		pair := &store.SyncPair{Mode: tc.mode}
		got := isNormalMode(pair)
		if got != tc.expected {
			t.Errorf("isNormalMode(%q) = %v, want %v", tc.mode, got, tc.expected)
		}
	}
}

// --- Directory sync tests ---

func assertDirExists(t *testing.T, root, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("expected directory %s to exist, got error: %v", rel, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory, got file", rel)
	}
}

func assertDirMissing(t *testing.T, root, rel string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, rel))
	if err == nil {
		t.Fatalf("expected directory %s to not exist, but it does", rel)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat %s returned unexpected error: %v", rel, err)
	}
}

func TestDirectorySync_RemoteHasDir_LocalDoesNot_CreatedLocally(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Remote has a directory with a file inside
	writeTestFile(t, remoteDir, "docs/readme.txt", "hello")

	pair := &store.SyncPair{
		Name:      "dir-down",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "down",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, localDir, "docs/readme.txt", "hello")
	assertDirExists(t, localDir, "docs")
}

func TestDirectorySync_LocalHasDir_RemoteDoesNot_CreatedRemotely(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Local has a directory with a file inside
	writeTestFile(t, localDir, "docs/readme.txt", "hello")

	pair := &store.SyncPair{
		Name:      "dir-up",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, "docs/readme.txt", "hello")
	assertDirExists(t, remoteDir, "docs")
}

func TestDirectorySync_RemoteDeletesDir_LocalDirDeleted(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Both sides have the directory with a file
	writeTestFile(t, localDir, "docs/readme.txt", "hello")
	writeTestFile(t, remoteDir, "docs/readme.txt", "hello")

	pair := &store.SyncPair{
		Name:      "dir-delete-down",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Verify both sides have the file
	assertFileContent(t, localDir, "docs/readme.txt", "hello")
	assertFileContent(t, remoteDir, "docs/readme.txt", "hello")

	// Delete the directory on the remote side
	os.RemoveAll(filepath.Join(remoteDir, "docs"))

	// Re-register with fresh providers so the remote scan reflects the deletion
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	// Second sync should propagate deletion
	runPairSync(t, eng, pair.ID, "")

	assertMissing(t, localDir, "docs/readme.txt")
}

func TestDirectorySync_LocalDeletesDir_RemoteDirDeleted(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Both sides have the directory with a file
	writeTestFile(t, localDir, "docs/readme.txt", "hello")
	writeTestFile(t, remoteDir, "docs/readme.txt", "hello")

	pair := &store.SyncPair{
		Name:      "dir-delete-up",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Verify both sides have the file
	assertFileContent(t, localDir, "docs/readme.txt", "hello")
	assertFileContent(t, remoteDir, "docs/readme.txt", "hello")

	// Delete the directory on the local side
	os.RemoveAll(filepath.Join(localDir, "docs"))

	// Re-register with fresh providers
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	// Second sync should propagate deletion
	runPairSync(t, eng, pair.ID, "")

	assertMissing(t, remoteDir, "docs/readme.txt")
}

func TestDirectorySync_EmptyDirPreserved(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Create an empty directory locally
	if err := os.MkdirAll(filepath.Join(localDir, "emptydir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	pair := &store.SyncPair{
		Name:      "dir-empty",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// The empty directory should be created on remote
	assertDirExists(t, remoteDir, "emptydir")
}

func TestDirectorySync_DirEntryRecordedInDB(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Create a directory with a file on local
	writeTestFile(t, localDir, "docs/readme.txt", "content")

	pair := &store.SyncPair{
		Name:      "dir-db",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Check that the directory entry is recorded in DB
	entry, err := s.GetFileEntry(pair.ID, "/docs")
	if err != nil {
		t.Fatalf("get dir entry: %v", err)
	}
	if entry == nil {
		t.Fatal("directory entry not found in DB")
	}
	if !entry.IsDir {
		t.Fatal("entry.IsDir = false, want true")
	}
	if entry.SyncState != "synced" {
		t.Fatalf("entry.SyncState = %q, want %q", entry.SyncState, "synced")
	}
}

func TestDirectorySync_NestedDirsCreatedRecursively(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Create deeply nested directory structure on local
	writeTestFile(t, localDir, "a/b/c/deep.txt", "deep-content")

	pair := &store.SyncPair{
		Name:      "dir-nested",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("a", "b", "c", "deep.txt"), "deep-content")
	assertDirExists(t, remoteDir, "a")
	assertDirExists(t, remoteDir, filepath.Join("a", "b"))
	assertDirExists(t, remoteDir, filepath.Join("a", "b", "c"))
}

func TestDirectorySync_DirWithMultipleFiles_Up(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Local directory with multiple files
	writeTestFile(t, localDir, "project/main.go", "package main")
	writeTestFile(t, localDir, "project/README.md", "# readme")
	writeTestFile(t, localDir, "project/go.mod", "module test")

	pair := &store.SyncPair{
		Name:      "dir-multi-up",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("project", "main.go"), "package main")
	assertFileContent(t, remoteDir, filepath.Join("project", "README.md"), "# readme")
	assertFileContent(t, remoteDir, filepath.Join("project", "go.mod"), "module test")
	assertDirExists(t, remoteDir, "project")
}

func TestDirectorySync_DirDeletionCleansUpChildDBEntries(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Both sides have a directory with multiple files
	writeTestFile(t, localDir, "project/main.go", "package main")
	writeTestFile(t, localDir, "project/util.go", "package main")
	writeTestFile(t, remoteDir, "project/main.go", "package main")
	writeTestFile(t, remoteDir, "project/util.go", "package main")

	pair := &store.SyncPair{
		Name:      "dir-cleanup-db",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Verify DB has entries
	entries, err := s.ListFileEntriesByPair(pair.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries after initial sync, got %d", len(entries))
	}

	// Delete directory on remote
	os.RemoveAll(filepath.Join(remoteDir, "project"))

	// Re-register with fresh providers
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	runPairSync(t, eng, pair.ID, "")

	// Verify child file entries are removed from DB
	entries, err = s.ListFileEntriesByPair(pair.ID)
	if err != nil {
		t.Fatalf("list entries after delete: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "/project") {
			t.Errorf("found leftover entry under /project: %s (is_dir=%v)", e.Path, e.IsDir)
		}
	}
}

func TestDirectorySync_SecondSyncIdempotent(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "docs/note.txt", "note")

	pair := &store.SyncPair{
		Name:      "dir-idempotent",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "docs/note.txt", "note")

	// Second sync should not generate any new tasks or change anything
	runPairSync(t, eng, pair.ID, "")
	assertFileContent(t, remoteDir, "docs/note.txt", "note")
	assertDirExists(t, remoteDir, "docs")

	// Verify DB state is stable
	entry, err := s.GetFileEntry(pair.ID, "/docs")
	if err != nil {
		t.Fatalf("get dir entry: %v", err)
	}
	if entry == nil || !entry.IsDir {
		t.Fatal("directory entry missing or not marked as dir after second sync")
	}
}

func TestDirectorySync_SelectedFoldersAppliesToDirectories(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	writeTestFile(t, localDir, "work/report.txt", "work-report")
	writeTestFile(t, localDir, "photos/vacation.txt", "photo")

	pair := &store.SyncPair{
		Name:            "dir-select-folders",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "normal",
		Direction:       "up",
		Enabled:         true,
		SelectedFolders: `["work"]`,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	assertFileContent(t, remoteDir, filepath.Join("work", "report.txt"), "work-report")
	assertDirExists(t, remoteDir, "work")
	assertMissing(t, remoteDir, filepath.Join("photos", "vacation.txt"))
	assertDirMissing(t, remoteDir, "photos")
}

// --- Virtual mode tests ---

func TestVirtualMode_ForcesDirectionBoth(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Remote has a file, local has nothing - virtual mode should index it.
	// Direction is set to "up" but virtual mode should force "both" internally.
	writeTestFile(t, remoteDir, "data.txt", "remote-data")

	pair := &store.SyncPair{
		Name:      "virtual-force-both",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "virtual",
		Direction: "up",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// The remote file should be indexed as virtual (not downloaded)
	assertMissing(t, localDir, "data.txt")
	entry, err := s.GetFileEntry(pair.ID, "/data.txt")
	if err != nil || entry == nil {
		t.Fatalf("get virtual entry: %v", err)
	}
	if entry.SyncState != "virtual" {
		t.Fatalf("sync state = %q, want virtual", entry.SyncState)
	}
}

func TestVirtualMode_LocalUploadWorks(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Local has a new file that should be uploaded
	writeTestFile(t, localDir, "local.txt", "local-content")
	// Remote has a file that should be virtualized (not downloaded)
	writeTestFile(t, remoteDir, "remote.txt", "remote-content")

	pair := &store.SyncPair{
		Name:      "virtual-upload",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "virtual",
		Direction: "down", // Even with "down" direction, virtual mode forces "both"
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Local file should have been uploaded to remote
	assertFileContent(t, remoteDir, "local.txt", "local-content")

	// Remote file should be virtual (not downloaded)
	assertMissing(t, localDir, "remote.txt")
	entry, err := s.GetFileEntry(pair.ID, "/remote.txt")
	if err != nil || entry == nil {
		t.Fatalf("get remote entry: %v", err)
	}
	if entry.SyncState != "virtual" {
		t.Fatalf("remote sync state = %q, want virtual", entry.SyncState)
	}
}

func TestVirtualMode_RemoteChangeReVirtualizes(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Both sides have the same file initially
	writeTestFile(t, localDir, "shared.txt", "original")
	writeTestFile(t, remoteDir, "shared.txt", "original")

	pair := &store.SyncPair{
		Name:      "virtual-revirt",
		LocalPath: localDir,
		RemotePath: remoteDir,
		Provider:  "local",
		Mode:      "mirror",
		Direction: "both",
		Enabled:   true,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})

	// First sync in normal mode: file is identical on both sides, gets indexed as synced
	runPairSync(t, eng, pair.ID, "")
	entry, err := s.GetFileEntry(pair.ID, "/shared.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry after first sync: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state after first sync = %q, want synced", entry.SyncState)
	}

	// Now switch to virtual mode
	pair.Mode = "virtual"
	if err := s.UpdateSyncPair(pair); err != nil {
		t.Fatalf("update pair to virtual: %v", err)
	}

	// Modify the file on remote side
	writeTestFile(t, remoteDir, "shared.txt", "remote-updated")

	// Re-register with fresh providers so the remote scan sees the new content
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	// Second sync: remote changed, should re-virtualize (NOT download)
	runPairSync(t, eng, pair.ID, "")

	// Local file should still have the original content (not overwritten)
	assertFileContent(t, localDir, "shared.txt", "original")

	// DB entry should now be virtual
	entry, err = s.GetFileEntry(pair.ID, "/shared.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry after re-virtualization: %v", err)
	}
	if entry.SyncState != "virtual" {
		t.Fatalf("sync state after re-virtualization = %q, want virtual", entry.SyncState)
	}

	// Remote metadata should be updated
	if entry.RemoteSize != int64(len("remote-updated")) {
		t.Fatalf("remote size = %d, want %d", entry.RemoteSize, len("remote-updated"))
	}
}

func TestVirtualMode_SelectedFoldersFiltering(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Remote has files in multiple folders
	writeTestFile(t, remoteDir, "work/report.txt", "work-report")
	writeTestFile(t, remoteDir, "photos/vacation.txt", "photo")
	writeTestFile(t, remoteDir, "docs/readme.txt", "docs-readme")

	// Only "work" folder is selected
	pair := &store.SyncPair{
		Name:            "virtual-filter",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "virtual",
		Direction:       "down",
		Enabled:         true,
		SelectedFolders: `["work"]`,
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})
	runPairSync(t, eng, pair.ID, "")

	// Only work/report.txt should be indexed as virtual
	entry, err := s.GetFileEntry(pair.ID, "/work/report.txt")
	if err != nil || entry == nil {
		t.Fatalf("get work entry: %v", err)
	}
	if entry.SyncState != "virtual" {
		t.Fatalf("work sync state = %q, want virtual", entry.SyncState)
	}

	// photos/vacation.txt should not be indexed (filtered out)
	photoEntry, _ := s.GetFileEntry(pair.ID, "/photos/vacation.txt")
	if photoEntry != nil {
		t.Fatal("photos/vacation.txt should not be indexed, but found entry")
	}

	// docs/readme.txt should not be indexed (filtered out)
	docsEntry, _ := s.GetFileEntry(pair.ID, "/docs/readme.txt")
	if docsEntry != nil {
		t.Fatal("docs/readme.txt should not be indexed, but found entry")
	}

	// Nothing should be downloaded
	assertMissing(t, localDir, "work/report.txt")
	assertMissing(t, localDir, filepath.Join("photos", "vacation.txt"))
	assertMissing(t, localDir, filepath.Join("docs", "readme.txt"))
}

func TestVirtualMode_BothChanged_UploadWinsOverConflict(t *testing.T) {
	s := newTestStore(t)
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// Both sides have the same file initially
	writeTestFile(t, localDir, "both.txt", "original")
	writeTestFile(t, remoteDir, "both.txt", "original")

	pair := &store.SyncPair{
		Name:            "virtual-both-change",
		LocalPath:       localDir,
		RemotePath:      remoteDir,
		Provider:        "local",
		Mode:            "mirror",
		Direction:       "both",
		Enabled:         true,
		ConflictStrategy: "latest_wins",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	eng := newStartedTestEngine(t, s, pair, Config{RetryMax: 0})

	// First sync in normal mode to establish "synced" state
	runPairSync(t, eng, pair.ID, "")

	// Verify synced state
	entry, err := s.GetFileEntry(pair.ID, "/both.txt")
	if err != nil || entry == nil {
		t.Fatalf("get entry after first sync: %v", err)
	}
	if entry.SyncState != "synced" {
		t.Fatalf("sync state = %q, want synced", entry.SyncState)
	}

	// Now switch to virtual mode
	pair.Mode = "virtual"
	if err := s.UpdateSyncPair(pair); err != nil {
		t.Fatalf("update pair to virtual: %v", err)
	}

	// Both sides modify the file
	writeTestFile(t, localDir, "both.txt", "local-change")
	writeTestFile(t, remoteDir, "both.txt", "remote-change")

	// Re-register with fresh providers and updated pair config
	eng.RegisterPair(pair, newTestLocalProvider(t, pair.LocalPath), newTestLocalProvider(t, pair.RemotePath))

	// Second sync: both changed, virtual mode should upload local (not create conflict)
	runPairSync(t, eng, pair.ID, "")

	// Remote should have local's content (upload wins)
	assertFileContent(t, remoteDir, "both.txt", "local-change")

	// No new conflicts should be recorded
	conflicts, err := s.ListConflicts(pair.ID, "open")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %d, want 0 (virtual mode should not create conflicts)", len(conflicts))
	}
}
