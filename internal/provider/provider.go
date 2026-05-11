package provider

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotSupported   = errors.New("operation not supported by this provider")
	ErrNotFound       = errors.New("file not found")
	ErrAlreadyExists  = errors.New("file already exists")
	ErrPermission     = errors.New("permission denied")
	ErrNetwork        = errors.New("network error")
	ErrAuthentication = errors.New("authentication failed")
)

// FileMeta represents unified file metadata across all storage backends.
type FileMeta struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	ETag    string    `json:"etag,omitempty"`
	Hash    string    `json:"hash,omitempty"`
	IsDir   bool      `json:"is_dir"`
}

// ChangeEvent represents a file change detected by a provider.
type ChangeEvent struct {
	Path      string    `json:"path"`
	Type      EventType `json:"type"`
	Source    string    `json:"source"` // local or remote
	Timestamp time.Time `json:"timestamp"`
	Meta      *FileMeta `json:"meta,omitempty"`
}

type EventType string

const (
	EventCreate EventType = "create"
	EventModify EventType = "modify"
	EventDelete EventType = "delete"
	EventRename EventType = "rename"
)

// Provider is the interface all storage backends must implement.
type Provider interface {
	// Init initializes the provider with the given config.
	Init(ctx context.Context, config Config) error

	// Close releases any resources held by the provider.
	Close() error

	// Name returns the provider type name (e.g. "local", "webdav").
	Name() string

	// GetFile downloads a file, returning its content and metadata.
	GetFile(ctx context.Context, path string) (io.ReadCloser, *FileMeta, error)

	// PutFile uploads a file with the given content and metadata.
	PutFile(ctx context.Context, path string, reader io.Reader, meta *FileMeta) error

	// DeleteFile removes a file at the given path.
	DeleteFile(ctx context.Context, path string) error

	// MoveFile moves a file from src to dst.
	MoveFile(ctx context.Context, src, dst string) error

	// ListDir lists files in a directory.
	ListDir(ctx context.Context, path string) ([]*FileMeta, error)

	// CreateDir creates a directory at the given path.
	CreateDir(ctx context.Context, path string) error

	// Stat returns metadata for a single file without downloading it.
	Stat(ctx context.Context, path string) (*FileMeta, error)

	// WatchChanges returns a channel that emits change events.
	// Providers that don't support watching should return ErrNotSupported.
	WatchChanges(ctx context.Context, path string) (<-chan ChangeEvent, error)

	// GetChangeToken returns a token representing the current state.
	// Can be used to detect remote changes by comparing tokens.
	GetChangeToken(ctx context.Context, path string) (string, error)
}

// IncrementalLister is an optional interface for providers that support
// efficient change detection via ETags or similar tokens.
type IncrementalLister interface {
	// IncrementalList returns directory contents with change detection.
	// If the directory's tag matches cachedTag, returns (nil, true, nil) meaning unchanged.
	// Otherwise returns the directory listing and false.
	IncrementalList(ctx context.Context, path string, cachedTag string) ([]*FileMeta, bool, error)
}

// Capabilities describe optional transport features a provider can expose.
type Capabilities struct {
	RangeRead   bool `json:"range_read"`
	ResumeWrite bool `json:"resume_write"`
}

// CapabilityProvider can report optional transport capabilities.
type CapabilityProvider interface {
	Capabilities() Capabilities
}

// RangeReader can read a byte range from a file.
type RangeReader interface {
	GetFileRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, *FileMeta, error)
}

// ResumeWriter can append/overwrite content at a known offset.
type ResumeWriter interface {
	PutFileResume(ctx context.Context, path string, reader io.Reader, meta *FileMeta, offset int64) error
}

func DetectCapabilities(p Provider) Capabilities {
	if p == nil {
		return Capabilities{}
	}
	if cp, ok := p.(CapabilityProvider); ok {
		return cp.Capabilities()
	}
	_, canRangeRead := p.(RangeReader)
	_, canResumeWrite := p.(ResumeWriter)
	return Capabilities{RangeRead: canRangeRead, ResumeWrite: canResumeWrite}
}

// Config holds provider-specific configuration.
type Config struct {
	Type   string            `yaml:"type" json:"type"`
	Params map[string]string `yaml:"params" json:"params"`
}

// Factory creates a new Provider instance.
type Factory func() Provider

var registry = map[string]Factory{}

// Register adds a provider factory to the global registry.
func Register(name string, factory Factory) {
	registry[name] = factory
}

// Create instantiates a provider by name from the registry.
func Create(name string) (Provider, bool) {
	f, ok := registry[name]
	if !ok {
		return nil, false
	}
	return f(), true
}

// ListRegistered returns all registered provider names.
func ListRegistered() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
