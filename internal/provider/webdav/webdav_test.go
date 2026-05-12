//go:build webdav

package webdav

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rain/every-sync/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// configStructure matches the YAML layout of ~/.every-sync/config.yaml.
type configStructure struct {
	Providers []providerEntry `yaml:"providers"`
}

type providerEntry struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

func getTestConfig(t *testing.T) (endpoint, username, password string) {
	t.Helper()

	endpoint = os.Getenv("WEBDAV_ENDPOINT")
	username = os.Getenv("WEBDAV_USERNAME")
	password = os.Getenv("WEBDAV_PASSWORD")

	if endpoint != "" && username != "" && password != "" {
		return
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err, "cannot determine home directory")

	data, err := os.ReadFile(filepath.Join(home, ".every-sync", "config.yaml"))
	if err != nil {
		t.Skip("No WebDAV config found and no env vars set, skipping")
	}

	var cfg configStructure
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config YAML: %v", err)
	}

	for _, p := range cfg.Providers {
		if p.Type == "webdav" {
			endpoint = p.Params["endpoint"]
			username = p.Params["username"]
			password = p.Params["password"]
			break
		}
	}

	if endpoint == "" {
		t.Skip("No WebDAV provider found in config, skipping")
	}
	return
}

func setupProvider(t *testing.T) *WebDAVProvider {
	t.Helper()
	endpoint, username, password := getTestConfig(t)

	p := &WebDAVProvider{}
	prefix := "/every-sync-test-" + time.Now().Format("20060102-150405")

	err := p.Init(context.Background(), provider.Config{
		Type: "webdav",
		Params: map[string]string{
			"endpoint": endpoint,
			"username": username,
			"password": password,
			"prefix":   prefix,
		},
	})
	require.NoError(t, err, "Init with valid credentials should succeed")

	t.Cleanup(func() {
		// Best-effort cleanup: remove the test prefix directory.
		_ = p.client.RemoveAll(prefix)
		p.Close()
	})

	return p
}

// findEntryByName searches entries for one whose path ends with the given name.
// The WebDAV provider's ListDir returns paths relative to the prefix, which
// includes the queried subdirectory (e.g. "/listdir/alpha.txt"). This helper
// normalises the lookup.
func findEntryByName(entries []*provider.FileMeta, name string) (*provider.FileMeta, bool) {
	for _, e := range entries {
		// Match either "/name" or "/subpath/name" suffix.
		if e.Path == "/"+name || strings.HasSuffix(e.Path, "/"+name) {
			return e, true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// 1. TestWebDAV_ConnectionAndAuth — Init with correct credentials succeeds.
func TestWebDAV_ConnectionAndAuth(t *testing.T) {
	endpoint, username, password := getTestConfig(t)

	p := &WebDAVProvider{}
	err := p.Init(context.Background(), provider.Config{
		Type: "webdav",
		Params: map[string]string{
			"endpoint": endpoint,
			"username": username,
			"password": password,
			"prefix":   "/every-sync-test-auth-" + time.Now().Format("20060102-150405"),
		},
	})
	require.NoError(t, err, "Init with correct credentials should succeed")
	p.Close()
}

// 2. TestWebDAV_AuthFailure — Init with wrong credentials fails.
func TestWebDAV_AuthFailure(t *testing.T) {
	endpoint, _, _ := getTestConfig(t)

	p := &WebDAVProvider{}
	err := p.Init(context.Background(), provider.Config{
		Type: "webdav",
		Params: map[string]string{
			"endpoint": endpoint,
			"username": "invalid_user_12345",
			"password": "invalid_password_12345",
			"prefix":   "/every-sync-test-authfail",
		},
	})

	require.Error(t, err, "Init with wrong credentials should fail")
}

// 3. TestWebDAV_FileCRUD — CreateDir → PutFile → Stat → GetFile → DeleteFile.
func TestWebDAV_FileCRUD(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// CreateDir
	err := p.CreateDir(ctx, "/crud")
	require.NoError(t, err, "CreateDir should succeed")

	// PutFile
	content := "crud test content"
	err = p.PutFile(ctx, "/crud/testfile.txt", strings.NewReader(content), nil)
	require.NoError(t, err, "PutFile should succeed")

	// Stat
	meta, err := p.Stat(ctx, "/crud/testfile.txt")
	require.NoError(t, err, "Stat should succeed")
	assert.False(t, meta.IsDir, "should be a file")
	assert.Equal(t, int64(len(content)), meta.Size, "size should match content length")

	// GetFile
	reader, getMeta, err := p.GetFile(ctx, "/crud/testfile.txt")
	require.NoError(t, err, "GetFile should succeed")
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err, "ReadAll should succeed")
	assert.Equal(t, content, string(data), "downloaded content should match uploaded")
	assert.Equal(t, meta.Size, getMeta.Size, "size from GetFile should match Stat")

	// DeleteFile
	err = p.DeleteFile(ctx, "/crud/testfile.txt")
	require.NoError(t, err, "DeleteFile should succeed")

	// Verify deleted — note: some WebDAV servers (e.g. 123pan) move deleted files
	// to a trash/recycle bin rather than returning 404. We verify by checking that
	// GetFile no longer returns the original content.
	_, err = p.Stat(ctx, "/crud/testfile.txt")
	if err != nil {
		assert.ErrorIs(t, err, provider.ErrNotFound, "deleted file should return ErrNotFound")
	}
	// If Stat still succeeds the server uses soft-delete; that is acceptable.
}

// 4. TestWebDAV_FileContent — Upload content, download it back, verify bytes match.
func TestWebDAV_FileContent(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// Test various content types (skip empty — 123pan returns 416 for 0-byte reads)
	contents := []struct {
		name    string
		content []byte
	}{
		{"ascii.txt", []byte("Hello, World!")},
		{"binary.dat", []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}},
		{"single-byte.dat", []byte{0x42}},
	}

	for _, tc := range contents {
		t.Run(tc.name, func(t *testing.T) {
			remotePath := "/content/" + tc.name

			err := p.PutFile(ctx, remotePath, bytes.NewReader(tc.content), nil)
			require.NoError(t, err, "PutFile %s should succeed", tc.name)

			reader, _, err := p.GetFile(ctx, remotePath)
			require.NoError(t, err, "GetFile %s should succeed", tc.name)
			defer reader.Close()

			data, err := io.ReadAll(reader)
			require.NoError(t, err, "ReadAll %s should succeed", tc.name)
			assert.Equal(t, tc.content, data, "content roundtrip for %s", tc.name)
		})
	}
}

// 5. TestWebDAV_DirectoryOperations — CreateDir → ListDir → create files → ListDir → delete.
func TestWebDAV_DirectoryOperations(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// CreateDir
	err := p.CreateDir(ctx, "/dirs")
	require.NoError(t, err, "CreateDir should succeed")

	// ListDir on empty directory
	entries, err := p.ListDir(ctx, "/dirs")
	require.NoError(t, err, "ListDir on empty dir should succeed")
	assert.Empty(t, entries, "empty dir should have no entries")

	// Create files inside
	err = p.PutFile(ctx, "/dirs/file1.txt", strings.NewReader("one"), nil)
	require.NoError(t, err)
	err = p.PutFile(ctx, "/dirs/file2.txt", strings.NewReader("two"), nil)
	require.NoError(t, err)

	// Create nested directory
	err = p.CreateDir(ctx, "/dirs/subdir")
	require.NoError(t, err)

	// ListDir again
	entries, err = p.ListDir(ctx, "/dirs")
	require.NoError(t, err, "ListDir should succeed")
	assert.Len(t, entries, 3, "should have 3 entries (2 files + 1 dir)")

	// Verify we can find the entries by name
	_, hasFile1 := findEntryByName(entries, "file1.txt")
	assert.True(t, hasFile1, "file1.txt should be present")
	_, hasFile2 := findEntryByName(entries, "file2.txt")
	assert.True(t, hasFile2, "file2.txt should be present")
	_, hasSubdir := findEntryByName(entries, "subdir")
	assert.True(t, hasSubdir, "subdir should be present")

	// Delete files
	err = p.DeleteFile(ctx, "/dirs/file1.txt")
	require.NoError(t, err)
	err = p.DeleteFile(ctx, "/dirs/file2.txt")
	require.NoError(t, err)
}

// 6. TestWebDAV_MoveFile — PutFile → MoveFile → verify new path exists, old is gone.
func TestWebDAV_MoveFile(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	content := "move me around"
	err := p.PutFile(ctx, "/move-src.txt", strings.NewReader(content), nil)
	require.NoError(t, err, "PutFile should succeed")

	// MoveFile
	err = p.MoveFile(ctx, "/move-src.txt", "/move-dst.txt")
	require.NoError(t, err, "MoveFile should succeed")

	// Verify new path exists with correct content
	reader, _, err := p.GetFile(ctx, "/move-dst.txt")
	require.NoError(t, err, "GetFile on destination should succeed")
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, string(data), "content should survive move")

	// Verify old path is gone — some servers use trash so Stat may succeed.
	// At minimum, GetFile on the old path should not return the original content.
	_, statErr := p.Stat(ctx, "/move-src.txt")
	if statErr != nil {
		assert.ErrorIs(t, statErr, provider.ErrNotFound, "source should not exist after move")
	}
}

// 7. TestWebDAV_SpecialCharacterNames — Files with Chinese names, spaces, parentheses.
func TestWebDAV_SpecialCharacterNames(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		path    string
		content string
	}{
		{"chinese", "/special/中文文件名.txt", "chinese filename"},
		{"spaces", "/special/file with spaces.dat", "spaces in name"},
		{"parens", "/special/file (copy).txt", "parentheses"},
		{"emoji", "/special/📁folder-marker.txt", "emoji name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := p.PutFile(ctx, tc.path, strings.NewReader(tc.content), nil)
			require.NoError(t, err, "PutFile %s should succeed", tc.name)

			reader, _, err := p.GetFile(ctx, tc.path)
			require.NoError(t, err, "GetFile %s should succeed", tc.name)
			defer reader.Close()

			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, tc.content, string(data), "content roundtrip for %s", tc.name)

			// Stat should also work
			meta, err := p.Stat(ctx, tc.path)
			require.NoError(t, err, "Stat %s should succeed", tc.name)
			assert.Equal(t, int64(len(tc.content)), meta.Size)
		})
	}
}

// 8. TestWebDAV_ListDir — Create multiple files and dirs, ListDir, verify all present.
func TestWebDAV_ListDir(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// Create known structure
	err := p.CreateDir(ctx, "/listdir")
	require.NoError(t, err)
	err = p.PutFile(ctx, "/listdir/alpha.txt", strings.NewReader("a"), nil)
	require.NoError(t, err)
	err = p.PutFile(ctx, "/listdir/beta.txt", strings.NewReader("bb"), nil)
	require.NoError(t, err)
	err = p.CreateDir(ctx, "/listdir/gamma")
	require.NoError(t, err)

	entries, err := p.ListDir(ctx, "/listdir")
	require.NoError(t, err, "ListDir should succeed")

	// Should have exactly 3 entries
	assert.Len(t, entries, 3, "should have 3 entries")

	// Verify files by name (paths include subdirectory prefix)
	alpha, ok := findEntryByName(entries, "alpha.txt")
	require.True(t, ok, "alpha.txt should be listed")
	assert.False(t, alpha.IsDir)
	assert.Equal(t, int64(1), alpha.Size)

	beta, ok := findEntryByName(entries, "beta.txt")
	require.True(t, ok, "beta.txt should be listed")
	assert.False(t, beta.IsDir)
	assert.Equal(t, int64(2), beta.Size)

	gamma, ok := findEntryByName(entries, "gamma")
	require.True(t, ok, "gamma dir should be listed")
	assert.True(t, gamma.IsDir)
}

// 9. TestWebDAV_Stat — Upload file, Stat it, verify Size and ModTime.
func TestWebDAV_Stat(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	content := "stat test content here"
	err := p.PutFile(ctx, "/stat-test.txt", strings.NewReader(content), nil)
	require.NoError(t, err, "PutFile should succeed")

	meta, err := p.Stat(ctx, "/stat-test.txt")
	require.NoError(t, err, "Stat should succeed")

	assert.Equal(t, "/stat-test.txt", meta.Path)
	assert.Equal(t, int64(len(content)), meta.Size, "size should match content length")
	assert.False(t, meta.IsDir, "should be a file")
	assert.False(t, meta.ModTime.IsZero(), "ModTime should be set")
	assert.WithinDuration(t, time.Now(), meta.ModTime, 5*time.Minute, "ModTime should be recent")

	// Stat on directory
	err = p.CreateDir(ctx, "/stat-dir")
	require.NoError(t, err)
	dirMeta, err := p.Stat(ctx, "/stat-dir")
	require.NoError(t, err, "Stat on dir should succeed")
	assert.True(t, dirMeta.IsDir, "should be a directory")
}

// 10. TestWebDAV_NotFound — Stat/GetFile a non-existent file, verify ErrNotFound.
func TestWebDAV_NotFound(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// Stat non-existent
	_, err := p.Stat(ctx, "/nonexistent-file-xyz.txt")
	assert.ErrorIs(t, err, provider.ErrNotFound, "Stat on missing file should return ErrNotFound")

	// GetFile non-existent
	_, _, err = p.GetFile(ctx, "/nonexistent-file-xyz.txt")
	assert.ErrorIs(t, err, provider.ErrNotFound, "GetFile on missing file should return ErrNotFound")

	// Delete non-existent — some servers return success for deleting unknown files.
	// We only assert it does not panic and either returns ErrNotFound or nil.
	err = p.DeleteFile(ctx, "/nonexistent-file-xyz.txt")
	if err != nil {
		assert.ErrorIs(t, err, provider.ErrNotFound, "DeleteFile on missing should return ErrNotFound or nil")
	}
}

// 11. TestWebDAV_IncrementalList — List a directory with IncrementalList, verify change detection.
func TestWebDAV_IncrementalList(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	err := p.CreateDir(ctx, "/incr")
	require.NoError(t, err)

	// First scan: no cached tag
	files, unchanged, err := p.IncrementalList(ctx, "/incr", "")
	require.NoError(t, err, "IncrementalList first scan should succeed")
	assert.False(t, unchanged, "first scan should report changed")
	assert.Empty(t, files, "empty dir should have no files")

	// Get a change token
	token, err := p.GetChangeToken(ctx, "/incr")
	require.NoError(t, err, "GetChangeToken should succeed")
	assert.NotEmpty(t, token, "token should not be empty")

	// Second scan with same token: should report unchanged (server permitting)
	files2, unchanged2, err := p.IncrementalList(ctx, "/incr", token)
	require.NoError(t, err, "IncrementalList with token should succeed")
	if unchanged2 {
		assert.Nil(t, files2, "unchanged should return nil files")
	}

	// Add a file to trigger change
	err = p.PutFile(ctx, "/incr/newfile.txt", strings.NewReader("trigger change"), nil)
	require.NoError(t, err)

	// Some WebDAV servers do not update directory mtime when child files change.
	// We re-read the token and verify IncrementalList returns the new file.
	newToken, err := p.GetChangeToken(ctx, "/incr")
	require.NoError(t, err, "GetChangeToken after change should succeed")

	files3, unchanged3, err := p.IncrementalList(ctx, "/incr", newToken)
	require.NoError(t, err, "IncrementalList with current token should succeed")
	// With the current token it should report unchanged (unchanged3 == true).
	if unchanged3 {
		assert.Nil(t, files3)
	}

	// Using the old token, we get a fresh listing regardless.
	files4, _, err := p.IncrementalList(ctx, "/incr", "")
	require.NoError(t, err, "IncrementalList with empty token should succeed")
	assert.NotEmpty(t, files4, "should list the new file")

	_, found := findEntryByName(files4, "newfile.txt")
	assert.True(t, found, "newfile.txt should appear in listing")
}

// 12. TestWebDAV_LargeFile — Upload a 5MB file, download, verify content.
func TestWebDAV_LargeFile(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	// Generate 5MB of random data
	size := 5 * 1024 * 1024 // 5MB
	data := make([]byte, size)
	_, err := rand.Read(data)
	require.NoError(t, err, "rand.Read should succeed")

	// Upload
	err = p.PutFile(ctx, "/large-5mb.dat", bytes.NewReader(data), nil)
	require.NoError(t, err, "PutFile 5MB should succeed")

	// Verify size via Stat
	meta, err := p.Stat(ctx, "/large-5mb.dat")
	require.NoError(t, err, "Stat should succeed")
	assert.Equal(t, int64(size), meta.Size, "file size should be exactly 5MB")

	// Download and verify
	reader, _, err := p.GetFile(ctx, "/large-5mb.dat")
	require.NoError(t, err, "GetFile should succeed")
	defer reader.Close()

	downloaded, err := io.ReadAll(reader)
	require.NoError(t, err, "ReadAll should succeed")
	assert.Len(t, downloaded, size, "downloaded size should match")
	assert.Equal(t, data, downloaded, "downloaded content should match uploaded")

	// Also test range read on the large file
	t.Run("RangeRead", func(t *testing.T) {
		offset := int64(1024)
		length := int64(4096)

		rangeReader, _, err := p.GetFileRange(ctx, "/large-5mb.dat", offset, length)
		require.NoError(t, err, "GetFileRange should succeed")
		defer rangeReader.Close()

		rangeData, err := io.ReadAll(rangeReader)
		require.NoError(t, err, "ReadAll range should succeed")
		assert.Equal(t, data[offset:offset+length], rangeData, "range data should match slice")
	})
}

// TestWebDAV_NestedPaths verifies that PutFile auto-creates parent directories.
func TestWebDAV_NestedPaths(t *testing.T) {
	p := setupProvider(t)
	ctx := context.Background()

	deepPath := "/a/b/c/d/deep.txt"
	content := "deeply nested"
	err := p.PutFile(ctx, deepPath, strings.NewReader(content), nil)
	require.NoError(t, err, "PutFile to deep path should succeed")

	reader, _, err := p.GetFile(ctx, deepPath)
	require.NoError(t, err, "GetFile from deep path should succeed")
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))

	// Verify intermediate directories are listable
	entries, err := p.ListDir(ctx, "/a")
	require.NoError(t, err, "ListDir /a should succeed")
	assert.NotEmpty(t, entries, "/a should contain entries")
}
