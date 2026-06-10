package contextmanager

import (
	"unicode/utf8"

	"github.com/KurisuNo1/InterviewAgent/config"
)

// EstimateTokens returns a rough token count for mixed Chinese/English text.
// English: chars/4, Chinese: chars/1.5, mixed fallback: chars/2.5.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	chars := utf8.RuneCountInString(text)
	bytes := len(text)
	// Heuristic: if byte count significantly exceeds rune count, we have CJK text
	// For pure ASCII: bytes ≈ chars (1 byte/char)
	// For pure Chinese: bytes ≈ 3*chars (3 bytes/char in UTF-8)
	// Use the ratio to estimate the language mix.
	ratio := float64(bytes) / float64(chars)
	if ratio < 1.3 {
		// Mostly ASCII/English: ~4 chars per token
		return chars / 4
	}
	if ratio > 2.5 {
		// Mostly CJK: ~1.5 chars per token
		return int(float64(chars) / 1.5)
	}
	// Mixed: ~2.5 chars per token
	return int(float64(chars) / 2.5)
}

// TokenBudget tracks remaining tokens during prompt assembly.
type TokenBudget struct {
	limit     int
	used      int
	allocated map[string]int
}

// NewTokenBudget creates a budget with the given total limit.
func NewTokenBudget(limit int) *TokenBudget {
	if limit <= 0 {
		limit = 32768
	}
	return &TokenBudget{limit: limit, allocated: make(map[string]int)}
}

// Remaining returns how many tokens are left in the budget.
func (b *TokenBudget) Remaining() int {
	rem := b.limit - b.used
	if rem < 0 {
		return 0
	}
	return rem
}

// Limit returns the total budget limit.
func (b *TokenBudget) Limit() int { return b.limit }

// Used returns tokens consumed so far.
func (b *TokenBudget) Used() int { return b.used }

// Spend deducts tokens from the budget. Returns false if it would exceed the limit.
func (b *TokenBudget) Spend(component string, tokens int) bool {
	if tokens <= 0 {
		return true
	}
	if b.used+tokens > b.limit {
		return false
	}
	b.used += tokens
	b.allocated[component] = tokens
	return true
}

// Reserve deducts tokens even if it exceeds the limit (for mandatory components).
func (b *TokenBudget) Reserve(component string, tokens int) {
	b.used += tokens
	b.allocated[component] = tokens
}

// Allocated returns the per-component allocation breakdown.
func (b *TokenBudget) Allocated() map[string]int {
	result := make(map[string]int, len(b.allocated))
	for k, v := range b.allocated {
		result[k] = v
	}
	return result
}

// Profile returns the ContextProfile for a given name.
// Falls back to defaultProfile if the named profile is not found.
func Profile(cfg *config.ContextConfig, name string) config.ContextProfile {
	if cfg != nil {
		if p, ok := cfg.Profiles[name]; ok {
			return p
		}
	}
	return defaultProfile()
}

func defaultProfile() config.ContextProfile {
	return config.ContextProfile{
		SystemMax:            2048,
		WorkingMemory:        16384,
		RAGMax:               4096,
		RecentVerbatimTurns:  3,
		HistoryMaxTurns:      8,
		CompressionThreshold: 8,
	}
}
