package providers

import (
	"fmt"
	"strings"
)

// Registry holds all registered provider adapters and routes
// incoming requests to the correct one.
type Registry struct {
	adapters []Adapter
}

// DefaultRegistry returns a Registry pre-loaded with all built-in adapters.
// To add a new provider, instantiate it here.
func DefaultRegistry() *Registry {
	r := &Registry{}
	r.Register(&AnthropicAdapter{})
	r.Register(&OpenAIAdapter{})
	r.Register(&GeminiAdapter{})
	return r
}

// Register adds an adapter to the registry.
func (r *Registry) Register(a Adapter) {
	r.adapters = append(r.adapters, a)
}

// AllDomains returns every domain across all registered adapters.
// Used to build the MITM intercept regex.
func (r *Registry) AllDomains() []string {
	seen := map[string]struct{}{}
	var domains []string
	for _, a := range r.adapters {
		for _, d := range a.Domains() {
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				domains = append(domains, d)
			}
		}
	}
	return domains
}

// MITMRegex returns a compiled-ready regex string that matches any of the
// registered domains (with an optional port suffix).
func (r *Registry) MITMRegex() string {
	escaped := make([]string, 0, len(r.AllDomains()))
	for _, d := range r.AllDomains() {
		escaped = append(escaped, strings.ReplaceAll(d, ".", "\\."))
	}
	return fmt.Sprintf(`^(%s)(:[0-9]+)?$`, strings.Join(escaped, "|"))
}

// FindByHost returns the adapter that claims the given hostname, or nil.
func (r *Registry) FindByHost(host string) Adapter {
	for _, a := range r.adapters {
		for _, d := range a.Domains() {
			if strings.Contains(host, d) {
				return a
			}
		}
	}
	return nil
}

// FindByPath returns the first adapter whose PathPatterns match the given path.
// This is preferred over host-based lookup when the same host can serve multiple schemas.
func (r *Registry) FindByPath(path string) Adapter {
	for _, a := range r.adapters {
		for _, pattern := range a.PathPatterns() {
			if strings.Contains(path, pattern) {
				return a
			}
		}
	}
	return nil
}

// Find returns the adapter for a given host+path combination.
// Path-based matching takes priority. If the path doesn't match any known
// generation endpoint, we return nil even if the host is registered — this
// prevents background telemetry (e.g. /api/event_logging) from being captured.
func (r *Registry) Find(host, path string) Adapter {
	// First try an exact path match — this is the most reliable signal.
	if a := r.FindByPath(path); a != nil {
		return a
	}
	// Only fall back to host-based lookup if the path looks like a generation request.
	if isGenerationPath(path) {
		return r.FindByHost(host)
	}
	return nil
}

// isGenerationPath returns true only for paths that indicate an LLM
// inference/generation request. Background paths (metrics, logging,
// registries, downloads) return false.
func isGenerationPath(path string) bool {
	generationKeywords := []string{
		"generate", "completion", "messages", "inference", "predict",
	}
	path = strings.ToLower(path)
	for _, kw := range generationKeywords {
		if strings.Contains(path, kw) {
			return true
		}
	}
	return false
}
