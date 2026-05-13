package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	syncengine "github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/store"
)

func TestWebSocketAccept(t *testing.T) {
	got := websocketAccept("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("websocketAccept = %q, want %q", got, want)
	}
}

func TestUpdatePairCanChangeNameAndProvider(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	pair := &store.SyncPair{
		Name:             "docs",
		LocalPath:        "/tmp/docs",
		RemotePath:       "/docs",
		Provider:         "old",
		Mode:             "mirror",
		Direction:        "both",
		ConflictStrategy: "latest_wins",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	engine := &fakeEngine{}
	h := New(s, engine, "")
	body, _ := json.Marshal(map[string]any{
		"name":      "docs-renamed",
		"provider":  "alist",
		"direction": "up",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/pairs/1", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.UpdatePair(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	updated, err := s.GetSyncPair(pair.ID)
	if err != nil {
		t.Fatalf("get pair: %v", err)
	}
	if updated.Name != "docs-renamed" || updated.Provider != "alist" || updated.Direction != "up" {
		t.Fatalf("updated pair = %+v", updated)
	}
	if engine.refreshes != 1 {
		t.Fatalf("RefreshPairs calls = %d, want 1", engine.refreshes)
	}
}

func TestListLogsReturnsArray(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	h := New(s, &fakeEngine{}, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?limit=10", nil)
	rec := httptest.NewRecorder()

	h.ListLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal logs: %v", err)
	}
	if got == nil {
		t.Fatalf("logs response is nil, want empty array")
	}
}

func TestDeletePairUnregistersFromEngine(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	pair := &store.SyncPair{
		Name:             "docs",
		LocalPath:        "/tmp/docs",
		RemotePath:       "/docs",
		Provider:         "webdav",
		Mode:             "mirror",
		Direction:        "both",
		ConflictStrategy: "latest_wins",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	engine := &trackingFakeEngine{}
	h := New(s, engine, "")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/pairs/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.DeletePair(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if engine.unregistered != 1 {
		t.Fatalf("UnregisterPair calls = %d, want 1", engine.unregistered)
	}
	if id := engine.lastUnregisteredID; id != pair.ID {
		t.Fatalf("UnregisterPair id = %d, want %d", id, pair.ID)
	}

	// Verify pair is deleted from store.
	got, _ := s.GetSyncPair(pair.ID)
	if got != nil {
		t.Fatalf("pair still exists in store after delete")
	}
}

func TestDeleteProviderRejectsWithDependentPairs(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	pc := &store.ProviderConfig{Name: "my-webdav", Type: "webdav", Params: map[string]string{"endpoint": "http://localhost"}}
	if err := s.CreateProviderConfig(pc); err != nil {
		t.Fatalf("create provider: %v", err)
	}

	pair := &store.SyncPair{
		Name: "docs", LocalPath: "/tmp/docs", RemotePath: "/docs",
		Provider: "my-webdav", Mode: "mirror", Direction: "both", ConflictStrategy: "latest_wins",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	engine := &trackingFakeEngine{}
	h := New(s, engine, "")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.DeleteProvider(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "provider has dependent sync pairs" {
		t.Fatalf("unexpected error: %v", resp["error"])
	}

	// Provider should still exist.
	got, _ := s.GetProviderConfig(pc.ID)
	if got == nil {
		t.Fatalf("provider should not be deleted without force")
	}
}

func TestDeleteProviderForceRemovesDependentPairs(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	pc := &store.ProviderConfig{Name: "my-webdav", Type: "webdav", Params: map[string]string{"endpoint": "http://localhost"}}
	if err := s.CreateProviderConfig(pc); err != nil {
		t.Fatalf("create provider: %v", err)
	}

	pair := &store.SyncPair{
		Name: "docs", LocalPath: "/tmp/docs", RemotePath: "/docs",
		Provider: "my-webdav", Mode: "mirror", Direction: "both", ConflictStrategy: "latest_wins",
	}
	if err := s.CreateSyncPair(pair); err != nil {
		t.Fatalf("create pair: %v", err)
	}

	engine := &trackingFakeEngine{}
	h := New(s, engine, "")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/1?force=true", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.DeleteProvider(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if engine.unregistered != 1 {
		t.Fatalf("UnregisterPair calls = %d, want 1", engine.unregistered)
	}

	// Both provider and pair should be gone.
	gotP, _ := s.GetProviderConfig(pc.ID)
	if gotP != nil {
		t.Fatalf("provider should be deleted")
	}
	gotPair, _ := s.GetSyncPair(pair.ID)
	if gotPair != nil {
		t.Fatalf("pair should be deleted")
	}
}

func TestListLogsReadsFile(t *testing.T) {
	dir := t.TempDir()
	logContent := `{"level":"info","time":"2024-01-01T12:00:00","message":"started","tag":"every-sync"}
{"level":"error","time":"2024-01-01T12:01:00","message":"failed","tag":"every-sync","pair_id":5}
{"level":"info","time":"2024-01-01T12:02:00","message":"done","tag":"every-sync"}
`
	if err := os.WriteFile(filepath.Join(dir, "every-sync.log"), []byte(logContent), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	h := New(s, &trackingFakeEngine{}, dir)

	// Test basic listing.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	rec := httptest.NewRecorder()
	h.ListLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &entries)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Newest first.
	if entries[0]["message"] != "done" {
		t.Fatalf("first entry message = %v, want done", entries[0]["message"])
	}

	// Test level filter.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/logs?level=error", nil)
	rec = httptest.NewRecorder()
	h.ListLogs(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Fatalf("got %d entries with level=error, want 1", len(entries))
	}
	if entries[0]["level"] != "error" {
		t.Fatalf("entry level = %v, want error", entries[0]["level"])
	}

	// Test pair_id filter.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/logs?pair_id=5", nil)
	rec = httptest.NewRecorder()
	h.ListLogs(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Fatalf("got %d entries with pair_id=5, want 1", len(entries))
	}
}

type trackingFakeEngine struct {
	refreshes          int
	unregistered       int32
	lastUnregisteredID int64
	progress           []syncengine.PairProgressSnapshot
}

func (f *trackingFakeEngine) RefreshPairs() error {
	f.refreshes++
	return nil
}

func (f *trackingFakeEngine) RefreshAllPairs() error {
	f.refreshes++
	return nil
}

func (f *trackingFakeEngine) SyncPair(context.Context, int64, string) error { return nil }
func (f *trackingFakeEngine) SyncAll(context.Context) error                 { return nil }
func (f *trackingFakeEngine) UnregisterPair(id int64) {
	atomic.AddInt32(&f.unregistered, 1)
	f.lastUnregisteredID = id
}
func (f *trackingFakeEngine) MaterializeVirtual(context.Context, int64, string) error {
	return nil
}
func (f *trackingFakeEngine) ResolveConflict(context.Context, int64, string) error { return nil }
func (f *trackingFakeEngine) Status() syncengine.Status                            { return syncengine.Status{} }
func (f *trackingFakeEngine) Subscribe(context.Context) <-chan syncengine.Event {
	return make(chan syncengine.Event)
}
func (f *trackingFakeEngine) ListPairFiles(context.Context, int64, string, string) ([]*syncengine.FileListEntry, error) {
	return nil, nil
}
func (f *trackingFakeEngine) Progress() []syncengine.PairProgressSnapshot { return f.progress }

type fakeEngine struct {
	refreshes int
	progress  []syncengine.PairProgressSnapshot
}

func (f *fakeEngine) RefreshPairs() error {
	f.refreshes++
	return nil
}

func (f *fakeEngine) RefreshAllPairs() error {
	f.refreshes++
	return nil
}

func (f *fakeEngine) SyncPair(context.Context, int64, string) error { return nil }
func (f *fakeEngine) SyncAll(context.Context) error                 { return nil }
func (f *fakeEngine) UnregisterPair(int64)                          {}
func (f *fakeEngine) MaterializeVirtual(context.Context, int64, string) error {
	return nil
}
func (f *fakeEngine) ResolveConflict(context.Context, int64, string) error { return nil }
func (f *fakeEngine) Status() syncengine.Status                            { return syncengine.Status{} }
func (f *fakeEngine) Subscribe(context.Context) <-chan syncengine.Event {
	return make(chan syncengine.Event)
}
func (f *fakeEngine) ListPairFiles(context.Context, int64, string, string) ([]*syncengine.FileListEntry, error) {
	return nil, nil
}
func (f *fakeEngine) Progress() []syncengine.PairProgressSnapshot { return f.progress }

func TestProgressReturnsEngineSnapshots(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	engine := &fakeEngine{
		progress: []syncengine.PairProgressSnapshot{{
			PairID:       7,
			PairName:     "photos",
			Status:       "syncing",
			Direction:    "up",
			FilesSynced:  2,
			FilesTotal:   5,
			PendingTasks: 3,
		}},
	}
	h := New(s, engine, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/progress", nil)
	rec := httptest.NewRecorder()

	h.Progress(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []syncengine.PairProgressSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].PairID != 7 || got[0].Status != "syncing" {
		t.Fatalf("progress = %+v", got)
	}
}
