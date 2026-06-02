package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
)

// Document represents a retrieved document with its source and score.
type Document struct {
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"` // "vector" or "keyword"
}

// RRFFusion performs Reciprocal Rank Fusion on vector and keyword results.
// k is the ranking constant (default 60).
func RRFFusion(vectorDocs, keywordDocs []Document, k int) []Document {
	if k <= 0 {
		k = 60
	}

	scores := make(map[string]float64)
	sourceMap := make(map[string]string)
	contentMap := make(map[string]string)

	for rank, doc := range vectorDocs {
		key := normalizeKey(doc.Content)
		scores[key] += 1.0 / float64(k+rank+1)
		if contentMap[key] == "" {
			contentMap[key] = doc.Content
		}
		sourceMap[key] = mergeSource(sourceMap[key], doc.Source)
	}

	for rank, doc := range keywordDocs {
		key := normalizeKey(doc.Content)
		scores[key] += 1.0 / float64(k+rank+1)
		if contentMap[key] == "" {
			contentMap[key] = doc.Content
		}
		sourceMap[key] = mergeSource(sourceMap[key], doc.Source)
	}

	type scoredDoc struct {
		key     string
		score   float64
		content string
		source  string
	}

	var sorted []scoredDoc
	for key, score := range scores {
		sorted = append(sorted, scoredDoc{
			key:     key,
			score:   score,
			content: contentMap[key],
			source:  sourceMap[key],
		})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	result := make([]Document, 0, len(sorted))
	for _, s := range sorted {
		result = append(result, Document{
			Content: s.content,
			Score:   s.score,
			Source:  s.source,
		})
	}

	return result
}

// LLMRerank re-ranks documents using an LLM to judge relevance to the query.
func LLMRerank(ctx context.Context, chatModel llm.ChatModel, query string, docs []Document, topK int) ([]Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	if topK <= 0 {
		topK = 5
	}

	// Build a prompt to ask the LLM to rank by relevance
	candidates := buildCandidatesText(docs)
	prompt := fmt.Sprintf(`You are a search relevance judge. Given a query and a list of candidate documents, rank them by relevance.

Query: %s

Candidate Documents:
%s

Rank the documents by relevance to the query. Output ONLY a JSON array of indices (0-based) in order of relevance, most relevant first.

Example: [3, 1, 0, 2, 4]

Respond with ONLY the JSON array, no other text.`, query, candidates)

	resp, err := chatModel.Chat(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		// Fallback: return top-K by existing score
		if len(docs) > topK {
			docs = docs[:topK]
		}
		return docs, nil
	}

	// Parse the ranked order
	indices := parseRankOrder(resp.Content)
	if len(indices) == 0 {
		if len(docs) > topK {
			docs = docs[:topK]
		}
		return docs, nil
	}

	// Reorder docs by LLM ranking
	reordered := make([]Document, 0, topK)
	seen := make(map[int]bool)
	for _, idx := range indices {
		if idx >= 0 && idx < len(docs) && !seen[idx] {
			reordered = append(reordered, docs[idx])
			seen[idx] = true
			if len(reordered) >= topK {
				break
			}
		}
	}

	// Add any remaining docs that weren't ranked
	for i, doc := range docs {
		if !seen[i] && len(reordered) < topK {
			reordered = append(reordered, doc)
		}
	}

	return reordered, nil
}

// HybridSearch runs vector + keyword search, fuses with RRF, and optionally re-ranks with LLM.
type HybridSearcher struct {
	vectorRetriever VectorRetriever
	keywordSearcher KeywordSearcher
	chatModel       llm.ChatModel
	rrfK            int
	useRerank       bool
}

// VectorRetriever wraps vector search capability.
type VectorRetriever interface {
	Search(ctx context.Context, query []float32, topK int, filters map[string]string) ([]Document, error)
}

// KeywordSearcher wraps keyword search capability.
type KeywordSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]Document, error)
}

// NewHybridSearcher creates a new hybrid searcher.
func NewHybridSearcher(vr VectorRetriever, ks KeywordSearcher, chatModel llm.ChatModel, rrfK int, useRerank bool) *HybridSearcher {
	return &HybridSearcher{
		vectorRetriever: vr,
		keywordSearcher: ks,
		chatModel:       chatModel,
		rrfK:            rrfK,
		useRerank:       useRerank,
	}
}

// Search performs hybrid search: vector + keyword → RRF fusion → optional LLM rerank → top-K.
func (h *HybridSearcher) Search(ctx context.Context, query string, embedding []float32, topK, finalK int) ([]Document, error) {
	var vectorDocs, keywordDocs []Document

	// Vector search
	if h.vectorRetriever != nil && embedding != nil {
		vd, err := h.vectorRetriever.Search(ctx, embedding, topK, nil)
		if err == nil {
			for i := range vd {
				vd[i].Source = "vector"
			}
			vectorDocs = vd
		}
	}

	// Keyword search
	if h.keywordSearcher != nil {
		kd, err := h.keywordSearcher.Search(ctx, query, topK)
		if err == nil {
			for i := range kd {
				kd[i].Source = "keyword"
			}
			keywordDocs = kd
		}
	}

	// RRF fusion
	fused := RRFFusion(vectorDocs, keywordDocs, h.rrfK)
	if len(fused) == 0 {
		return nil, fmt.Errorf("no documents retrieved from either source")
	}

	// LLM Rerank
	if h.useRerank && h.chatModel != nil && len(fused) > finalK {
		reranked, err := LLMRerank(ctx, h.chatModel, query, fused, finalK)
		if err == nil {
			return reranked, nil
		}
	}

	// Truncate to finalK
	if len(fused) > finalK {
		fused = fused[:finalK]
	}

	return fused, nil
}

func normalizeKey(content string) string {
	// Trim to first 200 chars for key matching
	if len(content) > 200 {
		return strings.TrimSpace(content[:200])
	}
	return strings.TrimSpace(content)
}

func mergeSource(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if strings.Contains(a, b) {
		return a
	}
	return a + "+" + b
}

func buildCandidatesText(docs []Document) string {
	var sb strings.Builder
	for i, doc := range docs {
		// Truncate content for LLM context
		content := doc.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] (source: %s) %s\n\n", i, doc.Source, content))
	}
	return sb.String()
}

func parseRankOrder(response string) []int {
	// Trim potential markdown wrapping
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")

	// Extract JSON array
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start == -1 || end == -1 || end <= start {
		return nil
	}

	arrayStr := response[start : end+1]
	var indices []int
	parts := strings.Split(arrayStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "[] ")
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
			indices = append(indices, idx)
		}
	}
	return indices
}
