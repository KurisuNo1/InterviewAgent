package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitHubMCP provides GitHub-specific MCP tool operations.
type GitHubMCP struct {
	manager *Manager
}

// GitHubRepo represents a GitHub repository search result.
type GitHubRepo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	URL         string `json:"html_url"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
}

// NewGitHubMCP creates a new GitHub MCP wrapper.
func NewGitHubMCP(manager *Manager) *GitHubMCP {
	return &GitHubMCP{manager: manager}
}

// SearchRepositories searches GitHub repositories for learning resources.
func (g *GitHubMCP) SearchRepositories(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	result, err := g.manager.CallTool(ctx, "github", "search_repositories", map[string]any{
		"query": query,
		"limit": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search GitHub repos: %w", err)
	}

	var repos []GitHubRepo
	if err := json.Unmarshal([]byte(result.Content), &repos); err != nil {
		// If unmarshal fails, return the raw content
		return []GitHubRepo{{
			FullName:    query,
			Description: result.Content,
			URL:         "",
		}}, nil
	}
	return repos, nil
}
