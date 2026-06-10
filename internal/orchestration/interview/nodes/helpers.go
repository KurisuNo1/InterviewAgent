package nodes

import (
	"fmt"
	"strings"
)

// escapeFmt escapes percent signs in user-provided strings so that
// fmt.Sprintf does not misinterpret them as format verbs.
func escapeFmt(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// safeFmt works like fmt.Sprintf but first escapes % signs in all string arguments
// to prevent user input from being misinterpreted as format verbs.
func safeFmt(format string, args ...any) string {
	safe := make([]any, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case string:
			safe[i] = escapeFmt(v)
		case []string:
			escaped := make([]string, len(v))
			for j, s := range v {
				escaped[j] = escapeFmt(s)
			}
			// Format slices as comma-separated for prompt readability
			safe[i] = strings.Join(escaped, ", ")
		case fmt.Stringer:
			safe[i] = escapeFmt(v.String())
		default:
			safe[i] = arg
		}
	}
	return fmt.Sprintf(format, safe...)
}

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
