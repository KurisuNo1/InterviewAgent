package nodes

import "strings"

// extractJSON extracts JSON content from LLM responses that may contain markdown fences.
func extractJSON(content string) string {
	// Try to extract from markdown code block
	if start := strings.Index(content, "```json"); start != -1 {
		content = content[start+7:]
		if end := strings.LastIndex(content, "```"); end != -1 {
			content = content[:end]
		}
	} else if start := strings.Index(content, "```"); start != -1 {
		content = content[start+3:]
		if end := strings.LastIndex(content, "```"); end != -1 {
			content = content[:end]
		}
	}

	// Trim whitespace
	content = strings.TrimSpace(content)

	// Try to find JSON object boundaries
	if start := strings.Index(content, "{"); start != -1 {
		if end := strings.LastIndex(content, "}"); end != -1 {
			content = content[start : end+1]
		}
	}

	return content
}
