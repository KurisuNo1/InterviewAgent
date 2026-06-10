package keyword

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// KeywordIndex performs BM25 keyword-based retrieval using schema.Document.
type KeywordIndex interface {
	Index(ctx context.Context, docs []*schema.Document) error
	Search(ctx context.Context, query string, topK int) ([]*schema.Document, error)
	ListAll(ctx context.Context) ([]*schema.Document, error)
	Delete(ctx context.Context, id string) error
	Close() error
}
