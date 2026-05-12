package webdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

	client := newClient(endpoint, username, password, config.Params["auth_mode"])

	if timeoutValue := strings.TrimSpace(config.Params["timeout"]); timeoutValue != "" {
		timeout, err := time.ParseDuration(timeoutValue)
		if err != nil {
			return fmt.Errorf("webdav provider: invalid timeout %q: %w", timeoutValue, err)
		}
		client.SetTimeout(timeout)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx

	if err := client.Connect(); err != nil {
		return fmt.Errorf("webdav provider: connection failed: %w", err)
	}

	w.client = client
	if w.prefix != "" {
		if err := w.client.MkdirAll(w.prefix, 0755); err != nil {
			return fmt.Errorf("webdav provider: create prefix %s: %w", w.prefix, w.mapError(err))
		}
	}
	return nil
}

func (w *WebDAVProvider) Close() error {
	return nil
}

func (w *WebDAVProvider) Name() string {
	return "webdav"
}

func (w *WebDAVProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{RangeRead: true, ResumeWrite: false}
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

func (w *WebDAVProvider) GetFileRange(_ context.Context, remotePath string, offset, length int64) (io.ReadCloser, *provider.FileMeta, error) {
	fullPath := w.resolve(remotePath)

	meta, err := w.Stat(context.Background(), remotePath)
	if err != nil {
		return nil, nil, err
	}
	if offset > meta.Size {
		return nil, nil, fmt.Errorf("range offset %d exceeds file size %d", offset, meta.Size)
	}
	if length <= 0 || offset+length > meta.Size {
		length = meta.Size - offset
	}

	reader, err := w.client.ReadStreamRange(fullPath, offset, length)
	if err != nil {
		return nil, nil, w.mapError(err)
	}

	return reader, meta, nil
}

func (w *WebDAVProvider) PutFile(_ context.Context, remotePath string, reader io.Reader, meta *provider.FileMeta) error {
	fullPath := w.resolve(remotePath)

	// Ensure parent directory exists
	dir := path.Dir(fullPath)
	if dir != "/" && dir != "." {
		if err := w.client.MkdirAll(dir, 0755); err != nil {
			return w.mapError(err)
		}
	}

	var err error
	if meta != nil && meta.Size >= 0 {
		err = w.client.WriteStreamWithLength(fullPath, reader, meta.Size, 0644)
	} else {
		err = w.client.WriteStream(fullPath, reader, 0644)
	}
	if err != nil {
		mapped := w.mapError(err)
		if errorsIsProvider(mapped) {
			return mapped
		}
		return fmt.Errorf("webdav upload: %w", mapped)
	}

	return nil
}

func newClient(endpoint, username, password, authMode string) *gowebdav.Client {
	if strings.EqualFold(strings.TrimSpace(authMode), "auto") || (username == "" && password == "") {
		return gowebdav.NewClient(endpoint, username, password)
	}
	return gowebdav.NewAuthClient(endpoint, basicAuthorizer{username: username, password: password})
}

type basicAuthorizer struct {
	username string
	password string
}

func (a basicAuthorizer) NewAuthenticator(body io.Reader) (gowebdav.Authenticator, io.Reader) {
	return basicAuthenticator{username: a.username, password: a.password}, body
}

func (a basicAuthorizer) AddAuthenticator(string, gowebdav.AuthFactory) {}

type basicAuthenticator struct {
	username string
	password string
}

func (a basicAuthenticator) Authorize(_ *http.Client, req *http.Request, _ string) error {
	req.SetBasicAuth(a.username, a.password)
	return nil
}

func (a basicAuthenticator) Verify(_ *http.Client, resp *http.Response, path string) (bool, error) {
	if resp.StatusCode == http.StatusUnauthorized {
		return false, gowebdav.NewPathError("Authorize", path, resp.StatusCode)
	}
	return false, nil
}

func (a basicAuthenticator) Clone() gowebdav.Authenticator {
	return a
}

func (a basicAuthenticator) Close() error {
	return nil
}

func errorsIsProvider(err error) bool {
	return err == provider.ErrNotFound ||
		err == provider.ErrAlreadyExists ||
		err == provider.ErrPermission ||
		err == provider.ErrNetwork ||
		err == provider.ErrAuthentication
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

// IncrementalList checks whether a directory has changed since the last scan.
// If the directory's ModTime (used as pseudo-ETag) matches cachedTag, it returns
// (nil, true, nil) indicating no change. Otherwise it lists the directory contents.
func (w *WebDAVProvider) IncrementalList(_ context.Context, remotePath string, cachedTag string) ([]*provider.FileMeta, bool, error) {
	// Stat the directory itself to get its current "tag" (ModTime)
	meta, err := w.Stat(context.Background(), remotePath)
	if err != nil {
		return nil, false, err
	}

	currentTag := meta.ModTime.Format(time.RFC3339Nano)
	if cachedTag != "" && currentTag == cachedTag {
		return nil, true, nil // unchanged
	}

	// Changed (or first scan) -> list contents
	files, err := w.ListDir(context.Background(), remotePath)
	if err != nil {
		return nil, false, err
	}

	return files, false, nil
}

func (w *WebDAVProvider) resolve(remotePath string) string {
	cleaned := path.Clean(remotePath)
	if w.prefix != "" {
		if cleaned == "/" {
			return w.prefix
		}
		return path.Join(w.prefix, cleaned)
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

	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "connection refused") ||
		strings.Contains(errText, "timeout") ||
		strings.Contains(errText, "deadline exceeded") ||
		strings.Contains(errText, "no such host") {
		return provider.ErrNetwork
	}

	return fmt.Errorf("webdav: %w", err)
}
