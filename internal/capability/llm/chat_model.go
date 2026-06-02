package llm

import "context"

// Message is a chat message.
type Message struct {
	Role    string
	Content string
}

// ChatResponse is the response from a chat model.
type ChatResponse struct {
	Content      string
	TokenUsage   TokenUsage
	FinishReason string
}

// ChatChunk represents a streaming response chunk.
type ChatChunk struct {
	Content string
	Delta   bool
}

// TokenUsage records token consumption.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ChatModel is the interface for LLM chat models.
type ChatModel interface {
	Chat(ctx context.Context, messages []Message) (*ChatResponse, error)
	ChatStream(ctx context.Context, messages []Message) (<-chan *ChatChunk, error)
}
