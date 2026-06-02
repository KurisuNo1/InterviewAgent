package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/google/uuid"
)

// BehavioralSkill provides behavioral interview practice (STAR method).
type BehavioralSkill struct {
	chatModel llm.ChatModel
}

// NewBehavioralSkill creates a new behavioral interview practice skill.
func NewBehavioralSkill(chatModel llm.ChatModel) *BehavioralSkill {
	return &BehavioralSkill{chatModel: chatModel}
}

func (s *BehavioralSkill) Name() string { return "behavioral" }
func (s *BehavioralSkill) Description() string {
	return "Behavioral interview practice using the STAR method"
}
func (s *BehavioralSkill) CanHandle(subIntent string) bool {
	return subIntent == "behavioral" || subIntent == "behavior" || subIntent == "star"
}

func (s *BehavioralSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *BehavioralSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 4
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Behavioral practice complete. Remember to always use the STAR method!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`You are a behavioral interview coach. Help the user practice answering behavioral questions using the STAR method (Situation, Task, Action, Result).

Current round: %d of %d

Previous exchange:
%s

The user said: "%s"

Common behavioral questions:
- Tell me about a time you faced a conflict at work
- Describe a project you're proud of
- How do you handle tight deadlines?
- Tell me about a time you failed

Evaluate the user's answer based on:
- S: Did they clearly describe the situation?
- T: Did they explain their task/responsibility?
- A: Did they detail the actions they took?
- R: Did they share measurable results?

Provide specific feedback and ask a follow-up question.`,
		state.Round, maxRounds,
		strings.Join(state.History, "\n"),
		input,
	)

	resp, err := s.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("behavioral skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Answer using the STAR format (Situation, Task, Action, Result).",
	}, nil
}
