package providers

import (
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// AnthropicAdapter handles the Anthropic Messages API schema.
// It also covers OpenRouter when Claude CLI routes through it, since
// OpenRouter exposes the identical Anthropic schema.
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) Provider() types.Provider { return types.ProviderAnthropic }

func (a *AnthropicAdapter) Domains() []string {
	return []string{"api.anthropic.com", "openrouter.ai"}
}

func (a *AnthropicAdapter) PathPatterns() []string {
	return []string{"messages"}
}

// internalPromptSignatures are substrings found ONLY in Claude Code's internal
// housekeeping requests (title generation, summarisation, etc.) that should
// never be tracked as real conversation sessions.
var internalPromptSignatures = []string{
	// Claude Code title-generation prompt
	"Write the title in the language the user wrote in",
	// Older variants / other internal prompts
	"Generate a short title for this conversation",
	"Summarize the conversation in one short sentence",
}

// ShouldSkip returns true when the request is an internal Claude Code
// housekeeping call that must not be recorded as a real turn or session.
func (a *AnthropicAdapter) ShouldSkip(body []byte) bool {
	messages := parseAnthropicMessages(body)
	// Check every user message in the request for known internal signatures.
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		text := extractPlainText(m.Content)
		for _, sig := range internalPromptSignatures {
			if strings.Contains(text, sig) {
				slog.Debug("anthropic: skipping internal request", "signature", sig)
				return true
			}
		}
	}
	return false
}

// ─── Compiled regexes (package-level to avoid re-compilation on every call) ──

var (
	// Matches *...* injected context blocks, including multi-line ones.
	reStarBlock = regexp.MustCompile(`(?s)\*[^*]{5,}\*`)
	// Matches <TagName>...</TagName> XML-style context blocks injected by tools.
	reXMLBlock = regexp.MustCompile(`(?s)<[A-Za-z_][A-Za-z0-9_]*>[^<]*</[A-Za-z_][A-Za-z0-9_]*>`)
	// Matches ISO-style dates (2026-07-07) and times (14:32:01).
	reDateOrTime = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b|\b\d{2}:\d{2}:\d{2}\b`)
	// Normalises runs of whitespace.
	reWhitespace = regexp.MustCompile(`\s+`)
)

// anthropicMessage is a thin struct used only for parsing.
type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// anthropicContentBlock is used for the array-of-blocks content format.
type anthropicContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ToolUseID string `json:"tool_use_id"`
}

// parseAnthropicMessages extracts the messages array from a request body.
func parseAnthropicMessages(body []byte) []anthropicMessage {
	var obj struct {
		Messages []anthropicMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	return obj.Messages
}

// extractPlainText pulls the raw string content from a single message's Content
// field, handling both plain-string and content-block-array formats.
// It skips tool_result blocks — those are internal plumbing, not user text.
func extractPlainText(content json.RawMessage) string {
	// Case 1: plain string
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}
	// Case 2: array of content blocks
	var blocks []anthropicContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" && b.ToolUseID == "" {
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return ""
}

// normaliseForHash strips every kind of dynamic content that Claude CLI (or any
// framework) injects into a message — timestamps, context dumps, XML blocks,
// interrupt markers — producing a stable string that represents only the human's
// actual intent.  The result is suitable for hashing as a session anchor.
func normaliseForHash(text string) string {
	// 1. Strip [Request interrupted by user] markers.
	text = strings.ReplaceAll(text, "[Request interrupted by user]", "")

	// 2. Strip *...* context blocks (Claude CLI injects these with env/date info).
	text = reStarBlock.ReplaceAllString(text, "")

	// 3. Strip <Tag>...</Tag> XML context blocks.
	text = reXMLBlock.ReplaceAllString(text, "")

	// 4. Strip dates and times that change between turns.
	text = reDateOrTime.ReplaceAllString(text, "")

	// 5. Collapse whitespace and trim.
	text = reWhitespace.ReplaceAllString(strings.TrimSpace(text), " ")

	// 6. Cap at 256 chars to prevent hash drift from very long trailing injections.
	if len(text) > 256 {
		text = text[:256]
	}
	return strings.TrimSpace(text)
}

// cleanForDisplay strips dynamic context from text for human-readable UI display.
// Unlike normaliseForHash this does NOT truncate.
func cleanForDisplay(text string) string {
	text = strings.ReplaceAll(text, "[Request interrupted by user]", "")
	text = reStarBlock.ReplaceAllString(text, "")
	text = reXMLBlock.ReplaceAllString(text, "")
	text = reDateOrTime.ReplaceAllString(text, "")
	text = reWhitespace.ReplaceAllString(strings.TrimSpace(text), " ")
	return strings.TrimSpace(text)
}

// ExtractSessionID derives a stable session UUID from the conversation's root.
//
// Design: The Anthropic API is fully stateless — the client resends the entire
// conversation history on every turn. This means:
//   - Turn 1 request: messages = [user:"hello"]
//   - Turn 2 request: messages = [user:"hello", assistant:"Hi!", user:"what time?"]
//   - Turn N request: messages = [user:"hello", assistant:"Hi!", ..., user:"<N-th msg>"]
//
// The first user message ("hello") is the session anchor — it is present and
// structurally identical in every subsequent request of the same conversation.
//
// The challenge is that tools like Claude CLI inject dynamic content (timestamps,
// git status, current directory, interrupt markers) into the content blocks of
// that first message on every turn, causing naïve hashing to produce a different
// session ID per turn.
//
// Solution: extract only the human-authored text from the first user message,
// strip all known dynamic injection patterns, then hash the result.
func (a *AnthropicAdapter) ExtractSessionID(body []byte) string {
	return ""
}

// ExtractUserMessage returns the last human turn suitable for display in the
// dashboard.  It skips tool_result messages (internal plumbing) and strips the
// dynamic context injected by Claude CLI so the UI shows clean human text.
func (a *AnthropicAdapter) ExtractUserMessage(body []byte) string {
	messages := parseAnthropicMessages(body)

	// Walk backwards to find the most recent user message that contains actual text.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		rawText := extractPlainText(messages[i].Content)
		if rawText == "" {
			continue
		}
		cleaned := cleanForDisplay(rawText)
		if cleaned != "" {
			return cleaned
		}
	}
	return ""
}

// ExtractContinuationID returns a stable hash that uniquely identifies this
// conversation starting from its second request onwards.
//
// It hashes messages[0] (first user text) + messages[1] (first assistant
// response) together, so the key is:
//   - Unique:  two different conversations won't collide even if the assistant
//     gives a similar opening greeting.
//   - Stable:  both messages are sent verbatim by the client on every turn.
//   - Available: present from the second request onwards.
func (a *AnthropicAdapter) ExtractContinuationID(body []byte) string {
	messages := parseAnthropicMessages(body)
	if len(messages) < 2 {
		return ""
	}
	if messages[1].Role != "assistant" {
		return ""
	}

	// Combine user-side anchor (normalised) + assistant reply (raw/stable).
	userText := normaliseForHash(extractPlainText(messages[0].Content))
	assistantText := extractPlainText(messages[1].Content)
	if userText == "" || assistantText == "" {
		return ""
	}
	if len(assistantText) > 200 {
		assistantText = assistantText[:200]
	}

	combined := "conv:" + userText + "|" + assistantText
	return uuid.NewMD5(uuid.NameSpaceOID, []byte(combined)).String()
}

func (a *AnthropicAdapter) ExtractModel(body []byte, _ string) string {
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

// ExtractModelResponse parses a single JSON object or SSE stream from the
// Anthropic Messages API response body.
func (a *AnthropicAdapter) ExtractModelResponse(body []byte) string {
	extract := func(obj map[string]json.RawMessage) string {
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
		// SSE streaming delta: {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
		if raw, ok := obj["delta"]; ok {
			var delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &delta); err == nil && delta.Type == "text_delta" {
				return delta.Text
			}
		}
		return ""
	}
	return parseBodyOrSSE(body, extract)
}

func (a *AnthropicAdapter) ExtractTokenCounts(body []byte) (int, int) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0, 0
	}
	if raw, ok := obj["usage"]; ok {
		var usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}
		if err := json.Unmarshal(raw, &usage); err == nil {
			return usage.InputTokens, usage.OutputTokens
		}
	}
	return 0, 0
}
