package webdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rain/every-sync/internal/provider"
	"github.com/studio-b12/gowebdav"
)

func TestPutFileStreamsKnownLengthWithoutPrebuffering(t *testing.T) {
	requestStarted := make(chan struct{})
	unblockReader := make(chan struct{})
	var once sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		once.Do(func() { close(requestStarted) })
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	p := &WebDAVProvider{client: gowebdav.NewClient(server.URL, "", "")}
	reader := &blockingEOFReader{
		data:  []byte("streamed-content"),
		gate:  unblockReader,
		delay: 2 * time.Second,
	}

	done := make(chan error, 1)
	go func() {
		done <- p.PutFile(context.Background(), "/large.bin", reader, &provider.FileMeta{
			Size:    int64(len(reader.data)),
			ModTime: time.Now(),
		})
	}()

	select {
	case <-requestStarted:
	case <-time.After(250 * time.Millisecond):
		close(unblockReader)
		err := <-done
		t.Fatalf("PUT request did not start until the reader reached EOF; err=%v", err)
	}

	close(unblockReader)
	if err := <-done; err != nil {
		t.Fatalf("PutFile: %v", err)
	}
}

func TestWebDAVProviderInitAcceptsTimeoutParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodOptions:
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			time.Sleep(150 * time.Millisecond)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	p := &WebDAVProvider{}
	if err := p.Init(context.Background(), provider.Config{Params: map[string]string{
		"endpoint": server.URL,
		"timeout":  "50ms",
	}}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := p.PutFile(context.Background(), "/slow.txt", strings.NewReader("slow"), &provider.FileMeta{Size: 4})
	if err == nil {
		t.Fatal("PutFile unexpectedly succeeded despite configured timeout")
	}
}

func TestPutFileSerializesConcurrentParentDirectoryCreation(t *testing.T) {
	firstStarted := make(chan struct{})
	var firstStartedOnce sync.Once
	var active int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			if strings.TrimSuffix(r.URL.Path, "/") == "/nested/dir" {
				firstStartedOnce.Do(func() { close(firstStarted) })
				if atomic.AddInt32(&active, 1) > 1 {
					atomic.AddInt32(&active, -1)
					w.WriteHeader(http.StatusLocked)
					return
				}
				time.Sleep(100 * time.Millisecond)
				atomic.AddInt32(&active, -1)
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	p := &WebDAVProvider{client: gowebdav.NewClient(server.URL, "", "")}
	meta := &provider.FileMeta{Size: int64(len("content"))}

	errs := make(chan error, 2)
	go func() {
		errs <- p.PutFile(context.Background(), "/nested/dir/one.txt", strings.NewReader("content"), meta)
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first MKCOL did not start")
	}
	go func() {
		errs <- p.PutFile(context.Background(), "/nested/dir/two.txt", strings.NewReader("content"), meta)
	}()

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("PutFile %d: %v", i+1, err)
		}
	}
}

func TestPutFileRetriesLockedParentDirectoryCreation(t *testing.T) {
	var mkdirs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			if strings.TrimSuffix(r.URL.Path, "/") == "/locked/dir" {
				mkdirs++
				if mkdirs == 1 {
					w.WriteHeader(http.StatusLocked)
					return
				}
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	p := &WebDAVProvider{client: gowebdav.NewClient(server.URL, "", "")}
	meta := &provider.FileMeta{Size: int64(len("content"))}

	if err := p.PutFile(context.Background(), "/locked/dir/file.txt", strings.NewReader("content"), meta); err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	if mkdirs != 2 {
		t.Fatalf("MKCOL /locked/dir count = %d, want 2", mkdirs)
	}
}

type blockingEOFReader struct {
	data  []byte
	gate  <-chan struct{}
	delay time.Duration
	sent  bool
}

func (r *blockingEOFReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.data), nil
	}
	select {
	case <-r.gate:
	case <-time.After(r.delay):
		return 0, io.ErrUnexpectedEOF
	}
	return 0, io.EOF
}
