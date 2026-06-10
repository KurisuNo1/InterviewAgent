package vector

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// VectorRetriever performs semantic search over vector embeddings.
type VectorRetriever interface {
	Search(ctx context.Context, queryVector []float32, topK int, filter map[string]string) ([]*schema.Document, error)
}

// VectorIndexer indexes documents with their vector embeddings.
// Documents must have their dense vectors set via doc.WithDenseVector().
type VectorIndexer interface {
	Insert(ctx context.Context, docs []*schema.Document) error
	Delete(ctx context.Context, ids []string) error
}
