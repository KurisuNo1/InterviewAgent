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

// EvaluationNode scores a candidate's answer to a question.
// When an agent is configured, it can search for reference answers to improve scoring accuracy.
type EvaluationNode struct {
	chatModel  einomodel.ToolCallingChatModel
	agent      *react.Agent // optional: ReAct agent for searching reference answers
	ctxBuilder *contextmanager.ContextBuilder
}

// NewEvaluationNode creates a new evaluation node.
func NewEvaluationNode(chatModel einomodel.ToolCallingChatModel, agent *react.Agent, ctxBuilder *contextmanager.ContextBuilder) *EvaluationNode {
	return &EvaluationNode{chatModel: chatModel, agent: agent, ctxBuilder: ctxBuilder}
}

// Execute evaluates the last answer in the conversation.
func (n *EvaluationNode) Execute(ctx context.Context, state *InterviewState) error {
	if state.CurrentQuestion == nil {
		return fmt.Errorf("no current question to evaluate")
	}

	// Extract the last answer from chat history
	answer := extractLastAnswer(state.ChatHistory)
	if answer == "" {
		return fmt.Errorf("no answer found to evaluate")
	}

	// Build follow-up exchange context
	followUps := extractFollowUps(state.ChatHistory)

	// Skip agent-based search during evaluation — it adds 30-70s latency
	// per web search round. Reference enrichment is best-effort and not
	// worth the delay during interview scoring.

	var (
		resp *schema.Message
		err  error
	)
	if n.ctxBuilder != nil {
		refDocs := ""
		if ref, ok := state.InterruptData["eval_reference"]; ok {
			if s, ok := ref.(string); ok {
				refDocs = s
			}
		}
		msgs := n.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    state.SessionID,
			ProfileName: "interview_eval",
			SystemPrompt: safeFmt(prompts.EvaluationSystemPrompt,
				state.CurrentQuestion.Content,
				state.CurrentQuestion.ScoringPoints,
				answer,
				followUps,
				state.CurrentQuestion.ID,
			),
			History:      state.ChatHistory,
			RAGDocuments: refDocs,
			CurrentQ:     state.CurrentQuestion.Content,
			LastAnswer:   answer,
			UserInput:    "Please evaluate this answer and output the JSON evaluation.",
		})
		resp, err = n.callLLM(ctx, msgs)
	} else {
		prompt := safeFmt(prompts.EvaluationSystemPrompt,
			state.CurrentQuestion.Content,
			state.CurrentQuestion.ScoringPoints,
			answer,
			followUps,
			state.CurrentQuestion.ID,
		)
		resp, err = n.callLLMWithPrompt(ctx, prompt)
	}
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	var eval model.Evaluation
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &eval); err != nil {
		return fmt.Errorf("failed to parse evaluation: %w", err)
	}

	// Record the answer
	state.Answers = append(state.Answers, model.Answer{
		QuestionID: state.CurrentQuestion.ID,
		Content:    answer,
	})
	state.Evaluations = append(state.Evaluations, eval)

	return nil
}

// callLLM uses direct LLM (not agent) for evaluation to keep latency predictable.
func (n *EvaluationNode) callLLM(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	return n.chatModel.Generate(ctx, msgs)
}

// callLLMWithPrompt is the fallback when ContextBuilder is not available.
func (n *EvaluationNode) callLLMWithPrompt(ctx context.Context, systemPrompt string) (*schema.Message, error) {
	msgs := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("Please evaluate this answer and output the JSON evaluation."),
	}
	return n.callLLM(ctx, msgs)
}

// enrichWithSearch uses the ReAct agent to search for reference information about the question topic.
func (n *EvaluationNode) enrichWithSearch(ctx context.Context, state *InterviewState, answer string) {
	searchPrompt := fmt.Sprintf(`## Role
You are a search assistant helping an interview evaluator. Search for reference answers or standard knowledge related to the question below. This will be used to validate the candidate's answer.

## Question
%s

## Candidate's Answer
%s

## Task
Search for authoritative references, correct answers, or key concepts related to this question. Summarize what you find in 1-2 sentences that would help the evaluator decide if the answer is correct.`,
		state.CurrentQuestion.Content, answer)

	msgs := []*schema.Message{
		schema.SystemMessage(searchPrompt),
		schema.UserMessage("Search for reference information to help evaluate this answer."),
	}

	resp, err := n.agent.Generate(ctx, msgs)
	if err != nil {
		return // best-effort; evaluation proceeds with prompt alone
	}

	// Store reference in state for the evaluation prompt to use
	if resp != nil && resp.Content != "" {
		if state.InterruptData == nil {
			state.InterruptData = make(map[string]any)
		}
		state.InterruptData["eval_reference"] = resp.Content
	}
}

func extractLastAnswer(history []model.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == model.RoleUser {
			return history[i].Content
		}
	}
	return ""
}

func extractFollowUps(history []model.Message) string {
	var exchange []string
	start := len(history) - 6 // last 3 turns (user + assistant)
	if start < 0 {
		start = 0
	}
	for _, msg := range history[start:] {
		exchange = append(exchange, fmt.Sprintf("[%s]: %s", msg.Role, msg.Content))
	}
	return strings.Join(exchange, "\n")
}
