package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/difficulty"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/rag"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// Embedder is a simplified embedding interface for the question planning node.
type Embedder interface {
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
}

// QuestionPlanningNode plans the interview questions based on JD, resume match, and RAG.
type QuestionPlanningNode struct {
	chatModel      llm.ChatModel
	hybridSearcher *rag.HybridSearcher
	embedder       Embedder
}

// NewQuestionPlanningNode creates a new question planning node.
func NewQuestionPlanningNode(chatModel llm.ChatModel, hs *rag.HybridSearcher, emb Embedder) *QuestionPlanningNode {
	return &QuestionPlanningNode{
		chatModel:      chatModel,
		hybridSearcher: hs,
		embedder:       emb,
	}
}

// Execute plans questions and populates the state.
func (n *QuestionPlanningNode) Execute(ctx context.Context, state *InterviewState) error {
	if state.JDAnalysis == nil || state.ResumeMatch == nil {
		return fmt.Errorf("JD analysis and resume matching must be completed before question planning")
	}

	// Build search query from JD and gaps
	query := buildSearchQuery(state.JDAnalysis, state.ResumeMatch)

	// Retrieve relevant questions using hybrid search (vector + keyword → RRF → LLM rerank)
	docs, err := n.retrieveQuestions(ctx, query, 10, 5)
	if err != nil {
		// If retrieval fails, fall back to LLM-only generation
		docs = ""
	}

	diffDist := "easy: 30%%, medium: 50%%, hard: 20%%"
	if state.Difficulty != nil {
		dist := state.Difficulty.GetDifficultyDistribution(10)
		diffDist = fmt.Sprintf("easy: %d, medium: %d, hard: %d (current level: %s)",
			dist[difficulty.LevelEasy], dist[difficulty.LevelMedium], dist[difficulty.LevelHard],
			state.Difficulty.CurrentLevel)
	}

	jdJSON, _ := json.Marshal(state.JDAnalysis)
	prompt := fmt.Sprintf(prompts.QuestionPlanSystemPrompt,
		string(jdJSON), state.ResumeMatch.Strengths, state.ResumeMatch.Gaps, diffDist, docs)

	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please create a question plan for this candidate interview."},
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

// retrieveQuestions fetches relevant questions using RRF fusion + LLM rerank hybrid search.
func (n *QuestionPlanningNode) retrieveQuestions(ctx context.Context, query string, topK, finalK int) (string, error) {
	if n.hybridSearcher == nil {
		return "", fmt.Errorf("hybrid searcher not available")
	}

	var embedding []float32
	if n.embedder != nil {
		vec, err := n.embedder.EmbedSingle(ctx, query)
		if err == nil {
			embedding = vec
		}
	}

	docs, err := n.hybridSearcher.Search(ctx, query, embedding, topK, finalK)
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
