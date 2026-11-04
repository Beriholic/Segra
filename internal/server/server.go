// Package server exposes the loopback HTTP API used by the Segra browser
// extension and other local clients.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/beriholic/segra/internal/download"
)

// Server is the loopback API server.
type Server struct {
	mgr   *download.Manager
	token string
	port  int
	srv   *http.Server
	ln    net.Listener
}

// New creates a server. Start picks the preferred port or the next free
// one.
func New(mgr *download.Manager, token string, port int) *Server {
	if port <= 0 {
		port = 47823
	}
	return &Server{mgr: mgr, token: token, port: port}
}

// Port is the port the server listens on after Start.
func (s *Server) Port() int { return s.port }

// Start listens on 127.0.0.1 and serves the API in the background.
func (s *Server) Start() (int, error) {
	var lastErr error
	for p := s.port; p < s.port+10 && p <= 65535; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			lastErr = err
			continue
		}
		s.ln = ln
		s.port = p
		break
	}
	if s.ln == nil {
		return 0, fmt.Errorf("listen: %w", lastErr)
	}
	s.srv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("segra api: %v", err)
		}
	}()
	return s.port, nil
}

// Stop shuts the server down.
func (s *Server) Stop() error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.Handle("GET /api/info", s.auth(http.HandlerFunc(s.handleInfo)))
	mux.Handle("GET /api/tasks", s.auth(http.HandlerFunc(s.handleList)))
	mux.Handle("POST /api/tasks", s.auth(http.HandlerFunc(s.handleCreate)))
	mux.Handle("GET /api/tasks/{id}", s.auth(http.HandlerFunc(s.handleGet)))
	mux.Handle("POST /api/tasks/{id}/pause", s.auth(http.HandlerFunc(s.handlePause)))
	mux.Handle("POST /api/tasks/{id}/resume", s.auth(http.HandlerFunc(s.handleResume)))
	mux.Handle("DELETE /api/tasks/{id}", s.auth(http.HandlerFunc(s.handleDelete)))
	mux.Handle("POST /api/pause-all", s.auth(http.HandlerFunc(s.handlePauseAll)))
	mux.Handle("POST /api/resume-all", s.auth(http.HandlerFunc(s.handleResumeAll)))
	return cors(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" {
			token = r.Header.Get("X-Segra-Token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Segra-Token")
		h.Set("Access-Control-Max-Age", "86400")
		h.Set("Access-Control-Allow-Private-Network", "true")
		h.Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app":     "segra",
		"api":     1,
		"version": "0.1.0",
		"running": true,
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"app":         "segra",
		"api":         1,
		"version":     "0.1.0",
		"port":        s.port,
		"downloadDir": cfg.Dir,
		"connections": cfg.Connections,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	tasks := s.mgr.List()
	if tasks == nil {
		tasks = []download.Task{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	t, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req download.CreateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	t, err := s.mgr.Create(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.mgr.Pause(r.PathValue("id"))
	t, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	s.mgr.Resume(r.PathValue("id"))
	t, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	files, _ := strconv.ParseBool(r.URL.Query().Get("files"))
	s.mgr.Remove(id, files)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePauseAll(w http.ResponseWriter, r *http.Request) {
	s.mgr.PauseAll()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResumeAll(w http.ResponseWriter, r *http.Request) {
	s.mgr.ResumeAll()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
