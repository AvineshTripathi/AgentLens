package providers

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// OpenAIAdapter handles the OpenAI Chat Completions API schema.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) Provider() types.Provider { return types.ProviderOpenAI }

func (a *OpenAIAdapter) Domains() []string {
	return []string{"api.openai.com"}
}

func (a *OpenAIAdapter) PathPatterns() []string {
	return []string{"chat/completions", "completions"}
}

// ExtractSessionID hashes the first message content to group turns into sessions.
func (a *OpenAIAdapter) ExtractSessionID(body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	rawMsgs, ok := obj["messages"]
	if !ok {
		return ""
	}
	var messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(rawMsgs, &messages); err != nil || len(messages) == 0 {
		return ""
	}
	anchor := string(messages[0].Content)
	return uuid.NewMD5(uuid.NameSpaceOID, []byte(anchor)).String()
}

// ExtractUserMessage returns the last user message, supporting both string and
// content-block array formats.
func (a *OpenAIAdapter) ExtractUserMessage(body []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	raw, ok := obj["messages"]
	if !ok {
		return ""
	}
	var messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		var text string
		if err := json.Unmarshal(messages[i].Content, &text); err == nil {
			return text
		}
		// Content-block array
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(messages[i].Content, &blocks); err == nil {
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					sb.WriteString(b.Text)
				}
			}
			return sb.String()
		}
	}
	return ""
}

func (a *OpenAIAdapter) ExtractModel(body []byte, _ string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return "unknown"
	}
	if raw, ok := obj["model"]; ok {
		var model string
		if err := json.Unmarshal(raw, &model); err == nil && model != "" {
			return model
		}
	}
	return "unknown"
}

// ExtractModelResponse parses choices from a Chat Completions response or SSE stream.
func (a *OpenAIAdapter) ExtractModelResponse(body []byte) string {
	extract := func(obj map[string]json.RawMessage) string {
		// Non-streaming: choices[0].message.content
		if raw, ok := obj["choices"]; ok {
			var choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(raw, &choices); err == nil && len(choices) > 0 {
				if choices[0].Message.Content != "" {
					return choices[0].Message.Content
				}
				// SSE streaming delta
				return choices[0].Delta.Content
			}
		}
		return ""
	}
	return parseBodyOrSSE(body, extract)
}

func (a *OpenAIAdapter) ExtractTokenCounts(body []byte) (int, int) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0, 0
	}
	if raw, ok := obj["usage"]; ok {
		var usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		}
		if err := json.Unmarshal(raw, &usage); err == nil {
			return usage.PromptTokens, usage.CompletionTokens
		}
	}
	return 0, 0
}
