package contextmanager

import "fmt"

// ContextUsage records the actual token consumption of each component for a single LLM call.
type ContextUsage struct {
	SystemPromptTokens int `json:"system_prompt_tokens"`
	ToolDefTokens      int `json:"tool_def_tokens"`
	HistoryTokens       int `json:"history_tokens"`
	RAGDocTokens        int `json:"rag_doc_tokens"`
	ToolResultTokens    int `json:"tool_result_tokens"`
	InputTokens         int `json:"input_tokens"`
	TotalTokens         int `json:"total_tokens"`
	WindowLimit         int `json:"window_limit"`
}

// UsagePercent returns usage as a percentage (0-100).
func (u ContextUsage) UsagePercent() float64 {
	if u.WindowLimit <= 0 {
		return 0
	}
	return float64(u.TotalTokens) / float64(u.WindowLimit) * 100
}

// IsWarning returns true when usage exceeds the given threshold (e.g. 0.80 for 80%).
func (u ContextUsage) IsWarning(threshold float64) bool {
	return u.UsagePercent() > threshold*100
}

// IsCritical returns true when usage exceeds 95%.
func (u ContextUsage) IsCritical() bool {
	return u.UsagePercent() > 95
}

// LogFields returns structured key-value pairs for logging.
func (u ContextUsage) LogFields() []any {
	return []any{
		"total", u.TotalTokens,
		"limit", u.WindowLimit,
		"percent", fmt.Sprintf("%.1f%%", u.UsagePercent()),
		"system", u.SystemPromptTokens,
		"history", u.HistoryTokens,
		"rag", u.RAGDocTokens,
		"tool_result", u.ToolResultTokens,
		"input", u.InputTokens,
	}
}
