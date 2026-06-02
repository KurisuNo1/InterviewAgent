package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/google/uuid"
)

// SystemDesignSkill provides system design interview practice.
type SystemDesignSkill struct {
	chatModel llm.ChatModel
}

// NewSystemDesignSkill creates a new system design practice skill.
func NewSystemDesignSkill(chatModel llm.ChatModel) *SystemDesignSkill {
	return &SystemDesignSkill{chatModel: chatModel}
}

func (s *SystemDesignSkill) Name() string        { return "system_design" }
func (s *SystemDesignSkill) Description() string { return "System design interview practice" }
func (s *SystemDesignSkill) CanHandle(subIntent string) bool {
	return subIntent == "system_design" || subIntent == "design" || subIntent == "architecture"
}

func (s *SystemDesignSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *SystemDesignSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 6
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Design discussion complete. Great work thinking through the architecture!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`You are a system design interviewer. Guide the user through designing a system.

Current round: %d of %d

Previous exchange:
%s

The user said: "%s"

System design interview phases:
1. Requirements clarification
2. Capacity estimation
3. High-level design
4. Deep dive into components
5. Trade-offs and bottlenecks

Guide the user through each phase based on the current round. Ask probing questions about their design choices. Provide constructive feedback.`,
		state.Round, maxRounds,
		strings.Join(state.History, "\n"),
		input,
	)

	resp, err := s.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("system design skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Describe your design approach or ask for clarification.",
	}, nil
}
