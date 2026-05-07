package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rain/every-sync/internal/provider"

	"github.com/fsnotify/fsnotify"
)

func init() {
	provider.Register("local", func() provider.Provider {
		return &LocalProvider{}
	})
}

type LocalProvider struct {
	rootPath string
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
}

func (l *LocalProvider) Init(_ context.Context, config provider.Config) error {
	rootPath, ok := config.Params["root_path"]
	if !ok || rootPath == "" {
		return fmt.Errorf("local provider: root_path is required")
	}

	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("local provider: invalid root_path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return fmt.Errorf("local provider: create root_path: %w", err)
			}
			info, err = os.Stat(absPath)
		}
		if err != nil {
			return fmt.Errorf("local provider: cannot access root_path: %w", err)
		}
	}
	if !info.IsDir() {
		return fmt.Errorf("local provider: root_path is not a directory")
	}

	l.rootPath = absPath
	return nil
}

func (l *LocalProvider) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.watcher != nil {
		return l.watcher.Close()
	}
	return nil
}

func (l *LocalProvider) Name() string {
	return "local"
}

func (l *LocalProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{RangeRead: true, ResumeWrite: true}
}

func (l *LocalProvider) GetFile(_ context.Context, path string) (io.ReadCloser, *provider.FileMeta, error) {
	fullPath := l.resolve(path)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, provider.ErrNotFound
		}
		return nil, nil, fmt.Errorf("open file: %w", err)
	}

	meta, err := l.fileMeta(fullPath, path)
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	return f, meta, nil
}

func (l *LocalProvider) GetFileRange(_ context.Context, path string, offset, length int64) (io.ReadCloser, *provider.FileMeta, error) {
	fullPath := l.resolve(path)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, provider.ErrNotFound
		}
		return nil, nil, fmt.Errorf("open file range: %w", err)
	}

	meta, err := l.fileMeta(fullPath, path)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if offset > meta.Size {
		f.Close()
		return nil, nil, fmt.Errorf("range offset %d exceeds file size %d", offset, meta.Size)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("seek file range: %w", err)
	}
	if length <= 0 || offset+length > meta.Size {
		length = meta.Size - offset
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(f, length), Closer: f}, meta, nil
}

func (l *LocalProvider) PutFile(_ context.Context, path string, reader io.Reader, _ *provider.FileMeta) error {
	fullPath := l.resolve(path)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (l *LocalProvider) PutFileResume(_ context.Context, path string, reader io.Reader, _ *provider.FileMeta, offset int64) error {
	fullPath := l.resolve(path)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open resumable file: %w", err)
	}
	defer f.Close()

	if offset == 0 {
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("truncate resumable file: %w", err)
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek resumable file: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write resumable file: %w", err)
	}

	return nil
}

func (l *LocalProvider) DeleteFile(_ context.Context, path string) error {
	fullPath := l.resolve(path)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return provider.ErrNotFound
		}
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (l *LocalProvider) MoveFile(_ context.Context, src, dst string) error {
	srcPath := l.resolve(src)
	dstPath := l.resolve(dst)

	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return fmt.Errorf("move file: %w", err)
	}
	return nil
}

func (l *LocalProvider) ListDir(_ context.Context, path string) ([]*provider.FileMeta, error) {
	fullPath := l.resolve(path)

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, provider.ErrNotFound
		}
		return nil, fmt.Errorf("read directory: %w", err)
	}

	result := make([]*provider.FileMeta, 0, len(entries))
	for _, entry := range entries {
		relPath := l.relative(fullPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, &provider.FileMeta{
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		})
	}

	return result, nil
}

func (l *LocalProvider) CreateDir(_ context.Context, path string) error {
	fullPath := l.resolve(path)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return nil
}

func (l *LocalProvider) Stat(_ context.Context, path string) (*provider.FileMeta, error) {
	fullPath := l.resolve(path)
	return l.fileMeta(fullPath, path)
}

func (l *LocalProvider) WatchChanges(ctx context.Context, path string) (<-chan provider.ChangeEvent, error) {
	l.mu.Lock()
	if l.watcher == nil {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			l.mu.Unlock()
			return nil, fmt.Errorf("create watcher: %w", err)
		}
		l.watcher = w
	}
	l.mu.Unlock()

	fullPath := l.resolve(path)
	if err := l.addWatchRecursive(fullPath); err != nil {
		return nil, fmt.Errorf("watch path: %w", err)
	}

	ch := make(chan provider.ChangeEvent, 256)

	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-l.watcher.Events:
				if !ok {
					return
				}
				relPath := l.relative("", event.Name)
				ce := provider.ChangeEvent{
					Path:      relPath,
					Source:    "local",
					Timestamp: time.Now(),
				}

				switch {
				case event.Op&fsnotify.Create == fsnotify.Create:
					ce.Type = provider.EventCreate
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						_ = l.addWatchRecursive(event.Name)
					}
				case event.Op&fsnotify.Write == fsnotify.Write:
					ce.Type = provider.EventModify
				case event.Op&fsnotify.Remove == fsnotify.Remove:
					ce.Type = provider.EventDelete
				case event.Op&fsnotify.Rename == fsnotify.Rename:
					ce.Type = provider.EventRename
				default:
					continue
				}

				select {
				case ch <- ce:
				case <-ctx.Done():
					return
				}

			case _, ok := <-l.watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return ch, nil
}

func (l *LocalProvider) GetChangeToken(_ context.Context, path string) (string, error) {
	fullPath := l.resolve(path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", provider.ErrNotFound
	}
	return info.ModTime().Format(time.RFC3339Nano), nil
}

func (l *LocalProvider) resolve(path string) string {
	cleaned := filepath.Clean(strings.TrimPrefix(path, "/"))
	return filepath.Join(l.rootPath, cleaned)
}

func (l *LocalProvider) addWatchRecursive(root string) error {
	l.mu.Lock()
	watcher := l.watcher
	l.mu.Unlock()
	if watcher == nil {
		return nil
	}

	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if err := watcher.Add(p); err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
		return nil
	})
}

func (l *LocalProvider) relative(base, name string) string {
	joined := filepath.Join(base, name)
	rel, err := filepath.Rel(l.rootPath, joined)
	if err != nil {
		return joined
	}
	return "/" + rel
}

func (l *LocalProvider) fileMeta(fullPath, displayPath string) (*provider.FileMeta, error) {
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, provider.ErrNotFound
		}
		return nil, fmt.Errorf("stat file: %w", err)
	}

	return &provider.FileMeta{
		Path:    displayPath,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}, nil
}
