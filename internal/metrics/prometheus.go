// Package metrics registers all Prometheus metrics for AgentLens.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ─── Session metrics ──────────────────────────────────────────────────

	SessionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_sessions_total",
		Help: "Total number of agent sessions started.",
	}, []string{"provider", "agent_id"})

	SessionsEnded = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_sessions_ended_total",
		Help: "Total number of sessions ended, by outcome.",
	}, []string{"provider", "agent_id", "outcome"})

	SessionDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_session_duration_seconds",
		Help:    "Duration of agent sessions from start to end.",
		Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 1800},
	}, []string{"provider", "outcome"})

	SessionTurns = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_session_turns",
		Help:    "Number of turns per session.",
		Buckets: []float64{1, 2, 3, 5, 8, 13, 21, 34},
	}, []string{"provider"})

	// ─── Turn / LLM metrics ───────────────────────────────────────────────

	TurnsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_turns_total",
		Help: "Total number of LLM request/response turns processed.",
	}, []string{"provider", "model"})

	TurnLatencyMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_turn_latency_ms",
		Help:    "Latency of LLM turns (user message to model response) in milliseconds.",
		Buckets: []float64{100, 250, 500, 1000, 2000, 4000, 8000, 15000, 30000},
	}, []string{"provider", "model"})

	TokensIn = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_tokens_in_total",
		Help: "Total input tokens sent to the model.",
	}, []string{"provider", "model"})

	TokensOut = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_tokens_out_total",
		Help: "Total output tokens received from the model.",
	}, []string{"provider", "model"})

	// ─── Tool Call metrics ────────────────────────────────────────────────

	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_tool_calls_total",
		Help: "Total tool calls intercepted.",
	}, []string{"tool_name", "category", "status"})

	ToolCallDurationMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_tool_call_duration_ms",
		Help:    "Duration of individual tool calls in milliseconds.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
	}, []string{"tool_name", "category"})

	ToolCallErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_tool_call_errors_total",
		Help: "Total tool call errors and denials.",
	}, []string{"tool_name", "status"})

	// ─── Hallucination metrics ────────────────────────────────────────────

	HallucinationSignalsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_hallucination_signals_total",
		Help: "Total hallucination signals detected.",
	}, []string{"signal_type", "provider"})

	HallucinationRiskScore = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_hallucination_risk_score",
		Help:    "Distribution of hallucination risk scores (0.0–1.0).",
		Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
	}, []string{"signal_type"})

	// ─── Frustration metrics ──────────────────────────────────────────────

	FrustrationEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_frustration_events_total",
		Help: "Total frustration events detected (score ≥ 0.6).",
	}, []string{"trigger", "provider"})

	FrustrationScore = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentlens_frustration_score",
		Help:    "Distribution of session frustration scores at event time (0.0–1.0).",
		Buckets: []float64{0.2, 0.4, 0.6, 0.7, 0.8, 0.9, 1.0},
	}, []string{"provider"})

	AbandonedSessions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_abandoned_sessions_total",
		Help: "Sessions abandoned due to user frustration.",
	}, []string{"provider", "agent_id"})

	// ─── Infra correlation metrics ────────────────────────────────────────

	InfraEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_infra_events_total",
		Help: "Infrastructure events recorded by service and type.",
	}, []string{"service", "event_type"})

	InfraCorrelationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentlens_infra_correlations_total",
		Help: "Infra events correlated to degraded model output turns.",
	}, []string{"service"})

	// ─── Active gauges ────────────────────────────────────────────────────

	ActiveSessions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agentlens_active_sessions",
		Help: "Number of currently in-progress agent sessions.",
	}, []string{"provider"})
)
