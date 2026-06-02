package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/mcp"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// ReviewPlanningNode generates a personalized review plan with resource recommendations.
type ReviewPlanningNode struct {
	chatModel llm.ChatModel
	githubMCP *mcp.GitHubMCP
	webMCP    *mcp.WebSearchMCP
}

// NewReviewPlanningNode creates a new review planning node.
func NewReviewPlanningNode(chatModel llm.ChatModel, githubMCP *mcp.GitHubMCP, webMCP *mcp.WebSearchMCP) *ReviewPlanningNode {
	return &ReviewPlanningNode{
		chatModel: chatModel,
		githubMCP: githubMCP,
		webMCP:    webMCP,
	}
}

// Execute generates a review plan and populates the state.
func (n *ReviewPlanningNode) Execute(ctx context.Context, state *InterviewState) error {
	if len(state.Evaluations) == 0 {
		return fmt.Errorf("no evaluations to base review plan on")
	}

	// Calculate overall score and weak areas
	report := buildReport(state)
	state.FinalReport = report

	// Search for learning resources
	resources := n.searchResources(ctx, report.WeakAreas)

	resourcesJSON, _ := json.Marshal(resources)
	evalJSON, _ := json.Marshal(state.Evaluations)

	prompt := fmt.Sprintf(prompts.ReviewPlanSystemPrompt,
		report.OverallScore,
		report.DimensionScore,
		report.WeakAreas,
		string(evalJSON),
		string(resourcesJSON),
		state.SessionID,
	)

	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please create a personalized review plan."},
	})
	if err != nil {
		return fmt.Errorf("review planning failed: %w", err)
	}

	var plan model.ReviewPlan
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &plan); err != nil {
		return fmt.Errorf("failed to parse review plan: %w", err)
	}

	state.ReviewPlan = &plan
	state.Phase = model.PhaseCompleted
	return nil
}

// searchResources finds learning resources for weak areas.
func (n *ReviewPlanningNode) searchResources(ctx context.Context, weakAreas []string) []model.Resource {
	var resources []model.Resource

	for _, area := range weakAreas {
		if n.githubMCP != nil {
			repos, err := n.githubMCP.SearchRepositories(ctx, area+" learning", 2)
			if err == nil {
				for _, repo := range repos {
					resources = append(resources, model.Resource{
						Title:       repo.FullName,
						URL:         repo.URL,
						Type:        "repo",
						Description: repo.Description,
						Source:      "github",
					})
				}
			}
		}
	}

	return resources
}

// buildReport creates a summary report from all evaluations.
func buildReport(state *InterviewState) *model.Report {
	dimensionTotals := make(map[string]float64)
	dimensionCounts := make(map[string]int)
	var totalScore float64

	for _, eval := range state.Evaluations {
		totalScore += eval.TotalScore
		for _, dim := range eval.Dimensions {
			dimensionTotals[dim.Name] += dim.Score
			dimensionCounts[dim.Name]++
		}
	}

	dimensionAvg := make(map[string]float64)
	for name, total := range dimensionTotals {
		if dimensionCounts[name] > 0 {
			dimensionAvg[name] = total / float64(dimensionCounts[name])
		}
	}

	overallScore := float64(0)
	if len(state.Evaluations) > 0 {
		overallScore = totalScore / float64(len(state.Evaluations))
	}

	// Identify weak areas (dimensions scoring below 6)
	var weakAreas []string
	for name, avg := range dimensionAvg {
		if avg < 6.0 {
			weakAreas = append(weakAreas, name)
		}
	}

	// Collect highlights
	var highlights []string
	for _, eval := range state.Evaluations {
		if eval.TotalScore >= 8.0 {
			highlights = append(highlights, fmt.Sprintf("Q%s: scored %.1f - %s", eval.QuestionID, eval.TotalScore, eval.Feedback))
		}
	}

	return &model.Report{
		SessionID:      state.SessionID,
		OverallScore:   overallScore,
		DimensionScore: dimensionAvg,
		Evaluations:    state.Evaluations,
		Highlights:     highlights,
		WeakAreas:      weakAreas,
		Summary:        fmt.Sprintf("Interview completed with %.1f questions on average. Overall score: %.2f. %d areas need improvement.", float64(len(state.Evaluations)), overallScore, len(weakAreas)),
	}
}
