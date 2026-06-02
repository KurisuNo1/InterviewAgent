package llm

import (
	"context"
	"fmt"
	"io"
	"os"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

// DeepSeekChatModel implements ChatModel using Tongyi Qianwen's OpenAI-compatible API.
type DeepSeekChatModel struct {
	model  *openai.ChatModel
	config DeepSeekConfig
}

// DeepSeekConfig holds configuration for the DeepSeek chat model.
type DeepSeekConfig struct {
	BaseURL     string
	APIKeyEnv   string
	Model       string
	MaxTokens   int
	Temperature float32
	Timeout     int
}

// NewDeepSeekChatModel creates a new DeepSeek chat model using eino-ext's OpenAI-compatible client.
func NewDeepSeekChatModel(ctx context.Context, cfg DeepSeekConfig) (*DeepSeekChatModel, error) {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("environment variable %s is not set", cfg.APIKeyEnv)
	}

	maxTokens := cfg.MaxTokens
	temp := cfg.Temperature

	einoCfg := &openai.ChatModelConfig{
		BaseURL:     cfg.BaseURL,
		APIKey:      apiKey,
		Model:       cfg.Model,
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	}

	chatModel, err := openai.NewChatModel(ctx, einoCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepSeek chat model: %w", err)
	}

	return &DeepSeekChatModel{
		model:  chatModel,
		config: cfg,
	}, nil
}

// convertMessages converts our Message type to Eino's schema.Message.
func convertMessages(messages []Message) []*schema.Message {
	result := make([]*schema.Message, len(messages))
	for i, m := range messages {
		result[i] = &schema.Message{
			Role:    schema.RoleType(m.Role),
			Content: m.Content,
		}
	}
	return result
}

// Chat sends a non-streaming chat request.
func (q *DeepSeekChatModel) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	einoMsgs := convertMessages(messages)
	resp, err := q.model.Generate(ctx, einoMsgs)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}

	return &ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.ResponseMeta.FinishReason,
		TokenUsage: TokenUsage{
			PromptTokens:     resp.ResponseMeta.Usage.PromptTokens,
			CompletionTokens: resp.ResponseMeta.Usage.CompletionTokens,
			TotalTokens:      resp.ResponseMeta.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream sends a streaming chat request.
func (q *DeepSeekChatModel) ChatStream(ctx context.Context, messages []Message) (<-chan *ChatChunk, error) {
	einoMsgs := convertMessages(messages)
	streamReader, err := q.model.Stream(ctx, einoMsgs)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	ch := make(chan *ChatChunk, 64)
	go func() {
		defer close(ch)
		defer streamReader.Close()

		for {
			msg, err := streamReader.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &ChatChunk{Content: fmt.Sprintf("stream error: %v", err), Delta: false}
				return
			}
			ch <- &ChatChunk{Content: msg.Content, Delta: true}
		}
	}()

	return ch, nil
}
