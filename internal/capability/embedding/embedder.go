package embedding

import "context"

// Embedder converts text into vector embeddings.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
}
