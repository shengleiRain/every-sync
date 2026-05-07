package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/server/handler"
	"github.com/rain/every-sync/internal/store"
)

type Server struct {
	httpServer *http.Server
	engine     *engine.Engine
	store      *store.Store
}

func New(s *store.Store, e *engine.Engine, addr string) *Server {
	mux := http.NewServeMux()
	h := handler.New(s)

	// API v1 routes
	api := http.NewServeMux()

	// Sync pairs
	api.HandleFunc("GET /api/v1/pairs", h.ListPairs)
	api.HandleFunc("POST /api/v1/pairs", h.CreatePair)
	api.HandleFunc("GET /api/v1/pairs/{id}", h.GetPair)
	api.HandleFunc("PUT /api/v1/pairs/{id}", h.UpdatePair)
	api.HandleFunc("DELETE /api/v1/pairs/{id}", h.DeletePair)

	// Providers
	api.HandleFunc("GET /api/v1/providers", h.ListProviders)
	api.HandleFunc("POST /api/v1/providers", h.CreateProvider)
	api.HandleFunc("GET /api/v1/providers/{id}", h.GetProvider)
	api.HandleFunc("PUT /api/v1/providers/{id}", h.UpdateProvider)
	api.HandleFunc("DELETE /api/v1/providers/{id}", h.DeleteProvider)

	// Health check
	api.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// CORS middleware
	corsHandler := corsMiddleware(api)

	mux.Handle("/", corsHandler)

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

func (s *Server) Start() error {
	fmt.Printf("EverySync server starting on %s\n", s.httpServer.Addr)
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
