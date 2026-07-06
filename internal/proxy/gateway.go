// Package proxy implements the HTTP proxy that intercepts LLM API calls
// and tool executions, feeding all signals into the intelligence pipeline.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/AvineshTripathi/AgentLens/internal/config"
	"github.com/AvineshTripathi/AgentLens/internal/intelligence"
	"github.com/AvineshTripathi/AgentLens/internal/metrics"
	"github.com/AvineshTripathi/AgentLens/internal/store"
	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// ─── Session Manager ──────────────────────────────────────────────────────

// SessionManager keeps in-memory session state for fast access during
// a live session, persisting to PostgreSQL asynchronously.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*sessionState
	store    *store.Store
}

type sessionState struct {
	session        *types.Session
	turns          []*types.Turn
	lastTurnAt     *time.Time
	recentMessages []string // last 3 user messages for repeat detection
}

// NewSessionManager creates a session manager backed by the given store.
func NewSessionManager(st *store.Store) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*sessionState),
		store:    st,
	}
}

// GetOrCreate returns an existing session state or creates a new one.
func (sm *SessionManager) GetOrCreate(sessionID, agentID string, provider types.Provider, model string) *sessionState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if state, ok := sm.sessions[sessionID]; ok {
		return state
	}

	sess := &types.Session{
		ID:        sessionID,
		AgentID:   agentID,
		Provider:  provider,
		Model:     model,
		StartedAt: time.Now(),
		Outcome:   types.OutcomeInProgress,
	}

	state := &sessionState{session: sess}
	sm.sessions[sessionID] = state
	metrics.ActiveSessions.WithLabelValues(string(provider)).Inc()
	metrics.SessionsTotal.WithLabelValues(string(provider), agentID).Inc()

	// Persist session start.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sm.store.UpsertSession(ctx, sess); err != nil {
			slog.Error("failed to persist session", "session_id", sessionID, "err", err)
		}
	}()

	return state
}

// RecordTurn adds a turn to the session and persists it.
func (sm *SessionManager) RecordTurn(state *sessionState, turn *types.Turn) {
	sm.mu.Lock()
	now := turn.CreatedAt
	state.session.TurnCount++
	state.session.TotalTokensIn += turn.TokensIn
	state.session.TotalTokensOut += turn.TokensOut
	state.lastTurnAt = &now

	// Rolling message history for repeat detection (keep last 3).
	state.recentMessages = append(state.recentMessages, turn.UserMessage)
	if len(state.recentMessages) > 3 {
		state.recentMessages = state.recentMessages[1:]
	}
	state.turns = append(state.turns, turn)
	sm.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sm.store.InsertTurn(ctx, turn); err != nil {
			slog.Error("failed to persist turn", "turn_id", turn.ID, "err", err)
		}
		if err := sm.store.UpsertSession(ctx, state.session); err != nil {
			slog.Error("failed to update session", "session_id", state.session.ID, "err", err)
		}
	}()
}

// ─── Gateway (HTTP Proxy) ─────────────────────────────────────────────────

// Gateway is the central HTTP proxy. It intercepts requests to LLM providers
// and tool servers, runs the intelligence pipeline, and emits all signals.
type Gateway struct {
	sessionMgr *SessionManager
	hallucDet  *intelligence.HallucinationDetector
	frustAnal  *intelligence.FrustrationAnalyzer
	infraCorr  *intelligence.InfraCorrelator
	traceBldr  *intelligence.DecisionTraceBuilder
	store      *store.Store
	router     *mux.Router
	proxyCfg   config.ProxyConfig
}

// NewGateway assembles the gateway with all dependencies wired up.
func NewGateway(st *store.Store, proxyCfg config.ProxyConfig) *Gateway {
	g := &Gateway{
		sessionMgr: NewSessionManager(st),
		hallucDet:  intelligence.NewHallucinationDetector(),
		frustAnal:  intelligence.NewFrustrationAnalyzer(),
		infraCorr:  intelligence.NewInfraCorrelator(),
		traceBldr:  intelligence.NewDecisionTraceBuilder(),
		store:      st,
		router:     mux.NewRouter(),
		proxyCfg:   proxyCfg,
	}
	g.routes()
	return g
}

// ServeHTTP implements http.Handler.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.router.ServeHTTP(w, r)
}

func (g *Gateway) routes() {
	// Model proxy: intercepts calls to LLM providers (Anthropic, OpenAI, etc.)
	g.router.PathPrefix("/proxy/anthropic/").HandlerFunc(g.handleModelProxy(g.proxyCfg.AnthropicUpstream, types.ProviderAnthropic))
	g.router.PathPrefix("/proxy/openai/").HandlerFunc(g.handleModelProxy(g.proxyCfg.OpenAIUpstream, types.ProviderOpenAI))
	g.router.PathPrefix("/proxy/gemini/").HandlerFunc(g.handleModelProxy(g.proxyCfg.GeminiUpstream, types.ProviderGemini))

	// Tool execution endpoint: agent frameworks send tool calls here.
	g.router.HandleFunc("/tools/execute", g.handleToolExecute).Methods(http.MethodPost)

	// Infra event ingestion: services report their own health events.
	g.router.HandleFunc("/infra/events", g.handleInfraEvent).Methods(http.MethodPost)

	// Session close: explicitly end a session with an outcome.
	g.router.HandleFunc("/sessions/{id}/close", g.handleSessionClose).Methods(http.MethodPost)
}

// ─── Model Proxy ──────────────────────────────────────────────────────────

// handleModelProxy returns an HTTP handler that proxies requests to the
// specified upstream LLM provider, recording the full request/response pair.
func (g *Gateway) handleModelProxy(upstreamBase string, provider types.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read and buffer the request body so we can both log it and forward it.
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 2<<20)) // 2 MB limit
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		// Disable compression so we can intercept plain JSON response
		r.Header.Del("Accept-Encoding")

		// Extract AgentLens metadata from headers.
		sessionID := r.Header.Get("X-AgentLens-Session-ID")
		if sessionID == "" {
			sessionID = uuid.NewString()
		}
		agentID := r.Header.Get("X-AgentLens-Agent-ID")
		if agentID == "" {
			agentID = "unknown"
		}
		userMessage := r.Header.Get("X-AgentLens-User-Message")
		model := extractModelFromBody(bodyBytes)
		if provider == types.ProviderGemini && model == "unknown" {
			parts := strings.Split(r.URL.Path, "/")
			for _, p := range parts {
				if strings.Contains(p, "gemini-") || strings.Contains(p, "gemma-") {
					model = strings.Split(p, ":")[0]
					break
				}
			}
		}

		state := g.sessionMgr.GetOrCreate(sessionID, agentID, provider, model)

		// Proxy the request upstream.
		target, _ := url.Parse(upstreamBase)
		proxy := httputil.NewSingleHostReverseProxy(target)

		// Intercept the response to capture model output.
		rec := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}}

		start := time.Now()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		r.TransferEncoding = nil
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host

		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/"+strings.ToLower(string(provider)))
		r.Host = target.Host

		proxy.ServeHTTP(rec, r)
		latency := time.Since(start)

		// Extract model response from captured body.
		modelResponse := extractModelResponse(provider, rec.body.Bytes())
		tokensIn, tokensOut := extractTokenCounts(provider, rec.body.Bytes())

		// Build the turn.
		turnID := uuid.NewString()
		turn := &types.Turn{
			ID:            turnID,
			SessionID:     sessionID,
			Index:         state.session.TurnCount,
			UserMessage:   userMessage,
			ModelResponse: modelResponse,
			TokensIn:      tokensIn,
			TokensOut:     tokensOut,
			LatencyMs:     int(latency.Milliseconds()),
			CreatedAt:     start,
		}

		// ── Run intelligence pipeline ──────────────────────────────────

		// 1. Frustration analysis
		var prevTurnAt *time.Time
		g.sessionMgr.mu.RLock()
		prevTurnAt = state.lastTurnAt
		recentMsgs := append([]string{}, state.recentMessages...)
		prevFrustration := state.session.FrustrationScore
		g.sessionMgr.mu.RUnlock()

		frustResult := g.frustAnal.Score(userMessage, prevFrustration, prevTurnAt, recentMsgs)
		turn.FrustrationDelta = frustResult.Delta
		state.session.FrustrationScore = frustResult.Score

		// 2. Hallucination analysis (after turn is populated with tool calls from tool proxy)
		// Note: tool calls are attached when processed via /tools/execute before this response.
		halSignals := g.hallucDet.Analyze(turn)
		turn.HallucinationRisk = g.hallucDet.AggregateRisk(halSignals)

		// 3. Emit metrics
		metrics.TurnsTotal.WithLabelValues(string(provider), model).Inc()
		metrics.TurnLatencyMs.WithLabelValues(string(provider), model).Observe(float64(latency.Milliseconds()))
		metrics.TokensIn.WithLabelValues(string(provider), model).Add(float64(tokensIn))
		metrics.TokensOut.WithLabelValues(string(provider), model).Add(float64(tokensOut))

		for _, sig := range halSignals {
			metrics.HallucinationSignalsTotal.WithLabelValues(string(sig.Type), string(provider)).Inc()
			metrics.HallucinationRiskScore.WithLabelValues(string(sig.Type)).Observe(sig.RiskScore)
		}

		if g.frustAnal.ShouldAlert(frustResult.Score) {
			for _, trigger := range frustResult.Triggers {
				metrics.FrustrationEventsTotal.WithLabelValues(string(trigger), string(provider)).Inc()
			}
			metrics.FrustrationScore.WithLabelValues(string(provider)).Observe(frustResult.Score)
		}

		if g.frustAnal.ShouldMarkAbandoned(frustResult.Score) {
			state.session.Outcome = types.OutcomeAbandoned
		}

		// 4. Persist turn + signals
		g.sessionMgr.RecordTurn(state, turn)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, sig := range halSignals {
				if err := g.store.InsertHallucinationSignal(ctx, sig); err != nil {
					slog.Error("failed to persist hallucination signal", "err", err)
				}
			}
			if g.frustAnal.ShouldAlert(frustResult.Score) {
				fe := &types.FrustrationEvent{
					ID:              uuid.NewString(),
					SessionID:       sessionID,
					TurnID:          turnID,
					Score:           frustResult.Score,
					Triggers:        frustResult.Triggers,
					UserMessageSnip: truncate(userMessage, 120),
					DetectedAt:      time.Now(),
				}
				if err := g.store.InsertFrustrationEvent(ctx, fe); err != nil {
					slog.Error("failed to persist frustration event", "err", err)
				}
			}
		}()

		// Headers are already written by the recorder — nothing more to do.
		slog.Info("turn processed",
			"session_id", sessionID,
			"turn_index", turn.Index,
			"latency_ms", turn.LatencyMs,
			"frustration", fmt.Sprintf("%.2f", frustResult.Score),
			"hallucination_risk", fmt.Sprintf("%.2f", turn.HallucinationRisk),
		)
	}
}

// ─── Tool Execution Endpoint ──────────────────────────────────────────────

// ToolExecuteRequest is the request body for /tools/execute.
type ToolExecuteRequest struct {
	SessionID   string          `json:"session_id"`
	TurnID      string          `json:"turn_id"`
	ToolName    string          `json:"tool_name"`
	Category    string          `json:"category"`
	Params      json.RawMessage `json:"params"`
	UpstreamURL string          `json:"upstream_url"` // where to actually execute the tool
}

// handleToolExecute proxies a tool call to its real implementation,
// recording the full lifecycle (pre-execution, result, duration).
func (g *Gateway) handleToolExecute(w http.ResponseWriter, r *http.Request) {
	var req ToolExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	callID := uuid.NewString()
	start := time.Now()

	category := types.ToolCategory(req.Category)
	if category == "" {
		category = types.CategoryCustom
	}

	if req.TurnID == "" {
		g.sessionMgr.mu.RLock()
		if state, ok := g.sessionMgr.sessions[req.SessionID]; ok && len(state.turns) > 0 {
			req.TurnID = state.turns[len(state.turns)-1].ID
		}
		g.sessionMgr.mu.RUnlock()
	}

	// Write pre-execution record.
	tc := &types.ToolCall{
		ID:        callID,
		TurnID:    req.TurnID,
		SessionID: req.SessionID,
		ToolName:  req.ToolName,
		Category:  category,
		Params:    req.Params,
		Status:    types.StatusRunning,
		StartedAt: start,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = g.store.InsertToolCall(ctx, tc)
	}()

	// Execute the real tool.
	var result json.RawMessage
	var execErr error
	var statusCode = http.StatusOK

	if req.UpstreamURL != "" {
		result, execErr = callUpstreamTool(req.UpstreamURL, req.Params)
	} else {
		result = json.RawMessage(`{"error":"no upstream_url provided"}`)
		execErr = fmt.Errorf("no upstream_url")
	}

	duration := time.Since(start)
	completed := time.Now()

	// Determine final status.
	if execErr != nil {
		tc.Status = types.StatusError
		tc.ErrorMessage = execErr.Error()
	} else {
		tc.Status = types.StatusSuccess
	}
	tc.Result = result
	tc.DurationMs = int(duration.Milliseconds())
	tc.CompletedAt = &completed

	// Emit metrics.
	metrics.ToolCallsTotal.WithLabelValues(req.ToolName, string(category), string(tc.Status)).Inc()
	metrics.ToolCallDurationMs.WithLabelValues(req.ToolName, string(category)).Observe(float64(duration.Milliseconds()))
	if tc.Status != types.StatusSuccess {
		metrics.ToolCallErrors.WithLabelValues(req.ToolName, string(tc.Status)).Inc()
	}

	// Detect infra issues from tool errors.
	if tc.Status == types.StatusError || tc.Status == types.StatusTimeout {
		ie := &types.InfraEvent{
			ID:         uuid.NewString(),
			Service:    req.ToolName,
			EventType:  types.InfraError,
			DurationMs: tc.DurationMs,
			ErrorMsg:   tc.ErrorMessage,
			OccurredAt: start,
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = g.store.InsertInfraEvent(ctx, ie)
			metrics.InfraEventsTotal.WithLabelValues(req.ToolName, string(types.InfraError)).Inc()
		}()
	}

	// Update persisted tool call record.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = g.store.UpdateToolCall(ctx, tc)
	}()

	// Return result to caller.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if result != nil {
		_, _ = w.Write(result)
	}

	slog.Info("tool call",
		"tool", req.ToolName,
		"session_id", req.SessionID,
		"status", tc.Status,
		"duration_ms", tc.DurationMs,
	)
}

// ─── Infra Event Ingestion ────────────────────────────────────────────────

// handleInfraEvent accepts manually reported infrastructure events.
func (g *Gateway) handleInfraEvent(w http.ResponseWriter, r *http.Request) {
	var ie types.InfraEvent
	if err := json.NewDecoder(r.Body).Decode(&ie); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if ie.ID == "" {
		ie.ID = uuid.NewString()
	}
	if ie.OccurredAt.IsZero() {
		ie.OccurredAt = time.Now()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := g.store.InsertInfraEvent(ctx, &ie); err != nil {
		http.Error(w, "failed to store event", http.StatusInternalServerError)
		return
	}
	metrics.InfraEventsTotal.WithLabelValues(ie.Service, string(ie.EventType)).Inc()
	w.WriteHeader(http.StatusCreated)
}

// ─── Session Close ────────────────────────────────────────────────────────

type closeRequest struct {
	Outcome string `json:"outcome"`
}

// handleSessionClose marks a session as complete with a final outcome.
func (g *Gateway) handleSessionClose(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]

	var req closeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Outcome = string(types.OutcomeSuccess)
	}

	g.sessionMgr.mu.Lock()
	state, ok := g.sessionMgr.sessions[sessionID]
	if ok {
		now := time.Now()
		state.session.EndedAt = &now
		outcome := types.OutcomeStatus(req.Outcome)
		if outcome == "" {
			outcome = types.OutcomeSuccess
		}
		state.session.Outcome = outcome
	}
	g.sessionMgr.mu.Unlock()

	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := g.store.UpsertSession(ctx, state.session); err != nil {
		http.Error(w, "failed to update session", http.StatusInternalServerError)
		return
	}

	duration := state.session.EndedAt.Sub(state.session.StartedAt)
	metrics.SessionsEnded.WithLabelValues(string(state.session.Provider), state.session.AgentID, req.Outcome).Inc()
	metrics.SessionDurationSeconds.WithLabelValues(string(state.session.Provider), req.Outcome).Observe(duration.Seconds())
	metrics.SessionTurns.WithLabelValues(string(state.session.Provider)).Observe(float64(state.session.TurnCount))
	metrics.ActiveSessions.WithLabelValues(string(state.session.Provider)).Dec()

	if state.session.Outcome == types.OutcomeAbandoned {
		metrics.AbandonedSessions.WithLabelValues(string(state.session.Provider), state.session.AgentID).Inc()
	}

	w.WriteHeader(http.StatusOK)
}

// ─── Helpers ──────────────────────────────────────────────────────────────

// responseRecorder captures the response body while still writing it to the client.
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher to support SSE streaming.
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap allows http.ResponseController to access the underlying writer.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// extractModelFromBody tries to pull the model name from the request JSON body.
func extractModelFromBody(body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return "unknown"
	}
	if raw, ok := obj["model"]; ok {
		var model string
		if err := json.Unmarshal(raw, &model); err == nil {
			return model
		}
	}
	return "unknown"
}

// extractModelResponse pulls the text content from provider-specific response formats.
func extractModelResponse(provider types.Provider, body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}

	switch provider {
	case types.ProviderAnthropic:
		// Anthropic: { "content": [{ "type": "text", "text": "..." }] }
		if raw, ok := obj["content"]; ok {
			var content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &content); err == nil {
				var sb strings.Builder
				for _, c := range content {
					if c.Type == "text" {
						sb.WriteString(c.Text)
					}
				}
				return sb.String()
			}
		}

	case types.ProviderOpenAI:
		// OpenAI: { "choices": [{ "message": { "content": "..." } }] }
		if raw, ok := obj["choices"]; ok {
			var choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal(raw, &choices); err == nil && len(choices) > 0 {
				return choices[0].Message.Content
			}
		}

	case types.ProviderGemini:
		if raw, ok := obj["candidates"]; ok {
			var candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			}
			if err := json.Unmarshal(raw, &candidates); err == nil && len(candidates) > 0 {
				var sb strings.Builder
				for _, p := range candidates[0].Content.Parts {
					sb.WriteString(p.Text)
				}
				return sb.String()
			}
		}
	}
	return ""
}

// extractTokenCounts pulls token usage from provider response bodies.
func extractTokenCounts(provider types.Provider, body []byte) (in, out int) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0, 0
	}

	switch provider {
	case types.ProviderAnthropic:
		// { "usage": { "input_tokens": N, "output_tokens": N } }
		if raw, ok := obj["usage"]; ok {
			var usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}
			if err := json.Unmarshal(raw, &usage); err == nil {
				return usage.InputTokens, usage.OutputTokens
			}
		}
	case types.ProviderOpenAI:
		// { "usage": { "prompt_tokens": N, "completion_tokens": N } }
		if raw, ok := obj["usage"]; ok {
			var usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			}
			if err := json.Unmarshal(raw, &usage); err == nil {
				return usage.PromptTokens, usage.CompletionTokens
			}
		}
	}
	return 0, 0
}

// callUpstreamTool makes a POST request to the real tool server.
func callUpstreamTool(upstreamURL string, params json.RawMessage) (json.RawMessage, error) {
	resp, err := http.Post(upstreamURL, "application/json", bytes.NewReader(params))
	if err != nil {
		return nil, fmt.Errorf("upstream call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read upstream response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	return body, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
