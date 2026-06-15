package contextmanager

import (
	"context"
	"fmt"
	"log"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// OverflowStrategy describes a single degradation action.
type OverflowStrategy struct {
	Name     string
	Priority int // 1 = apply first
	Action   OverflowAction
}

// OverflowAction is a function that attempts to reduce context usage.
// Returns the (possibly) modified messages and the new estimated token count.
type OverflowAction func(ctx context.Context, input OverflowInput) (OverflowOutput, error)

// OverflowInput carries the data needed to apply degradation.
type OverflowInput struct {
	SystemPrompt string
	Messages     []*schema.Message
	History      []model.Message
	RAGDocuments string
	Profile      string
	MaxTokens    int
	CurrentUsage ContextUsage
}

// OverflowOutput is the result of applying a degradation strategy.
type OverflowOutput struct {
	SystemPrompt string
	Messages     []*schema.Message
	NewUsage     ContextUsage
	Applied      bool
}

// OverflowHandler manages tiered degradation when context windows approach limits.
type OverflowHandler struct {
	strategies []OverflowStrategy
	chatModel  einomodel.ToolCallingChatModel
}

// NewOverflowHandler creates a handler with the standard degradation priority chain.
func NewOverflowHandler(chatModel einomodel.ToolCallingChatModel) *OverflowHandler {
	h := &OverflowHandler{chatModel: chatModel}
	h.strategies = []OverflowStrategy{
		{Name: "trim_tool_results", Priority: 1, Action: h.trimToolResults},
		{Name: "compress_history", Priority: 2, Action: h.compressHistory},
		{Name: "reduce_rag_docs", Priority: 3, Action: h.reduceRAGDocs},
		{Name: "simplify_system_prompt", Priority: 4, Action: h.simplifySystemPrompt},
		{Name: "suggest_new_session", Priority: 5, Action: h.suggestNewSession},
	}
	return h
}

// Degrade attempts to reduce context usage below the limit by applying strategies in priority order.
// Returns the first successful degradation, or the last result if all fail.
func (h *OverflowHandler) Degrade(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	log.Printf("[OverflowHandler] Degrading: profile=%s current=%d limit=%d",
		input.Profile, input.CurrentUsage.TotalTokens, input.MaxTokens)

	for _, s := range h.strategies {
		output, err := s.Action(ctx, input)
		if err != nil {
			log.Printf("[OverflowHandler] Strategy %q error: %v", s.Name, err)
			continue
		}
		if !output.Applied {
			continue
		}
		if output.NewUsage.TotalTokens <= input.MaxTokens {
			log.Printf("[OverflowHandler] Degradation successful: strategy=%s before=%d after=%d",
				s.Name, input.CurrentUsage.TotalTokens, output.NewUsage.TotalTokens)
			return output, nil
		}
		// Strategy helped but not enough — continue with updated input
		input = OverflowInput{
			SystemPrompt: output.SystemPrompt,
			Messages:     output.Messages,
			History:      input.History,
			RAGDocuments: input.RAGDocuments,
			Profile:      input.Profile,
			MaxTokens:    input.MaxTokens,
			CurrentUsage: output.NewUsage,
		}
		log.Printf("[OverflowHandler] Strategy %q partially helped: %d -> %d (still over limit)",
			s.Name, input.CurrentUsage.TotalTokens, output.NewUsage.TotalTokens)
	}

	// All strategies applied but still over limit
	log.Printf("[OverflowHandler] All degradation strategies exhausted, still at %d/%d tokens",
		input.CurrentUsage.TotalTokens, input.MaxTokens)
	return OverflowOutput{
		SystemPrompt: input.SystemPrompt,
		Messages:     input.Messages,
		NewUsage:     input.CurrentUsage,
		Applied:      false,
	}, nil
}

// ShouldDegrade checks if the given usage warrants degradation.
func (h *OverflowHandler) ShouldDegrade(usage ContextUsage, maxTokens int) bool {
	return usage.TotalTokens > maxTokens || usage.IsWarning(0.85)
}

// --- Strategy implementations ---

func (h *OverflowHandler) trimToolResults(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	// Tool results are typically embedded in history messages with role "tool" or system markers.
	// Find and truncate large tool result messages.
	var newMsgs []*schema.Message
	trimmed := false
	for _, m := range input.Messages {
		if (m.Role == schema.RoleType("tool") || strings.Contains(m.Content, "[tool_result")) &&
			EstimateTokens(m.Content) > 1024 {
			truncated := fitToBudget(m.Content, 1024)
			newMsgs = append(newMsgs, &schema.Message{Role: m.Role, Content: truncated})
			trimmed = true
		} else {
			newMsgs = append(newMsgs, m)
		}
	}

	if !trimmed {
		return OverflowOutput{Applied: false}, nil
	}

	newTotal := estimateTotalTokens(newMsgs, input.SystemPrompt)
	return OverflowOutput{
		SystemPrompt: input.SystemPrompt,
		Messages:     newMsgs,
		NewUsage:     ContextUsage{TotalTokens: newTotal, WindowLimit: input.MaxTokens},
		Applied:      true,
	}, nil
}

func (h *OverflowHandler) compressHistory(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	if len(input.Messages) <= 6 {
		return OverflowOutput{Applied: false}, nil
	}

	// Keep system message + last 3 turns (6 messages) + compress the rest
	sysMsg := input.Messages[0] // system prompt
	keepStart := len(input.Messages) - 6
	if keepStart < 1 {
		keepStart = 1
	}

	compressed := []*schema.Message{sysMsg}

	// Insert a summary for the dropped messages
	if keepStart > 1 && h.chatModel != nil {
		var sb strings.Builder
		for _, m := range input.Messages[1:keepStart] {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, truncateText(m.Content, 200)))
		}
		summary, err := h.chatModel.Generate(ctx, []*schema.Message{
			schema.UserMessage("Summarize this conversation into 2-3 sentences:\n" + sb.String()),
		})
		if err == nil && summary != nil && summary.Content != "" {
			compressed = append(compressed, schema.SystemMessage(
				"[Earlier conversation summary]\n" + summary.Content))
		}
	}

	compressed = append(compressed, input.Messages[keepStart:]...)

	newTotal := estimateTotalTokens(compressed, input.SystemPrompt)
	return OverflowOutput{
		SystemPrompt: input.SystemPrompt,
		Messages:     compressed,
		NewUsage:     ContextUsage{TotalTokens: newTotal, WindowLimit: input.MaxTokens},
		Applied:      true,
	}, nil
}

func (h *OverflowHandler) reduceRAGDocs(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	sysContent := input.Messages[0].Content
	ragMarker := "## Reference Knowledge"
	idx := strings.Index(sysContent, ragMarker)
	if idx < 0 {
		return OverflowOutput{Applied: false}, nil
	}

	// Keep only the first half of RAG content
	ragContent := sysContent[idx:]
	lines := strings.Split(ragContent, "\n")
	keep := len(lines) / 2
	if keep < 3 {
		keep = 3
	}
	if keep > len(lines) {
		keep = len(lines)
	}
	truncatedRAG := strings.Join(lines[:keep], "\n") + "\n\n[RAG content truncated to fit context window]"

	newSysPrompt := sysContent[:idx] + truncatedRAG
	newMsgs := make([]*schema.Message, len(input.Messages))
	copy(newMsgs, input.Messages)
	newMsgs[0] = schema.SystemMessage(newSysPrompt)

	newTotal := estimateTotalTokens(newMsgs, newSysPrompt)
	return OverflowOutput{
		SystemPrompt: newSysPrompt,
		Messages:     newMsgs,
		NewUsage:     ContextUsage{TotalTokens: newTotal, WindowLimit: input.MaxTokens},
		Applied:      true,
	}, nil
}

func (h *OverflowHandler) simplifySystemPrompt(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	sysContent := input.Messages[0].Content
	if len(sysContent) < 500 {
		return OverflowOutput{Applied: false}, nil
	}

	// Keep only role definition and key rules, drop examples and verbose guidelines
	// Simple heuristic: drop everything after "## 行为准则" or "## Behavior"
	sections := []string{"## 行为准则", "## Behavior", "## 示例", "## Examples", "## 详细说明", "## Details"}
	for _, marker := range sections {
		if idx := strings.Index(sysContent, marker); idx > 100 {
			sysContent = sysContent[:idx]
			break
		}
	}
	sysContent = strings.TrimSpace(sysContent)
	sysContent += "\n\n[Instructions simplified to fit context window]"

	newMsgs := make([]*schema.Message, len(input.Messages))
	copy(newMsgs, input.Messages)
	newMsgs[0] = schema.SystemMessage(sysContent)

	newTotal := estimateTotalTokens(newMsgs, sysContent)
	return OverflowOutput{
		SystemPrompt: sysContent,
		Messages:     newMsgs,
		NewUsage:     ContextUsage{TotalTokens: newTotal, WindowLimit: input.MaxTokens},
		Applied:      true,
	}, nil
}

func (h *OverflowHandler) suggestNewSession(ctx context.Context, input OverflowInput) (OverflowOutput, error) {
	// Last resort: suggest user start a new session
	log.Printf("[OverflowHandler] Suggesting new session for profile=%s (usage=%d/%d)",
		input.Profile, input.CurrentUsage.TotalTokens, input.MaxTokens)
	return OverflowOutput{
		SystemPrompt: input.SystemPrompt,
		Messages:     input.Messages,
		NewUsage:     input.CurrentUsage,
		Applied:      true, // Mark as applied so caller knows we've exhausted options
	}, nil
}

// estimateTotalTokens sums estimated tokens across messages and system prompt.
func estimateTotalTokens(msgs []*schema.Message, systemPrompt string) int {
	total := EstimateTokens(systemPrompt)
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	return total
}
