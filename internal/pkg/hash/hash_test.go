package hash

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileHash verifies that the same file always produces the same hash and
// that different files produce different hashes.
func TestFileHash(t *testing.T) {
	dir := t.TempDir()

	content := []byte("hello every-sync")
	f1 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f1, content, 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := FileHash(f1)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// Same file must produce the same hash.
	h2, err := FileHash(f1)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("same file produced different hashes: %s vs %s", h1, h2)
	}

	// Different content must produce a different hash.
	f2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(f2, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}
	h3, err := FileHash(f2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatal("different files produced the same hash")
	}
}

// TestQuickHash_LargeFile verifies that QuickHash detects changes at the head
// and tail of a large file but does NOT detect changes in the middle.
func TestQuickHash_LargeFile(t *testing.T) {
	dir := t.TempDir()

	// Create a file larger than 2MB (> quickHashThreshold).
	// Layout: 1.5MB of 'A' + 1MB of 'B' + 1.5MB of 'C' = 4MB total.
	head := make([]byte, 1536*1024)   // 1.5MB
	middle := make([]byte, 1024*1024)  // 1MB
	tail := make([]byte, 1536*1024)    // 1.5MB
	for i := range head {
		head[i] = 'A'
	}
	for i := range middle {
		middle[i] = 'B'
	}
	for i := range tail {
		tail[i] = 'C'
	}
	original := append(append(head, middle...), tail...)

	f1 := filepath.Join(dir, "large1.bin")
	if err := os.WriteFile(f1, original, 0644); err != nil {
		t.Fatal(err)
	}

	origHash, err := QuickHash(f1)
	if err != nil {
		t.Fatal(err)
	}

	// Change the head: should be detected.
	modified := make([]byte, len(original))
	copy(modified, original)
	modified[0] = 'Z'
	f2 := filepath.Join(dir, "large2.bin")
	if err := os.WriteFile(f2, modified, 0644); err != nil {
		t.Fatal(err)
	}
	h, err := QuickHash(f2)
	if err != nil {
		t.Fatal(err)
	}
	if h == origHash {
		t.Fatal("QuickHash failed to detect head change")
	}

	// Change the tail: should be detected.
	modified2 := make([]byte, len(original))
	copy(modified2, original)
	modified2[len(modified2)-1] = 'Z'
	f3 := filepath.Join(dir, "large3.bin")
	if err := os.WriteFile(f3, modified2, 0644); err != nil {
		t.Fatal(err)
	}
	h, err = QuickHash(f3)
	if err != nil {
		t.Fatal(err)
	}
	if h == origHash {
		t.Fatal("QuickHash failed to detect tail change")
	}

	// Change only the middle: should NOT be detected by quick hash.
	modified3 := make([]byte, len(original))
	copy(modified3, original)
	midStart := len(head)
	modified3[midStart+512*1024] = 'X' // change a byte in the middle region
	f4 := filepath.Join(dir, "large4.bin")
	if err := os.WriteFile(f4, modified3, 0644); err != nil {
		t.Fatal(err)
	}
	h, err = QuickHash(f4)
	if err != nil {
		t.Fatal(err)
	}
	if h != origHash {
		t.Fatal("QuickHash should NOT detect middle-only change, but it did")
	}
}

// TestQuickHash_SmallFile verifies that files <= 2MB use full hashing via
// delegation to FileHash.
func TestQuickHash_SmallFile(t *testing.T) {
	dir := t.TempDir()

	content := []byte("small file content")
	f1 := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(f1, content, 0644); err != nil {
		t.Fatal(err)
	}

	quickHash, err := QuickHash(f1)
	if err != nil {
		t.Fatal(err)
	}

	fullHash, err := FileHash(f1)
	if err != nil {
		t.Fatal(err)
	}

	// For small files, QuickHash should delegate to FileHash so they must match.
	if quickHash != fullHash {
		t.Fatalf("QuickHash and FileHash mismatch for small file: %s vs %s", quickHash, fullHash)
	}
}

// TestContentEqual verifies that identical files return true and different files
// return false.
func TestContentEqual(t *testing.T) {
	dir := t.TempDir()

	content := []byte("check equality")
	f1 := filepath.Join(dir, "same1.txt")
	f2 := filepath.Join(dir, "same2.txt")
	f3 := filepath.Join(dir, "diff.txt")

	if err := os.WriteFile(f1, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f3, []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}

	eq, err := ContentEqual(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if !eq {
		t.Fatal("identical files reported as not equal")
	}

	eq, err = ContentEqual(f1, f3)
	if err != nil {
		t.Fatal(err)
	}
	if eq {
		t.Fatal("different files reported as equal")
	}
}

// TestFileHash_EmptyFile verifies that an empty file produces a valid hash.
func TestFileHash_EmptyFile(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(f, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	h, err := FileHash(f)
	if err != nil {
		t.Fatal(err)
	}
	if h == "" {
		t.Fatal("expected non-empty hash for empty file")
	}

	// Empty file hash should be consistent.
	h2, err := FileHash(f)
	if err != nil {
		t.Fatal(err)
	}
	if h != h2 {
		t.Fatalf("inconsistent hash for empty file: %s vs %s", h, h2)
	}
}
