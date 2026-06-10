package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitHubMCP provides GitHub-specific MCP tool operations.
// All calls go through Eino's InvokableTool, ensuring unified callbacks and observability.
type GitHubMCP struct {
	bridge *EinoBridge
}

// GitHubRepo represents a GitHub repository search result.
type GitHubRepo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	URL         string `json:"html_url"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
}

// NewGitHubMCP creates a new GitHub MCP wrapper that uses the Eino bridge.
func NewGitHubMCP(bridge *EinoBridge) *GitHubMCP {
	return &GitHubMCP{bridge: bridge}
}

// FormatGitHubResults formats GitHub repo results as a string for LLM prompts.
func FormatGitHubResults(repos []GitHubRepo) string {
	if len(repos) == 0 {
		return ""
	}
	text := ""
	for i, r := range repos {
		text += fmt.Sprintf("%d. **%s** (★%d, %s)\n   %s\n   %s\n\n", i+1, r.FullName, r.Stars, r.Language, r.Description, r.URL)
	}
	return text
}

// SearchRepositories searches GitHub repositories via the Eino MCP tool.
func (g *GitHubMCP) SearchRepositories(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	argsJSON, _ := json.Marshal(map[string]any{
		"query": query,
		"limit": limit,
	})

	result, err := g.bridge.CallTool(ctx, "github_search_repositories", string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to search GitHub repos: %w", err)
	}

	var repos []GitHubRepo
	if err := json.Unmarshal([]byte(result), &repos); err != nil {
		return []GitHubRepo{{
			FullName:    query,
			Description: result,
		}}, nil
	}
	return repos, nil
}
