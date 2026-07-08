package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/config"
	"github.com/AvineshTripathi/AgentLens/internal/store"
	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// EvaluatorWorker runs in the background and evaluates pending sessions.
type EvaluatorWorker struct {
	cfg   config.EvaluatorConfig
	store *store.Store
}

func NewEvaluatorWorker(cfg config.EvaluatorConfig, s *store.Store) *EvaluatorWorker {
	return &EvaluatorWorker{
		cfg:   cfg,
		store: s,
	}
}

func (w *EvaluatorWorker) Start(ctx context.Context) {
	if !w.cfg.Enabled {
		return
	}

	interval := time.Duration(w.cfg.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("Starting LLM Evaluator Worker", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runEvaluationCycle(ctx)
		}
	}
}

func (w *EvaluatorWorker) runEvaluationCycle(ctx context.Context) {
	sessions, err := w.store.GetPendingEvaluations(ctx, 10)
	if err != nil {
		slog.Error("Failed to fetch pending evaluations", "err", err)
		return
	}

	for _, sess := range sessions {
		slog.Info("Evaluating session", "session_id", sess.ID)
		err := w.evaluateSession(ctx, sess)
		if err != nil {
			slog.Error("Failed to evaluate session", "session_id", sess.ID, "err", err)
			continue
		}
	}
}

func (w *EvaluatorWorker) evaluateSession(ctx context.Context, sess *types.Session) error {
	sysPrompt := fmt.Sprintf(`You are a Senior Site Reliability Engineer (SRE) on-call. Your responsibility is to audit an agentic session and determine its success or failure in production, as well as accurately grade the user's frustration level. You must act as an objective, forensic investigator.

SESSION ID: %s

## ON-CALL RUNBOOK: SESSION EVALUATION

### Phase 1: Context Gathering (Do not skip)
1. Invoke the 'get_session_summary' tool.
   - Purpose: Understand the length of the session, total token usage, and duration.
   - If the session is extremely short (1-2 turns) and ends abruptly, it may be an abandoned session.
2. Invoke the 'get_timeline' tool.
   - Purpose: Read the raw transcript of the user's conversation with the agent, including all tool calls (database, file ops, http) made by the agent.

### Phase 2: Forensic Analysis
Carefully review the timeline and evaluate the following:
1. Core Intent: What was the user explicitly asking the agent to do?
2. Agent Competence: Did the agent use the correct tools? Did it hallucinate paths or parameters? Did it enter an endless loop?
3. Resolution: Did the agent definitively solve the user's request, or did the conversation end with an error, an apology, or user silence after a failure?
4. User Sentiment: Look for signs of frustration in the user's prompts:
   - Repetition (asking the same thing multiple times because the agent failed).
   - Tone (ALL CAPS, short/curt responses, swearing).
   - Manual corrections ("No, I meant...", "Stop doing that").

### Phase 3: Final Judgement
Invoke the 'submit_evaluation' tool with your final verdict using the following strict rubric:

**Outcome (String)**:
- 'success': The agent successfully completed the user's request.
- 'failed': The agent failed to complete the request (due to endless loops, hallucinations, technical errors, or giving up).
- 'abandoned': The user left the session prematurely before the agent could attempt a solution, or without confirming success/failure.

**Frustration Score (Float 0.0 - 1.0)**:
- 0.0 - 0.2: Perfectly normal, calm interaction. The user is happy or neutral.
- 0.3 - 0.5: Mild annoyance. The user had to correct the agent once or repeat themselves mildly.
- 0.6 - 0.8: High frustration. The user explicitly told the agent it was wrong multiple times, used curt language, or the agent repeatedly failed a simple task.
- 0.9 - 1.0: Extreme anger. Swearing, ALL CAPS demands to stop, or completely giving up due to the agent's sheer incompetence.

**Hallucination Detection (String)**:
- If you detect that the agent hallucinated (e.g., fabricated facts, claimed it did an action but didn't make a tool call, or hallucinated a parameter), provide a short summary of the hallucination in the 'hallucination_reason' parameter.
- If no hallucination occurred, leave it as an empty string.

**Summary (String)**:
- Provide a brief 2-3 sentence summary explaining *why* you chose the outcome, why you gave that frustration score, and any hallucination details.`, sess.ID)

	messages := []map[string]any{
		{"role": "system", "content": sysPrompt},
		{"role": "user", "content": "Please begin your evaluation."},
	}

	for i := 0; i < 10; i++ {
		resp, err := w.chatCompletion(ctx, messages)
		if err != nil {
			return err
		}

		choices, ok := resp["choices"].([]any)
		if !ok || len(choices) == 0 {
			return fmt.Errorf("no choices returned")
		}

		choice, ok := choices[0].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid choice format")
		}

		message, ok := choice["message"].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid message format")
		}

		messages = append(messages, message)

		toolCallsRaw, hasTools := message["tool_calls"]
		if !hasTools || toolCallsRaw == nil {
			break
		}

		toolCalls, ok := toolCallsRaw.([]any)
		if !ok || len(toolCalls) == 0 {
			break
		}

		for _, tcAny := range toolCalls {
			tc, ok := tcAny.(map[string]any)
			if !ok {
				continue
			}

			tcID, _ := tc["id"].(string)
			fn, ok := tc["function"].(map[string]any)
			if !ok {
				continue
			}

			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)

			var result string

			// Some compatibility layers prefix tools with default_api: or similar
			if len(name) > 12 && name[:12] == "default_api:" {
				name = name[12:]
			}

			switch name {
			case "get_session_summary":
				result = fmt.Sprintf("Turn count: %d, Started At: %s", sess.TurnCount, sess.StartedAt)

			case "get_timeline":
				entries, _ := w.store.ListTimelineEntries(ctx, sess.ID)
				b, _ := json.Marshal(entries)
				if len(b) > 4000 {
					b = b[:4000]
				}
				result = string(b)

			case "submit_evaluation":
				var args struct {
					Outcome             string  `json:"outcome"`
					FrustrationScore    float64 `json:"frustration_score"`
					HallucinationReason string  `json:"hallucination_reason"`
					Summary             string  `json:"summary"`
				}
				if err := json.Unmarshal([]byte(argsStr), &args); err == nil {
					outcome := types.OutcomeStatus(args.Outcome)
					return w.store.SaveEvaluation(ctx, sess.ID, outcome, args.FrustrationScore, args.HallucinationReason, args.Summary)
				}
				result = "Invalid arguments"

			default:
				result = "Unknown tool"
			}

			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tcID,
				"content":      result,
			})
		}
	}

	return fmt.Errorf("exceeded max tool call iterations")
}

// ─── OpenAI-Compatible HTTP Client ────────────────────────────────────────

type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []map[string]any `json:"messages"`
	Tools    []Tool           `json:"tools,omitempty"`
}

type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

func (w *EvaluatorWorker) chatCompletion(ctx context.Context, messages []map[string]any) (map[string]any, error) {
	reqBody := ChatRequest{
		Model:    w.cfg.Model,
		Messages: messages,
		Tools: []Tool{
			{
				Type: "function",
				Function: struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Parameters  any    `json:"parameters"`
				}{
					Name:        "get_session_summary",
					Description: "Get basic stats about the session",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
			{
				Type: "function",
				Function: struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Parameters  any    `json:"parameters"`
				}{
					Name:        "get_timeline",
					Description: "Get the conversation transcript",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
			{
				Type: "function",
				Function: struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Parameters  any    `json:"parameters"`
				}{
					Name:        "submit_evaluation",
					Description: "Submit final judgement",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"outcome": map[string]any{
								"type": "string",
								"enum": []string{"success", "failed", "abandoned"},
							},
							"frustration_score": map[string]any{
								"type": "number",
							},
							"hallucination_reason": map[string]any{
								"type":        "string",
								"description": "Describe the hallucination if one occurred, otherwise empty",
							},
							"summary": map[string]any{
								"type":        "string",
								"description": "Brief 2-3 sentence summary explaining the judgement",
							},
						},
						"required": []string{"outcome", "frustration_score", "hallucination_reason", "summary"},
					},
				},
			},
		},
	}

	b, _ := json.Marshal(reqBody)
	url := w.cfg.APIBase + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status: %d from %s, body: %s", resp.StatusCode, url, string(b))
	}

	var chatResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	return chatResp, nil
}
