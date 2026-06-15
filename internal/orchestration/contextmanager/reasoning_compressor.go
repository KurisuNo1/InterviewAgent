package contextmanager

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ReasoningStep captures a single step in a multi-step agent reasoning chain.
type ReasoningStep struct {
	StepNum   int      `json:"step"`
	Summary   string   `json:"summary"`
	KeyFacts  []string `json:"key_facts"`
	RawThought string  `json:"-"` // stored externally, not in LLM context
}

// ReasoningCompressor reduces multi-step agent reasoning traces into compact summaries.
// Recent steps (default 3) are kept in full; older steps are replaced with summaries.
type ReasoningCompressor struct {
	chatModel       einomodel.ToolCallingChatModel
	maxRecentSteps  int
}

// NewReasoningCompressor creates a new reasoning compressor.
func NewReasoningCompressor(chatModel einomodel.ToolCallingChatModel, maxRecentSteps int) *ReasoningCompressor {
	if maxRecentSteps <= 0 {
		maxRecentSteps = 3
	}
	return &ReasoningCompressor{chatModel: chatModel, maxRecentSteps: maxRecentSteps}
}

// CompressSteps takes a list of raw reasoning steps and returns a compressed version.
// The most recent maxRecentSteps are kept as-is; older ones are replaced with summaries.
// rawSteps is a slice of "Thought: ... Action: ... Observation: ..." strings.
func (rc *ReasoningCompressor) CompressSteps(rawSteps []string) []ReasoningStep {
	if len(rawSteps) <= rc.maxRecentSteps {
		// Not enough steps to compress — convert to simple summaries
		steps := make([]ReasoningStep, len(rawSteps))
		for i, raw := range rawSteps {
			steps[i] = ReasoningStep{
				StepNum:   i + 1,
				Summary:   extractOneLineSummary(raw),
				KeyFacts:  extractKeyFacts(raw),
			}
		}
		return steps
	}

	steps := make([]ReasoningStep, 0, len(rawSteps))
	compressUpTo := len(rawSteps) - rc.maxRecentSteps

	// Older steps: compress to summaries
	for i := 0; i < compressUpTo; i++ {
		steps = append(steps, ReasoningStep{
			StepNum:   i + 1,
			Summary:   extractOneLineSummary(rawSteps[i]),
			KeyFacts:  extractKeyFacts(rawSteps[i]),
		})
	}

	// Recent steps: keep as-is
	for i := compressUpTo; i < len(rawSteps); i++ {
		steps = append(steps, ReasoningStep{
			StepNum:    i + 1,
			Summary:    rawSteps[i],
			KeyFacts:   extractKeyFacts(rawSteps[i]),
			RawThought: rawSteps[i],
		})
	}

	return steps
}

// CompressStepsWithLLM uses the chat model to produce high-quality summaries of older steps.
func (rc *ReasoningCompressor) CompressStepsWithLLM(ctx context.Context, rawSteps []string) ([]ReasoningStep, error) {
	if len(rawSteps) <= rc.maxRecentSteps {
		return rc.CompressSteps(rawSteps), nil
	}

	steps := make([]ReasoningStep, 0, len(rawSteps))
	compressUpTo := len(rawSteps) - rc.maxRecentSteps

	// Use LLM to summarize older steps in batches
	if rc.chatModel != nil && compressUpTo > 0 {
		batchSummary, err := rc.summarizeBatch(ctx, rawSteps[:compressUpTo])
		if err == nil && batchSummary != "" {
			steps = append(steps, ReasoningStep{
				StepNum:   1,
				Summary:   "[Steps 1-" + fmt.Sprintf("%d", compressUpTo) + "] " + batchSummary,
				KeyFacts:  extractKeyFacts(batchSummary),
			})
		} else {
			// Fall back to rule-based
			for i := 0; i < compressUpTo; i++ {
				steps = append(steps, ReasoningStep{
					StepNum:  i + 1,
					Summary:  extractOneLineSummary(rawSteps[i]),
					KeyFacts: extractKeyFacts(rawSteps[i]),
				})
			}
		}
	} else {
		for i := 0; i < compressUpTo; i++ {
			steps = append(steps, ReasoningStep{
				StepNum:  i + 1,
				Summary:  extractOneLineSummary(rawSteps[i]),
				KeyFacts: extractKeyFacts(rawSteps[i]),
			})
		}
	}

	// Recent steps: keep verbatim
	for i := compressUpTo; i < len(rawSteps); i++ {
		steps = append(steps, ReasoningStep{
			StepNum:    i + 1,
			Summary:    rawSteps[i],
			KeyFacts:   extractKeyFacts(rawSteps[i]),
			RawThought: rawSteps[i],
		})
	}

	return steps, nil
}

// FormatSteps formats compressed steps for inclusion in the LLM context.
func (rc *ReasoningCompressor) FormatSteps(steps []ReasoningStep) string {
	var sb strings.Builder
	sb.WriteString("## Reasoning Summary\n")
	for _, s := range steps {
		sb.WriteString(fmt.Sprintf("- Step %d: %s\n", s.StepNum, s.Summary))
		if len(s.KeyFacts) > 0 {
			sb.WriteString(fmt.Sprintf("  Key facts: %s\n", strings.Join(s.KeyFacts, "; ")))
		}
	}
	return sb.String()
}

// summarizeBatch uses the LLM to summarize a batch of reasoning steps.
func (rc *ReasoningCompressor) summarizeBatch(ctx context.Context, steps []string) (string, error) {
	if rc.chatModel == nil {
		return "", fmt.Errorf("no chat model available")
	}

	var sb strings.Builder
	for i, s := range steps {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, truncateText(s, 300)))
	}

	prompt := fmt.Sprintf(`Summarize the following agent reasoning steps into a single concise paragraph (max 150 words).
Focus on: what was done, key findings, and any important data retrieved.

%s`, sb.String())

	resp, err := rc.chatModel.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
}

// extractOneLineSummary extracts the first meaningful sentence from a raw step.
func extractOneLineSummary(raw string) string {
	// Try to extract the key action from the thought/observation
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip prefix labels
		for _, prefix := range []string{"Thought:", "Action:", "Observation:", "思考：", "行动：", "观察："} {
			if after, ok := strings.CutPrefix(line, prefix); ok {
				line = strings.TrimSpace(after)
				break
			}
		}
		if len(line) > 10 {
			if len(line) > 200 {
				line = string([]rune(line)[:200]) + "..."
			}
			return line
		}
	}
	// Fallback: first 150 chars
	runes := []rune(raw)
	if len(runes) > 150 {
		return string(runes[:150]) + "..."
	}
	return raw
}

// extractKeyFacts extracts key entities and facts from a reasoning step.
func extractKeyFacts(raw string) []string {
	var facts []string

	// Simple heuristic: look for lines with key indicators
	indicators := []string{
		"found:", "result:", "answer:", "score:", "decision:",
		"发现：", "结果：", "答案：", "评分：", "决定：",
		"contains:", "total:", "count:",
	}

	lower := strings.ToLower(raw)
	for _, indicator := range indicators {
		idx := strings.Index(lower, indicator)
		if idx >= 0 {
			end := idx + len(indicator)
			rest := raw[end:]
			if newline := strings.IndexAny(rest, "\n。！？.!?"); newline > 0 {
				rest = rest[:newline]
			}
			rest = strings.TrimSpace(rest)
			if len(rest) > 0 && len(rest) < 200 {
				facts = append(facts, rest)
			}
		}
	}

	if len(facts) > 5 {
		facts = facts[:5]
	}
	return facts
}
