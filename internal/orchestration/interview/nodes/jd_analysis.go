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

// JDAnalysisNode parses a job description into structured requirements.
type JDAnalysisNode struct {
	chatModel llm.ChatModel
}

// NewJDAnalysisNode creates a new JD analysis node.
func NewJDAnalysisNode(chatModel llm.ChatModel) *JDAnalysisNode {
	return &JDAnalysisNode{chatModel: chatModel}
}

// Execute parses the JD text and populates the state.
func (n *JDAnalysisNode) Execute(ctx context.Context, state *InterviewState) error {
	// Extract JD text from session
	jdText, ok := state.InterruptData["jd_text"].(string)
	if !ok || jdText == "" {
		log.Printf("[JDAnalysis] Warning: jd_text is empty or missing in InterruptData")
	}

	prompt := fmt.Sprintf(prompts.JDAnalysisSystemPrompt, jdText)
	resp, err := n.chatModel.Chat(ctx, []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please analyze this job description."},
	})
	if err != nil {
		return fmt.Errorf("JD analysis failed: %w", err)
	}

	var analysis model.JDAnalysis
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &analysis); err != nil {
		return fmt.Errorf("failed to parse JD analysis: %w, content: %s", err, resp.Content)
	}

	analysis.RawText = jdText
	state.JDAnalysis = &analysis
	state.Phase = model.PhaseResumeMatching
	return nil
}
