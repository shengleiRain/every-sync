package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rain/every-sync/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(s *store.Store) *Handler {
	return &Handler{store: s}
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

	pair := &store.SyncPair{
		Name:       req.Name,
		LocalPath:  req.LocalPath,
		RemotePath: req.RemotePath,
		Provider:   req.Provider,
		Mode:       req.Mode,
		Direction:  req.Direction,
		Enabled:    true,
		Schedule:   req.Schedule,
	}

	if err := h.store.CreateSyncPair(pair); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

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
		pair.Direction = *req.Direction
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
