package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// InterviewerNode handles the interview loop: asking questions, processing answers, and deciding follow-ups.
type InterviewerNode struct {
	chatModel    llm.ChatModel
	maxFollowUps int
}

// NewInterviewerNode creates a new interviewer node.
func NewInterviewerNode(chatModel llm.ChatModel, maxFollowUps int) *InterviewerNode {
	if maxFollowUps <= 0 {
		maxFollowUps = 3
	}
	return &InterviewerNode{
		chatModel:    chatModel,
		maxFollowUps: maxFollowUps,
	}
}

// AskQuestion generates and sends the current question.
func (n *InterviewerNode) AskQuestion(ctx context.Context, state *InterviewState) (string, error) {
	if state.CurrentQIndex >= len(state.QuestionQueue) {
		state.Phase = model.PhaseCompleted
		return "", fmt.Errorf("all questions have been asked")
	}

	question := state.QuestionQueue[state.CurrentQIndex]
	state.CurrentQuestion = &question
	state.CurrentFollowUp = 0

	// Build conversation history
	history := buildHistory(state.ChatHistory)

	diffLevel := "medium"
	if state.Difficulty != nil {
		diffLevel = string(state.Difficulty.CurrentLevel)
	}

	position := "unknown position"
	techStack := []string{}
	if state.JDAnalysis != nil {
		position = state.JDAnalysis.Position
		techStack = state.JDAnalysis.TechStack
	}

	prompt := fmt.Sprintf(prompts.InterviewerSystemPrompt,
		position,
		techStack,
		state.CurrentQIndex+1,
		len(state.QuestionQueue),
		diffLevel,
		n.maxFollowUps,
		history,
		question.Content,
		"", // no answer yet
	)

	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please begin asking the current question."},
	})
	if err != nil {
		return "", fmt.Errorf("interviewer failed: %w", err)
	}

	response := resp.Content
	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleAssistant,
		Content: response,
	})

	return response, nil
}

// ProcessAnswer evaluates the user's answer and decides the next step.
func (n *InterviewerNode) ProcessAnswer(ctx context.Context, state *InterviewState, answer string) (string, string, error) {
	// Record the answer
	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleUser,
		Content: answer,
	})

	question := state.CurrentQuestion
	history := buildHistory(state.ChatHistory)

	diffLevel := "medium"
	if state.Difficulty != nil {
		diffLevel = string(state.Difficulty.CurrentLevel)
	}

	position := "unknown position"
	techStack := []string{}
	if state.JDAnalysis != nil {
		position = state.JDAnalysis.Position
		techStack = state.JDAnalysis.TechStack
	}

	qContent := ""
	if question != nil {
		qContent = question.Content
	}

	prompt := fmt.Sprintf(prompts.InterviewerSystemPrompt,
		position,
		techStack,
		state.CurrentQIndex+1,
		len(state.QuestionQueue),
		diffLevel,
		n.maxFollowUps,
		history,
		qContent,
		answer,
	)

	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Evaluate the answer and decide the next step."},
	})
	if err != nil {
		return "", "", fmt.Errorf("answer processing failed: %w", err)
	}

	response := resp.Content
	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleAssistant,
		Content: response,
	})

	// Determine action based on response
	if strings.Contains(response, "NEXT_QUESTION") {
		state.CurrentQIndex++
		state.CurrentFollowUp = 0
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.Phase = model.PhaseCompleted
			return "complete", response, nil
		}
		return "next_question", response, nil
	}

	if strings.Contains(response, "INTERVIEW_COMPLETE") {
		state.Phase = model.PhaseCompleted
		return "complete", response, nil
	}

	// It's a follow-up
	if state.CurrentFollowUp < n.maxFollowUps {
		state.CurrentFollowUp++
		return "follow_up", response, nil
	}

	// Max follow-ups reached
	state.CurrentQIndex++
	state.CurrentFollowUp = 0
	if state.CurrentQIndex >= len(state.QuestionQueue) {
		state.Phase = model.PhaseCompleted
		return "complete", response, nil
	}
	return "next_question", response, nil
}

func buildHistory(messages []model.Message) string {
	if len(messages) == 0 {
		return "(no previous conversation)"
	}

	var sb strings.Builder
	start := 0
	if len(messages) > 20 {
		start = len(messages) - 20
	}
	for _, msg := range messages[start:] {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}
