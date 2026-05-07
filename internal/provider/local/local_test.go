package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rain/every-sync/internal/provider"
)

func setupTestProvider(t *testing.T) (*LocalProvider, string) {
	t.Helper()
	root := t.TempDir()
	p := &LocalProvider{}
	err := p.Init(context.Background(), provider.Config{
		Type:   "local",
		Params: map[string]string{"root_path": root},
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p, root
}

func TestLocalProvider_Init(t *testing.T) {
	p := &LocalProvider{}

	// Missing root_path
	err := p.Init(context.Background(), provider.Config{Params: map[string]string{}})
	if err == nil {
		t.Fatal("expected error for missing root_path")
	}

	// Valid path
	tmp := t.TempDir()
	err = p.Init(context.Background(), provider.Config{
		Params: map[string]string{"root_path": tmp},
	})
	if err != nil {
		t.Fatalf("Init with valid path: %v", err)
	}
	p.Close()

	// Non-existent path should be created so down/both sync can materialize a missing local root.
	missing := filepath.Join(t.TempDir(), "new-root")
	err = p.Init(context.Background(), provider.Config{
		Params: map[string]string{"root_path": missing},
	})
	if err != nil {
		t.Fatalf("Init should create missing path: %v", err)
	}
	if info, err := os.Stat(missing); err != nil || !info.IsDir() {
		t.Fatalf("missing path was not created as directory: info=%v err=%v", info, err)
	}
}

func TestLocalProvider_PutAndGetFile(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	content := "hello world"
	err := p.PutFile(ctx, "/test.txt", strings.NewReader(content), nil)
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	reader, meta, err := p.GetFile(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, len(content))
	n, _ := reader.Read(buf)
	if string(buf[:n]) != content {
		t.Fatalf("content mismatch: got %q, want %q", string(buf[:n]), content)
	}

	if meta.IsDir {
		t.Fatal("expected file, got dir")
	}
	if meta.Size != int64(len(content)) {
		t.Fatalf("size mismatch: got %d, want %d", meta.Size, len(content))
	}
}

func TestLocalProvider_RangeReadAndResumeWrite(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	if err := p.PutFile(ctx, "/range.txt", strings.NewReader("0123456789"), nil); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	reader, meta, err := p.GetFileRange(ctx, "/range.txt", 4, 3)
	if err != nil {
		t.Fatalf("GetFileRange: %v", err)
	}
	buf := make([]byte, 3)
	n, err := reader.Read(buf)
	reader.Close()
	if n != 3 || string(buf) != "456" || err != nil {
		t.Fatalf("range read = n:%d buf:%q err:%v", n, string(buf), err)
	}
	if meta.Size != 10 {
		t.Fatalf("range meta size = %d, want 10", meta.Size)
	}

	if err := p.PutFileResume(ctx, "/resume.txt", strings.NewReader("0123"), nil, 0); err != nil {
		t.Fatalf("PutFileResume first: %v", err)
	}
	if err := p.PutFileResume(ctx, "/resume.txt", strings.NewReader("456789"), nil, 4); err != nil {
		t.Fatalf("PutFileResume second: %v", err)
	}
	got, _, err := p.GetFile(ctx, "/resume.txt")
	if err != nil {
		t.Fatalf("GetFile resume: %v", err)
	}
	defer got.Close()
	data, err := io.ReadAll(got)
	if err != nil {
		t.Fatalf("read resume: %v", err)
	}
	if string(data) != "0123456789" {
		t.Fatalf("resume content = %q, want 0123456789", string(data))
	}
}

func TestLocalProvider_DeleteFile(t *testing.T) {
	p, root := setupTestProvider(t)
	ctx := context.Background()

	p.PutFile(ctx, "/del.txt", strings.NewReader("delete me"), nil)
	err := p.DeleteFile(ctx, "/del.txt")
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if _, _, err := p.GetFile(ctx, "/del.txt"); err != provider.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// Delete non-existent
	if err := p.DeleteFile(ctx, "/nope.txt"); err != provider.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing file, got: %v", err)
	}

	_ = root
}

func TestLocalProvider_MoveFile(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	p.PutFile(ctx, "/src.txt", strings.NewReader("move me"), nil)
	err := p.MoveFile(ctx, "/src.txt", "/dst.txt")
	if err != nil {
		t.Fatalf("MoveFile: %v", err)
	}

	if _, _, err := p.GetFile(ctx, "/src.txt"); err != provider.ErrNotFound {
		t.Fatal("src should not exist after move")
	}

	reader, _, err := p.GetFile(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("dst should exist: %v", err)
	}
	reader.Close()
}

func TestLocalProvider_ListDir(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	p.PutFile(ctx, "/a.txt", strings.NewReader("a"), nil)
	p.PutFile(ctx, "/b.txt", strings.NewReader("bb"), nil)
	p.CreateDir(ctx, "/subdir")
	p.PutFile(ctx, "/subdir/c.txt", strings.NewReader("ccc"), nil)

	entries, err := p.ListDir(ctx, "/")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	foundFiles := 0
	foundDir := false
	for _, e := range entries {
		if e.IsDir && filepath.Base(e.Path) == "subdir" {
			foundDir = true
		}
		if !e.IsDir {
			foundFiles++
		}
	}

	if foundFiles != 2 {
		t.Fatalf("expected 2 files, got %d", foundFiles)
	}
	if !foundDir {
		t.Fatal("subdir not found")
	}
}

func TestLocalProvider_Stat(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	p.PutFile(ctx, "/stat.txt", strings.NewReader("stat test"), nil)

	meta, err := p.Stat(ctx, "/stat.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if meta.IsDir {
		t.Fatal("expected file")
	}
	if meta.Size != 9 {
		t.Fatalf("size: got %d, want 9", meta.Size)
	}

	// Non-existent
	_, err = p.Stat(ctx, "/nope.txt")
	if err != provider.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLocalProvider_CreateDir(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	err := p.CreateDir(ctx, "/new/nested/dir")
	if err != nil {
		t.Fatalf("CreateDir nested: %v", err)
	}

	meta, err := p.Stat(ctx, "/new/nested/dir")
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if !meta.IsDir {
		t.Fatal("expected dir")
	}
}

func TestLocalProvider_WatchChanges(t *testing.T) {
	p, root := setupTestProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := p.WatchChanges(ctx, "/")
	if err != nil {
		t.Fatalf("WatchChanges: %v", err)
	}

	// Write a file to trigger event
	os.WriteFile(filepath.Join(root, "watched.txt"), []byte("test"), 0644)

	select {
	case event := <-ch:
		if event.Type != provider.EventCreate && event.Type != provider.EventModify {
			t.Fatalf("unexpected event type: %s", event.Type)
		}
		if event.Source != "local" {
			t.Fatalf("expected source 'local', got %q", event.Source)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func TestLocalProvider_GetChangeToken(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	token1, err := p.GetChangeToken(ctx, "/")
	if err != nil {
		t.Fatalf("GetChangeToken: %v", err)
	}
	if token1 == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestLocalProvider_NestedDirectories(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	// PutFile should create parent directories automatically
	err := p.PutFile(ctx, "/deep/nested/path/file.txt", strings.NewReader("deep"), nil)
	if err != nil {
		t.Fatalf("PutFile nested: %v", err)
	}

	reader, _, err := p.GetFile(ctx, "/deep/nested/path/file.txt")
	if err != nil {
		t.Fatalf("GetFile nested: %v", err)
	}
	reader.Close()
}
