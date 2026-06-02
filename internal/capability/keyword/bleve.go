package keyword

import (
	"context"
	"fmt"

	"github.com/blevesearch/bleve/v2"
)

// BleveConfig holds configuration for the Bleve BM25 index.
type BleveConfig struct {
	IndexPath string
}

// BleveIndex implements KeywordIndex using blevesearch.
type BleveIndex struct {
	cfg   BleveConfig
	index bleve.Index
}

// NewBleveIndex creates a new Bleve keyword index.
func NewBleveIndex(cfg BleveConfig) (*BleveIndex, error) {
	var idx bleve.Index
	var err error

	// Try to open existing index
	idx, err = bleve.Open(cfg.IndexPath)
	if err != nil {
		// Create new index
		mapping := bleve.NewIndexMapping()
		idx, err = bleve.New(cfg.IndexPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create Bleve index at %s: %w", cfg.IndexPath, err)
		}
	}

	return &BleveIndex{cfg: cfg, index: idx}, nil
}

// Index adds documents to the Bleve index.
func (b *BleveIndex) Index(ctx context.Context, docs []*Document) error {
	batch := b.index.NewBatch()
	for _, doc := range docs {
		data := map[string]interface{}{
			"content": doc.Content,
		}
		for k, v := range doc.Metadata {
			data[k] = v
		}
		if err := batch.Index(doc.ID, data); err != nil {
			return fmt.Errorf("failed to batch index document %s: %w", doc.ID, err)
		}
	}
	return b.index.Batch(batch)
}

// Search performs BM25 keyword search.
func (b *BleveIndex) Search(ctx context.Context, query string, topK int) ([]*Document, error) {
	searchReq := bleve.NewSearchRequest(bleve.NewQueryStringQuery(query))
	searchReq.Size = topK
	searchReq.Fields = []string{"*"}

	result, err := b.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	docs := make([]*Document, 0, len(result.Hits))
	for _, hit := range result.Hits {
		content := ""
		if c, ok := hit.Fields["content"]; ok {
			if cs, ok := c.(string); ok {
				content = cs
			}
		}
		meta := map[string]string{
			"score": fmt.Sprintf("%.4f", hit.Score),
		}
		for k, v := range hit.Fields {
			if k == "content" {
				continue
			}
			if vs, ok := v.(string); ok {
				meta[k] = vs
			}
		}
		docs = append(docs, &Document{
			ID:       hit.ID,
			Content:  content,
			Metadata: meta,
		})
	}

	return docs, nil
}

// Close releases resources held by the index.
func (b *BleveIndex) Close() error {
	return b.index.Close()
}
