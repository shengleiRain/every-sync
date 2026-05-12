package webdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
