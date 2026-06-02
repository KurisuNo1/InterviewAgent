package rag

import (
	"context"
	"strconv"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/keyword"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/vector"
)

// VectorRetrieverAdapter wraps vector.VectorRetriever to match the RAG VectorRetriever interface.
type VectorRetrieverAdapter struct {
	store vector.VectorRetriever
}

// NewVectorRetrieverAdapter creates a new adapter.
func NewVectorRetrieverAdapter(store vector.VectorRetriever) *VectorRetrieverAdapter {
	if store == nil {
		return nil
	}
	return &VectorRetrieverAdapter{store: store}
}

// Search delegates to the underlying vector store.
func (a *VectorRetrieverAdapter) Search(ctx context.Context, query []float32, topK int, filters map[string]string) ([]Document, error) {
	docs, err := a.store.Search(ctx, query, topK, filters)
	if err != nil {
		return nil, err
	}

	result := make([]Document, len(docs))
	for i, d := range docs {
		result[i] = Document{
			Content: d.Content,
			Score:   0.5, // Vector search doesn't return explicit scores
			Source:  "vector",
		}
	}
	return result, nil
}

// KeywordSearcherAdapter wraps keyword.KeywordIndex to match the RAG KeywordSearcher interface.
type KeywordSearcherAdapter struct {
	index keyword.KeywordIndex
}

// NewKeywordSearcherAdapter creates a new adapter.
func NewKeywordSearcherAdapter(index keyword.KeywordIndex) *KeywordSearcherAdapter {
	if index == nil {
		return nil
	}
	return &KeywordSearcherAdapter{index: index}
}

// Search delegates to the underlying keyword index.
func (a *KeywordSearcherAdapter) Search(ctx context.Context, query string, topK int) ([]Document, error) {
	docs, err := a.index.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	result := make([]Document, len(docs))
	for i, d := range docs {
		score := 0.0
		// Extract BM25 score from metadata if available
		if s, ok := d.Metadata["score"]; ok {
			score, _ = strconv.ParseFloat(s, 64)
		}
		result[i] = Document{
			Content: d.Content,
			Score:   score,
			Source:  "keyword",
		}
	}
	return result, nil
}
