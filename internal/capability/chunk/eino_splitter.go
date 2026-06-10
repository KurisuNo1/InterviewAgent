package chunk

import (
	"context"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// NewTransformer adapts a chunk.Strategy to Eino's document.Transformer interface.
// Each input document's Content is split into chunks, with parent metadata preserved.
func NewTransformer(strategy Strategy) document.Transformer {
	return &transformerAdapter{strategy: strategy}
}

type transformerAdapter struct {
	strategy Strategy
}

func (a *transformerAdapter) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	var result []*schema.Document
	for _, doc := range src {
		chunks := a.strategy.Split(doc.Content)
		for _, c := range chunks {
			// Merge parent metadata into chunk
			if c.MetaData == nil {
				c.MetaData = make(map[string]any)
			}
			for k, v := range doc.MetaData {
				if _, exists := c.MetaData[k]; !exists {
					c.MetaData[k] = v
				}
			}
			result = append(result, c)
		}
	}
	return result, nil
}
