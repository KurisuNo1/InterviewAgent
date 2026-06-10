package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// WebSearchMCP provides web search MCP tool operations.
// All calls go through Eino's InvokableTool, ensuring unified callbacks and observability.
type WebSearchMCP struct {
	bridge *EinoBridge
}

// WebSearchResult represents a web search result.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// braveSearchResult is the raw response from the Brave Search MCP server.
type braveSearchResult struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	} `json:"results"`
}

// NewWebSearchMCP creates a new web search MCP wrapper that uses the Eino bridge.
func NewWebSearchMCP(bridge *EinoBridge) *WebSearchMCP {
	return &WebSearchMCP{bridge: bridge}
}

// Search searches the web via the Eino MCP tool.
func (w *WebSearchMCP) Search(ctx context.Context, query string, count int) ([]WebSearchResult, error) {
	argsJSON, _ := json.Marshal(map[string]any{
		"query": query,
		"count": count,
	})

	// Find the search tool from the web_search server (tool name varies per MCP server)
	_, toolName, err := w.bridge.FindToolByServer("web_search", "search", "web_search", "ddg_search")
	if err != nil {
		return nil, fmt.Errorf("failed to find web search tool: %w", err)
	}

	result, err := w.bridge.CallTool(ctx, toolName, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to search web: %w", err)
	}

	// Try to parse structured results
	var braveResp braveSearchResult
	if err := json.Unmarshal([]byte(result), &braveResp); err == nil && len(braveResp.Results) > 0 {
		results := make([]WebSearchResult, 0, len(braveResp.Results))
		for _, r := range braveResp.Results {
			if count <= 0 {
				break
			}
			results = append(results, WebSearchResult{
				Title:   r.Title,
				URL:     r.URL,
				Snippet: r.Description,
			})
			count--
		}
		return results, nil
	}

	// Fallback
	return []WebSearchResult{{
		Title:   query,
		Snippet: result,
	}}, nil
}

// FormatWebResults formats search results as a string for inclusion in LLM prompts.
func FormatWebResults(results []WebSearchResult) string {
	if len(results) == 0 {
		return ""
	}
	text := ""
	for i, r := range results {
		text += fmt.Sprintf("%d. **%s**\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return text
}
