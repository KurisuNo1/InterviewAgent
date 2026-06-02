package embedding

import (
	"context"
	"fmt"
	"os"

	openaiembed "github.com/cloudwego/eino-ext/components/embedding/openai"
)

// OpenAIEmbedder implements Embedder using text-embedding-v3 via OpenAI-compatible API.
type OpenAIEmbedder struct {
	embedder *openaiembed.Embedder
}

// OpenAIEmbeddingConfig holds configuration for the OpenAI embedder.
type OpenAIEmbeddingConfig struct {
	BaseURL    string
	APIKeyEnv  string
	Model      string
	Dimensions int
}

// NewOpenAIEmbedder creates a new OpenAI embedder using eino-ext's OpenAI-compatible embedder.
func NewOpenAIEmbedder(ctx context.Context, cfg OpenAIEmbeddingConfig) (*OpenAIEmbedder, error) {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("environment variable %s is not set", cfg.APIKeyEnv)
	}

	dimensions := cfg.Dimensions
	einoCfg := &openaiembed.EmbeddingConfig{
		BaseURL:    cfg.BaseURL,
		APIKey:     apiKey,
		Model:      cfg.Model,
		Dimensions: &dimensions,
	}

	embedder, err := openaiembed.NewEmbedder(ctx, einoCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI embedder: %w", err)
	}

	return &OpenAIEmbedder{embedder: embedder}, nil
}

// Embed returns embeddings for multiple texts.
func (q *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	floatVectors, err := q.embedder.EmbedStrings(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	result := make([][]float32, len(floatVectors))
	for i, vec := range floatVectors {
		result[i] = make([]float32, len(vec))
		for j, v := range vec {
			result[i][j] = float32(v)
		}
	}
	return result, nil
}

// EmbedSingle returns embedding for a single text.
func (q *OpenAIEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	vecs, err := q.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vecs[0], nil
}
