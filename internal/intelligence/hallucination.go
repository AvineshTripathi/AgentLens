package intelligence

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/types"
	"github.com/google/uuid"
)

// HallucinationDetector implements the Analyzer interface.
// It cross-references model claims against tool results to detect likely hallucinations.
type HallucinationDetector struct{}

func NewHallucinationDetector() *HallucinationDetector {
	return &HallucinationDetector{}
}

func (d *HallucinationDetector) Name() string {
	return "HallucinationDetector"
}

func (d *HallucinationDetector) Analyze(ctx context.Context, actx *AnalysisContext) error {
	turn := actx.Turn
	if turn.ModelResponse == "" {
		return nil
	}

	var signals []*types.HallucinationSignal

	if s := d.detectFabricatedAction(turn); s != nil {
		signals = append(signals, s)
	}
	if s := d.detectFalseConfidence(turn); s != nil {
		signals = append(signals, s)
	}
	if s := d.detectToolContradiction(turn); s != nil {
		signals = append(signals, s)
	}
	if s := d.detectDeadReferences(turn); s != nil {
		signals = append(signals, s)
	}
	if s := d.detectDelusionalSuccess(turn); s != nil {
		signals = append(signals, s)
	}

	if len(signals) > 0 {
		turn.HallucinationRisk = d.aggregateRisk(signals)

		// Persist signals asynchronously
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, sig := range signals {
				if err := actx.Store.InsertHallucinationSignal(bgCtx, sig); err != nil {
					slog.Error("failed to persist hallucination signal", "err", err)
				}
			}
		}()
	}
	return nil
}

func (d *HallucinationDetector) aggregateRisk(signals []*types.HallucinationSignal) float64 {
	if len(signals) == 0 {
		return 0.0
	}
	max := 0.0
	sum := 0.0
	for _, s := range signals {
		if s.RiskScore > max {
			max = s.RiskScore
		}
		sum += s.RiskScore * 0.2
	}
	return clamp(max+sum-0.2, 0, 1.0)
}

var (
	actionClaims = []string{
		"i ran", "i executed", "i called", "i fetched", "i read",
		"i wrote", "i saved", "i queried", "i searched", "i retrieved",
		"i accessed", "i updated", "i deleted", "i created",
	}

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
	if claimedAction != "" && len(turn.ToolCalls) == 0 {
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
	if usedConfidence != "" {
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
	for _, tc := range turn.ToolCalls {
		if tc.Result == nil {
			continue
		}
		result := strings.ToLower(string(tc.Result))
		if strings.Contains(result, "not found") || strings.Contains(result, "404") ||
			strings.Contains(result, "no such file") || strings.Contains(result, "does not exist") {

			resp := strings.ToLower(turn.ModelResponse)
			if !strings.Contains(resp, "not found") && !strings.Contains(resp, "error") && len(turn.ModelResponse) > 100 {
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

// detectDelusionalSuccess checks for blatant tool errors that the model interprets as successful execution.
func (d *HallucinationDetector) detectDelusionalSuccess(turn *types.Turn) *types.HallucinationSignal {
	for _, tc := range turn.ToolCalls {
		if tc.Result == nil {
			continue
		}
		result := strings.ToLower(string(tc.Result))

		errorSignatures := []string{
			"traceback (most recent call last)",
			"syntaxerror:",
			"referenceerror:",
			"typeerror:",
			"connection refused",
			"permission denied",
			"command not found",
		}

		hasError := false
		for _, sig := range errorSignatures {
			if strings.Contains(result, sig) {
				hasError = true
				break
			}
		}

		if hasError {
			resp := strings.ToLower(turn.ModelResponse)
			successPhrases := []string{"i have successfully", "i've fixed", "everything is working", "the issue is resolved"}
			for _, phrase := range successPhrases {
				if strings.Contains(resp, phrase) {
					return &types.HallucinationSignal{
						ID:          uuid.NewString(),
						SessionID:   turn.SessionID,
						TurnID:      turn.ID,
						Type:        types.HallucinationToolContradiction,
						RiskScore:   0.95,
						ModelClaim:  "model explicitly claims issue is resolved or action successful",
						ActualValue: "tool returned a raw stack trace or fatal error",
						Evidence:    "model delusion: tool output is a fatal error but model claims complete success",
						DetectedAt:  time.Now(),
					}
				}
			}
		}
	}
	return nil
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func percentStr(f float64) string {
	return func() string {
		// Just a simple format
		return "high"
	}()
}
