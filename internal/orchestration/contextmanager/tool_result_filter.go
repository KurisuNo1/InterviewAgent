package contextmanager

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolPersistence marks whether a tool's results should be kept in full or trimmed.
type ToolPersistence int

const (
	// ToolPersistent — results carry useful reference information; keep as much as budget allows.
	ToolPersistent ToolPersistence = iota
	// ToolTransient — results are only useful for the immediate next step; trim aggressively.
	ToolTransient
	// ToolEphemeral — results are disposable after a single use; strip to minimal summary.
	ToolEphemeral
)

// ToolMeta holds per-tool context management settings.
type ToolMeta struct {
	Name        string
	Persistence ToolPersistence
	KeepFields  []string // JSON field names to preserve; if empty, keep all
	MaxTokens   int      // 0 = no limit
}

// ToolResultFilter trims MCP tool results to fit within context budgets.
type ToolResultFilter struct {
	toolMeta map[string]ToolMeta // keyed by full tool name (server_tool)
}

// NewToolResultFilter creates a filter with the given tool metadata.
func NewToolResultFilter(metas []ToolMeta) *ToolResultFilter {
	m := make(map[string]ToolMeta, len(metas))
	for _, meta := range metas {
		m[meta.Name] = meta
	}
	return &ToolResultFilter{toolMeta: m}
}

// Filter applies the appropriate trimming strategy for the given tool.
// Returns the (possibly trimmed) result string.
func (f *ToolResultFilter) Filter(toolName string, result string) string {
	meta, ok := f.toolMeta[toolName]
	if !ok {
		// Unknown tools: apply a generous default cap
		meta = ToolMeta{Persistence: ToolPersistent, MaxTokens: 4096}
	}

	// Apply max token cap first
	if meta.MaxTokens > 0 {
		result = fitToBudget(result, meta.MaxTokens)
	}

	// Apply field filtering for JSON results
	if len(meta.KeepFields) > 0 {
		result = filterJSONFields(result, meta.KeepFields)
	}

	// Apply persistence-based trimming
	switch meta.Persistence {
	case ToolEphemeral:
		result = trimToSummary(result, 150)
	case ToolTransient:
		result = trimToSummary(result, 500)
	default: // ToolPersistent — no additional trimming
	}

	return result
}

// filterJSONFields keeps only the specified fields from a JSON object/array.
func filterJSONFields(raw string, keepFields []string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return raw // not JSON, return as-is
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw // not valid JSON object, return as-is
	}

	filtered := make(map[string]any)
	for _, field := range keepFields {
		if v, ok := data[field]; ok {
			filtered[field] = v
		}
	}

	if len(filtered) == 0 {
		return raw // nothing matched, return original
	}

	out, err := json.Marshal(filtered)
	if err != nil {
		return raw
	}
	return string(out)
}

// trimToSummary truncates the result to approximately maxChars characters,
// breaking at a sentence boundary when possible.
func trimToSummary(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}

	// Try to break at a sentence boundary near the cutoff
	cutoff := maxChars
	endChars := ".!?。！？\n"
	for i := maxChars - 1; i > maxChars/2; i-- {
		if strings.ContainsRune(endChars, runes[i]) {
			cutoff = i + 1
			break
		}
	}
	return string(runes[:cutoff]) + "\n\n[trimmed]"
}

// DefaultToolMetas returns sensible default metadata for known MCP tools.
func DefaultToolMetas() []ToolMeta {
	return []ToolMeta{
		{
			Name:        "github_search_repositories",
			Persistence: ToolPersistent,
			KeepFields:  []string{"name", "full_name", "description", "html_url", "stargazers_count", "language", "topics"},
			MaxTokens:   2048,
		},
		{
			Name:        "web_search_search",
			Persistence: ToolPersistent,
			MaxTokens:   3072,
		},
	}
}

// FormatSummary formats a tool result filter summary for logging.
func (f *ToolResultFilter) FormatSummary(toolName string, originalLen int, filteredLen int) string {
	reduction := 0.0
	if originalLen > 0 {
		reduction = float64(originalLen-filteredLen) / float64(originalLen) * 100
	}
	return fmt.Sprintf("tool=%s orig=%d filtered=%d reduction=%.0f%%", toolName, originalLen, filteredLen, reduction)
}
