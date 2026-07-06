// Package types defines the core domain types for AgentLens.
// Every signal the platform captures — sessions, turns, tool calls,
// hallucinations, frustration events — lives here.
package types

import (
	"encoding/json"
	"time"
)

// ─── Provider / Model ──────────────────────────────────────────────────────

// Provider identifies the LLM provider.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
	ProviderCustom    Provider = "custom"
)

// ─── Tool Categories ───────────────────────────────────────────────────────

// ToolCategory classifies the kind of operation a tool performs.
type ToolCategory string

const (
	CategoryFileOps  ToolCategory = "file_ops"
	CategoryHTTP     ToolCategory = "http"
	CategoryDatabase ToolCategory = "database"
	CategoryCompute  ToolCategory = "compute"
	CategoryCustom   ToolCategory = "custom"
)

// ─── Session ───────────────────────────────────────────────────────────────

// OutcomeStatus describes how a session ended.
type OutcomeStatus string

const (
	OutcomeSuccess    OutcomeStatus = "success"
	OutcomeAbandoned  OutcomeStatus = "abandoned"
	OutcomeEscalated  OutcomeStatus = "escalated"
	OutcomeFailed     OutcomeStatus = "failed"
	OutcomeInProgress OutcomeStatus = "in_progress"
)

// Session represents a full agent interaction — a sequence of turns
// between a user and an AI agent from start to finish.
type Session struct {
	ID               string            `json:"id"`
	UserID           string            `json:"user_id,omitempty"`
	AgentID          string            `json:"agent_id"`
	Provider         Provider          `json:"provider"`
	Model            string            `json:"model"`
	StartedAt        time.Time         `json:"started_at"`
	EndedAt          *time.Time        `json:"ended_at,omitempty"`
	Outcome          OutcomeStatus     `json:"outcome"`
	TurnCount        int               `json:"turn_count"`
	TotalTokensIn    int               `json:"total_tokens_in"`
	TotalTokensOut   int               `json:"total_tokens_out"`
	FrustrationScore float64           `json:"frustration_score"` // 0.0 (calm) → 1.0 (rage)
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ─── Turn ──────────────────────────────────────────────────────────────────

// Turn is one request/response cycle within a session.
// It contains the user message, model response, all tool calls made,
// and computed signals (frustration, hallucination risk).
type Turn struct {
	ID               string     `json:"id"`
	SessionID        string     `json:"session_id"`
	Index            int        `json:"index"`
	UserMessage      string     `json:"user_message"`
	ModelResponse    string     `json:"model_response"`
	ThinkingTrace    string     `json:"thinking_trace,omitempty"` // CoT if available
	TokensIn         int        `json:"tokens_in"`
	TokensOut        int        `json:"tokens_out"`
	LatencyMs        int        `json:"latency_ms"`
	FrustrationDelta float64    `json:"frustration_delta"` // change from previous turn
	HallucinationRisk float64   `json:"hallucination_risk"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ─── Tool Call ─────────────────────────────────────────────────────────────

// ToolCallStatus describes the result of a tool execution.
type ToolCallStatus string

const (
	StatusSuccess ToolCallStatus = "success"
	StatusError   ToolCallStatus = "error"
	StatusDenied  ToolCallStatus = "denied"
	StatusTimeout ToolCallStatus = "timeout"
	StatusRunning ToolCallStatus = "running"
)

// ToolCall is a single invocation of a tool within a Turn.
type ToolCall struct {
	ID           string          `json:"id"`
	TurnID       string          `json:"turn_id"`
	SessionID    string          `json:"session_id"`
	TraceID      string          `json:"trace_id,omitempty"`
	SpanID       string          `json:"span_id,omitempty"`
	ToolName     string          `json:"tool_name"`
	Category     ToolCategory    `json:"category"`
	Params       json.RawMessage `json:"params,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	Status       ToolCallStatus  `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	DurationMs   int             `json:"duration_ms"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// ─── Hallucination Signal ──────────────────────────────────────────────────

// HallucinationType classifies the nature of a detected hallucination.
type HallucinationType string

const (
	// Model's claim directly contradicts what a tool returned.
	HallucinationToolContradiction HallucinationType = "tool_contradiction"

	// Model cited a resource (URL, file, DB row) that doesn't exist.
	HallucinationDeadReference HallucinationType = "dead_reference"

	// Model claimed it performed an action but no tool call was made.
	HallucinationFabricatedAction HallucinationType = "fabricated_action"

	// Model used high-confidence language ("definitely") but the tool errored.
	HallucinationFalseConfidence HallucinationType = "false_confidence"

	// Same prompt produced wildly different answers across sessions.
	HallucinationNonDeterminism HallucinationType = "non_determinism"
)

// HallucinationSignal records a detected or suspected hallucination.
type HallucinationSignal struct {
	ID          string            `json:"id"`
	SessionID   string            `json:"session_id"`
	TurnID      string            `json:"turn_id"`
	Type        HallucinationType `json:"type"`
	RiskScore   float64           `json:"risk_score"` // 0.0 → 1.0
	ModelClaim  string            `json:"model_claim,omitempty"`
	ActualValue string            `json:"actual_value,omitempty"`
	Evidence    string            `json:"evidence,omitempty"`
	DetectedAt  time.Time         `json:"detected_at"`
}

// ─── Frustration Event ────────────────────────────────────────────────────

// FrustrationTrigger identifies what caused a frustration spike.
type FrustrationTrigger string

const (
	TriggerRepeatedQuestion   FrustrationTrigger = "repeated_question"
	TriggerRagePrompting      FrustrationTrigger = "rage_prompting"   // very fast repeated messages
	TriggerNegativeSentiment  FrustrationTrigger = "negative_sentiment"
	TriggerExplicitCorrection FrustrationTrigger = "explicit_correction"
	TriggerAbandonmentSignal  FrustrationTrigger = "abandonment_signal"
	TriggerAllCaps            FrustrationTrigger = "all_caps"
	TriggerExclamations       FrustrationTrigger = "exclamations"
)

// FrustrationEvent records a notable spike in user frustration.
type FrustrationEvent struct {
	ID              string               `json:"id"`
	SessionID       string               `json:"session_id"`
	TurnID          string               `json:"turn_id"`
	Score           float64              `json:"score"`
	Triggers        []FrustrationTrigger `json:"triggers"`
	UserMessageSnip string               `json:"user_message_snip,omitempty"` // first 120 chars
	DetectedAt      time.Time            `json:"detected_at"`
}

// ─── Infrastructure Event ─────────────────────────────────────────────────

// InfraEventType classifies the infrastructure issue.
type InfraEventType string

const (
	InfraTimeout   InfraEventType = "timeout"
	InfraRateLimit InfraEventType = "rate_limit"
	InfraError     InfraEventType = "error"
	InfraSlow      InfraEventType = "slow" // above-threshold latency
)

// InfraEvent records an infrastructure-level issue (DB timeout, API error, etc.)
// that may have contributed to poor agent output.
type InfraEvent struct {
	ID         string         `json:"id"`
	Service    string         `json:"service"`   // "postgres", "redis", "openai-api"
	EventType  InfraEventType `json:"event_type"`
	DurationMs int            `json:"duration_ms,omitempty"`
	ErrorMsg   string         `json:"error_msg,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

// InfraCorrelation links an InfraEvent to a Turn where it likely influenced output.
type InfraCorrelation struct {
	InfraEventID string  `json:"infra_event_id"`
	TurnID       string  `json:"turn_id"`
	SessionID    string  `json:"session_id"`
	Confidence   float64 `json:"confidence"` // how strongly correlated
	WindowSecs   int     `json:"window_secs"`
}

// ─── Agent Health Snapshot ────────────────────────────────────────────────

// AgentHealth is an aggregated health snapshot for a given time window.
type AgentHealth struct {
	AgentID              string    `json:"agent_id"`
	WindowStart          time.Time `json:"window_start"`
	WindowEnd            time.Time `json:"window_end"`
	TotalSessions        int       `json:"total_sessions"`
	SuccessfulSessions   int       `json:"successful_sessions"`
	AbandonedSessions    int       `json:"abandoned_sessions"`
	SuccessRate          float64   `json:"success_rate"`
	AvgFrustrationScore  float64   `json:"avg_frustration_score"`
	HallucinationRate    float64   `json:"hallucination_rate"` // pct of turns flagged
	AvgSessionTurns      float64   `json:"avg_session_turns"`
	AvgLatencyMs         float64   `json:"avg_latency_ms"`
	FrustrationEvents    int       `json:"frustration_events"`
	InfraCorrelations    int       `json:"infra_correlations"`
}

// ─── Proxy Request/Response ───────────────────────────────────────────────

// ProxyRequest is the normalized request body AgentLens receives from
// any ingestion surface (HTTP proxy, SDK, MCP adapter).
type ProxyRequest struct {
	SessionID   string            `json:"session_id"`
	UserID      string            `json:"user_id,omitempty"`
	AgentID     string            `json:"agent_id"`
	Provider    Provider          `json:"provider"`
	Model       string            `json:"model"`
	UserMessage string            `json:"user_message"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// ProxyResponse wraps what AgentLens returns after processing a model response.
type ProxyResponse struct {
	TurnID            string                `json:"turn_id"`
	SessionID         string                `json:"session_id"`
	ModelResponse     string                `json:"model_response"`
	ToolCalls         []ToolCall            `json:"tool_calls,omitempty"`
	FrustrationScore  float64               `json:"frustration_score"`
	HallucinationRisk float64               `json:"hallucination_risk"`
	Signals           []HallucinationSignal `json:"signals,omitempty"`
}
