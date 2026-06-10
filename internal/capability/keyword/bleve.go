package keyword

import (
	"context"
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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

	idx, err = bleve.Open(cfg.IndexPath)
	if err != nil {
		mapping := bleve.NewIndexMapping()
		idx, err = bleve.New(cfg.IndexPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create Bleve index at %s: %w", cfg.IndexPath, err)
		}
	}

	return &BleveIndex{cfg: cfg, index: idx}, nil
}

// Index adds documents to the Bleve index.
func (b *BleveIndex) Index(ctx context.Context, docs []*schema.Document) error {
	batch := b.index.NewBatch()
	for _, doc := range docs {
		data := map[string]interface{}{
			"content": doc.Content,
		}
		for k, v := range doc.MetaData {
			data[k] = v
		}
		if err := batch.Index(doc.ID, data); err != nil {
			return fmt.Errorf("failed to batch index document %s: %w", doc.ID, err)
		}
	}
	return b.index.Batch(batch)
}

// Search performs BM25 keyword search, returning schema.Document with scores.
func (b *BleveIndex) Search(ctx context.Context, query string, topK int) ([]*schema.Document, error) {
	searchReq := bleve.NewSearchRequest(bleve.NewQueryStringQuery(query))
	searchReq.Size = topK
	searchReq.Fields = []string{"*"}

	result, err := b.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	docs := make([]*schema.Document, 0, len(result.Hits))
	for _, hit := range result.Hits {
		content := ""
		if c, ok := hit.Fields["content"]; ok {
			if cs, ok := c.(string); ok {
				content = cs
			}
		}
		meta := map[string]any{}
		for k, v := range hit.Fields {
			if k == "content" {
				continue
			}
			meta[k] = v
		}
		doc := &schema.Document{
			ID:       hit.ID,
			Content:  content,
			MetaData: meta,
		}
		doc.WithScore(float64(hit.Score))
		docs = append(docs, doc)
	}

	return docs, nil
}

// ListAll returns all documents in the index (up to 10000).
func (b *BleveIndex) ListAll(ctx context.Context) ([]*schema.Document, error) {
	searchReq := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
	searchReq.Size = 10000
	searchReq.Fields = []string{"*"}

	result, err := b.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("list all failed: %w", err)
	}

	docs := make([]*schema.Document, 0, len(result.Hits))
	for _, hit := range result.Hits {
		content := ""
		if c, ok := hit.Fields["content"]; ok {
			if cs, ok := c.(string); ok {
				content = cs
			}
		}
		meta := map[string]any{}
		for k, v := range hit.Fields {
			if k == "content" {
				continue
			}
			meta[k] = v
		}
		docs = append(docs, &schema.Document{
			ID:       hit.ID,
			Content:  content,
			MetaData: meta,
		})
	}
	return docs, nil
}

// Delete removes a document from the index by ID.
func (b *BleveIndex) Delete(ctx context.Context, id string) error {
	return b.index.Delete(id)
}

// Retrieve implements retriever.Retriever for Eino integration.
func (b *BleveIndex) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	topK := 10
	if options.TopK != nil {
		topK = *options.TopK
	}

	docs, err := b.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	for _, d := range docs {
		if d.MetaData == nil {
			d.MetaData = make(map[string]any)
		}
		d.MetaData["source"] = "keyword"
	}
	return docs, nil
}

// Close releases resources held by the index.
func (b *BleveIndex) Close() error {
	return b.index.Close()
}
