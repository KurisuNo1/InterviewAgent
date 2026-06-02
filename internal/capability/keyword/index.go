package keyword

import "context"

// Document represents a keyword-indexed document.
type Document struct {
	ID       string
	Content  string
	Metadata map[string]string
}

// KeywordIndex performs BM25 keyword-based retrieval.
type KeywordIndex interface {
	Index(ctx context.Context, docs []*Document) error
	Search(ctx context.Context, query string, topK int) ([]*Document, error)
	Close() error
}
