package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/difficulty"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// QuestionPlanningNode plans the interview questions based on JD, resume match, and RAG.
type QuestionPlanningNode struct {
	chatModel       einomodel.ToolCallingChatModel
	hybridRetriever retriever.Retriever
	embedder        embedding.Embedder
}

// NewQuestionPlanningNode creates a new question planning node.
func NewQuestionPlanningNode(chatModel einomodel.ToolCallingChatModel, hybridRetriever retriever.Retriever, emb embedding.Embedder) *QuestionPlanningNode {
	return &QuestionPlanningNode{
		chatModel:       chatModel,
		hybridRetriever: hybridRetriever,
		embedder:        emb,
	}
}

// Execute plans questions and populates the state.
func (n *QuestionPlanningNode) Execute(ctx context.Context, state *InterviewState) error {
	if state.JDAnalysis == nil {
		return fmt.Errorf("JD analysis must be completed before question planning")
	}

	strengths := []string{}
	gaps := []string{}
	if state.ResumeMatch != nil {
		strengths = state.ResumeMatch.Strengths
		gaps = state.ResumeMatch.Gaps
	}

	query := buildSearchQueryFromJD(state.JDAnalysis)
	if state.ResumeMatch != nil {
		query = buildSearchQuery(state.JDAnalysis, state.ResumeMatch)
	}

	docs, err := n.retrieveQuestions(ctx, query, 10, 5)
	if err != nil {
		docs = ""
	} else {
		state.RAGDocuments = docs
	}

	diffDist := "easy: 30%%, medium: 50%%, hard: 20%%"
	if state.Difficulty != nil {
		dist := state.Difficulty.GetDifficultyDistribution(10)
		diffDist = fmt.Sprintf("easy: %d, medium: %d, hard: %d (current level: %s)",
			dist[difficulty.LevelEasy], dist[difficulty.LevelMedium], dist[difficulty.LevelHard],
			state.Difficulty.CurrentLevel)
	}

	jdJSON, _ := json.Marshal(state.JDAnalysis)
	prompt := safeFmt(prompts.QuestionPlanSystemPrompt,
		string(jdJSON), strengths, gaps, diffDist, docs)

	resp, err := n.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompt),
		schema.UserMessage("Please create a question plan for this candidate interview."),
	})
	if err != nil {
		return fmt.Errorf("question planning failed: %w", err)
	}

	var plan model.QuestionPlan
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &plan); err != nil {
		return fmt.Errorf("failed to parse question plan: %w", err)
	}

	state.QuestionPlan = &plan
	state.QuestionQueue = plan.Questions
	state.CurrentQIndex = 0
	state.Phase = model.PhaseInterviewing
	return nil
}

func (n *QuestionPlanningNode) retrieveQuestions(ctx context.Context, query string, topK, finalK int) (string, error) {
	if n.hybridRetriever == nil {
		return "", fmt.Errorf("hybrid retriever not available")
	}

	docs, err := n.hybridRetriever.Retrieve(ctx, query, retriever.WithTopK(finalK))
	if err != nil {
		return "", err
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("no questions retrieved")
	}

	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	return strings.Join(contents, "\n---\n"), nil
}

func buildSearchQuery(jd *model.JDAnalysis, match *model.ResumeMatch) string {
	parts := append(jd.TechStack, jd.CoreSkills...)
	parts = append(parts, match.Gaps...)
	return strings.Join(parts, " ")
}

func buildSearchQueryFromJD(jd *model.JDAnalysis) string {
	parts := append(jd.TechStack, jd.CoreSkills...)
	return strings.Join(parts, " ")
}
