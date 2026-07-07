package providers

import (
	"encoding/json"
	"strings"
)

// parseBodyOrSSE tries to extract text from a response body that may be either
// a plain JSON object/array or a Server-Sent Events (SSE) stream.
// extract is called for each parsed JSON object found.
func parseBodyOrSSE(body []byte, extract func(map[string]json.RawMessage) string) string {
	bodyStr := string(body)

	if strings.Contains(bodyStr, "data: ") {
		// SSE stream: each line is "data: {...}" or "data: [DONE]"
		var sb strings.Builder
		for _, line := range strings.Split(bodyStr, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			jsonStr := strings.TrimPrefix(line, "data: ")
			if jsonStr == "[DONE]" {
				continue
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(jsonStr), &obj); err == nil {
				sb.WriteString(extract(obj))
			}
		}
		return sb.String()
	}

	if len(body) > 0 && body[0] == '[' {
		// JSON array (e.g. batched responses)
		var arr []map[string]json.RawMessage
		if err := json.Unmarshal(body, &arr); err == nil {
			var sb strings.Builder
			for _, o := range arr {
				sb.WriteString(extract(o))
			}
			return sb.String()
		}
	}

	// Single JSON object
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err == nil {
		return extract(obj)
	}
	return ""
}
