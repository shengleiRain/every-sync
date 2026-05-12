//go:build webdav && integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/provider/local"
	"github.com/rain/every-sync/internal/provider/webdav"
	"github.com/rain/every-sync/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// apiConfigStructure matches the YAML layout of ~/.every-sync/config.yaml.
type apiConfigStructure struct {
	Providers []apiProviderEntry `yaml:"providers"`
}

type apiProviderEntry struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

func apiGetWebDAVConfig(t *testing.T) (endpoint, username, password string) {
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

	var cfg apiConfigStructure
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

// testServer holds the test server components.
type testServer struct {
	handler  *Handler
	store    *store.Store
	engine   *engine.Engine
	baseURL  string
	cleanup  func()
	endpoint string
	username string
	password string
}

func setupAPITestServer(t *testing.T) *testServer {
	t.Helper()

	// Create temp store
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	require.NoError(t, err, "open store")

	// Create engine with registrar
	cfg := engine.DefaultConfig()
	cfg.DryRun = false
	cfg.MaxWorkers = 2
	cfg.QueueSize = 100
	cfg.ScanInterval = time.Hour
	e := engine.New(s, cfg)

	endpoint, username, password := apiGetWebDAVConfig(t)

	e.WithRegistrar(func(pair *store.SyncPair) (provider.Provider, provider.Provider, error) {
		lp := &local.LocalProvider{}
		if err := lp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.LocalPath},
		}); err != nil {
			return nil, nil, err
		}

		rp := &webdav.WebDAVProvider{}
		prefix := "/every-sync-api-" + time.Now().Format("20060102-150405")
		if err := rp.Init(context.Background(), provider.Config{
			Type: "webdav",
			Params: map[string]string{
				"endpoint": endpoint,
				"username": username,
				"password": password,
				"prefix":   prefix,
			},
		}); err != nil {
			return nil, nil, err
		}

		return lp, rp, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, e.Start(ctx), "start engine")

	h := New(s, e)

	// Create mux (replicate server.go route registration)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/pairs", h.ListPairs)
	mux.HandleFunc("POST /api/v1/pairs", h.CreatePair)
	mux.HandleFunc("GET /api/v1/pairs/{id}", h.GetPair)
	mux.HandleFunc("PUT /api/v1/pairs/{id}", h.UpdatePair)
	mux.HandleFunc("DELETE /api/v1/pairs/{id}", h.DeletePair)
	mux.HandleFunc("POST /api/v1/pairs/{id}/materialize", h.MaterializePairFile)
	mux.HandleFunc("GET /api/v1/pairs/{id}/files", h.ListPairFiles)
	mux.HandleFunc("POST /api/v1/pairs/{id}/folders/select", h.SelectFolders)

	mux.HandleFunc("GET /api/v1/conflicts", h.ListConflicts)
	mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", h.ResolveConflict)
	mux.HandleFunc("GET /api/v1/versions", h.ListVersions)

	mux.HandleFunc("GET /api/v1/providers", h.ListProviders)
	mux.HandleFunc("POST /api/v1/providers", h.CreateProvider)
	mux.HandleFunc("GET /api/v1/providers/{id}", h.GetProvider)
	mux.HandleFunc("PUT /api/v1/providers/{id}", h.UpdateProvider)
	mux.HandleFunc("DELETE /api/v1/providers/{id}", h.DeleteProvider)

	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /api/v1/status", h.Status)
	mux.HandleFunc("POST /api/v1/sync", h.TriggerSync)

	// Start server on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen on random port")
	port := listener.Addr().(*net.TCPAddr).Port

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)

	cleanup := func() {
		cancel()
		srv.Close()
		e.Stop()
		s.Close()
	}
	t.Cleanup(cleanup)

	return &testServer{
		handler:  h,
		store:    s,
		engine:   e,
		baseURL:  baseURL,
		cleanup:  cleanup,
		endpoint: endpoint,
		username: username,
		password: password,
	}
}

// doRequest performs an HTTP request against the test server and returns the
// status code and decoded JSON body.
func doRequest(t *testing.T, method, url string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err, "marshal request body")
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err, "create request")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "execute request")
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")

	var result map[string]interface{}
	if len(respBody) > 0 {
		err = json.Unmarshal(respBody, &result)
		require.NoError(t, err, "unmarshal response body")
	}
	return resp.StatusCode, result
}

// doRequestRaw returns the raw body bytes.
func doRequestRaw(t *testing.T, method, url string, body interface{}) (int, []byte) {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err, "marshal request body")
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err, "create request")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "execute request")
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	return resp.StatusCode, respBody
}

// =============================================================================
// Test 1: Create and get pair — POST /api/v1/pairs, GET /api/v1/pairs/{id}
// =============================================================================

func TestAPI_CreateAndGetPair(t *testing.T) {
	ts := setupAPITestServer(t)
	localDir := t.TempDir()

	// Create pair
	createBody := map[string]string{
		"name":       "api-test-pair",
		"local_path": localDir,
		"remote_path": "/",
		"provider":   "webdav",
		"mode":       "normal",
		"direction":  "both",
	}

	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status, "create pair status")
	require.Equal(t, "api-test-pair", result["name"])

	// Get the ID
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Get pair by ID
	status, result = doRequest(t, http.MethodGet, ts.baseURL+"/pairs/"+id, nil)
	require.Equal(t, http.StatusOK, status, "get pair status")
	require.Equal(t, "api-test-pair", result["name"])
	require.Equal(t, localDir, result["local_path"])
}

// =============================================================================
// Test 2: List pairs — Create multiple pairs, GET /api/v1/pairs, verify list
// =============================================================================

func TestAPI_ListPairs(t *testing.T) {
	ts := setupAPITestServer(t)
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Create two pairs
	for i, dir := range []string{dir1, dir2} {
		createBody := map[string]string{
			"name":        fmt.Sprintf("list-pair-%d", i),
			"local_path":  dir,
			"remote_path": "/",
			"provider":    "webdav",
		}
		status, _ := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
		require.Equal(t, http.StatusCreated, status, "create pair %d", i)
	}

	// List all pairs
	status, rawBody := doRequestRaw(t, http.MethodGet, ts.baseURL+"/pairs", nil)
	require.Equal(t, http.StatusOK, status, "list pairs status")

	var pairs []map[string]interface{}
	require.NoError(t, json.Unmarshal(rawBody, &pairs))
	require.Len(t, pairs, 2, "should have 2 pairs")
}

// =============================================================================
// Test 3: Update pair — PUT /api/v1/pairs/{id} with new fields
// =============================================================================

func TestAPI_UpdatePair(t *testing.T) {
	ts := setupAPITestServer(t)
	localDir := t.TempDir()

	// Create pair
	createBody := map[string]string{
		"name":        "update-test",
		"local_path":  localDir,
		"remote_path": "/",
		"provider":    "webdav",
	}
	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status)
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Update pair
	enabled := true
	updateBody := map[string]interface{}{
		"enabled":   enabled,
		"direction": "up",
	}
	status, result = doRequest(t, http.MethodPut, ts.baseURL+"/pairs/"+id, updateBody)
	require.Equal(t, http.StatusOK, status, "update pair status")
	require.Equal(t, true, result["enabled"])
	require.Equal(t, "up", result["direction"])

	// Verify via GET
	status, result = doRequest(t, http.MethodGet, ts.baseURL+"/pairs/"+id, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, result["enabled"])
	require.Equal(t, "up", result["direction"])
}

// =============================================================================
// Test 4: Sync trigger — POST /api/v1/sync with pair_id, wait, verify sync
// =============================================================================

func TestAPI_SyncTrigger(t *testing.T) {
	ts := setupAPITestServer(t)
	localDir := t.TempDir()

	// Create a file locally
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "trigger-test.txt"), []byte("sync content"), 0644))

	// Create and enable a pair
	createBody := map[string]string{
		"name":        "sync-trigger",
		"local_path":  localDir,
		"remote_path": "/",
		"provider":    "webdav",
		"mode":        "mirror",
		"direction":   "up",
	}
	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status)
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Enable the pair
	enabled := true
	status, _ = doRequest(t, http.MethodPut, ts.baseURL+"/pairs/"+id, map[string]interface{}{"enabled": enabled})
	require.Equal(t, http.StatusOK, status)

	// Trigger sync
	pairID, _ := result["id"].(float64)
	syncBody := map[string]interface{}{
		"pair_id": int64(pairID),
	}
	status, _ = doRequest(t, http.MethodPost, ts.baseURL+"/sync", syncBody)
	require.Equal(t, http.StatusAccepted, status, "sync trigger status")

	// Wait for sync to complete by polling DB for the file entry
	pairIDInt := int64(pairID)
	var entry *store.FileEntry
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		entry, _ = ts.store.GetFileEntry(pairIDInt, "/trigger-test.txt")
		if entry != nil && entry.SyncState == "synced" {
			break
		}
	}
	require.NotNil(t, entry, "file should be indexed after sync")
	require.Equal(t, "synced", entry.SyncState, "file should be synced")
}

// =============================================================================
// Test 5: File browser — GET /api/v1/pairs/{id}/files?path=/&side=local
// =============================================================================

func TestAPI_FileBrowser(t *testing.T) {
	ts := setupAPITestServer(t)
	localDir := t.TempDir()

	// Create files locally
	require.NoError(t, os.MkdirAll(filepath.Join(localDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "browse.txt"), []byte("browse"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "subdir/nested.txt"), []byte("nested"), 0644))

	// Create and enable pair
	createBody := map[string]string{
		"name":        "file-browser",
		"local_path":  localDir,
		"remote_path": "/",
		"provider":    "webdav",
		"mode":        "mirror",
		"direction":   "up",
	}
	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status)
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Enable the pair
	enabled := true
	status, _ = doRequest(t, http.MethodPut, ts.baseURL+"/pairs/"+id, map[string]interface{}{"enabled": enabled})
	require.Equal(t, http.StatusOK, status)

	// List root files
	status, rawBody := doRequestRaw(t, http.MethodGet, ts.baseURL+"/pairs/"+id+"/files?path=/&side=local", nil)
	require.Equal(t, http.StatusOK, status, "list files status")

	var fileList map[string]interface{}
	require.NoError(t, json.Unmarshal(rawBody, &fileList), "unmarshal file list")
	require.Equal(t, "/", fileList["path"])

	entries, ok := fileList["entries"].([]interface{})
	require.True(t, ok, "entries should be an array")
	require.True(t, len(entries) >= 2, "should have at least 2 entries (file + dir)")

	// Verify we can find the expected entries
	names := make(map[string]bool)
	for _, entry := range entries {
		entryMap := entry.(map[string]interface{})
		names[entryMap["name"].(string)] = true
	}
	require.True(t, names["browse.txt"], "should have browse.txt")
	require.True(t, names["subdir"], "should have subdir")
}

// =============================================================================
// Test 6: Folder selection — POST /api/v1/pairs/{id}/folders/select
// =============================================================================

func TestAPI_FolderSelection(t *testing.T) {
	ts := setupAPITestServer(t)
	localDir := t.TempDir()

	// Create pair
	createBody := map[string]string{
		"name":        "folder-select",
		"local_path":  localDir,
		"remote_path": "/",
		"provider":    "webdav",
	}
	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status)
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Select folders
	selectBody := map[string]interface{}{
		"selected_folders": []string{"docs", "photos"},
	}
	status, result = doRequest(t, http.MethodPost, ts.baseURL+"/pairs/"+id+"/folders/select", selectBody)
	require.Equal(t, http.StatusOK, status, "folder selection status")

	// Verify the pair was updated
	folders := result["selected_folders"]
	require.NotNil(t, folders, "selected_folders should be set")

	// The folders should be normalized
	foldersStr, ok := folders.(string)
	require.True(t, ok, "selected_folders should be a string")
	require.Contains(t, foldersStr, "docs")
	require.Contains(t, foldersStr, "photos")
}

// =============================================================================
// Test 7: Conflict workflow — Create conflict via sync, GET conflicts,
// POST resolve
// =============================================================================

func TestAPI_ConflictWorkflow(t *testing.T) {
	// Use a dedicated setup with local-to-local providers to reliably create conflicts
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	ctx := context.Background()

	// Create files on both sides with different content
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "conflict.txt"), []byte("local-ver"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "conflict.txt"), []byte("remote-ver"), 0644))

	// Set mtimes so local is newer (to ensure consistent behavior)
	now := time.Now()
	require.NoError(t, os.Chtimes(filepath.Join(localDir, "conflict.txt"), now, now.Add(2*time.Second)))
	require.NoError(t, os.Chtimes(filepath.Join(remoteDir, "conflict.txt"), now, now.Add(1*time.Second)))

	// Create store and engine with local-to-local providers
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	require.NoError(t, err)
	defer s.Close()

	cfg := engine.DefaultConfig()
	cfg.DryRun = false
	cfg.MaxWorkers = 2
	cfg.QueueSize = 100
	cfg.ScanInterval = time.Hour
	e := engine.New(s, cfg)

	e.WithRegistrar(func(pair *store.SyncPair) (provider.Provider, provider.Provider, error) {
		lp := &local.LocalProvider{}
		if err := lp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.LocalPath},
		}); err != nil {
			return nil, nil, err
		}

		rp := &local.LocalProvider{}
		if err := rp.Init(context.Background(), provider.Config{
			Type:   "local",
			Params: map[string]string{"root_path": pair.RemotePath},
		}); err != nil {
			return nil, nil, err
		}

		return lp, rp, nil
	})

	engineCtx, engineCancel := context.WithCancel(ctx)
	require.NoError(t, e.Start(engineCtx), "start engine")
	defer func() {
		engineCancel()
		e.Stop()
	}()

	h := New(s, e)

	// Create mux
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/pairs", h.ListPairs)
	mux.HandleFunc("POST /api/v1/pairs", h.CreatePair)
	mux.HandleFunc("GET /api/v1/pairs/{id}", h.GetPair)
	mux.HandleFunc("PUT /api/v1/pairs/{id}", h.UpdatePair)
	mux.HandleFunc("DELETE /api/v1/pairs/{id}", h.DeletePair)
	mux.HandleFunc("POST /api/v1/pairs/{id}/materialize", h.MaterializePairFile)
	mux.HandleFunc("GET /api/v1/pairs/{id}/files", h.ListPairFiles)
	mux.HandleFunc("POST /api/v1/pairs/{id}/folders/select", h.SelectFolders)
	mux.HandleFunc("GET /api/v1/conflicts", h.ListConflicts)
	mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", h.ResolveConflict)
	mux.HandleFunc("GET /api/v1/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/providers", h.ListProviders)
	mux.HandleFunc("POST /api/v1/providers", h.CreateProvider)
	mux.HandleFunc("GET /api/v1/providers/{id}", h.GetProvider)
	mux.HandleFunc("PUT /api/v1/providers/{id}", h.UpdateProvider)
	mux.HandleFunc("DELETE /api/v1/providers/{id}", h.DeleteProvider)
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /api/v1/status", h.Status)
	mux.HandleFunc("POST /api/v1/sync", h.TriggerSync)

	// Start server on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Close()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)

	// Create pair via API with manual conflict strategy
	createBody := map[string]string{
		"name":              "conflict-workflow",
		"local_path":        localDir,
		"remote_path":       remoteDir,
		"provider":          "local",
		"mode":              "mirror",
		"direction":         "both",
		"conflict_strategy": "manual",
	}
	status, result := doRequest(t, http.MethodPost, baseURL+"/pairs", createBody)
	require.Equal(t, http.StatusCreated, status)
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Enable the pair
	enabled := true
	status, _ = doRequest(t, http.MethodPut, baseURL+"/pairs/"+id, map[string]interface{}{"enabled": enabled})
	require.Equal(t, http.StatusOK, status)

	// Trigger sync via API to create the conflict
	pairID := result["id"].(float64)
	syncBody := map[string]interface{}{
		"pair_id": int64(pairID),
	}
	status, _ = doRequest(t, http.MethodPost, baseURL+"/sync", syncBody)
	require.Equal(t, http.StatusAccepted, status)

	// Wait for sync to complete
	time.Sleep(3 * time.Second)
	e.Drain(5 * time.Second)

	// GET conflicts via API
	status, rawBody := doRequestRaw(t, http.MethodGet, baseURL+"/conflicts?status=open", nil)
	require.Equal(t, http.StatusOK, status, "list conflicts status")

	var conflicts []map[string]interface{}
	require.NoError(t, json.Unmarshal(rawBody, &conflicts))
	require.True(t, len(conflicts) >= 1, "should have at least 1 conflict")

	// Get the first conflict ID
	conflictID := fmt.Sprintf("%.0f", conflicts[0]["id"].(float64))

	// Resolve the conflict via API
	resolveBody := map[string]string{
		"strategy": "local_wins",
	}
	status, _ = doRequest(t, http.MethodPost, baseURL+"/conflicts/"+conflictID+"/resolve", resolveBody)
	require.Equal(t, http.StatusOK, status, "resolve conflict status")

	// Verify conflict is resolved
	time.Sleep(2 * time.Second)
	status, rawBody = doRequestRaw(t, http.MethodGet, baseURL+"/conflicts?status=open", nil)
	require.Equal(t, http.StatusOK, status)
	var openConflicts []map[string]interface{}
	require.NoError(t, json.Unmarshal(rawBody, &openConflicts))
	require.Len(t, openConflicts, 0, "no open conflicts should remain")

	// Verify the file was resolved correctly — remote should have local content
	data, err := os.ReadFile(filepath.Join(remoteDir, "conflict.txt"))
	require.NoError(t, err, "read remote file after resolve")
	require.Equal(t, "local-ver", string(data), "remote should have local content after resolve")
}

// =============================================================================
// Test 8: Providers CRUD — /api/v1/providers endpoints
// =============================================================================

func TestAPI_Providers(t *testing.T) {
	ts := setupAPITestServer(t)

	// Create provider
	createBody := map[string]interface{}{
		"name": "test-webdav",
		"type": "webdav",
		"params": map[string]string{
			"endpoint": "https://example.com/dav",
			"username": "user1",
			"password": "pass1",
		},
	}
	status, result := doRequest(t, http.MethodPost, ts.baseURL+"/providers", createBody)
	require.Equal(t, http.StatusCreated, status, "create provider status")
	require.Equal(t, "test-webdav", result["name"])
	id := fmt.Sprintf("%.0f", result["id"].(float64))

	// Get provider
	status, result = doRequest(t, http.MethodGet, ts.baseURL+"/providers/"+id, nil)
	require.Equal(t, http.StatusOK, status, "get provider status")
	require.Equal(t, "test-webdav", result["name"])

	// List providers
	status, rawBody := doRequestRaw(t, http.MethodGet, ts.baseURL+"/providers", nil)
	require.Equal(t, http.StatusOK, status, "list providers status")
	var providers []map[string]interface{}
	require.NoError(t, json.Unmarshal(rawBody, &providers))
	require.True(t, len(providers) >= 1, "should have at least 1 provider")

	// Update provider
	updateBody := map[string]interface{}{
		"name": "test-webdav-updated",
	}
	status, result = doRequest(t, http.MethodPut, ts.baseURL+"/providers/"+id, updateBody)
	require.Equal(t, http.StatusOK, status, "update provider status")
	require.Equal(t, "test-webdav-updated", result["name"])

	// Delete provider
	status, _ = doRequest(t, http.MethodDelete, ts.baseURL+"/providers/"+id, nil)
	require.Equal(t, http.StatusOK, status, "delete provider status")

	// Verify deleted
	status, _ = doRequest(t, http.MethodGet, ts.baseURL+"/providers/"+id, nil)
	require.Equal(t, http.StatusNotFound, status, "deleted provider should 404")
}
