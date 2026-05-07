package server

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/logger"
	"github.com/rain/every-sync/internal/server/handler"
	"github.com/rain/every-sync/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	httpServer *http.Server
	engine     *engine.Engine
	store      *store.Store
}

func New(s *store.Store, e *engine.Engine, addr string) *Server {
	mux := http.NewServeMux()
	h := handler.New(s, e)

	api := http.NewServeMux()

	api.HandleFunc("GET /api/v1/pairs", h.ListPairs)
	api.HandleFunc("POST /api/v1/pairs", h.CreatePair)
	api.HandleFunc("GET /api/v1/pairs/{id}", h.GetPair)
	api.HandleFunc("PUT /api/v1/pairs/{id}", h.UpdatePair)
	api.HandleFunc("DELETE /api/v1/pairs/{id}", h.DeletePair)

	api.HandleFunc("GET /api/v1/providers", h.ListProviders)
	api.HandleFunc("POST /api/v1/providers", h.CreateProvider)
	api.HandleFunc("GET /api/v1/providers/{id}", h.GetProvider)
	api.HandleFunc("PUT /api/v1/providers/{id}", h.UpdateProvider)
	api.HandleFunc("DELETE /api/v1/providers/{id}", h.DeleteProvider)

	api.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	api.HandleFunc("GET /api/v1/status", h.Status)
	api.HandleFunc("POST /api/v1/sync", h.TriggerSync)
	api.HandleFunc("GET /api/v1/events", h.Events)

	corsHandler := corsMiddleware(api)
	mux.Handle("/api/v1/", corsHandler)
	mux.Handle("/", staticHandler())

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		engine: e,
		store:  s,
	}
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if f, err := files.Open(r.URL.Path[1:]); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) Start() error {
	logger.L.Info().Str("addr", s.httpServer.Addr).Msg("server starting")
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
