package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// JDAnalysisNode parses a job description into structured requirements.
type JDAnalysisNode struct {
	chatModel einomodel.ToolCallingChatModel
}

// NewJDAnalysisNode creates a new JD analysis node.
func NewJDAnalysisNode(chatModel einomodel.ToolCallingChatModel) *JDAnalysisNode {
	return &JDAnalysisNode{chatModel: chatModel}
}

// Execute parses the JD text and populates the state.
func (n *JDAnalysisNode) Execute(ctx context.Context, state *InterviewState) error {
	// Extract JD text from session
	jdText, ok := state.InterruptData["jd_text"].(string)
	if !ok || jdText == "" {
		log.Printf("[JDAnalysis] Warning: jd_text is empty or missing in InterruptData")
	}

	prompt := safeFmt(prompts.JDAnalysisSystemPrompt, jdText)
	resp, err := n.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompt),
		schema.UserMessage("Please analyze this job description."),
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
