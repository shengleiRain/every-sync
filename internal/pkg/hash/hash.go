// Package hash provides file content hashing using xxhash for sync conflict detection.
// For large files (>2MB), QuickHash reads only the head and tail portions for speed.
package hash

import (
	"encoding/hex"
	"io"
	"os"

	"github.com/cespare/xxhash/v2"
)

const (
	// quickHashThreshold is the file size above which QuickHash uses sampling.
	quickHashThreshold = 2 * 1024 * 1024 // 2MB

	// chunkSize is the amount read from head and tail for quick hashing.
	chunkSize = 1024 * 1024 // 1MB
)

// FileHash computes the full xxhash of a file's contents and returns it as a
// hex-encoded string.
func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// QuickHash computes a fast hash for large files by reading only the head 1MB,
// tail 1MB, and file size. For files <= 2MB it delegates to FileHash for a full
// content hash.
func QuickHash(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.Size() <= quickHashThreshold {
		return FileHash(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := xxhash.New()

	// Encode file size first so that files with same head/tail but different
	// sizes produce different hashes.
	if _, err := h.Write([]byte{byte(info.Size())}); err != nil {
		return "", err
	}

	// Read head (first 1MB).
	head := make([]byte, chunkSize)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	if _, err := h.Write(head[:n]); err != nil {
		return "", err
	}

	// Seek to tail position: file_size - chunkSize (but not before 0).
	tailOffset := info.Size() - chunkSize
	if tailOffset < 0 {
		tailOffset = 0
	}
	if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
		return "", err
	}

	// Read tail.
	tail := make([]byte, chunkSize)
	n, err = io.ReadFull(f, tail)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	if _, err := h.Write(tail[:n]); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ContentEqual compares two files by hash and returns true if their contents are
// identical.
func ContentEqual(p1, p2 string) (bool, error) {
	h1, err := FileHash(p1)
	if err != nil {
		return false, err
	}
	h2, err := FileHash(p2)
	if err != nil {
		return false, err
	}
	return h1 == h2, nil
}
