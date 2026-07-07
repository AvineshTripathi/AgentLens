// Package intelligence implements the core signal analyzers:
// hallucination detection, frustration scoring, and infra correlation.
package intelligence

import (
	"strings"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// ─── Infra Correlator ─────────────────────────────────────────────────────

// InfraCorrelator links infrastructure events to turns where they likely
// degraded the model's output. Uses a time-window proximity heuristic.
type InfraCorrelator struct {
	// CorrelationWindowSecs is the look-back window: if an infra event occurred
	// within this many seconds before a turn, it's considered correlated.
	CorrelationWindowSecs int
}

// NewInfraCorrelator creates a correlator with a 45-second default window.
func NewInfraCorrelator() *InfraCorrelator {
	return &InfraCorrelator{CorrelationWindowSecs: 45}
}

// Correlate returns correlations between a set of infra events and a turn.
// Confidence is higher when the infra event involved a tool the turn used.
func (c *InfraCorrelator) Correlate(
	turn *types.Turn,
	events []types.InfraEvent,
) []types.InfraCorrelation {
	var correlations []types.InfraCorrelation

	window := time.Duration(c.CorrelationWindowSecs) * time.Second

	// Build a set of service names actually used in this turn's tool calls.
	usedServices := map[string]bool{}
	for _, tc := range turn.ToolCalls {
		usedServices[serviceFromTool(tc.ToolName)] = true
	}

	for _, ev := range events {
		// Only consider events that happened before this turn.
		if ev.OccurredAt.After(turn.CreatedAt) {
			continue
		}
		if turn.CreatedAt.Sub(ev.OccurredAt) > window {
			continue
		}

		// Base confidence from how close in time the event was.
		timeProximity := 1.0 - (turn.CreatedAt.Sub(ev.OccurredAt).Seconds() / float64(c.CorrelationWindowSecs))
		confidence := timeProximity * 0.5 // 0–0.5 from time alone

		// Boost if the event's service matches a tool used in this turn.
		if usedServices[ev.Service] {
			confidence += 0.3
		}

		// Boost if turn's hallucination risk is elevated.
		if turn.HallucinationRisk > 0.5 {
			confidence += 0.2
		}

		correlations = append(correlations, types.InfraCorrelation{
			InfraEventID: ev.ID,
			TurnID:       turn.ID,
			SessionID:    turn.SessionID,
			Confidence:   clamp(confidence, 0, 1.0),
			WindowSecs:   c.CorrelationWindowSecs,
		})
	}
	return correlations
}

// ─── Decision Trace Builder ───────────────────────────────────────────────

// DecisionTrace describes the "why" behind a turn: what the model decided,
// which tools it chose, and what signals were detected.
type DecisionTrace struct {
	TurnID            string
	SessionID         string
	TurnIndex         int
	UserIntent        string   // best-effort classification
	ToolsChosen       []string // ordered list of tool names called
	PolicyFlags       []string // any policy checks that fired
	HallucinationRisk float64
	FrustrationDelta  float64
	InfraIssues       []string // correlated infra events
	Summary           string   // human-readable one-liner
}

// DecisionTraceBuilder reconstructs decision traces from existing turn data.
type DecisionTraceBuilder struct{}

// NewDecisionTraceBuilder creates a builder.
func NewDecisionTraceBuilder() *DecisionTraceBuilder {
	return &DecisionTraceBuilder{}
}

// Build constructs a DecisionTrace for a given turn.
func (b *DecisionTraceBuilder) Build(
	turn *types.Turn,
	hallucinationSignals []*types.HallucinationSignal,
	infraCorrelations []types.InfraCorrelation,
) *DecisionTrace {
	tools := make([]string, 0, len(turn.ToolCalls))
	for _, tc := range turn.ToolCalls {
		tools = append(tools, tc.ToolName)
	}

	infraIssues := make([]string, 0, len(infraCorrelations))
	for _, c := range infraCorrelations {
		if c.Confidence > 0.4 {
			infraIssues = append(infraIssues, "correlated infra event (confidence: "+percentStr(c.Confidence)+")")
		}
	}

	summary := buildSummary(turn, tools, hallucinationSignals, infraIssues)

	return &DecisionTrace{
		TurnID:            turn.ID,
		SessionID:         turn.SessionID,
		TurnIndex:         turn.Index,
		UserIntent:        classifyIntent(turn.UserMessage),
		ToolsChosen:       tools,
		HallucinationRisk: turn.HallucinationRisk,
		FrustrationDelta:  turn.FrustrationDelta,
		InfraIssues:       infraIssues,
		Summary:           summary,
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func serviceFromTool(toolName string) string {
	lower := strings.ToLower(toolName)
	switch {
	case strings.Contains(lower, "db") || strings.Contains(lower, "sql") || strings.Contains(lower, "query"):
		return "postgres"
	case strings.Contains(lower, "redis") || strings.Contains(lower, "cache"):
		return "redis"
	case strings.Contains(lower, "s3") || strings.Contains(lower, "bucket"):
		return "s3"
	case strings.Contains(lower, "http") || strings.Contains(lower, "api") || strings.Contains(lower, "fetch"):
		return "http"
	default:
		return toolName
	}
}

// classifyIntent does a very lightweight intent classification
// by looking for keyword signals in the user message.
func classifyIntent(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case containsAny(lower, []string{"write", "create", "generate", "build", "make"}):
		return "generation"
	case containsAny(lower, []string{"read", "show", "get", "find", "list", "what is", "tell me"}):
		return "retrieval"
	case containsAny(lower, []string{"fix", "debug", "error", "issue", "problem", "wrong"}):
		return "debugging"
	case containsAny(lower, []string{"convert", "transform", "change", "update", "modify"}):
		return "transformation"
	case containsAny(lower, []string{"summarize", "summary", "overview", "explain", "describe"}):
		return "summarization"
	default:
		return "unknown"
	}
}

func buildSummary(
	turn *types.Turn,
	tools []string,
	signals []*types.HallucinationSignal,
	infraIssues []string,
) string {
	parts := []string{}
	if len(tools) > 0 {
		parts = append(parts, strings.Join(tools, "→"))
	}
	if len(signals) > 0 {
		parts = append(parts, "⚠ hallucination risk: "+percentStr(turn.HallucinationRisk))
	}
	if turn.FrustrationDelta > 0.2 {
		parts = append(parts, "🔥 frustration spike: +"+percentStr(turn.FrustrationDelta))
	}
	if len(infraIssues) > 0 {
		parts = append(parts, "⚡ infra correlation")
	}
	if len(parts) == 0 {
		return "normal turn"
	}
	return strings.Join(parts, " | ")
}
