package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// ResumeMatchingNode compares a resume against the JD.
type ResumeMatchingNode struct {
	chatModel llm.ChatModel
}

// NewResumeMatchingNode creates a new resume matching node.
func NewResumeMatchingNode(chatModel llm.ChatModel) *ResumeMatchingNode {
	return &ResumeMatchingNode{chatModel: chatModel}
}

// Execute matches the resume against the JD and populates the state.
func (n *ResumeMatchingNode) Execute(ctx context.Context, state *InterviewState) error {
	if state.JDAnalysis == nil {
		return fmt.Errorf("JD analysis must be completed before resume matching")
	}

	resumeText, ok := state.InterruptData["resume_text"].(string)
	if !ok || resumeText == "" {
		log.Printf("[ResumeMatching] Warning: resume_text is empty or missing in InterruptData")
	}
	jdJSON, _ := json.Marshal(state.JDAnalysis)

	prompt := fmt.Sprintf(prompts.ResumeMatchSystemPrompt, string(jdJSON), resumeText)
	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please compare this resume with the job requirements."},
	})
	if err != nil {
		return fmt.Errorf("resume matching failed: %w", err)
	}

	var match model.ResumeMatch
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &match); err != nil {
		return fmt.Errorf("failed to parse resume match: %w", err)
	}

	state.ResumeMatch = &match
	state.Phase = model.PhaseQuestionPlanning
	return nil
}
