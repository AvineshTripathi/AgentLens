// Package dashboard serves the AgentLens web UI and REST API.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/AvineshTripathi/AgentLens/internal/events"
	"github.com/AvineshTripathi/AgentLens/internal/store"
)

// Server serves the dashboard UI and REST API.
type Server struct {
	store  *store.Store
	router *mux.Router
}

// NewServer creates a dashboard server.
func NewServer(st *store.Store) *Server {
	s := &Server{store: st, router: mux.NewRouter()}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Prometheus metrics
	s.router.Handle("/metrics", promhttp.Handler())

	// REST API
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/sessions", s.listSessions).Methods(http.MethodGet)
	api.HandleFunc("/sessions/{id}", s.getSession).Methods(http.MethodGet)
	api.HandleFunc("/sessions/{id}/turns", s.listTurns).Methods(http.MethodGet)
	api.HandleFunc("/sessions/{id}/tool-calls", s.listToolCalls).Methods(http.MethodGet)
	api.HandleFunc("/sessions/{id}/signals", s.listSignals).Methods(http.MethodGet)
	api.HandleFunc("/sessions/{id}/timeline", s.getTimeline).Methods(http.MethodGet)
	api.HandleFunc("/health/{agent_id}", s.getAgentHealth).Methods(http.MethodGet)

	// SSE: real-time event stream for the dashboard.
	s.router.HandleFunc("/api/v1/stream", s.sseStream).Methods(http.MethodGet)

	// UI: embedded HTML dashboard.
	s.router.HandleFunc("/", s.serveDashboard).Methods(http.MethodGet)
	s.router.PathPrefix("/static/").HandlerFunc(s.serveStatic)
}

// ─── API Handlers ─────────────────────────────────────────────────────────

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	sessions, err := s.store.ListSessions(r.Context(), limit)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, sessions)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		jsonError(w, err, http.StatusNotFound)
		return
	}
	jsonOK(w, sess)
}

func (s *Server) listTurns(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	turns, err := s.store.ListTurns(r.Context(), id)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, turns)
}

func (s *Server) listToolCalls(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	calls, err := s.store.ListToolCalls(r.Context(), id)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, calls)
}

func (s *Server) listSignals(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	signals, err := s.store.ListHallucinationSignals(r.Context(), id)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, signals)
}

func (s *Server) getTimeline(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	entries, err := s.store.ListTimelineEntries(r.Context(), id)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}

	jsonOK(w, entries)
}

func (s *Server) getAgentHealth(w http.ResponseWriter, r *http.Request) {
	agentID := mux.Vars(r)["agent_id"]
	windowStr := r.URL.Query().Get("window")
	window := 1 * time.Hour
	if windowStr != "" {
		if d, err := time.ParseDuration(windowStr); err == nil {
			window = d
		}
	}
	health, err := s.store.GetAgentHealth(r.Context(), agentID, window)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, health)
}

// ─── SSE Stream ───────────────────────────────────────────────────────────

// sseStream sends real-time events to the dashboard.
// Currently sends a keep-alive ping; real events are pushed via the gateway's event bus.
func (s *Server) sseStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	sub := events.Subscribe()
	defer events.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sub:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case t := <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: %s\n\n", t.Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

// ─── Dashboard HTML ───────────────────────────────────────────────────────

func (s *Server) serveDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// ─── JSON helpers ─────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
