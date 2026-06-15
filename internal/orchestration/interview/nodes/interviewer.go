package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
			SessionID:    state.SessionID,
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
			SessionID:    state.SessionID,
			ProfileName:  "interview_ask",
			SystemPrompt: n.baseSystemPrompt(state),
			StateContext: stateCtx,
			History:      state.ChatHistory,
			RAGDocuments: state.RAGDocuments,
			CurrentQ:     qContent,
			LastAnswer:   answer,
			UserInput:    "Evaluate the candidate's answer briefly. Decide: follow_up, next_question, or complete. " +
				"Output a short evaluation feedback and a brief transition phrase (e.g. 'Let's move to the next question.'). " +
				"Do NOT output the next question itself — it will be asked separately. " +
				"Append your decision in JSON format at the very end of your response.",
		})
	} else {
		sysPrompt := n.buildSystemPrompt(state, qContent, answer)
		msgs = []*schema.Message{
			schema.SystemMessage(sysPrompt),
			schema.UserMessage("Evaluate the candidate's answer briefly. Decide: follow_up, next_question, or complete. " +
				"Output a short evaluation and transition phrase. Do NOT output the next question itself. " +
				"Append your decision in JSON format at the very end of your response."),
		}
	}

	response, err := n.callLLM(ctx, msgs)
	if err != nil {
		return "", "", fmt.Errorf("answer processing failed: %w", err)
	}

	// Try JSON parsing first; if that fails, use reliable fallback classification
	decision, fromJSON := n.parseDecisionWithSource(response)
	visibleResponse := stripDecisionBlock(response)

	if !fromJSON {
		decision = n.classifyDecisionFallback(ctx, visibleResponse)
		log.Printf("[InterviewerNode] JSON not found in response, fallback classifier returned: action=%s reason=%s",
			decision.Action, decision.Reason)
	}

		log.Printf("[InterviewerNode] ProcessAnswer decision: action=%s reason=%s fromJSON=%v q=%d/%d",
			decision.Action, decision.Reason, fromJSON,
			state.CurrentQIndex+1, len(state.QuestionQueue))

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
			SessionID:    state.SessionID,
			ProfileName:  "interview_ask",
			SystemPrompt: n.baseSystemPrompt(state),
			StateContext: stateCtx,
			History:      state.ChatHistory,
			RAGDocuments: state.RAGDocuments,
			CurrentQ:     qContent,
			LastAnswer:   answer,
			UserInput:    "Evaluate the candidate's answer briefly. Decide: follow_up, next_question, or complete. " +
				"Output a short evaluation feedback and a brief transition phrase. " +
				"Do NOT output the next question itself — it will be asked separately. " +
				"Append your decision as JSON at the very end of your response.",
		})
	} else {
		sysPrompt := n.buildSystemPrompt(state, qContent, answer)
		msgs = []*schema.Message{
			schema.SystemMessage(sysPrompt),
			schema.UserMessage("Evaluate the candidate's answer briefly. Decide: follow_up, next_question, or complete. " +
				"Output a short evaluation and transition phrase. Do NOT output the next question itself. " +
				"Append your decision as JSON at the very end of your response."),
		}
	}

	return n.chatModel.Stream(ctx, msgs)
}

// callLLM uses direct LLM (not agent) for interview Q&A to keep latency predictable.
// The ReAct agent with MCP tools is only needed for autonomous resource search tasks.
func (n *InterviewerNode) callLLM(ctx context.Context, msgs []*schema.Message) (string, error) {
	resp, err := n.chatModel.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ParseDecision extracts the structured decision JSON from an LLM response.
// Deprecated: prefer parseDecisionWithSource which indicates whether JSON was found.
func (n *InterviewerNode) ParseDecision(response string) InterviewerDecision {
	d, _ := n.parseDecisionWithSource(response)
	return d
}

// parseDecisionWithSource tries to extract a JSON decision block from the response.
// Returns the decision and true if it came from valid JSON, false if fallback was used.
func (n *InterviewerNode) parseDecisionWithSource(response string) (InterviewerDecision, bool) {
	start := strings.LastIndex(response, `{"action"`)
	if start < 0 {
		start = strings.LastIndex(response, `{ "action"`)
	}
	if start >= 0 {
		end := strings.LastIndex(response, "}")
		if end > start {
			jsonStr := response[start : end+1]
			var decision InterviewerDecision
			if err := json.Unmarshal([]byte(jsonStr), &decision); err == nil && decision.Action != "" {
				return decision, true
			}
		}
	}
	return n.legacyKeywordDecision(response), false
}

// classifyDecisionFallback makes a tiny LLM call to classify the interviewer's intent.
// Used when JSON decision parsing fails. The prompt asks for ONE word, making it highly reliable.
func (n *InterviewerNode) classifyDecisionFallback(ctx context.Context, response string) InterviewerDecision {
	// Step 1: Quick keyword check (zero latency)
	kw := n.legacyKeywordDecision(response)
	if kw.Action != "follow_up" {
		return kw // keyword confidently matched next_question or complete
	}

	// Step 2: Check if the response itself contains a question mark (indicates follow_up)
	trimmed := strings.TrimSpace(response)
	lastLine := trimmed[strings.LastIndex(trimmed, "\n")+1:]
	if strings.Contains(lastLine, "?") || strings.Contains(lastLine, "？") {
		return InterviewerDecision{Action: "follow_up", Reason: "response ends with question"}
	}

	// Step 3: Check for strong transition signals in the last few lines
	lastLines := trimmed
	if len(trimmed) > 300 {
		lastLines = trimmed[len(trimmed)-300:]
	}
	lowerLast := strings.ToLower(lastLines)
	transitionSignals := []string{
		"move to the next", "let's move on", "moving on",
		"next question", "下一题", "进入下一题",
		"let's continue", "moving forward",
	}
	for _, sig := range transitionSignals {
		if strings.Contains(lowerLast, sig) {
			return InterviewerDecision{Action: "next_question", Reason: "transition signal: " + sig}
		}
	}

	// Step 4: Tiny LLM classification as final fallback
	classifyPrompt := `Classify the interviewer's last response into ONE action word only.
- If asking a follow-up question or requesting more details: output "follow_up"
- If transitioning to the next topic (praise, summary, no question asked): output "next_question"
- If the interview is complete (final summary, goodbye): output "complete"

Output exactly one word: follow_up, next_question, or complete.

Response:
"""` + truncateForClassify(response, 500) + `"""

Action:`

	msgs := []*schema.Message{schema.UserMessage(classifyPrompt)}
	resp, err := n.chatModel.Generate(ctx, msgs)
	if err != nil {
		log.Printf("[InterviewerNode] classifyDecisionFallback LLM error: %v", err)
		return kw
	}
	if resp == nil {
		return kw
	}

	result := strings.TrimSpace(strings.ToLower(resp.Content))
	log.Printf("[InterviewerNode] classifyDecisionFallback: LLM returned %q", result)

	switch {
	case strings.Contains(result, "complete"):
		return InterviewerDecision{Action: "complete", Reason: "classifier: complete"}
	case strings.Contains(result, "next_question"), strings.Contains(result, "next"):
		return InterviewerDecision{Action: "next_question", Reason: "classifier: next_question"}
	default:
		return InterviewerDecision{Action: "follow_up", Reason: "classifier: follow_up"}
	}
}

func truncateForClassify(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "..."
}

// ApplyDecision updates the interview state based on the LLM's decision.
// It sets state.NextAction so that EvaluateAnswer knows whether to run evaluation.
func (n *InterviewerNode) ApplyDecision(state *InterviewState, d InterviewerDecision) string {
	switch d.Action {
	case "complete":
		state.Phase = model.PhaseCompleted
		state.NextAction = "complete"
		return "complete"
	case "next_question":
		state.CurrentQIndex++
		state.CurrentFollowUp = 0
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.Phase = model.PhaseCompleted
			state.NextAction = "complete"
			return "complete"
		}
		state.NextAction = "next_question"
		return "next_question"
	default: // "follow_up" or anything else
		if state.CurrentFollowUp < n.maxFollowUps {
			state.CurrentFollowUp++
			state.NextAction = "follow_up"
			return "follow_up"
		}
		// Max follow-ups exhausted → force move to next question
		state.CurrentQIndex++
		state.CurrentFollowUp = 0
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.Phase = model.PhaseCompleted
			state.NextAction = "complete"
			return "complete"
		}
		state.NextAction = "next_question"
		return "next_question"
	}
}

// legacyKeywordDecision is the fallback when JSON parsing fails.
func (n *InterviewerNode) legacyKeywordDecision(response string) InterviewerDecision {
	lower := strings.ToLower(response)
	if strings.Contains(response, "INTERVIEW_COMPLETE") || strings.Contains(lower, "interview is complete") {
		return InterviewerDecision{Action: "complete", Reason: "keyword: INTERVIEW_COMPLETE"}
	}
	if strings.Contains(response, "NEXT_QUESTION") ||
		strings.Contains(lower, "move to the next question") ||
		strings.Contains(lower, "let's move on") ||
		strings.Contains(lower, "moving on to") ||
		strings.Contains(response, "下一题") ||
		strings.Contains(response, "进入下一题") {
		return InterviewerDecision{Action: "next_question", Reason: "keyword: next_question"}
	}
	return InterviewerDecision{Action: "follow_up", Reason: "fallback: default to follow_up"}
}

// stripDecisionBlock removes the JSON decision block from visible output.
func stripDecisionBlock(response string) string {
	if idx := strings.LastIndex(response, "---DECISION---"); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	if idx := strings.LastIndex(response, `{"action"`); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	if idx := strings.LastIndex(response, `{ "action"`); idx >= 0 {
		return strings.TrimSpace(response[:idx])
	}
	return response
}

// baseSystemPrompt returns the interviewer system prompt template without state injection.
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
		"%s",
		"%s",
		"%s",
	)
}

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
		position, techStack,
		state.CurrentQIndex+1, len(state.QuestionQueue),
		diffLevel, ragSection, n.maxFollowUps,
		history, currentQ, lastAnswer,
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
