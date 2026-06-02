package vector

import "context"

// Document represents a document in a vector store.
type Document struct {
	ID       string
	Content  string
	Vector   []float32
	Metadata map[string]string
}

// VectorRetriever performs semantic search over vector embeddings.
type VectorRetriever interface {
	Search(ctx context.Context, queryVector []float32, topK int, filter map[string]string) ([]*Document, error)
}

// VectorIndexer indexes documents with their vector embeddings.
type VectorIndexer interface {
	Insert(ctx context.Context, docs []*Document) error
	Delete(ctx context.Context, ids []string) error
}
