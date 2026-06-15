package contextmanager

import (
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/config"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// BuildParams holds all inputs needed to assemble a context window for an LLM call.
type BuildParams struct {
	SessionID     string          // optional: session for per-session context monitoring
	ProfileName   string          // e.g. "casual_chat", "interview_ask", "skill"
	SystemPrompt  string          // the base system prompt (before state injection)
	StateContext  map[string]any  // key-value pairs injected into system prompt
	MemorySummary string          // optional: compressed summary from memory hierarchy
	KeyEntities   string          // optional: key entities extracted from conversation
	History       []model.Message // full conversation history (uncompressed)
	RAGDocuments  string          // pre-retrieved reference text
	CurrentQ      string          // current question content (interview only)
	LastAnswer    string          // candidateʼs last answer (interview/eval only)
	UserInput     string          // the current user message
}

// ContextBuilder assembles LLM prompts with budget awareness, compression, and priority-based packing.
type ContextBuilder struct {
	cfg        *config.ContextConfig
	compressor *ConversationCompressor
	monitor    ContextMonitor
	lastUsage  ContextUsage
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder(cfg *config.ContextConfig, compressor *ConversationCompressor, monitor ContextMonitor) *ContextBuilder {
	return &ContextBuilder{cfg: cfg, compressor: compressor, monitor: monitor}
}

// LastUsage returns the token usage from the most recent Build call.
func (b *ContextBuilder) LastUsage() ContextUsage {
	return b.lastUsage
}

// Build assembles the final message list for an LLM call, respecting the token budget.
func (b *ContextBuilder) Build(params BuildParams) []*schema.Message {
	profile := Profile(b.cfg, params.ProfileName)
	budget := NewTokenBudget(b.totalBudget())

	var messages []*schema.Message

	// --- Step 1: System Prompt (mandatory, trimmed to profile limit) ---
	sysPrompt := b.buildSystemPrompt(params)
	sysTokens := EstimateTokens(sysPrompt)
	if sysTokens > profile.SystemMax {
		sysPrompt = truncateText(sysPrompt, profile.SystemMax*3) // ~3 chars per token for mixed text
		sysTokens = EstimateTokens(sysPrompt)
	}
	budget.Reserve("system", sysTokens)
	messages = append(messages, schema.SystemMessage(sysPrompt))

	// --- Step 2: Conversation History (priority-based packing) ---
	historyMsgs := b.packHistory(params.History, params.UserInput, profile, budget)
	messages = append(messages, historyMsgs...)

	// --- Step 3: RAG Documents (medium priority, truncated to fit) ---
	if params.RAGDocuments != "" {
		ragBudget := profile.RAGMax
		if rem := budget.Remaining(); rem < ragBudget {
			ragBudget = rem
		}
		if ragBudget > 0 {
			ragText := fitToBudget(params.RAGDocuments, ragBudget)
			ragTokens := EstimateTokens(ragText)
			budget.Spend("rag", ragTokens)
			// Inject RAG into system message as reference knowledge
			messages[0] = schema.SystemMessage(messages[0].Content +
				"\n\n## Reference Knowledge\n" + ragText)
		}
	}

	log.Printf("[ContextBuilder] profile=%s budget=%d used=%d system=%d rag=%d history=%d msgs",
		params.ProfileName, budget.Limit(), budget.Used(),
		budget.Allocated()["system"], budget.Allocated()["rag"], budget.Allocated()["history"])

	// Store usage for external monitoring.
	// Note: system budget already includes MemorySummary and KeyEntities since
	// buildSystemPrompt injects them before token estimation.
	b.lastUsage = ContextUsage{
		SystemPromptTokens: budget.Allocated()["system"],
		HistoryTokens:       budget.Allocated()["history"],
		RAGDocTokens:        budget.Allocated()["rag"],
		ToolResultTokens:     budget.Allocated()["tool_result"],
		InputTokens:          EstimateTokens(params.UserInput),
		TotalTokens:          budget.Used(),
		WindowLimit:          budget.Limit(),
	}

	// Auto-report to monitor if configured
	if b.monitor != nil {
		b.monitor.RecordUsage(params.SessionID, params.ProfileName, b.lastUsage)
	}

	return messages
}

// buildSystemPrompt assembles the full system prompt with state context injection.
func (b *ContextBuilder) buildSystemPrompt(params BuildParams) string {
	var sb strings.Builder
	sb.WriteString(params.SystemPrompt)

	// Inject key entities first (critical information that must be remembered)
	if params.KeyEntities != "" {
		sb.WriteString("\n\n")
		sb.WriteString(params.KeyEntities)
	}

	// Inject memory summary (compressed conversation from prior interactions)
	if params.MemorySummary != "" {
		sb.WriteString("\n\n## Conversation Context\n")
		sb.WriteString(params.MemorySummary)
	}

	// Inject state context as key-value pairs
	if len(params.StateContext) > 0 {
		sb.WriteString("\n\n## Current State\n")
		for key, val := range params.StateContext {
			sb.WriteString(fmt.Sprintf("- %s: %v\n", key, val))
		}
	}

	return sb.String()
}

// packHistory builds the history portion of the prompt with compression if needed.
func (b *ContextBuilder) packHistory(history []model.Message, userInput string, profile config.ContextProfile, budget *TokenBudget) []*schema.Message {
	if len(history) == 0 {
		// No history: just the current user input
		inputTokens := EstimateTokens(userInput)
		budget.Spend("history", inputTokens)
		return []*schema.Message{schema.UserMessage(userInput)}
	}

	verbatimTurns := profile.RecentVerbatimTurns
	maxTurns := profile.HistoryMaxTurns
	if verbatimTurns <= 0 {
		verbatimTurns = 3
	}
	if maxTurns <= 0 {
		maxTurns = 8
	}

	historyBudget := profile.WorkingMemory
	if rem := budget.Remaining(); rem < historyBudget {
		historyBudget = rem
	}

	var finalMsgs []model.Message

	// Compress if history exceeds budget or threshold
	if b.compressor != nil && (b.compressor.ShouldCompress(history, historyBudget) || len(history)/2 > profile.CompressionThreshold) {
		finalMsgs = b.compressor.Compress(history, verbatimTurns, maxTurns, historyBudget)
	} else {
		// Truncate to last N turns
		turns := messagesToTurns(history)
		if len(turns) > maxTurns {
			turns = turns[len(turns)-maxTurns:]
		}
		finalMsgs = turnsToMessages(turns)
	}

	// Cap at budget
	if len(finalMsgs) > maxTurns*2+2 {
		finalMsgs = finalMsgs[len(finalMsgs)-maxTurns*2-2:]
	}

	// Convert to Eino schema messages and append user input
	var result []*schema.Message
	histTokens := 0
	for _, m := range finalMsgs {
		if m.Role == "" || m.Content == "" {
			continue
		}
		tok := EstimateTokens(m.Content)
		if histTokens+tok > historyBudget {
			break
		}
		histTokens += tok
		result = append(result, &schema.Message{
			Role:    schema.RoleType(m.Role),
			Content: m.Content,
		})
	}

	inputTokens := EstimateTokens(userInput)
	histTokens += inputTokens
	budget.Spend("history", histTokens)
	result = append(result, schema.UserMessage(userInput))

	return result
}

// totalBudget returns the configured max token budget or a sensible default.
func (b *ContextBuilder) totalBudget() int {
	if b.cfg != nil && b.cfg.MaxTokens > 0 {
		return b.cfg.MaxTokens
	}
	return 32768
}

// fitToBudget truncates text to approximately fit within the given token budget.
func fitToBudget(text string, maxTokens int) string {
	// Estimate: 3 bytes per token for mixed text (conservative)
	maxBytes := maxTokens * 3
	if len(text) <= maxBytes {
		return text
	}
	// Try to break at a paragraph boundary
	cutoff := maxBytes
	if idx := strings.LastIndex(text[:cutoff], "\n\n"); idx > cutoff/2 {
		cutoff = idx
	}
	return text[:cutoff] + "\n\n[truncated]"
}
