// Package proxy implements the HTTP proxy that intercepts LLM API calls
// and tool executions, feeding all signals into the intelligence pipeline.
package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/AvineshTripathi/AgentLens/internal/config"
	"github.com/AvineshTripathi/AgentLens/internal/intelligence"
	"github.com/AvineshTripathi/AgentLens/internal/metrics"
	"github.com/AvineshTripathi/AgentLens/internal/proxy/providers"
	"github.com/AvineshTripathi/AgentLens/internal/store"
	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// ─── Session Manager ──────────────────────────────────────────────────────

// sessionTTL is how long a session is eligible for continuation after its
// last turn.  Requests arriving after this window are treated as a new session
// even if the first-message hash matches an older one.
const sessionTTL = 4 * time.Hour

// SessionManager keeps in-memory session state for fast access during
// a live session, persisting to PostgreSQL asynchronously.
//
// Two-level session index:
//   - sessions:   primary map — session_id → *sessionState
//   - rootIndex:  secondary map — anchor_hash → session_id
//     (anchor_hash == the value returned by adapter.ExtractSessionID)
//
// The rootIndex lets the manager coalesce all turns of the same conversation
// into one session even when the request body drifts slightly between turns
// (e.g. Claude CLI injects dynamic git-status into the first message).
type SessionManager struct {
	mu        sync.RWMutex
	sessions  map[string]*sessionState
	rootIndex map[string]string // anchor_hash → session_id
	store     *store.Store
}

type sessionState struct {
	session        *types.Session
	turns          []*types.Turn
	lastTurnAt     *time.Time
	lastActiveAt   time.Time
	recentMessages []string // last 3 user messages for repeat detection
}

// NewSessionManager creates a session manager backed by the given store.
func NewSessionManager(st *store.Store) *SessionManager {
	return &SessionManager{
		sessions:  make(map[string]*sessionState),
		rootIndex: make(map[string]string),
		store:     st,
	}
}

// GetOrCreate returns an existing session state or creates a new one.
//
// anchorHash (same value as sessionID for stateless providers) is stored in
// rootIndex so that subsequent turns of the same conversation — even if the
// computed hash drifts by a few characters — still resolve to the original
// session as long as it is within sessionTTL.
// GetOrCreate resolves a session using a three-level lookup:
//
//  1. Exact sessionID match (fastest path, O(1)).
//  2. continuationID match — the stable hash of the first assistant response,
//     used as a fallback when the first-message hash drifts between turns
//     (e.g. Claude CLI injects dynamic context into messages[0]).
//  3. rootIndex anchor match — handles minor hash variation for the same anchor.
//
// If none match, a new session is created and both sessionID and continuationID
// are registered in the rootIndex for future lookups.
func (sm *SessionManager) GetOrCreate(sessionID, continuationID, agentID string, provider types.Provider, model string) *sessionState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// ── Level-1: exact session ID ─────────────────────────────────────────
	if state, ok := sm.sessions[sessionID]; ok {
		state.lastActiveAt = time.Now()
		return state
	}

	// ── Level-2: continuationID — first assistant response hash ──────────
	// messages[1] in subsequent requests is the first assistant reply,
	// sent verbatim by the client. Its hash is perfectly stable across all
	// turns of the same conversation.
	if continuationID != "" {
		if existingID, ok := sm.rootIndex[continuationID]; ok {
			if state, ok2 := sm.sessions[existingID]; ok2 {
				if time.Since(state.lastActiveAt) < sessionTTL {
					sm.sessions[sessionID] = state // alias so level-1 hits next time
					state.lastActiveAt = time.Now()
					slog.Debug("session: continued via continuationID",
						"cont_id", continuationID, "resolved_to", existingID)
					return state
				}
				delete(sm.rootIndex, continuationID)
			}
		}
	}

	// ── Level-3: rootIndex anchor (handles minor hash drift on sessionID) ─
	if existingID, ok := sm.rootIndex[sessionID]; ok {
		if state, ok2 := sm.sessions[existingID]; ok2 {
			if time.Since(state.lastActiveAt) < sessionTTL {
				sm.sessions[sessionID] = state
				state.lastActiveAt = time.Now()
				slog.Debug("session: continued via rootIndex",
					"anchor", sessionID, "resolved_to", existingID)
				return state
			}
			delete(sm.rootIndex, sessionID)
		}
	}

	// ── Create new session ───────────────────────────────────────────────
	sess := &types.Session{
		ID:        sessionID,
		AgentID:   agentID,
		Provider:  provider,
		Model:     model,
		StartedAt: time.Now(),
		Outcome:   types.OutcomeInProgress,
	}
	state := &sessionState{
		session:      sess,
		lastActiveAt: time.Now(),
	}
	sm.sessions[sessionID] = state
	sm.rootIndex[sessionID] = sessionID
	if continuationID != "" {
		sm.rootIndex[continuationID] = sessionID
	}
	metrics.ActiveSessions.WithLabelValues(string(provider)).Inc()
	metrics.SessionsTotal.WithLabelValues(string(provider), agentID).Inc()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sm.store.UpsertSession(ctx, sess); err != nil {
			slog.Error("failed to persist session", "session_id", sessionID, "err", err)
		}
	}()

	return state
}

// RecordTurn adds a turn to the session, persists it, and registers the model
// response as a continuation key so the NEXT request can find this session via
// the assistant-response hash (level-2 lookup in GetOrCreate).
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

	// Register the combined user+assistant hash as a continuation key.
	// On the NEXT request the client will resend both messages verbatim,
	// so ExtractContinuationID can recompute the same key and find this session.
	if turn.ModelResponse != "" && turn.UserMessage != "" {
		respKey := turn.ModelResponse
		if len(respKey) > 200 {
			respKey = respKey[:200]
		}
		combined := "conv:" + turn.UserMessage + "|" + respKey
		continuationKey := uuid.NewMD5(uuid.NameSpaceOID, []byte(combined)).String()
		sm.rootIndex[continuationKey] = state.session.ID
	}
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
	forward    *goproxy.ProxyHttpServer
	proxyCfg   config.ProxyConfig
	registry   *providers.Registry
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
		forward:    goproxy.NewProxyHttpServer(),
		proxyCfg:   proxyCfg,
		registry:   providers.DefaultRegistry(),
	}
	g.routes()
	g.setupForwardProxy()
	return g
}

// ServeHTTP implements http.Handler.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect || r.URL.IsAbs() {
		g.forward.ServeHTTP(w, r)
		return
	}
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

func (g *Gateway) setupForwardProxy() {
	g.forward.Verbose = true

	// Attempt to load local CA
	homeDir, err := os.UserHomeDir()
	if err == nil {
		caCertPath := filepath.Join(homeDir, ".agentlens", "ca.crt")
		caKeyPath := filepath.Join(homeDir, ".agentlens", "ca.key")
		caCert, err1 := os.ReadFile(caCertPath)
		caKey, err2 := os.ReadFile(caKeyPath)
		if err1 == nil && err2 == nil {
			if tlsc, err := tls.X509KeyPair(caCert, caKey); err == nil {
				if tlsc.Leaf, err = x509.ParseCertificate(tlsc.Certificate[0]); err == nil {
					goproxy.GoproxyCa = tlsc
					goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&tlsc)}
					goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&tlsc)}
					goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&tlsc)}
					goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&tlsc)}
					slog.Info("Loaded AgentLens Root CA for MITM interception")
				}
			}
		} else {
			slog.Warn("AgentLens Root CA not found. MITM interception will not work. Run cmd/agentlens-ca to generate it.")
		}
	}

	// Always MITM all registered provider domains — built automatically from the registry.
	g.forward.OnRequest(goproxy.ReqHostMatches(regexp.MustCompile(g.registry.MITMRegex()))).HandleConnect(goproxy.AlwaysMitm)

	// Route intercepted traffic through our pipeline
	g.forward.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		host := req.URL.Hostname()
		path := req.URL.Path

		// Look up the right adapter by path first, then host.
		adapter := g.registry.Find(host, path)
		if adapter == nil {
			return req, nil
		}

		g.handleInterceptedRequest(req, ctx, adapter)
		return req, nil
	})

	g.forward.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if ctx.UserData != nil {
			g.handleInterceptedResponse(resp, ctx)
		}
		return resp
	})
}

type proxyState struct {
	state       *sessionState
	adapter     providers.Adapter
	model       string
	userMessage string
	start       time.Time
}

func (g *Gateway) handleInterceptedRequest(req *http.Request, ctx *goproxy.ProxyCtx, adapter providers.Adapter) {
	if req.Body == nil {
		return
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 2<<20))
	if err == nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Drop internal housekeeping requests (like Claude Code title generation)
	// before they pollute the session tracking system.
	if sa, ok := adapter.(interface{ ShouldSkip([]byte) bool }); ok {
		if sa.ShouldSkip(bodyBytes) {
			slog.Debug("gateway: skipping MITM internal request", "host", req.URL.Hostname())
			return
		}
	}

	sessionID := req.Header.Get("X-AgentLens-Session-ID")
	if sessionID == "" {
		sessionID = req.Header.Get("X-Claude-Code-Session-Id")
	}
	if sessionID == "" {
		sessionID = adapter.ExtractSessionID(bodyBytes)
		if sessionID == "" {
			sessionID = uuid.NewString()
		}
	}
	agentID := req.Header.Get("X-AgentLens-Agent-ID")
	if agentID == "" {
		agentID = "unknown"
	}

	userMessage := req.Header.Get("X-AgentLens-User-Message")
	if userMessage == "" {
		userMessage = adapter.ExtractUserMessage(bodyBytes)
	}

	model := adapter.ExtractModel(bodyBytes, req.URL.Path)

	// Extract a stable continuation ID from the request if the adapter supports it.
	// This is the hash of the first assistant response (messages[1]) which the
	// client resends verbatim on every turn — immune to dynamic context injection.
	continuationID := ""
	if ca, ok := adapter.(interface{ ExtractContinuationID([]byte) string }); ok {
		continuationID = ca.ExtractContinuationID(bodyBytes)
	}
	state := g.sessionMgr.GetOrCreate(sessionID, continuationID, agentID, adapter.Provider(), model)

	ctx.UserData = &proxyState{
		state:       state,
		adapter:     adapter,
		model:       model,
		userMessage: userMessage,
		start:       time.Now(),
	}
}

func (g *Gateway) handleInterceptedResponse(resp *http.Response, ctx *goproxy.ProxyCtx) {
	if resp == nil || resp.Body == nil {
		return
	}
	pState, ok := ctx.UserData.(*proxyState)
	if !ok {
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5 MB limit
	if err == nil {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	latency := time.Since(pState.start)
	modelResponse := pState.adapter.ExtractModelResponse(bodyBytes)
	tokensIn, tokensOut := pState.adapter.ExtractTokenCounts(bodyBytes)
	provider := pState.adapter.Provider()

	// ── Duplicate Turn (Tool Loop) Detection ──────────────────────
	var prevTurn *types.Turn
	g.sessionMgr.mu.RLock()
	if len(pState.state.turns) > 0 {
		prevTurn = pState.state.turns[len(pState.state.turns)-1]
	}
	g.sessionMgr.mu.RUnlock()

	isToolLoop := false
	if prevTurn != nil && pState.userMessage != "" && pState.userMessage == prevTurn.UserMessage {
		// If exact same user message within 30 seconds, it's almost certainly a tool loop step.
		if time.Since(prevTurn.CreatedAt) < 30*time.Second {
			isToolLoop = true
		}
	}

	if isToolLoop {
		// Update the previous turn instead of creating a new one
		prevTurn.ModelResponse = modelResponse
		prevTurn.TokensIn += tokensIn
		prevTurn.TokensOut += tokensOut
		prevTurn.LatencyMs += int(latency.Milliseconds())

		// Persist update in background
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := g.store.UpdateTurn(ctx, prevTurn); err != nil {
				slog.Error("failed to update turn", "err", err)
			}
		}()
		
		slog.Info("tool loop turn aggregated", "session_id", pState.state.session.ID, "turn_id", prevTurn.ID)
		return
	}

	// Build a new turn.
	turnID := uuid.NewString()
	turn := &types.Turn{
		ID:            turnID,
		SessionID:     pState.state.session.ID,
		Index:         pState.state.session.TurnCount,
		UserMessage:   pState.userMessage,
		ModelResponse: modelResponse,
		TokensIn:      tokensIn,
		TokensOut:     tokensOut,
		LatencyMs:     int(latency.Milliseconds()),
		CreatedAt:     pState.start,
	}

	// 1. Frustration analysis
	var prevTurnAt *time.Time
	g.sessionMgr.mu.RLock()
	prevTurnAt = pState.state.lastTurnAt
	recentMsgs := append([]string{}, pState.state.recentMessages...)
	prevFrustration := pState.state.session.FrustrationScore
	g.sessionMgr.mu.RUnlock()

	frustResult := g.frustAnal.Score(pState.userMessage, prevFrustration, prevTurnAt, recentMsgs)
	turn.FrustrationDelta = frustResult.Delta
	pState.state.session.FrustrationScore = frustResult.Score

	// 2. Hallucination analysis
	halSignals := g.hallucDet.Analyze(turn)
	turn.HallucinationRisk = g.hallucDet.AggregateRisk(halSignals)

	// 3. Emit metrics
	metrics.TurnsTotal.WithLabelValues(string(provider), pState.model).Inc()
	metrics.TurnLatencyMs.WithLabelValues(string(provider), pState.model).Observe(float64(latency.Milliseconds()))
	metrics.TokensIn.WithLabelValues(string(provider), pState.model).Add(float64(tokensIn))
	metrics.TokensOut.WithLabelValues(string(provider), pState.model).Add(float64(tokensOut))

	if g.frustAnal.ShouldAlert(frustResult.Score) {
		pState.state.session.Outcome = types.OutcomeAbandoned
	}

	// 4. Persist
	g.sessionMgr.RecordTurn(pState.state, turn)

	slog.Info("mitm turn processed",
		"session_id", pState.state.session.ID,
		"provider", provider,
		"latency_ms", turn.LatencyMs,
		"frustration", fmt.Sprintf("%.2f", frustResult.Score),
	)
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

		// Resolve adapter: prefer path-based lookup, fall back to provider type.
		adapter := g.registry.Find(r.Host, r.URL.Path)
		if adapter == nil {
			switch provider {
			case types.ProviderAnthropic:
				adapter = &providers.AnthropicAdapter{}
			case types.ProviderOpenAI:
				adapter = &providers.OpenAIAdapter{}
			default:
				adapter = &providers.GeminiAdapter{}
			}
		}

		// Extract AgentLens metadata from headers.
		
		// Drop internal housekeeping requests (like Claude Code title generation)
		// before they pollute the session tracking system.
		if sa, ok := adapter.(interface{ ShouldSkip([]byte) bool }); ok {
			if sa.ShouldSkip(bodyBytes) {
				slog.Debug("gateway: skipping proxy internal request", "provider", provider)
				target, _ := url.Parse(upstreamBase)
				proxy := httputil.NewSingleHostReverseProxy(target)
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				r.ContentLength = int64(len(bodyBytes))
				r.TransferEncoding = nil
				r.URL.Scheme = target.Scheme
				r.URL.Host = target.Host
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/"+strings.ToLower(string(provider)))
				r.Host = target.Host
				proxy.ServeHTTP(w, r)
				return
			}
		}

		sessionID := r.Header.Get("X-AgentLens-Session-ID")
		if sessionID == "" {
			sessionID = r.Header.Get("X-Claude-Code-Session-Id")
		}
		if sessionID == "" {
			sessionID = adapter.ExtractSessionID(bodyBytes)
			if sessionID == "" {
				sessionID = uuid.NewString()
			}
		}
		agentID := r.Header.Get("X-AgentLens-Agent-ID")
		if agentID == "" {
			agentID = "unknown"
		}
		userMessage := r.Header.Get("X-AgentLens-User-Message")
		if userMessage == "" {
			userMessage = adapter.ExtractUserMessage(bodyBytes)
		}

		model := adapter.ExtractModel(bodyBytes, r.URL.Path)

		contID := ""
		if ca, ok := adapter.(interface{ ExtractContinuationID([]byte) string }); ok {
			contID = ca.ExtractContinuationID(bodyBytes)
		}
		state := g.sessionMgr.GetOrCreate(sessionID, contID, agentID, provider, model)

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
		modelResponse := adapter.ExtractModelResponse(rec.body.Bytes())
		tokensIn, tokensOut := adapter.ExtractTokenCounts(rec.body.Bytes())

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
