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
