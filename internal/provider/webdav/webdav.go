package webdav

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/rain/every-sync/internal/provider"

	"github.com/studio-b12/gowebdav"
)

func init() {
	provider.Register("webdav", func() provider.Provider {
		return &WebDAVProvider{}
	})
}

type WebDAVProvider struct {
	client *gowebdav.Client
	prefix string
}

func (w *WebDAVProvider) Init(_ context.Context, config provider.Config) error {
	endpoint, ok := config.Params["endpoint"]
	if !ok || endpoint == "" {
		return fmt.Errorf("webdav provider: endpoint is required")
	}

	username := config.Params["username"]
	password := config.Params["password"]
	w.prefix = strings.TrimSuffix(config.Params["prefix"], "/")

	client := gowebdav.NewClient(endpoint, username, password)

	// Set reasonable timeouts
	client.SetTimeout(30 * time.Second)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx

	if err := client.Connect(); err != nil {
		return fmt.Errorf("webdav provider: connection failed: %w", err)
	}

	w.client = client
	return nil
}

func (w *WebDAVProvider) Close() error {
	return nil
}

func (w *WebDAVProvider) Name() string {
	return "webdav"
}

func (w *WebDAVProvider) GetFile(_ context.Context, remotePath string) (io.ReadCloser, *provider.FileMeta, error) {
	fullPath := w.resolve(remotePath)

	meta, err := w.Stat(context.Background(), remotePath)
	if err != nil {
		return nil, nil, err
	}

	reader, err := w.client.ReadStream(fullPath)
	if err != nil {
		return nil, nil, w.mapError(err)
	}

	return reader, meta, nil
}

func (w *WebDAVProvider) PutFile(_ context.Context, remotePath string, reader io.Reader, _ *provider.FileMeta) error {
	fullPath := w.resolve(remotePath)

	// Ensure parent directory exists
	dir := path.Dir(fullPath)
	if err := w.client.MkdirAll(dir, 0755); err != nil {
		return w.mapError(err)
	}

	if err := w.client.WriteStream(fullPath, reader, 0644); err != nil {
		return fmt.Errorf("webdav upload: %w", err)
	}

	return nil
}

func (w *WebDAVProvider) DeleteFile(_ context.Context, remotePath string) error {
	fullPath := w.resolve(remotePath)
	if err := w.client.Remove(fullPath); err != nil {
		return w.mapError(err)
	}
	return nil
}

func (w *WebDAVProvider) MoveFile(_ context.Context, src, dst string) error {
	srcPath := w.resolve(src)
	dstPath := w.resolve(dst)

	// Ensure target directory exists
	dir := path.Dir(dstPath)
	if err := w.client.MkdirAll(dir, 0755); err != nil {
		return w.mapError(err)
	}

	if err := w.client.Rename(srcPath, dstPath, true); err != nil {
		return w.mapError(err)
	}
	return nil
}

func (w *WebDAVProvider) ListDir(_ context.Context, remotePath string) ([]*provider.FileMeta, error) {
	fullPath := w.resolve(remotePath)

	files, err := w.client.ReadDir(fullPath)
	if err != nil {
		return nil, w.mapError(err)
	}

	result := make([]*provider.FileMeta, 0, len(files))
	for _, f := range files {
		relPath := w.relative(fullPath, f.Name())
		result = append(result, &provider.FileMeta{
			Path:    relPath,
			Size:    f.Size(),
			ModTime: f.ModTime(),
			IsDir:   f.IsDir(),
		})
	}

	return result, nil
}

func (w *WebDAVProvider) CreateDir(_ context.Context, remotePath string) error {
	fullPath := w.resolve(remotePath)
	if err := w.client.MkdirAll(fullPath, 0755); err != nil {
		return w.mapError(err)
	}
	return nil
}

func (w *WebDAVProvider) Stat(_ context.Context, remotePath string) (*provider.FileMeta, error) {
	fullPath := w.resolve(remotePath)

	stat, err := w.client.Stat(fullPath)
	if err != nil {
		return nil, w.mapError(err)
	}

	return &provider.FileMeta{
		Path:    remotePath,
		Size:    stat.Size(),
		ModTime: stat.ModTime(),
		ETag:    stat.ModTime().Format(time.RFC3339Nano),
		IsDir:   stat.IsDir(),
	}, nil
}

func (w *WebDAVProvider) WatchChanges(_ context.Context, _ string) (<-chan provider.ChangeEvent, error) {
	return nil, provider.ErrNotSupported
}

func (w *WebDAVProvider) GetChangeToken(_ context.Context, remotePath string) (string, error) {
	fullPath := w.resolve(remotePath)

	stat, err := w.client.Stat(fullPath)
	if err != nil {
		return "", w.mapError(err)
	}

	return stat.ModTime().Format(time.RFC3339Nano), nil
}

func (w *WebDAVProvider) resolve(remotePath string) string {
	cleaned := path.Clean(remotePath)
	if w.prefix != "" {
		return w.prefix + cleaned
	}
	return cleaned
}

func (w *WebDAVProvider) relative(base, name string) string {
	joined := path.Join(base, name)
	if w.prefix != "" {
		rel := strings.TrimPrefix(joined, w.prefix)
		if rel == "" {
			rel = "/"
		}
		return rel
	}
	return joined
}

func (w *WebDAVProvider) mapError(err error) error {
	if err == nil {
		return nil
	}

	// gowebdav wraps StatusError inside os.PathError
	if pathErr, ok := err.(*os.PathError); ok {
		if statusErr, ok := pathErr.Err.(gowebdav.StatusError); ok {
			switch statusErr.Status {
			case 404:
				return provider.ErrNotFound
			case 409:
				return provider.ErrNotFound
			case 401:
				return provider.ErrAuthentication
			case 403:
				return provider.ErrPermission
			}
		}
	}

	// Also check direct StatusError
	if statusErr, ok := err.(gowebdav.StatusError); ok {
		switch statusErr.Status {
		case 404:
			return provider.ErrNotFound
		case 409:
			return provider.ErrNotFound
		case 401:
			return provider.ErrAuthentication
		case 403:
			return provider.ErrPermission
		}
	}

	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "no such host") {
		return provider.ErrNetwork
	}

	return fmt.Errorf("webdav: %w", err)
}
