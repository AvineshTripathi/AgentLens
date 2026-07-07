package providers

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// GeminiAdapter handles the Google Generative Language API schema,
// including the standard REST API and the internal `agy` (antigravity) format.
type GeminiAdapter struct{}

func (a *GeminiAdapter) Provider() types.Provider { return types.ProviderGemini }

func (a *GeminiAdapter) Domains() []string {
	return []string{
		"generativelanguage.googleapis.com",
		"daily-cloudcode-pa.googleapis.com",
		"antigravity-unleash.goog",
	}
}

func (a *GeminiAdapter) PathPatterns() []string {
	return []string{"generateContent", "GenerateContent", "streamGenerateContent"}
}

// ExtractSessionID hashes the first user message to produce a stateless,
// deterministic session UUID. This ensures AGY behaves exactly like Claude:
// starting a new conversation (fresh history) creates a new session, while
// continuing a conversation groups into the existing one.
func (a *GeminiAdapter) ExtractSessionID(body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	found := false

	if raw, ok := obj["contents"]; ok {
		if err := json.Unmarshal(raw, &contents); err == nil {
			found = true
		}
	}
	if !found {
		if rawReq, ok := obj["request"]; ok {
			var reqObj map[string]json.RawMessage
			if err := json.Unmarshal(rawReq, &reqObj); err == nil {
				if raw, ok := reqObj["contents"]; ok {
					if err := json.Unmarshal(raw, &contents); err == nil {
						found = true
					}
				}
			}
		}
	}
	
	if found && len(contents) > 0 {
		// Use the first user message as the anchor for the session ID
		for _, c := range contents {
			if c.Role == "user" || c.Role == "" {
				var sb strings.Builder
				for _, p := range c.Parts {
					sb.WriteString(p.Text)
				}
				// Clean the text to ensure metadata changes don't break the hash
				cleanText := cleanAGYMessage(sb.String())
				sID := uuid.NewMD5(uuid.NameSpaceOID, []byte(cleanText)).String()
				slog.Debug("gemini: session ID from first message hash", "session_id", sID)
				return sID
			}
		}
	}

	slog.Debug("gemini: no contents found for session ID", "body_keys", jsonKeys(obj))
	return ""
}

// ExtractUserMessage reads the last user turn from `contents` or `request.contents`.
func (a *GeminiAdapter) ExtractUserMessage(body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	found := false

	if raw, ok := obj["contents"]; ok {
		if err := json.Unmarshal(raw, &contents); err == nil {
			found = true
			slog.Debug("gemini: found root contents", "count", len(contents))
		}
	}
	if !found {
		if rawReq, ok := obj["request"]; ok {
			var reqObj map[string]json.RawMessage
			if err := json.Unmarshal(rawReq, &reqObj); err == nil {
				if raw, ok := reqObj["contents"]; ok {
					if err := json.Unmarshal(raw, &contents); err == nil {
						found = true
						slog.Debug("gemini: found request.contents", "count", len(contents))
					}
				}
			}
		}
	}
	if !found {
		slog.Debug("gemini: no contents found", "body_keys", jsonKeys(obj))
		return ""
	}

	for i := len(contents) - 1; i >= 0; i-- {
		if contents[i].Role == "user" || contents[i].Role == "" {
			var sb strings.Builder
			for _, p := range contents[i].Parts {
				sb.WriteString(p.Text)
			}
			result := cleanAGYMessage(sb.String())
			slog.Debug("gemini: user message", "preview", result[:min(80, len(result))])
			return result
		}
	}
	return ""
}

// cleanAGYMessage strips agy's internal XML wrapping to show just the user text.
// agy sends: <USER_REQUEST>\nhello\n</USER_REQUEST>\n<ADDITIONAL_METADATA>...</ADDITIONAL_METADATA>
func cleanAGYMessage(raw string) string {
	// Extract content between <USER_REQUEST> and </USER_REQUEST>
	const openTag = "<USER_REQUEST>"
	const closeTag = "</USER_REQUEST>"
	start := strings.Index(raw, openTag)
	end := strings.Index(raw, closeTag)
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(raw[start+len(openTag) : end])
	}
	// No XML wrapper — return as-is (truncated if very long)
	if len(raw) > 500 {
		return raw[:500] + "…"
	}
	return raw
}

// jsonKeys returns the top-level keys of a JSON object map (for debug logging).
func jsonKeys(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

func (a *GeminiAdapter) ExtractModel(body []byte, path string) string {
	if strings.Contains(path, "models/") {
		parts := strings.Split(path, "models/")
		if len(parts) > 1 {
			return strings.Split(parts[1], ":")[0]
		}
	}
	
	// Fallback to reading from the body for AGY/internal endpoints
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err == nil {
		extractStr := func(raw json.RawMessage) string {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				// Models are often in format "models/gemini-1.5-pro"
				return strings.TrimPrefix(s, "models/")
			}
			return ""
		}
		
		if raw, ok := obj["model"]; ok {
			if m := extractStr(raw); m != "" {
				return m
			}
		}
		if rawReq, ok := obj["request"]; ok {
			var reqObj map[string]json.RawMessage
			if err := json.Unmarshal(rawReq, &reqObj); err == nil {
				if raw, ok := reqObj["model"]; ok {
					if m := extractStr(raw); m != "" {
						return m
					}
				}
			}
		}
	}
	return "unknown"
}

// ExtractModelResponse reads candidates from a Gemini response or SSE stream.
func (a *GeminiAdapter) ExtractModelResponse(body []byte) string {
	extract := func(obj map[string]json.RawMessage) string {
		// Internal Google wraps inside "response"
		if rawResp, ok := obj["response"]; ok {
			var respObj map[string]json.RawMessage
			if err := json.Unmarshal(rawResp, &respObj); err == nil {
				obj = respObj
			}
		}
		if raw, ok := obj["candidates"]; ok {
			var candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			}
			if err := json.Unmarshal(raw, &candidates); err == nil {
				var sb strings.Builder
				for _, cand := range candidates {
					for _, p := range cand.Content.Parts {
						sb.WriteString(p.Text)
					}
				}
				return sb.String()
			}
		}
		return ""
	}
	return parseBodyOrSSE(body, extract)
}

func (a *GeminiAdapter) ExtractTokenCounts(body []byte) (int, int) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0, 0
	}
	// Try root usageMetadata
	if raw, ok := obj["usageMetadata"]; ok {
		var usage struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		}
		if err := json.Unmarshal(raw, &usage); err == nil {
			return usage.PromptTokenCount, usage.CandidatesTokenCount
		}
	}
	return 0, 0
}
