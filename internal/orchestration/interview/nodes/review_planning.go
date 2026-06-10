package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/mcp"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// ReviewPlanningNode generates a personalized review plan with resource recommendations.
// When an agent is configured, it autonomously searches for and evaluates learning resources.
type ReviewPlanningNode struct {
	chatModel einomodel.ToolCallingChatModel
	agent     *react.Agent // optional: ReAct agent for autonomous resource search
	githubMCP *mcp.GitHubMCP
	webMCP    *mcp.WebSearchMCP
}

// NewReviewPlanningNode creates a new review planning node.
func NewReviewPlanningNode(chatModel einomodel.ToolCallingChatModel, githubMCP *mcp.GitHubMCP, webMCP *mcp.WebSearchMCP, agent *react.Agent) *ReviewPlanningNode {
	return &ReviewPlanningNode{
		chatModel: chatModel,
		githubMCP: githubMCP,
		webMCP:    webMCP,
		agent:     agent,
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

	// Search for learning resources — use agent if available, otherwise manual MCP search
	var resources []model.Resource
	if n.agent != nil {
		resources = n.searchWithAgent(ctx, report.WeakAreas)
	} else {
		resources = n.searchResources(ctx, report.WeakAreas)
	}

	resourcesJSON, _ := json.Marshal(resources)
	evalJSON, _ := json.Marshal(state.Evaluations)

	prompt := safeFmt(prompts.ReviewPlanSystemPrompt,
		report.OverallScore,
		report.DimensionScore,
		report.WeakAreas,
		string(evalJSON),
		string(resourcesJSON),
		state.SessionID,
	)

	resp, err := n.callLLM(ctx, prompt)
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

// callLLM routes through the ReAct Agent if available, otherwise falls back to direct LLM.
func (n *ReviewPlanningNode) callLLM(ctx context.Context, systemPrompt string) (*schema.Message, error) {
	msgs := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("Please create a personalized review plan in JSON format."),
	}
	if n.agent != nil {
		return n.agent.Generate(ctx, msgs)
	}
	return n.chatModel.Generate(ctx, msgs)
}

// searchWithAgent uses the ReAct agent to autonomously search for and evaluate learning resources.
func (n *ReviewPlanningNode) searchWithAgent(ctx context.Context, weakAreas []string) []model.Resource {
	var allResources []model.Resource

	for _, area := range weakAreas {
		searchPrompt := fmt.Sprintf(`## Role
You are a learning resource curator. Search for high-quality learning materials about "%s".

## Task
1. Search GitHub for relevant repositories about "%s"
2. Search the web for tutorials, articles, or courses about "%s"
3. For each resource found, evaluate its quality (is it current? from a reputable source? practically useful?)
4. Return up to 3 best resources

## Output Format
For each resource, output one line in this format:
TYPE | TITLE | URL | DESCRIPTION
Where TYPE is "repo" or "article".`,
			area, area, area)

		msgs := []*schema.Message{
			schema.SystemMessage(searchPrompt),
			schema.UserMessage(fmt.Sprintf("Find the best learning resources for: %s", area)),
		}

		resp, err := n.agent.Generate(ctx, msgs)
		if err != nil {
			continue // best-effort; skip this area on failure
		}

		// Parse resources from agent response
		for _, line := range strings.Split(resp.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !strings.Contains(line, "|") {
				continue
			}
			parts := strings.SplitN(line, "|", 4)
			if len(parts) >= 4 {
				allResources = append(allResources, model.Resource{
					Type:        strings.TrimSpace(parts[0]),
					Title:       strings.TrimSpace(parts[1]),
					URL:         strings.TrimSpace(parts[2]),
					Description: strings.TrimSpace(parts[3]),
					Source:      "agent_search",
				})
			}
		}
	}

	return allResources
}

// searchResources finds learning resources for weak areas (manual MCP fallback).
func (n *ReviewPlanningNode) searchResources(ctx context.Context, weakAreas []string) []model.Resource {
	var resources []model.Resource

	for _, area := range weakAreas {
		// Search GitHub for relevant repositories
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

		// Search web for articles and tutorials
		if n.webMCP != nil {
			webResults, err := n.webMCP.Search(ctx, area+" tutorial guide best practices", 2)
			if err == nil {
				for _, wr := range webResults {
					resources = append(resources, model.Resource{
						Title:       wr.Title,
						URL:         wr.URL,
						Type:        "article",
						Description: wr.Snippet,
						Source:      "web_search",
					})
				}
			}
		}
	}

	return resources
}

// buildReport creates a comprehensive guidance-oriented report from all evaluations.
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
	score100 := overallScore * 10 // normalize to 0-100 scale
	grade := scoreToGrade(score100)

	// Build strengths from actual praise, not just score thresholds
	strengths := buildStrengths(state.Evaluations)

	// Build improvement areas from actual issues
	improvements := buildImprovements(state.Evaluations)

	// Build per-question detailed reviews
	questionReviews := buildQuestionReviews(state, state.Evaluations)

	// Build dimension commentary
	dimCommentary := buildDimCommentary(dimensionAvg)

	// Build executive summary
	summary := buildExecutiveSummary(score100, grade, len(state.Evaluations), strengths, improvements)

	return &model.Report{
		SessionID:       state.SessionID,
		OverallScore:    overallScore,
		Score100:        score100,
		Grade:           grade,
		DimensionScore:  dimensionAvg,
		Evaluations:     state.Evaluations,
		Highlights:      strengths,
		WeakAreas:       improvements,
		Summary:         summary,
		QuestionReviews: questionReviews,
		OverallAdvice:   dimCommentary,
	}
}

func scoreToGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B+"
	case score >= 70:
		return "B"
	case score >= 60:
		return "C"
	default:
		return "D"
	}
}

func buildStrengths(evaluations []model.Evaluation) []string {
	var strengths []string
	for _, eval := range evaluations {
		if eval.Praise != "" && eval.Praise != "本次回答暂无明显亮点" &&
			eval.TotalScore >= 6.0 {
			strengths = append(strengths, eval.Praise)
		}
	}
	// Deduplicate and limit
	return uniqueFirstN(strengths, 5)
}

func buildImprovements(evaluations []model.Evaluation) []string {
	var improvements []string
	for _, eval := range evaluations {
		if eval.Improvement != "" {
			improvements = append(improvements, eval.Improvement)
		}
	}
	return uniqueFirstN(improvements, 5)
}

func buildQuestionReviews(state *InterviewState, evaluations []model.Evaluation) []string {
	var reviews []string
	for i, eval := range evaluations {
		// Find the question text
		qText := fmt.Sprintf("Q%d", i+1)
		for _, q := range state.QuestionQueue {
			if q.ID == eval.QuestionID {
				qText = q.Content
				break
			}
		}
		review := fmt.Sprintf("【第%d题】(%.0f分)\n题目：%s", i+1, eval.TotalScore*10, qText)
		if eval.Praise != "" && eval.Praise != "本次回答暂无明显亮点" {
			review += fmt.Sprintf("\n✅ 亮点：%s", eval.Praise)
		}
		if eval.Issues != "" && eval.Issues != "本次回答无明显问题" {
			review += fmt.Sprintf("\n⚠️ 不足：%s", eval.Issues)
		}
		if eval.Improvement != "" {
			review += fmt.Sprintf("\n💡 建议：%s", eval.Improvement)
		}
		if eval.KeyTakeaway != "" {
			review += fmt.Sprintf("\n📌 要点：%s", eval.KeyTakeaway)
		}
		reviews = append(reviews, review)
	}
	return reviews
}

func buildDimCommentary(dimAvg map[string]float64) string {
	dimNames := map[string]string{
		"technical_accuracy": "基础知识",
		"answer_depth":       "回答深度",
		"communication":      "沟通表达",
		"project_experience": "项目经验",
	}
	var sb strings.Builder
	for name, score := range dimAvg {
		label := dimNames[name]
		if label == "" {
			label = name
		}
		level := "优秀"
		if score < 5 {
			level = "需重点加强"
		} else if score < 7 {
			level = "良好，仍有提升空间"
		} else if score < 8.5 {
			level = "扎实"
		}
		sb.WriteString(fmt.Sprintf("%s: %.0f分(%s)\n", label, score*10, level))
	}
	return sb.String()
}

func buildExecutiveSummary(score float64, grade string, totalQ int, strengths, improvements []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("综合得分：%.1f/100 (%s级)\n", score, grade))
	sb.WriteString(fmt.Sprintf("共完成 %d 道题目\n\n", totalQ))
	if len(strengths) > 0 {
		sb.WriteString("核心优势：\n")
		for _, s := range strengths {
			sb.WriteString(fmt.Sprintf("• %s\n", s))
		}
	}
	if len(improvements) > 0 {
		sb.WriteString("\n重点提升方向：\n")
		for _, imp := range improvements {
			sb.WriteString(fmt.Sprintf("• %s\n", imp))
		}
	}
	return sb.String()
}

func uniqueFirstN(items []string, n int) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if len(item) > 150 {
			item = item[:150]
		}
		if !seen[item] && item != "" {
			seen[item] = true
			result = append(result, item)
			if len(result) >= n {
				break
			}
		}
	}
	return result
}

