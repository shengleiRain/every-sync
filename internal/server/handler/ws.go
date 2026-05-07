package handler

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	syncengine "github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/logger"
	"github.com/rain/every-sync/internal/store"
)

type Handler struct {
	store  *store.Store
	engine interface {
		RefreshPairs() error
		SyncPair(ctx context.Context, pairID int64, direction string) error
		SyncAll(ctx context.Context) error
		Status() syncengine.Status
		Subscribe(ctx context.Context) <-chan syncengine.Event
	}
}

func New(s *store.Store, e interface {
	RefreshPairs() error
	SyncPair(ctx context.Context, pairID int64, direction string) error
	SyncAll(ctx context.Context) error
	Status() syncengine.Status
	Subscribe(ctx context.Context) <-chan syncengine.Event
}) *Handler {
	return &Handler{store: s, engine: e}
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.Status())
}

type TriggerSyncRequest struct {
	PairID    int64  `json:"pair_id"`
	Direction string `json:"direction"`
}

func (h *Handler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	var req TriggerSyncRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()
		if req.PairID > 0 {
			if err := h.engine.SyncPair(ctx, req.PairID, req.Direction); err != nil {
				logger.L.Error().Err(err).Int64("pair_id", req.PairID).Msg("api-triggered sync failed")
			}
			return
		}
		if err := h.engine.SyncAll(ctx); err != nil {
			logger.L.Error().Err(err).Msg("api-triggered sync all failed")
		}
	}()

	logger.Audit("sync.trigger").Int64("pair_id", req.PairID).Str("direction", req.Direction).Msg("sync triggered via API")
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeError(w, http.StatusBadRequest, "websocket upgrade required")
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing Sec-WebSocket-Key")
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		writeError(w, http.StatusInternalServerError, "websocket hijack unsupported")
		return
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		logger.L.Error().Err(err).Msg("websocket hijack failed")
		return
	}
	defer conn.Close()

	accept := websocketAccept(key)
	fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\n")
	fmt.Fprintf(rw, "Upgrade: websocket\r\n")
	fmt.Fprintf(rw, "Connection: Upgrade\r\n")
	fmt.Fprintf(rw, "Sec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err := rw.Flush(); err != nil {
		return
	}

	events := h.engine.Subscribe(r.Context())
	for event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := writeWebSocketText(conn, payload); err != nil {
			return
		}
	}
}

// --- Sync Pairs ---

func (h *Handler) ListPairs(w http.ResponseWriter, r *http.Request) {
	pairs, err := h.store.ListSyncPairs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pairs == nil {
		pairs = []*store.SyncPair{}
	}
	writeJSON(w, http.StatusOK, pairs)
}

type CreatePairRequest struct {
	Name       string `json:"name"`
	LocalPath  string `json:"local_path"`
	RemotePath string `json:"remote_path"`
	Provider   string `json:"provider"`
	Mode       string `json:"mode"`
	Direction  string `json:"direction"`
	Schedule   string `json:"schedule"`
}

func (h *Handler) CreatePair(w http.ResponseWriter, r *http.Request) {
	var req CreatePairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.LocalPath == "" || req.RemotePath == "" {
		writeError(w, http.StatusBadRequest, "name, local_path, and remote_path are required")
		return
	}

	if req.Provider == "" {
		req.Provider = "webdav"
	}
	if req.Mode == "" {
		req.Mode = "mirror"
	}
	if req.Direction == "" {
		req.Direction = "both"
	}
	dir, err := syncengine.ResolveDirection(req.Direction, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Direction = string(dir)

	pair := &store.SyncPair{
		Name:       req.Name,
		LocalPath:  req.LocalPath,
		RemotePath: req.RemotePath,
		Provider:   req.Provider,
		Mode:       req.Mode,
		Direction:  req.Direction,
		Enabled:    false,
		Schedule:   req.Schedule,
	}

	if err := h.store.CreateSyncPair(pair); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Audit("pair.create").Str("name", pair.Name).Int64("id", pair.ID).Msg("pair created via API")
	writeJSON(w, http.StatusCreated, pair)
}

func (h *Handler) GetPair(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pair id")
		return
	}

	pair, err := h.store.GetSyncPair(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pair == nil {
		writeError(w, http.StatusNotFound, "pair not found")
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

func (h *Handler) DeletePair(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pair id")
		return
	}

	if err := h.store.DeleteSyncPair(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Audit("pair.delete").Int64("id", id).Msg("pair deleted via API")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type UpdatePairRequest struct {
	LocalPath  *string `json:"local_path"`
	RemotePath *string `json:"remote_path"`
	Mode       *string `json:"mode"`
	Direction  *string `json:"direction"`
	Enabled    *bool   `json:"enabled"`
	Schedule   *string `json:"schedule"`
}

func (h *Handler) UpdatePair(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pair id")
		return
	}

	pair, err := h.store.GetSyncPair(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pair == nil {
		writeError(w, http.StatusNotFound, "pair not found")
		return
	}

	var req UpdatePairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.LocalPath != nil {
		pair.LocalPath = *req.LocalPath
	}
	if req.RemotePath != nil {
		pair.RemotePath = *req.RemotePath
	}
	if req.Mode != nil {
		pair.Mode = *req.Mode
	}
	if req.Direction != nil {
		dir, err := syncengine.ResolveDirection(*req.Direction, "")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		pair.Direction = string(dir)
	}
	if req.Enabled != nil {
		pair.Enabled = *req.Enabled
	}
	if req.Schedule != nil {
		pair.Schedule = *req.Schedule
	}

	if err := h.store.UpdateSyncPair(pair); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Refresh engine pairs if enabled state changed
	if req.Enabled != nil {
		if err := h.engine.RefreshPairs(); err != nil {
			logger.L.Error().Err(err).Msg("failed to refresh pairs after update")
		}
	}

	logger.Audit("pair.update").Int64("id", id).Bool("enabled", pair.Enabled).Msg("pair updated via API")
	writeJSON(w, http.StatusOK, pair)
}

// --- Providers ---

func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	configs, err := h.store.ListProviderConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if configs == nil {
		configs = []*store.ProviderConfig{}
	}
	writeJSON(w, http.StatusOK, configs)
}

type CreateProviderRequest struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
}

func (h *Handler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}

	pc := &store.ProviderConfig{
		Name:   req.Name,
		Type:   req.Type,
		Params: req.Params,
	}

	if err := h.store.CreateProviderConfig(pc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Audit("provider.create").Str("name", pc.Name).Int64("id", pc.ID).Msg("provider created via API")
	writeJSON(w, http.StatusCreated, pc)
}

func (h *Handler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	pc, err := h.store.GetProviderConfig(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pc == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	writeJSON(w, http.StatusOK, pc)
}

type UpdateProviderRequest struct {
	Name   *string            `json:"name"`
	Type   *string            `json:"type"`
	Params *map[string]string `json:"params"`
}

func (h *Handler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	pc, err := h.store.GetProviderConfig(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pc == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	var req UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		pc.Name = *req.Name
	}
	if req.Type != nil {
		pc.Type = *req.Type
	}
	if req.Params != nil {
		pc.Params = *req.Params
	}

	if err := h.store.UpdateProviderConfig(pc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Audit("provider.update").Int64("id", id).Str("name", pc.Name).Msg("provider updated via API")
	writeJSON(w, http.StatusOK, pc)
}

func (h *Handler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	if err := h.store.DeleteProviderConfig(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Audit("provider.delete").Int64("id", id).Msg("provider deleted via API")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebSocketText(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 127,
			byte(uint64(len(payload))>>56),
			byte(uint64(len(payload))>>48),
			byte(uint64(len(payload))>>40),
			byte(uint64(len(payload))>>32),
			byte(uint64(len(payload))>>24),
			byte(uint64(len(payload))>>16),
			byte(uint64(len(payload))>>8),
			byte(uint64(len(payload))),
		)
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}
