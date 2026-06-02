package mcp

import (
	"context"
	"fmt"
)

// WebSearchMCP provides web search MCP tool operations.
type WebSearchMCP struct {
	manager *Manager
}

// WebSearchResult represents a web search result.
type WebSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// NewWebSearchMCP creates a new web search MCP wrapper.
func NewWebSearchMCP(manager *Manager) *WebSearchMCP {
	return &WebSearchMCP{manager: manager}
}

// Search searches the web via MCP.
func (w *WebSearchMCP) Search(ctx context.Context, query string, limit int) ([]WebSearchResult, error) {
	result, err := w.manager.CallTool(ctx, "web_search", "search", map[string]any{
		"query": query,
		"count": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search web: %w", err)
	}

	return []WebSearchResult{{
		Title:   query,
		Snippet: result.Content,
	}}, nil
}
