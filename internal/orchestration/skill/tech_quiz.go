package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/google/uuid"
)

// TechQuizSkill provides rapid-fire technical knowledge quizzes.
type TechQuizSkill struct {
	chatModel llm.ChatModel
}

// NewTechQuizSkill creates a new technical quiz skill.
func NewTechQuizSkill(chatModel llm.ChatModel) *TechQuizSkill {
	return &TechQuizSkill{chatModel: chatModel}
}

func (s *TechQuizSkill) Name() string { return "tech_quiz" }
func (s *TechQuizSkill) Description() string {
	return "Rapid-fire technical knowledge quiz on specific tech stacks"
}
func (s *TechQuizSkill) CanHandle(subIntent string) bool {
	return subIntent == "tech_quiz" || subIntent == "quiz" || subIntent == "knowledge"
}

func (s *TechQuizSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data: map[string]any{
			"score": 0,
			"total": 0,
		},
	}, nil
}

func (s *TechQuizSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	totalQuestions := 10
	if state.Round > totalQuestions {
		score, _ := state.Data["score"].(int)
		return &SkillResponse{
			Message:    fmt.Sprintf("Quiz complete! Your score: %d/%d", score, totalQuestions),
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`You are a technical quiz master. You are testing the user's knowledge.

Current round: %d of %d

Previous exchange:
%s

The user said: "%s"

Rules:
- Ask one multiple-choice or short-answer technical question at a time
- After the user answers, tell them if they were correct and explain the right answer
- Keep track of score
- Cover a variety of topics relevant to the user's tech stack
- Make questions progressively harder

For round 1: start with an easy warm-up question.
For subsequent rounds: evaluate the previous answer, then ask the next question.

Your response should be in this format:
[CORRECT] or [INCORRECT] - explanation
Score: X/%d

Next question: [your question here]`,
		state.Round, totalQuestions,
		strings.Join(state.History, "\n"),
		input,
		totalQuestions,
	)

	resp, err := s.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("tech quiz skill error: %w", err)
	}

	// Simple score tracking
	response := resp.Content
	total, _ := state.Data["total"].(int)
	total++
	score, _ := state.Data["score"].(int)
	if strings.Contains(response, "[CORRECT]") {
		score++
	}
	state.Data["total"] = total
	state.Data["score"] = score

	return &SkillResponse{
		Message:    response,
		IsComplete: false,
		NextPrompt: "Type your answer (A, B, C, D or short answer).",
	}, nil
}
