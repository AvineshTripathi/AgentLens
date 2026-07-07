// Package providers contains pluggable adapters for each LLM provider.
// To add a new provider (e.g. AWS Bedrock), implement the Adapter interface
// and register it in registry.go.
package providers

import "github.com/AvineshTripathi/AgentLens/internal/types"

// Adapter is the plugin interface every provider must implement.
// Each adapter encapsulates all parsing logic for one API schema.
type Adapter interface {
	// Provider returns the canonical provider label used for DB storage and metrics.
	Provider() types.Provider

	// Domains returns the hostnames this adapter claims (exact hostname, no port).
	// The registry uses these to build the MITM TLS intercept list.
	Domains() []string

	// PathPatterns returns URL path substrings that identify a generation request
	// from this provider (e.g. "messages", "chat/completions").
	PathPatterns() []string

	// ExtractSessionID returns a stable, UUID-format session identifier from the
	// raw request body. Stateless APIs should hash the first message to produce
	// a deterministic ID. Returns "" if no ID can be derived.
	ExtractSessionID(body []byte) string

	// ExtractUserMessage returns the last user-role message from the request.
	ExtractUserMessage(body []byte) string

	// ExtractModel returns the model name from the request body or URL path.
	ExtractModel(body []byte, path string) string

	// ExtractModelResponse returns the assistant text from the response body.
	// Handles both single-object JSON and SSE streams.
	ExtractModelResponse(body []byte) string

	// ExtractTokenCounts returns (inputTokens, outputTokens) from the response body.
	ExtractTokenCounts(body []byte) (in, out int)
}
