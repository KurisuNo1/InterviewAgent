package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/google/uuid"
)

// AlgorithmSkill provides LeetCode-style algorithm coding practice.
type AlgorithmSkill struct {
	chatModel llm.ChatModel
}

// NewAlgorithmSkill creates a new algorithm practice skill.
func NewAlgorithmSkill(chatModel llm.ChatModel) *AlgorithmSkill {
	return &AlgorithmSkill{chatModel: chatModel}
}

func (s *AlgorithmSkill) Name() string        { return "algorithm" }
func (s *AlgorithmSkill) Description() string { return "LeetCode-style algorithm coding practice" }
func (s *AlgorithmSkill) CanHandle(subIntent string) bool {
	return subIntent == "algorithm" || subIntent == "algo" || subIntent == "coding"
}

func (s *AlgorithmSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *AlgorithmSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 5
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Practice session complete. Review your solutions and try more problems!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`You are an algorithm coding coach. You are conducting a practice session.

Current round: %d of %d

Previous exchange:
%s

The user said: "%s"

If this is the first round, present a coding problem appropriate for the user's level.
If the user submitted a solution, review it and provide feedback. Then:
- If the solution is correct: offer a harder follow-up or move to a new problem
- If the solution needs work: give hints, don't reveal the full solution

Keep responses concise and focused. Use Chinese if the user uses Chinese.`,
		state.Round, maxRounds,
		strings.Join(state.History, "\n"),
		input,
	)

	resp, err := s.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("algorithm skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Type your solution or 'hint' for a hint, 'skip' to move on.",
	}, nil
}
