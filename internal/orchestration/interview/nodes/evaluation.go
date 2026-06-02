package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// EvaluationNode scores a candidate's answer to a question.
type EvaluationNode struct {
	chatModel llm.ChatModel
}

// NewEvaluationNode creates a new evaluation node.
func NewEvaluationNode(chatModel llm.ChatModel) *EvaluationNode {
	return &EvaluationNode{chatModel: chatModel}
}

// Execute evaluates the last answer in the conversation.
func (n *EvaluationNode) Execute(ctx context.Context, state *InterviewState) error {
	if state.CurrentQuestion == nil {
		return fmt.Errorf("no current question to evaluate")
	}

	// Extract the last answer from chat history
	answer := extractLastAnswer(state.ChatHistory)
	if answer == "" {
		return fmt.Errorf("no answer found to evaluate")
	}

	// Build follow-up exchange context
	followUps := extractFollowUps(state.ChatHistory)

	prompt := fmt.Sprintf(prompts.EvaluationSystemPrompt,
		state.CurrentQuestion.Content,
		state.CurrentQuestion.ScoringPoints,
		answer,
		followUps,
		state.CurrentQuestion.ID,
	)

	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please evaluate this answer."},
	})
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	var eval model.Evaluation
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &eval); err != nil {
		return fmt.Errorf("failed to parse evaluation: %w", err)
	}

	// Record the answer
	state.Answers = append(state.Answers, model.Answer{
		QuestionID: state.CurrentQuestion.ID,
		Content:    answer,
	})
	state.Evaluations = append(state.Evaluations, eval)

	return nil
}

func extractLastAnswer(history []model.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == model.RoleUser {
			return history[i].Content
		}
	}
	return ""
}

func extractFollowUps(history []model.Message) string {
	var exchange []string
	start := len(history) - 6 // last 3 turns (user + assistant)
	if start < 0 {
		start = 0
	}
	for _, msg := range history[start:] {
		exchange = append(exchange, fmt.Sprintf("[%s]: %s", msg.Role, msg.Content))
	}
	return strings.Join(exchange, "\n")
}
