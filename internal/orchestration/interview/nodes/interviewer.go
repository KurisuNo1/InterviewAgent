package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes/prompts"
)

// InterviewerDecision is the structured output the LLM must embed at the end of its response.
type InterviewerDecision struct {
	Action string `json:"action"` // "follow_up", "next_question", "complete"
	Reason string `json:"reason"`
}

// InterviewerNode handles the interview loop: asking questions, processing answers, and deciding follow-ups.
type InterviewerNode struct {
	chatModel    einomodel.ToolCallingChatModel
	maxFollowUps int
	agent        *react.Agent // optional: when set, LLM can use MCP tools during Q&A
	ctxBuilder   *contextmanager.ContextBuilder
}

// NewInterviewerNode creates a new interviewer node.
func NewInterviewerNode(chatModel einomodel.ToolCallingChatModel, maxFollowUps int, agent *react.Agent, ctxBuilder *contextmanager.ContextBuilder) *InterviewerNode {
	if maxFollowUps <= 0 {
		maxFollowUps = 3
	}
	return &InterviewerNode{
		chatModel:    chatModel,
		maxFollowUps: maxFollowUps,
		agent:        agent,
		ctxBuilder:   ctxBuilder,
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

	stateCtx := n.buildStateContext(state)
	var msgs []*schema.Message
	if n.ctxBuilder != nil {
		msgs = n.ctxBuilder.Build(contextmanager.BuildParams{
			ProfileName:  "interview_ask",
			SystemPrompt: n.baseSystemPrompt(state),
			StateContext: stateCtx,
			History:      state.ChatHistory,
			RAGDocuments: state.RAGDocuments,
			CurrentQ:     question.Content,
			UserInput:    "Ask the current question to the candidate. Provide context if needed. Do NOT include the decision block in your response when asking a question.",
		})
	} else {
		sysPrompt := n.buildSystemPrompt(state, "", "")
		msgs = []*schema.Message{
			schema.SystemMessage(sysPrompt),
			schema.UserMessage("Ask the current question to the candidate. Provide context if needed. Do NOT include the decision block in your response when asking a question."),
		}
	}

	response, err := n.callLLM(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("interviewer failed: %w", err)
	}

	// Strip any accidental decision block from question output
	response = stripDecisionBlock(response)

	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleAssistant,
		Content: response,
	})

	return response, nil
}

// ProcessAnswer evaluates the user's answer and decides the next step via LLM-driven decision.
func (n *InterviewerNode) ProcessAnswer(ctx context.Context, state *InterviewState, answer string) (string, string, error) {
	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleUser,
		Content: answer,
	})

	qContent := ""
	if state.CurrentQuestion != nil {
		qContent = state.CurrentQuestion.Content
	}

	stateCtx := n.buildStateContext(state)
	var msgs []*schema.Message
	if n.ctxBuilder != nil {
		msgs = n.ctxBuilder.Build(contextmanager.BuildParams{
			ProfileName:  "interview_ask",
			SystemPrompt: n.baseSystemPrompt(state),
			StateContext: stateCtx,
			History:      state.ChatHistory,
			RAGDocuments: state.RAGDocuments,
			CurrentQ:     qContent,
			LastAnswer:   answer,
			UserInput:    "Evaluate the candidate's answer. Then decide the next step. Append your decision in JSON format at the very end of your response.",
		})
	} else {
		sysPrompt := n.buildSystemPrompt(state, qContent, answer)
		msgs = []*schema.Message{
			schema.SystemMessage(sysPrompt),
			schema.UserMessage("Evaluate the candidate's answer. Then decide the next step. Append your decision in JSON format at the very end of your response."),
		}
	}

	response, err := n.callLLM(ctx, msgs)
	if err != nil {
		return "", "", fmt.Errorf("answer processing failed: %w", err)
	}

	// Parse LLM-driven decision from the structured decision block
	decision := n.ParseDecision(response)
	visibleResponse := stripDecisionBlock(response)

	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleAssistant,
		Content: visibleResponse,
	})

	// Apply the decision to state
	action := n.ApplyDecision(state, decision)
	return action, visibleResponse, nil
}

// ProcessAnswerStream streams the interviewer's evaluation and next step via SSE.
func (n *InterviewerNode) ProcessAnswerStream(ctx context.Context, state *InterviewState, answer string) (*schema.StreamReader[*schema.Message], error) {
	state.ChatHistory = append(state.ChatHistory, model.Message{
		Role:    model.RoleUser,
		Content: answer,
	})

	qContent := ""
	if state.CurrentQuestion != nil {
		qContent = state.CurrentQuestion.Content
	}

	stateCtx := n.buildStateContext(state)
	var msgs []*schema.Message
	if n.ctxBuilder != nil {
		msgs = n.ctxBuilder.Build(contextmanager.BuildParams{
			ProfileName:  "interview_ask",
			SystemPrompt: n.baseSystemPrompt(state),
			StateContext: stateCtx,
			History:      state.ChatHistory,
			RAGDocuments: state.RAGDocuments,
			CurrentQ:     qContent,
			LastAnswer:   answer,
			UserInput:    "Evaluate the candidate's answer. Decide the next step. Append your decision as JSON at the very end of your response.",
		})
	} else {
		sysPrompt := n.buildSystemPrompt(state, qContent, answer)
		msgs = []*schema.Message{
			schema.SystemMessage(sysPrompt),
			schema.UserMessage("Evaluate the candidate's answer. Decide the next step. Append your decision as JSON at the very end of your response."),
		}
	}

	return n.chatModel.Stream(ctx, msgs)
}

// callLLM routes through the React Agent if available, otherwise falls back to direct LLM.
func (n *InterviewerNode) callLLM(ctx context.Context, msgs []*schema.Message) (string, error) {
	if n.agent != nil {
		resp, err := n.agent.Generate(ctx, msgs)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
	resp, err := n.chatModel.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// parseDecision extracts the structured decision JSON from the LLM response.
// ParseDecision extracts the structured decision JSON from an LLM response.
func (n *InterviewerNode) ParseDecision(response string) InterviewerDecision {
	// Look for JSON decision block
	start := strings.LastIndex(response, `{"action"`)
	if start < 0 {
		start = strings.LastIndex(response, `{ "action"`)
	}
	if start < 0 {
		// Fallback: try to find any JSON-like block at the end
		start = strings.LastIndex(response, "{")
		if start < 0 {
			return n.legacyKeywordDecision(response)
		}
	}

	end := strings.LastIndex(response, "}")
	if end < 0 || end <= start {
		return n.legacyKeywordDecision(response)
	}

	jsonStr := response[start : end+1]
	var decision InterviewerDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return n.legacyKeywordDecision(response)
	}

	if decision.Action == "" {
		return n.legacyKeywordDecision(response)
	}

	return decision
}

// applyDecision updates the interview state based on the LLM's decision.
func (n *InterviewerNode) ApplyDecision(state *InterviewState, d InterviewerDecision) string {
	switch d.Action {
	case "complete":
		state.Phase = model.PhaseCompleted
		return "complete"
	case "next_question":
		state.CurrentQIndex++
		state.CurrentFollowUp = 0
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.Phase = model.PhaseCompleted
			return "complete"
		}
		return "next_question"
	default: // "follow_up" or anything else
		if state.CurrentFollowUp < n.maxFollowUps {
			state.CurrentFollowUp++
			return "follow_up"
		}
		// Max follow-ups exhausted → force move to next question
		state.CurrentQIndex++
		state.CurrentFollowUp = 0
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.Phase = model.PhaseCompleted
			return "complete"
		}
		return "next_question"
	}
}

// legacyKeywordDecision is the fallback when JSON parsing fails.
func (n *InterviewerNode) legacyKeywordDecision(response string) InterviewerDecision {
	if strings.Contains(response, "INTERVIEW_COMPLETE") {
		return InterviewerDecision{Action: "complete", Reason: "keyword: INTERVIEW_COMPLETE"}
	}
	if strings.Contains(response, "NEXT_QUESTION") {
		return InterviewerDecision{Action: "next_question", Reason: "keyword: NEXT_QUESTION"}
	}
	return InterviewerDecision{Action: "follow_up", Reason: "fallback: default to follow_up"}
}

// stripDecisionBlock removes the JSON decision block from visible output.
func stripDecisionBlock(response string) string {
	// Remove ---DECISION--- marker block
	if idx := strings.LastIndex(response, "---DECISION---"); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	// Remove trailing JSON decision
	if idx := strings.LastIndex(response, `{"action"`); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	if idx := strings.LastIndex(response, `{ "action"`); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	return response
}

// baseSystemPrompt returns the interviewer system prompt template without state injection.
// State context is injected separately by ContextBuilder.
func (n *InterviewerNode) baseSystemPrompt(state *InterviewState) string {
	position := "unknown position"
	techStack := []string{}
	if state.JDAnalysis != nil {
		position = state.JDAnalysis.Position
		techStack = state.JDAnalysis.TechStack
	}
	diffLevel := "medium"
	if state.Difficulty != nil {
		diffLevel = string(state.Difficulty.CurrentLevel)
	}
	ragSection := buildRAGSection(state.RAGDocuments)

	return safeFmt(prompts.InterviewerSystemPrompt,
		position,
		techStack,
		state.CurrentQIndex+1,
		len(state.QuestionQueue),
		diffLevel,
		ragSection,
		n.maxFollowUps,
		"%s", // history placeholder (filled by ContextBuilder)
		"%s", // current question placeholder
		"%s", // last answer placeholder
	)
}

// buildStateContext extracts interview state into a map for ContextBuilder injection.
func (n *InterviewerNode) buildStateContext(state *InterviewState) map[string]any {
	m := map[string]any{
		"question_index": fmt.Sprintf("%d/%d", state.CurrentQIndex+1, len(state.QuestionQueue)),
	}
	if state.JDAnalysis != nil {
		m["position"] = state.JDAnalysis.Position
		m["tech_stack"] = state.JDAnalysis.TechStack
	}
	if state.Difficulty != nil {
		m["difficulty"] = string(state.Difficulty.CurrentLevel)
	}
	return m
}

func (n *InterviewerNode) buildSystemPrompt(state *InterviewState, currentQ, lastAnswer string) string {
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

	ragSection := buildRAGSection(state.RAGDocuments)

	return safeFmt(prompts.InterviewerSystemPrompt,
		position,
		techStack,
		state.CurrentQIndex+1,
		len(state.QuestionQueue),
		diffLevel,
		ragSection,
		n.maxFollowUps,
		history,
		currentQ,
		lastAnswer,
	)
}

func buildRAGSection(ragDocs string) string {
	if ragDocs == "" {
		return ""
	}
	return "## Reference Knowledge\nUse the following reference material to evaluate the candidate's answers and guide your follow-up questions. Refer to this knowledge to assess correctness, depth, and coverage:\n\n" + ragDocs
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
