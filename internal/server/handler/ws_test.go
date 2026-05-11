package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	h := New(s, engine)
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

type fakeEngine struct {
	refreshes int
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
