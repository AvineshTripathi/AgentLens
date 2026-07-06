// Package intelligence implements the core signal analyzers:
// hallucination detection, frustration scoring, and infra correlation.
package intelligence

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/AvineshTripathi/AgentLens/internal/types"
	"github.com/google/uuid"
)

// ─── Hallucination Detector ───────────────────────────────────────────────

// HallucinationDetector cross-references model claims against tool results
// to detect likely hallucinations without requiring a secondary LLM call.
// All checks are deterministic and run in < 5ms.
type HallucinationDetector struct{}

// NewHallucinationDetector creates a ready-to-use detector.
func NewHallucinationDetector() *HallucinationDetector {
	return &HallucinationDetector{}
}

// Analyze runs all detection heuristics for a turn and returns any signals found.
func (d *HallucinationDetector) Analyze(turn *types.Turn) []*types.HallucinationSignal {
	var signals []*types.HallucinationSignal

	if turn.ModelResponse == "" {
		return signals
	}

	// Check 1: fabricated tool actions — model claims it ran something it didn't.
	if s := d.detectFabricatedAction(turn); s != nil {
		signals = append(signals, s)
	}

	// Check 2: false confidence — model uses certain language but tool errored.
	if s := d.detectFalseConfidence(turn); s != nil {
		signals = append(signals, s)
	}

	// Check 3: tool contradiction — tool returned error but model says it succeeded.
	if s := d.detectToolContradiction(turn); s != nil {
		signals = append(signals, s)
	}

	// Check 4: dead references — model mentions specific paths/URLs not in tool results.
	if s := d.detectDeadReferences(turn); s != nil {
		signals = append(signals, s)
	}

	return signals
}

// AggregateRisk computes a single 0.0→1.0 risk score from a set of signals.
func (d *HallucinationDetector) AggregateRisk(signals []*types.HallucinationSignal) float64 {
	if len(signals) == 0 {
		return 0.0
	}
	// Use max + accumulation: one high-confidence signal matters most,
	// but multiple lower-confidence ones compound.
	max := 0.0
	sum := 0.0
	for _, s := range signals {
		if s.RiskScore > max {
			max = s.RiskScore
		}
		sum += s.RiskScore * 0.2 // each extra signal adds diminishing weight
	}
	return clamp(max+sum-0.2, 0, 1.0)
}

var (
	// Phrases the model uses when it claims to have performed an action.
	actionClaims = []string{
		"i ran", "i executed", "i called", "i fetched", "i read",
		"i wrote", "i saved", "i queried", "i searched", "i retrieved",
		"i accessed", "i updated", "i deleted", "i created",
	}

	// Phrases indicating high confidence.
	confidenceMarkers = []string{
		"definitely", "certainly", "absolutely", "clearly", "obviously",
		"i'm sure", "without a doubt", "i can confirm", "guaranteed",
	}
)

func (d *HallucinationDetector) detectFabricatedAction(turn *types.Turn) *types.HallucinationSignal {
	resp := strings.ToLower(turn.ModelResponse)

	claimedAction := ""
	for _, phrase := range actionClaims {
		if strings.Contains(resp, phrase) {
			claimedAction = phrase
			break
		}
	}
	if claimedAction == "" {
		return nil
	}

	// If there are no tool calls but the model claims to have done something, flag it.
	if len(turn.ToolCalls) == 0 {
		return &types.HallucinationSignal{
			ID:         uuid.NewString(),
			SessionID:  turn.SessionID,
			TurnID:     turn.ID,
			Type:       types.HallucinationFabricatedAction,
			RiskScore:  0.75,
			ModelClaim: claimedAction,
			Evidence:   "model claims action but no tool calls were recorded in this turn",
			DetectedAt: time.Now(),
		}
	}
	return nil
}

func (d *HallucinationDetector) detectFalseConfidence(turn *types.Turn) *types.HallucinationSignal {
	resp := strings.ToLower(turn.ModelResponse)

	usedConfidence := ""
	for _, marker := range confidenceMarkers {
		if strings.Contains(resp, marker) {
			usedConfidence = marker
			break
		}
	}
	if usedConfidence == "" {
		return nil
	}

	// Check if any tool call in this turn errored.
	for _, tc := range turn.ToolCalls {
		if tc.Status == types.StatusError || tc.Status == types.StatusTimeout {
			return &types.HallucinationSignal{
				ID:          uuid.NewString(),
				SessionID:   turn.SessionID,
				TurnID:      turn.ID,
				Type:        types.HallucinationFalseConfidence,
				RiskScore:   0.65,
				ModelClaim:  usedConfidence,
				ActualValue: string(tc.Status) + " on tool: " + tc.ToolName,
				Evidence:    "high-confidence language used but tool execution failed",
				DetectedAt:  time.Now(),
			}
		}
	}
	return nil
}

func (d *HallucinationDetector) detectToolContradiction(turn *types.Turn) *types.HallucinationSignal {
	resp := strings.ToLower(turn.ModelResponse)

	successPhrases := []string{"successfully", "completed", "done", "finished", "worked"}

	claimsSuccess := false
	for _, phrase := range successPhrases {
		if strings.Contains(resp, phrase) {
			claimsSuccess = true
			break
		}
	}
	if !claimsSuccess {
		return nil
	}

	failedTools := 0
	for _, tc := range turn.ToolCalls {
		if tc.Status == types.StatusError || tc.Status == types.StatusTimeout || tc.Status == types.StatusDenied {
			failedTools++
		}
	}

	if failedTools > 0 && len(turn.ToolCalls) > 0 {
		ratio := float64(failedTools) / float64(len(turn.ToolCalls))
		if ratio >= 0.5 {
			return &types.HallucinationSignal{
				ID:          uuid.NewString(),
				SessionID:   turn.SessionID,
				TurnID:      turn.ID,
				Type:        types.HallucinationToolContradiction,
				RiskScore:   0.5 + ratio*0.4,
				ModelClaim:  "model claims success",
				ActualValue: "tool failure rate: " + percentStr(ratio),
				Evidence:    "model response implies success but majority of tool calls failed",
				DetectedAt:  time.Now(),
			}
		}
	}
	return nil
}

func (d *HallucinationDetector) detectDeadReferences(turn *types.Turn) *types.HallucinationSignal {
	// Look for 404/not found in tool results while model describes content.
	for _, tc := range turn.ToolCalls {
		if tc.Result == nil {
			continue
		}
		result := strings.ToLower(string(tc.Result))
		if strings.Contains(result, "not found") || strings.Contains(result, "404") ||
			strings.Contains(result, "no such file") || strings.Contains(result, "does not exist") {

			// Did the model still describe content from this resource?
			resp := strings.ToLower(turn.ModelResponse)
			if !strings.Contains(resp, "not found") && !strings.Contains(resp, "error") &&
				len(turn.ModelResponse) > 100 {
				return &types.HallucinationSignal{
					ID:          uuid.NewString(),
					SessionID:   turn.SessionID,
					TurnID:      turn.ID,
					Type:        types.HallucinationDeadReference,
					RiskScore:   0.8,
					ModelClaim:  "model describes content from resource",
					ActualValue: "tool returned: not found / 404",
					Evidence:    "tool '" + tc.ToolName + "' returned not-found but model response describes content",
					DetectedAt:  time.Now(),
				}
			}
		}
	}
	return nil
}

// ─── Frustration Analyzer ─────────────────────────────────────────────────

// FrustrationAnalyzer scores user frustration per turn using behavioral
// and linguistic signals. No LLM required — pure heuristics.
type FrustrationAnalyzer struct {
	// RagePromptWindowSecs is how fast consecutive messages must arrive to
	// be considered "rage prompting". Default: 8 seconds.
	RagePromptWindowSecs int
}

// NewFrustrationAnalyzer creates an analyzer with sensible defaults.
func NewFrustrationAnalyzer() *FrustrationAnalyzer {
	return &FrustrationAnalyzer{RagePromptWindowSecs: 8}
}

// FrustrationResult holds the score and all triggers that fired.
type FrustrationResult struct {
	Score    float64
	Triggers []types.FrustrationTrigger
	Delta    float64 // change from session's previous score
}

// Score computes the frustration score for an incoming turn.
// prevScore is the session's current frustration_score before this turn.
// prevTurnAt is when the previous turn was created (for timing analysis).
func (a *FrustrationAnalyzer) Score(
	msg string,
	prevScore float64,
	prevTurnAt *time.Time,
	recentMessages []string, // last 3 user messages for repeat detection
) FrustrationResult {
	var triggers []types.FrustrationTrigger
	newPoints := 0.0

	lower := strings.ToLower(msg)

	// ── Linguistic signals ──────────────────────────────────────────────

	// All-caps (excluding very short messages like "OK", "YES")
	if len(msg) > 8 && isAllCaps(msg) {
		triggers = append(triggers, types.TriggerAllCaps)
		newPoints += 0.20
	}

	// Excessive exclamation marks
	if strings.Count(msg, "!") >= 3 {
		triggers = append(triggers, types.TriggerExclamations)
		newPoints += 0.15
	}

	// Negative sentiment keywords
	negativeKeywords := []string{
		"useless", "wrong", "not working", "broken", "stupid", "idiot",
		"terrible", "horrible", "awful", "ridiculous", "pathetic",
		"disappointed", "frustrated", "angry", "annoying", "trash",
		"garbage", "nonsense", "pointless",
	}
	if containsAny(lower, negativeKeywords) {
		triggers = append(triggers, types.TriggerNegativeSentiment)
		newPoints += 0.25
	}

	// Abandonment signals
	abandonmentKeywords := []string{
		"forget it", "never mind", "give up", "forget this", "screw it",
		"this is pointless", "i quit", "done with this", "not worth it",
	}
	if containsAny(lower, abandonmentKeywords) {
		triggers = append(triggers, types.TriggerAbandonmentSignal)
		newPoints += 0.40
	}

	// Explicit corrections
	correctionPhrases := []string{
		"no, i said", "that's wrong", "i didn't ask", "you misunderstood",
		"that's not what i", "again,", "i already told you", "read it again",
		"pay attention",
	}
	if containsAny(lower, correctionPhrases) {
		triggers = append(triggers, types.TriggerExplicitCorrection)
		newPoints += 0.20
	}

	// ── Behavioral signals ──────────────────────────────────────────────

	// Rage prompting: message sent very quickly after previous one
	if prevTurnAt != nil && time.Since(*prevTurnAt) < time.Duration(a.RagePromptWindowSecs)*time.Second {
		triggers = append(triggers, types.TriggerRagePrompting)
		newPoints += 0.15
	}

	// Repeated question detection (same message sent before)
	for _, prev := range recentMessages {
		if similarityRatio(lower, strings.ToLower(prev)) > 0.80 {
			triggers = append(triggers, types.TriggerRepeatedQuestion)
			newPoints += 0.30
			break
		}
	}

	// ── Rolling weighted score ──────────────────────────────────────────
	// New score = 70% of previous (decays slowly) + new points from this turn.
	// This prevents a single angry message from permanently tanking the score,
	// but keeps memory of sustained frustration.
	newScore := clamp(prevScore*0.70+newPoints, 0.0, 1.0)
	delta := newScore - prevScore

	return FrustrationResult{
		Score:    newScore,
		Triggers: triggers,
		Delta:    delta,
	}
}

// ShouldAlert returns true when the frustration score crosses a meaningful threshold.
func (a *FrustrationAnalyzer) ShouldAlert(score float64) bool {
	return score >= 0.60
}

// ShouldMarkAbandoned returns true when the score suggests the user will leave.
func (a *FrustrationAnalyzer) ShouldMarkAbandoned(score float64) bool {
	return score >= 0.85
}

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

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func isAllCaps(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if unicode.IsLower(r) {
				return false
			}
		}
	}
	return hasLetter
}

// similarityRatio computes a rough character-level overlap ratio.
// Good enough for detecting near-identical repeated questions.
func similarityRatio(a, b string) float64 {
	if a == b {
		return 1.0
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if len(longer) == 0 {
		return 1.0
	}
	matches := 0
	for i := 0; i < len(shorter); i++ {
		if i < len(longer) && shorter[i] == longer[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(longer))
}

func percentStr(f float64) string {
	return fmt.Sprintf("%.0f%%", f*100)
}

// serviceFromTool maps common tool name patterns to service names
// for infra correlation matching.
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
