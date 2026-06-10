package contextmanager

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// CompressedTurn is a structured, compact representation of a Q&A exchange.
type CompressedTurn struct {
	QSummary   string   `json:"qs"`
	AKeyPoints []string `json:"akp"`
	Score      float64  `json:"sc"`
	WeakDims   []string `json:"wd"`
}

// ConversationCompressor reduces conversation history size for LLM context.
type ConversationCompressor struct {
	chatModel einomodel.ToolCallingChatModel // used for LLM summarization (strategy C)
}

// NewConversationCompressor creates a new compressor.
func NewConversationCompressor(chatModel einomodel.ToolCallingChatModel) *ConversationCompressor {
	return &ConversationCompressor{chatModel: chatModel}
}

// ShouldCompress checks whether the total estimated tokens exceed the budget.
func (c *ConversationCompressor) ShouldCompress(messages []model.Message, budget int) bool {
	total := 0
	for _, m := range messages {
		total += EstimateTokens(m.Content)
	}
	return total > budget
}

// Compress applies compression to fit messages within the given token budget.
// Returns the compressed message list.
func (c *ConversationCompressor) Compress(messages []model.Message, verbatimTurns int, maxTurns int, budget int) []model.Message {
	n := len(messages)
	if n == 0 {
		return messages
	}

	// Convert to turn pairs: [user, assistant] pairs
	turns := messagesToTurns(messages)

	// If within budget, return as-is
	if !c.ShouldCompress(messages, budget) && len(turns) <= maxTurns {
		return messages
	}

	return c.slidingWindow(turns, verbatimTurns, maxTurns)
}

// slidingWindow implements Strategy A: recent turns verbatim, middle turns compressed, oldest summarized.
func (c *ConversationCompressor) slidingWindow(turns []turnPair, verbatimCount int, maxTurns int) []model.Message {
	total := len(turns)

	// If few turns, return verbatim
	if total <= maxTurns {
		return turnsToMessages(turns)
	}

	var result []model.Message

	// Determine boundaries
	verbatimStart := total - verbatimCount
	if verbatimStart < 0 {
		verbatimStart = 0
	}
	midStart := total - maxTurns
	if midStart < 0 {
		midStart = 0
	}

	// Oldest turns → summarized
	if midStart > 0 {
		summary := c.summarizeTurns(turns[:midStart])
		if summary != "" {
			result = append(result, model.Message{
				Role:    model.RoleSystem,
				Content: "[Earlier conversation summary]\n" + summary,
			})
		}
	}

	// Middle turns → compressed (structured extraction)
	for i := midStart; i < verbatimStart; i++ {
		compressed := c.structuredExtract(turns[i])
		result = append(result, compressed)
	}

	// Recent turns → verbatim
	for i := verbatimStart; i < total; i++ {
		result = append(result, turns[i].User, turns[i].Assistant)
	}

	return result
}

// structuredExtract implements Strategy B: compress a turn into a compact representation.
func (c *ConversationCompressor) structuredExtract(t turnPair) model.Message {
	var sb strings.Builder
	sb.WriteString("[Q&A Summary] ")
	// Truncate long user content to key points
	userContent := truncateText(t.User.Content, 200)
	sb.WriteString("Q: ")
	sb.WriteString(userContent)
	sb.WriteString(" | A: ")
	assistantContent := truncateText(t.Assistant.Content, 300)
	sb.WriteString(assistantContent)
	return model.Message{
		Role:    model.RoleSystem,
		Content: sb.String(),
	}
}

// summarizeTurns generates a brief summary of older conversation turns (Strategy C light).
// Uses rule-based extraction; full LLM summarization is done asynchronously.
func (c *ConversationCompressor) summarizeTurns(turns []turnPair) string {
	if len(turns) == 0 {
		return ""
	}
	topics := make([]string, 0, len(turns))
	for _, t := range turns {
		// Extract the first sentence of each question as a topic marker
		firstSentence := extractFirstSentence(t.User.Content)
		if firstSentence != "" {
			topics = append(topics, firstSentence)
		}
	}
	if len(topics) == 0 {
		return ""
	}
	return "Covered topics: " + strings.Join(topics, "; ")
}

// SummarizeWithLLM uses the chat model to produce a concise summary (Strategy C, async).
func (c *ConversationCompressor) SummarizeWithLLM(ctx context.Context, messages []model.Message) (string, error) {
	if c.chatModel == nil || len(messages) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, truncateText(m.Content, 500)))
	}

	prompt := `Summarize the following conversation into a concise paragraph (max 200 words). Focus on:
- Key topics discussed
- Important facts or decisions made
- Any unresolved questions

Conversation:
` + sb.String()

	resp, err := c.chatModel.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
}

// turnPair represents a user-assistant exchange.
type turnPair struct {
	User      model.Message
	Assistant model.Message
}

func messagesToTurns(messages []model.Message) []turnPair {
	var turns []turnPair
	var current *turnPair
	for i := range messages {
		m := &messages[i]
		switch m.Role {
		case model.RoleUser:
			if current != nil {
				turns = append(turns, *current)
			}
			current = &turnPair{User: *m}
		case model.RoleAssistant:
			if current != nil {
				current.Assistant = *m
				turns = append(turns, *current)
				current = nil
			}
		}
	}
	if current != nil {
		turns = append(turns, *current)
	}
	return turns
}

func turnsToMessages(turns []turnPair) []model.Message {
	var msgs []model.Message
	for _, t := range turns {
		if t.User.Content != "" {
			msgs = append(msgs, t.User)
		}
		if t.Assistant.Content != "" {
			msgs = append(msgs, t.Assistant)
		}
	}
	return msgs
}

func extractFirstSentence(content string) string {
	// Find first sentence ending with . ! ? 。 ！ ？
	endChars := ".!?。！？\n"
	idx := strings.IndexFunc(content, func(r rune) bool {
		return strings.ContainsRune(endChars, r)
	})
	if idx > 0 {
		return strings.TrimSpace(content[:idx+1])
	}
	if len(content) > 100 {
		return content[:100] + "..."
	}
	return content
}

func truncateText(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "..."
}
