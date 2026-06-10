package rag

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/flow/retriever/router"
	"github.com/cloudwego/eino/schema"
)

// LLMRerank re-ranks documents using an LLM to judge relevance to the query.
func LLMRerank(ctx context.Context, chatModel einomodel.ToolCallingChatModel, query string, docs []*schema.Document, topK int) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	if topK <= 0 {
		topK = 5
	}

	candidates := buildCandidatesText(docs)
	prompt := fmt.Sprintf(`You are a search relevance judge. Given a query and a list of candidate documents, rank them by relevance.

Query: %s

Candidate Documents:
%s

Rank the documents by relevance to the query. Output ONLY a JSON array of indices (0-based) in order of relevance, most relevant first.

Example: [3, 1, 0, 2, 4]

Respond with ONLY the JSON array, no other text.`, query, candidates)

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		if len(docs) > topK {
			docs = docs[:topK]
		}
		return docs, nil
	}

	indices := parseRankOrder(resp.Content)
	if len(indices) == 0 {
		if len(docs) > topK {
			docs = docs[:topK]
		}
		return docs, nil
	}

	reordered := make([]*schema.Document, 0, topK)
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
	for i, doc := range docs {
		if !seen[i] && len(reordered) < topK {
			reordered = append(reordered, doc)
		}
	}

	return reordered, nil
}

// NewHybridRetriever creates a hybrid retriever combining vector + keyword search.
// Uses Eino's router when both are available; falls back to a single retriever otherwise.
func NewHybridRetriever(ctx context.Context, vectorRetriever, keywordRetriever retriever.Retriever) (retriever.Retriever, error) {
	if vectorRetriever == nil && keywordRetriever == nil {
		return nil, fmt.Errorf("at least one retriever is required")
	}

	// Single retriever: return directly (avoids Eino router overhead)
	if vectorRetriever == nil {
		return keywordRetriever, nil
	}
	if keywordRetriever == nil {
		return vectorRetriever, nil
	}

	// Both available: use Eino router with RRF fusion
	r, err := router.NewRetriever(ctx, &router.Config{
		Retrievers: map[string]retriever.Retriever{
			"vector":  vectorRetriever,
			"keyword": keywordRetriever,
		},
	})
	if err != nil {
		return nil, err
	}
	return &safeRetriever{inner: r}, nil
}

// safeRetriever wraps a retriever with panic recovery.
type safeRetriever struct{ inner retriever.Retriever }

func (s *safeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) (docs []*schema.Document, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("retriever panic: %v", r)
		}
	}()
	return s.inner.Retrieve(ctx, query, opts...)
}

// FormatDocuments joins schema.Document contents into a single string for LLM prompts.
func FormatDocuments(docs []*schema.Document) string {
	if len(docs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, doc := range docs {
		content := doc.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("--- Document %d ---\n%s\n\n", i+1, content))
	}
	return sb.String()
}

func buildCandidatesText(docs []*schema.Document) string {
	var sb strings.Builder
	for i, doc := range docs {
		content := doc.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		source := ""
		if s, ok := doc.MetaData["source"]; ok {
			source = fmt.Sprintf("%v", s)
		}
		sb.WriteString(fmt.Sprintf("[%d] (source: %s) %s\n\n", i, source, content))
	}
	return sb.String()
}

func parseRankOrder(response string) []int {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")

	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start == -1 || end == -1 || end <= start {
		return nil
	}

	arrayStr := response[start : end+1]
	var indices []int
	for _, part := range strings.Split(arrayStr, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "[] ")
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
			indices = append(indices, idx)
		}
	}
	return indices
}
