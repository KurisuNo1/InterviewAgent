package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/cloudwego/eino/components/embedding"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/mcp"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/resume"

	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/ingestion"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/memory"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/rag"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/router"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/skill"
)

// Orchestrator implements interaction.InterviewService.
// It coordinates the Eino Graph, intent router, skill registry, and memory.
type Orchestrator struct {
	intentRouter    *router.Host
	interviewRunner *interview.Runner
	skillRegistry   *skill.Registry
	memory          *memory.Manager
	docIngestor     *ingestion.DocumentIngestor
	chatModel       einomodel.ToolCallingChatModel
	hybridRetriever  retriever.Retriever
	embedder        embedding.Embedder
	githubMCP       *mcp.GitHubMCP
	webMCP          *mcp.WebSearchMCP
	casualAgent     *react.Agent // nil when MCP unavailable
	mcpBridge       *mcp.EinoBridge
	ctxBuilder      *contextmanager.ContextBuilder
	memHierarchy    *contextmanager.MemoryHierarchy
	overflowHandler *contextmanager.OverflowHandler
	ctxMonitor      *contextmanager.DefaultMonitor

	mu     sync.RWMutex
	states map[string]*nodes.InterviewState
	events *eventBus
}

// eventBus holds per-session subscriber channels.
type eventBus struct {
	mu     sync.RWMutex
	subs   map[string][]chan *interaction.InterviewEvent
}

func newEventBus() *eventBus {
	return &eventBus{subs: make(map[string][]chan *interaction.InterviewEvent)}
}

func (eb *eventBus) subscribe(sessionID string) chan *interaction.InterviewEvent {
	ch := make(chan *interaction.InterviewEvent, 64)
	eb.mu.Lock()
	eb.subs[sessionID] = append(eb.subs[sessionID], ch)
	eb.mu.Unlock()
	return ch
}

func (eb *eventBus) publish(sessionID string, event *interaction.InterviewEvent) {
	eb.mu.RLock()
	channels := eb.subs[sessionID]
	eb.mu.RUnlock()
	for _, ch := range channels {
		select {
		case ch <- event:
		default:
		}
	}
}

func (eb *eventBus) unsubscribe(sessionID string, ch chan *interaction.InterviewEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	subs := eb.subs[sessionID]
	for i, sub := range subs {
		if sub == ch {
			eb.subs[sessionID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	if len(eb.subs[sessionID]) == 0 {
		delete(eb.subs, sessionID)
	}
}

func NewOrchestrator(
	intentRouter *router.Host,
	interviewRunner *interview.Runner,
	skillRegistry *skill.Registry,
	memoryManager *memory.Manager,
	docIngestor *ingestion.DocumentIngestor,
	chatModel einomodel.ToolCallingChatModel,
	hybridRetriever retriever.Retriever,
	embedder embedding.Embedder,
	githubMCP *mcp.GitHubMCP,
	webMCP *mcp.WebSearchMCP,
	mcpBridge *mcp.EinoBridge,
	casualAgent *react.Agent,
	ctxBuilder *contextmanager.ContextBuilder,
	memHierarchy *contextmanager.MemoryHierarchy,
	overflowHandler *contextmanager.OverflowHandler,
	ctxMonitor *contextmanager.DefaultMonitor,
) *Orchestrator {
	o := &Orchestrator{
		intentRouter:    intentRouter,
		interviewRunner: interviewRunner,
		skillRegistry:   skillRegistry,
		memory:          memoryManager,
		docIngestor:     docIngestor,
		chatModel:       chatModel,
		hybridRetriever:  hybridRetriever,
		embedder:        embedder,
		githubMCP:       githubMCP,
		webMCP:          webMCP,
		mcpBridge:       mcpBridge,
		casualAgent:     casualAgent,
		ctxBuilder:      ctxBuilder,
		memHierarchy:    memHierarchy,
		overflowHandler: overflowHandler,
		ctxMonitor:      ctxMonitor,
		states:          make(map[string]*nodes.InterviewState),
		events:          newEventBus(),
	}

	// Register specialists for all intents
	intentRouter.Register(router.IntentInterview, router.NewInterviewSpecialist(o))
	intentRouter.Register(router.IntentSkillPractice, router.NewSkillPracticeSpecialist(o))
	log.Printf("[Orchestrator] Intent router specialists registered (interview, skill_practice, casual_chat)")

	return o
}

func (o *Orchestrator) getState(sessionID string) (*nodes.InterviewState, error) {
	o.mu.RLock()
	state, ok := o.states[sessionID]
	o.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	// Return a deep copy to prevent concurrent modification races.
	// Callers receive their own copy and must call setState to persist changes.
	data, _ := json.Marshal(state)
	var copy nodes.InterviewState
	json.Unmarshal(data, &copy)
	// Ensure maps are initialized (omitempty in JSON tags can produce nil maps)
	if copy.InterruptData == nil {
		copy.InterruptData = make(map[string]any)
	}
	return &copy, nil
}

func (o *Orchestrator) setState(sessionID string, state *nodes.InterviewState) {
	o.mu.Lock()
	o.states[sessionID] = state
	o.mu.Unlock()
}

func (o *Orchestrator) CreateSession(ctx context.Context, req interaction.CreateSessionReq) (*model.Session, error) {
	session := &model.Session{
		ID:        uuid.New().String(),
		UserID:    req.UserID,
		Status:    model.PhaseCreated,
		JDText:    req.JDText,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if o.memory != nil {
		if err := o.memory.SaveSession(ctx, session); err != nil {
			log.Printf("Warning: failed to save session: %v", err)
		}
	}

	state := o.interviewRunner.CreateState(session.ID)
	if req.JDText != "" {
		state.InterruptData["jd_text"] = req.JDText
	}
	o.setState(session.ID, state)

	return session, nil
}

func (o *Orchestrator) getOrRestoreState(ctx context.Context, sessionID string) (*nodes.InterviewState, error) {
	state, err := o.getState(sessionID)
	if err == nil {
		return state, nil
	}
	// Fall back to checkpoint (server restart recovery)
	state, err = o.interviewRunner.LoadCheckpoint(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	o.setState(sessionID, state)
	return state, nil
}

func (o *Orchestrator) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &model.Session{ID: sessionID, Status: state.Phase}, nil
}

// GetConversationHistory returns the recent conversation messages for a session.
func (o *Orchestrator) GetConversationHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	if o.memory == nil {
		return nil, fmt.Errorf("memory not available")
	}
	return o.memory.GetConversationContext(ctx, sessionID, 50)
}

func (o *Orchestrator) ResumeSession(ctx context.Context, sessionID string) (*model.Session, error) {
	state, err := o.interviewRunner.LoadCheckpoint(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to resume session %s: %w", sessionID, err)
	}
	o.setState(sessionID, state)
	return &model.Session{ID: sessionID, Status: state.Phase}, nil
}

func (o *Orchestrator) ParseJD(ctx context.Context, sessionID string, rawJD string) (*model.JDAnalysis, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	state.InterruptData["jd_text"] = rawJD
	state.Phase = model.PhaseJDParsing

	_, err = o.interviewRunner.InvokeSetup(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	o.setState(sessionID, state)

	// Save checkpoint after setup
	if err := o.interviewRunner.SaveCheckpoint(ctx, state); err != nil {
		log.Printf("Warning: checkpoint save after setup failed for session %s: %v", sessionID, err)
	}

	// Update session status in database
	if o.memory != nil {
		if err := o.memory.UpdateSessionStatus(ctx, sessionID, state.Phase); err != nil {
			log.Printf("Warning: failed to update session status after JD parse: %v", err)
		}
	}

	if state.JDAnalysis == nil {
		return nil, fmt.Errorf("JD analysis not yet generated")
	}
	return state.JDAnalysis, nil
}

func (o *Orchestrator) UploadResume(ctx context.Context, sessionID string, fileData []byte, fileName string) (*model.ResumeMatch, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	resumeText, err := resume.Parse(fileData)
	if err != nil {
		return nil, fmt.Errorf("cannot parse resume: %w", err)
	}

	state.InterruptData["resume_text"] = resumeText

	if err := o.interviewRunner.Graph().ResumeMatching.Execute(ctx, state); err != nil {
		return nil, fmt.Errorf("resume matching failed: %w", err)
	}

	if err := o.interviewRunner.Graph().QuestionPlanning.Execute(ctx, state); err != nil {
		return nil, fmt.Errorf("question planning failed: %w", err)
	}

	if err := o.interviewRunner.SaveCheckpoint(ctx, state); err != nil {
		log.Printf("Warning: checkpoint save after resume failed: %v", err)
	}

	o.setState(sessionID, state)
	return state.ResumeMatch, nil
}

func (o *Orchestrator) GetQuestionPlan(ctx context.Context, sessionID string) (*model.QuestionPlan, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	log.Printf("[GetQuestionPlan] state.Phase=%s, JDAnalysis=%v, ResumeMatch=%v, QuestionPlan=%v",
		state.Phase, state.JDAnalysis != nil, state.ResumeMatch != nil, state.QuestionPlan != nil)

	if state.QuestionPlan == nil {
		if state.JDAnalysis != nil {
			log.Printf("[GetQuestionPlan] Auto-generating questions from JD only (no resume)")
			if err := o.interviewRunner.Graph().QuestionPlanning.Execute(ctx, state); err != nil {
				return nil, fmt.Errorf("question planning failed: %w", err)
			}
			o.interviewRunner.SaveCheckpoint(ctx, state)
			o.setState(sessionID, state)
			log.Printf("[GetQuestionPlan] Questions generated: %d", len(state.QuestionQueue))
		} else {
			log.Printf("[GetQuestionPlan] JDAnalysis is nil, cannot auto-generate")
		}
	}
	if state.QuestionPlan == nil {
		return nil, fmt.Errorf("question plan not yet generated — JD analysis required first")
	}
	return state.QuestionPlan, nil
}

func (o *Orchestrator) StartInterview(ctx context.Context, sessionID string) (*interaction.InterviewEvent, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	action, response, isComplete, err := o.interviewRunner.InvokeInterview(ctx, sessionID, "", state)
	if err != nil {
		// If all questions are exhausted, treat as interview completion rather than error
		if state.Phase == model.PhaseCompleted {
			// Generate final report from whatever evaluations we have (may be empty)
			if state.FinalReport == nil {
				state.FinalReport = o.buildReportFromState(state)
			}
			if state.ReviewPlan == nil {
				if err := o.interviewRunner.Graph().ReviewPlanning.ExecuteLightweight(ctx, state); err != nil {
					log.Printf("Warning: review planning on complete failed: %v", err)
				}
			}
			if o.memory != nil && state.FinalReport != nil {
				o.memory.UpdateSessionStatus(ctx, sessionID, model.PhaseCompleted)
				o.memory.SaveInterviewResult(ctx, state.FinalReport, state.ReviewPlan)
			}
			if o.memHierarchy != nil {
				go o.memHierarchy.ArchiveSessionSummary(context.Background(), sessionID, state.ChatHistory)
			}
			o.interviewRunner.SaveCheckpoint(ctx, state)
			result := &interaction.InterviewEvent{
				Type:      "complete",
				SessionID: sessionID,
				Data:      "All questions have been completed.",
			}
			o.setState(sessionID, state)
			o.events.publish(sessionID, result)
			return result, nil
		}
		return nil, fmt.Errorf("start interview failed: %w", err)
	}

	if o.memory != nil {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleAssistant,
			Content: response,
		})
	}

	eventType := action
	result := &interaction.InterviewEvent{
		Type:      eventType,
		SessionID: sessionID,
		Data:      response,
	}
	if isComplete {
		result.Type = "complete"
	}

	// Publish event to WebSocket subscribers
	o.events.publish(sessionID, result)

	o.setState(sessionID, state)
	return result, nil
}

func (o *Orchestrator) SubmitAnswer(ctx context.Context, sessionID string, answer string) (*interaction.InterviewEvent, error) {
	if o.memory != nil {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleUser,
			Content: answer,
		})
	}

	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	action, response, isComplete, err := o.interviewRunner.InvokeInterview(ctx, sessionID, answer, state)
	if err != nil {
		return nil, fmt.Errorf("submit answer failed: %w", err)
	}

	if o.memory != nil {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleAssistant,
			Content: response,
		})
	}

	result := &interaction.InterviewEvent{
		SessionID: sessionID,
		Data:      response,
	}

	switch action {
	case "follow_up":
		result.Type = "follow_up"
	case "question":
		result.Type = "next_question"
	case "complete":
		result.Type = "complete"
	default:
		result.Type = action
	}

	if isComplete {
		result.Type = "complete"
	}

	// If the decision is next_question, immediately ask the next question and append it
	if action == "next_question" && !isComplete {
		// Check if ProcessAnswer already leaked the next question into its response.
		// LLMs sometimes ignore "do NOT output the next question" instructions.
		// If the response already contains the question text, skip AskCurrentQuestion.
		dup := false
		if state.CurrentQIndex < len(state.QuestionQueue) {
			qContent := state.QuestionQueue[state.CurrentQIndex].Content
			if len(qContent) > 60 && len(response) > len(qContent)/2 &&
				strings.Contains(response, qContent[:60]) {
				dup = true
				log.Printf("[SubmitAnswer] next question already in ProcessAnswer response, skipping AskCurrentQuestion")
			}
		}
		if !dup {
			nextQ, qErr := o.interviewRunner.AskCurrentQuestion(ctx, state)
			if qErr != nil {
				log.Printf("[SubmitAnswer] AskCurrentQuestion failed: %v", qErr)
				response = response + "\n\n[系统提示：下一题生成失败，请刷新页面重试]"
			} else {
				response = response + "\n\n" + nextQ
			}
		}
		result.Data = response
	}

	// Publish event to WebSocket subscribers immediately
	o.events.publish(sessionID, result)

	if isComplete {
		// Interview complete: save state immediately, then run evaluation + review planning
		// asynchronously with background context to avoid HTTP context cancellation.
		// The frontend will fetch the report separately via GET /sessions/{id}/report.
		o.setState(sessionID, state)
		o.interviewRunner.SaveCheckpoint(ctx, state)

		go func() {
			bgCtx := context.Background()
			o.interviewRunner.EvaluateAnswer(bgCtx, state)

			if state.FinalReport == nil {
				state.FinalReport = o.buildReportFromState(state)
			}
			o.setState(sessionID, state)
			o.interviewRunner.SaveCheckpoint(bgCtx, state)

			if o.memory != nil {
				o.memory.UpdateSessionStatus(bgCtx, sessionID, model.PhaseCompleted)
				if err := o.memory.SaveInterviewResult(bgCtx, state.FinalReport, state.ReviewPlan); err != nil {
					log.Printf("[SubmitAnswer] Warning: failed to persist report: %v", err)
				}
			}
			if o.memHierarchy != nil {
				o.memHierarchy.ArchiveSessionSummary(bgCtx, sessionID, state.ChatHistory)
			}
		}()
	} else {
		// Normal answer: save state to memory immediately, then persist checkpoint
		// with background context so it survives even if the HTTP request times out.
		o.setState(sessionID, state)

		go func() {
			bgCtx := context.Background()
			o.interviewRunner.SaveCheckpoint(bgCtx, state)
			o.interviewRunner.EvaluateAnswer(bgCtx, state)
			o.setState(sessionID, state)
			o.interviewRunner.SaveCheckpoint(bgCtx, state)

			// Trigger async compression if conversation exceeds threshold
			if o.memHierarchy != nil && len(state.ChatHistory) > 20 {
				profile := contextmanager.Profile(nil, "interview_ask")
				if len(state.ChatHistory)/2 > profile.CompressionThreshold {
					compressed := o.ctxBuilder.Build(contextmanager.BuildParams{
						SessionID:    sessionID,
						ProfileName: "interview_ask",
						History:     state.ChatHistory,
						UserInput:   "",
					})
					if len(compressed) < len(state.ChatHistory) {
						state.CompressedSummary = fmt.Sprintf("[compressed %d turns]", len(state.ChatHistory)/2)
						state.CompressedUpToRound = state.CurrentQIndex
						log.Printf("[Orchestrator] Compressed conversation for session %s (round %d)", sessionID, state.CurrentQIndex)
					}
				}
			}
		}()
	}

	return result, nil
}

// StreamSubmitAnswer streams the interviewer's response via SSE for real-time display.
func (o *Orchestrator) StreamSubmitAnswer(ctx context.Context, sessionID string, answer string) (*schema.StreamReader[*schema.Message], error) {
	if o.memory != nil {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleUser,
			Content: answer,
		})
	}

	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	stream, err := o.interviewRunner.Graph().Interviewer.ProcessAnswerStream(ctx, state, answer)
	if err != nil {
		return nil, fmt.Errorf("stream answer failed: %w", err)
	}

	// Collect the full response for state + memory, then run evaluation in background
	reader, writer := schema.Pipe[*schema.Message](64)
	go func() {
		defer writer.Close()
		defer stream.Close()
		var full strings.Builder
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			if msg != nil {
				full.WriteString(msg.Content)
				writer.Send(msg, nil)
			}
		}
		response := full.String()

		// Parse decision and update state
		decision := o.interviewRunner.Graph().Interviewer.ParseDecision(response)
		// Strip decision block from visible output
		visible := response
		if idx := strings.LastIndex(response, `{"action"`); idx >= 0 {
			visible = strings.TrimSpace(response[:idx])
		} else if idx := strings.LastIndex(response, `{ "action"`); idx >= 0 {
			visible = strings.TrimSpace(response[:idx])
		}
		action := o.interviewRunner.Graph().Interviewer.ApplyDecision(state, decision)

		state.ChatHistory = append(state.ChatHistory, model.Message{
			Role:    model.RoleAssistant,
			Content: visible,
		})

		if o.memory != nil {
			o.memory.AppendConversation(ctx, sessionID, model.Message{
				Role:    model.RoleAssistant,
				Content: visible,
			})
		}

		// Run evaluation in background goroutine
		go func() {
			bgCtx := context.Background()
			o.interviewRunner.EvaluateAnswer(bgCtx, state)
			o.setState(sessionID, state)
			o.interviewRunner.SaveCheckpoint(bgCtx, state)

			if action == "complete" {
				if o.memory != nil && state.FinalReport != nil {
					o.memory.UpdateSessionStatus(bgCtx, sessionID, model.PhaseCompleted)
					o.memory.SaveInterviewResult(bgCtx, state.FinalReport, state.ReviewPlan)
				}
				if o.memHierarchy != nil {
					o.memHierarchy.ArchiveSessionSummary(bgCtx, sessionID, state.ChatHistory)
				}
			}
		}()
	}()

	return reader, nil
}

// CompleteInterview forces the interview to end early and generates the final report.
func (o *Orchestrator) CompleteInterview(ctx context.Context, sessionID string) (*interaction.InterviewEvent, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Force completion
	state.Phase = model.PhaseCompleted
	state.NextAction = "complete"

	// Save state to memory immediately and return response to the user.
	// Run evaluation and persistence asynchronously with background context
	// so they survive HTTP timeout/cancellation.
	o.setState(sessionID, state)

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		o.interviewRunner.EvaluateAnswer(bgCtx, state)

		if state.FinalReport == nil {
			state.FinalReport = o.buildReportFromState(state)
		}

		// Generate review plan alongside the report so it's persisted once,
		// not re-generated on every GetReviewPlan query.
		if state.ReviewPlan == nil && len(state.Evaluations) > 0 {
			if err := o.interviewRunner.Graph().ReviewPlanning.ExecuteLightweight(bgCtx, state); err != nil {
				log.Printf("[CompleteInterview] Warning: review plan generation failed: %v", err)
			}
		}

		o.setState(sessionID, state)
		o.interviewRunner.SaveCheckpoint(bgCtx, state)

		if o.memory != nil {
			o.memory.UpdateSessionStatus(bgCtx, sessionID, model.PhaseCompleted)
			if err := o.memory.SaveInterviewResult(bgCtx, state.FinalReport, state.ReviewPlan); err != nil {
				log.Printf("[CompleteInterview] Warning: failed to persist report: %v", err)
			}
		}
		if o.memHierarchy != nil {
			o.memHierarchy.ArchiveSessionSummary(bgCtx, sessionID, state.ChatHistory)
		}
	}()

	return &interaction.InterviewEvent{
		Type:      "complete",
		SessionID: sessionID,
		Data:      "面试已结束，报告即将生成。",
	}, nil
}

func (o *Orchestrator) SkipQuestion(ctx context.Context, sessionID string) (*interaction.InterviewEvent, error) {
	state, err := o.getOrRestoreState(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	state.CurrentQIndex++
	state.CurrentFollowUp = 0
	state.CurrentQuestion = nil
	o.setState(sessionID, state)

	if err := o.interviewRunner.SaveCheckpoint(ctx, state); err != nil {
		log.Printf("Warning: checkpoint save after skip failed: %v", err)
	}

	return o.StartInterview(ctx, sessionID)
}

func (o *Orchestrator) GetReport(ctx context.Context, sessionID string) (*model.Report, error) {
	// Try in-memory state first
	state, err := o.getState(sessionID)
	if err == nil {
		if state.FinalReport == nil {
			state.FinalReport = o.buildReportFromState(state)
			o.setState(sessionID, state)
		}
		if state.FinalReport != nil {
			return state.FinalReport, nil
		}
	}

	// Fall back to checkpoint
	state, err = o.interviewRunner.LoadCheckpoint(ctx, sessionID)
	if err == nil {
		if state.FinalReport == nil {
			state.FinalReport = o.buildReportFromState(state)
			o.setState(sessionID, state)
		}
		if state.FinalReport != nil {
			return state.FinalReport, nil
		}
	}

	// Final fallback: long-term database
	if o.memory != nil {
		report, err := o.memory.GetInterviewResult(ctx, sessionID)
		if err == nil && report != nil {
			return report, nil
		}
		log.Printf("[GetReport] DB fallback failed for session %s: %v", sessionID, err)
	}

	return nil, fmt.Errorf("report not found for session %s — the interview may not have been completed or the data has expired", sessionID)
}

func (o *Orchestrator) GetReviewPlan(ctx context.Context, sessionID string) (*model.ReviewPlan, error) {
	// Try in-memory state first
	state, err := o.getState(sessionID)
	if err == nil && state.ReviewPlan != nil {
		return state.ReviewPlan, nil
	}

	// Fall back to checkpoint
	state, err = o.interviewRunner.LoadCheckpoint(ctx, sessionID)
	if err == nil && state != nil && state.ReviewPlan != nil {
		return state.ReviewPlan, nil
	}

	// Final fallback: long-term database
	if o.memory != nil {
		plan, err := o.memory.GetReviewPlanFromDB(ctx, sessionID)
		if err == nil && plan != nil {
			return plan, nil
		}
		log.Printf("[GetReviewPlan] DB fallback failed for session %s: %v", sessionID, err)
	}

	return nil, fmt.Errorf("review plan not found for session %s", sessionID)
}

// GetContextStats returns aggregated context usage stats across all sessions.
func (o *Orchestrator) GetContextStats(ctx context.Context) (*interaction.ContextStats, error) {
	if o.ctxMonitor == nil {
		return &interaction.ContextStats{}, nil
	}
	snap := o.ctxMonitor.Snapshot()
	return &interaction.ContextStats{
		TotalCalls:      snap.TotalCalls,
		AvgUsagePercent: snap.AvgUsagePercent,
		MaxUsagePercent: snap.MaxUsagePercent,
		WarningCount:    snap.WarningCount,
		CriticalCount:   snap.CriticalCount,
	}, nil
}

// GetSessionContextStats returns context usage stats for a specific session.
func (o *Orchestrator) GetSessionContextStats(ctx context.Context, sessionID string) (*interaction.ContextStats, error) {
	if o.ctxMonitor == nil {
		return &interaction.ContextStats{}, nil
	}
	snap := o.ctxMonitor.SnapshotSession(sessionID)
	return &interaction.ContextStats{
		TotalCalls:      snap.TotalCalls,
		AvgUsagePercent: snap.AvgUsagePercent,
		MaxUsagePercent: snap.MaxUsagePercent,
		WarningCount:    snap.WarningCount,
		CriticalCount:   snap.CriticalCount,
	}, nil
}

// buildReportFromState constructs a report from in-memory evaluations.
func (o *Orchestrator) buildReportFromState(state *nodes.InterviewState) *model.Report {
	dimensionTotals := make(map[string]float64)
	dimensionCounts := make(map[string]int)
	var totalScore float64

	for _, eval := range state.Evaluations {
		totalScore += eval.TotalScore
		for _, dim := range eval.Dimensions {
			dimensionTotals[dim.Name] += dim.Score
			dimensionCounts[dim.Name]++
		}
	}

	dimensionAvg := make(map[string]float64)
	for name, total := range dimensionTotals {
		if dimensionCounts[name] > 0 {
			dimensionAvg[name] = total / float64(dimensionCounts[name])
		}
	}

	overallScore := float64(0)
	if len(state.Evaluations) > 0 {
		overallScore = totalScore / float64(len(state.Evaluations))
	}

	var weakAreas []string
	for name, avg := range dimensionAvg {
		if avg < 6.0 {
			weakAreas = append(weakAreas, name)
		}
	}

	var highlights []string
	for _, eval := range state.Evaluations {
		if eval.TotalScore >= 8.0 {
			highlights = append(highlights, fmt.Sprintf("Q%s: scored %.1f - %s",
				eval.QuestionID, eval.TotalScore, eval.Feedback))
		}
	}

	score100 := overallScore * 10

	questionReviews := make([]string, 0, len(state.Evaluations))
	for _, eval := range state.Evaluations {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Q%s (%.0f分)", eval.QuestionID, eval.TotalScore*10))
		if eval.Praise != "" {
			sb.WriteString(fmt.Sprintf("\n👍 亮点：%s", eval.Praise))
		}
		if eval.Issues != "" {
			sb.WriteString(fmt.Sprintf("\n⚠️ 不足：%s", eval.Issues))
		}
		if eval.Improvement != "" {
			sb.WriteString(fmt.Sprintf("\n💡 建议：%s", eval.Improvement))
		}
		questionReviews = append(questionReviews, sb.String())
	}

	return &model.Report{
		SessionID:       state.SessionID,
		OverallScore:    overallScore,
		Score100:        score100,
		DimensionScore:  dimensionAvg,
		Evaluations:     state.Evaluations,
		Highlights:      highlights,
		WeakAreas:       weakAreas,
		QuestionReviews: questionReviews,
		Summary:         fmt.Sprintf("Interview completed. Overall score: %.2f. %d areas need improvement.", overallScore, len(weakAreas)),
	}
}

func (o *Orchestrator) HandleMessage(ctx context.Context, sessionID string, msg string) (*interaction.MessageResponse, error) {
	var history []model.Message
	if o.memory != nil && sessionID != "" {
		history, _ = o.memory.GetConversationContext(ctx, sessionID, 10)
	}

	// Pre-parse skill:name:input prefix to bypass LLM classification for skill messages.
	var response string
	var intent string

	if parts := strings.SplitN(msg, ":", 3); len(parts) == 3 && parts[0] == "skill" {
		skillName := parts[1]
		actualInput := parts[2]
		intent = string(router.IntentSkillPractice)

		resp, err := o.HandleSkill(ctx, sessionID, skillName, actualInput, "")
		if err != nil {
			return &interaction.MessageResponse{
				Intent: "error",
				Reply:  fmt.Sprintf("Sorry, I encountered an error: %v", err),
			}, err
		}
		response = resp
	} else {
		var err error
		response, err = o.intentRouter.Route(ctx, sessionID, msg, history)
		if err != nil {
			return &interaction.MessageResponse{
				Intent: "error",
				Reply:  fmt.Sprintf("Sorry, I encountered an error: %v", err),
			}, err
		}

		result, _ := o.intentRouter.Classify(ctx, sessionID, msg, history)
		if result != nil {
			intent = string(result.Intent)
		}
	}

	// Persist both user message and assistant response atomically so the next
	// request sees the full turn.
	if o.memory != nil && sessionID != "" {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleUser,
			Content: msg,
		})
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleAssistant,
			Content: response,
		})
		o.memory.UpdateSessionStatus(ctx, sessionID, model.PhaseActive)
	}

	return &interaction.MessageResponse{
		Intent: intent,
		Reply:  response,
	}, nil
}

func (o *Orchestrator) Subscribe(ctx context.Context, sessionID string) (<-chan *interaction.InterviewEvent, error) {
	ch := o.events.subscribe(sessionID)
	go func() {
		<-ctx.Done()
		o.events.unsubscribe(sessionID, ch)
	}()
	return ch, nil
}

// --- Delegate methods for Intent Router specialists ---

// CreateSessionStr implements router.InterviewServiceDelegate.
func (o *Orchestrator) CreateSessionStr(ctx context.Context, userID, jdText string) (string, error) {
	req := interaction.CreateSessionReq{UserID: userID, JDText: jdText}
	session, err := o.CreateSession(ctx, req)
	if err != nil {
		return "", err
	}
	return session.ID, nil
}

// ParseJDStr implements router.InterviewServiceDelegate.
func (o *Orchestrator) ParseJDStr(ctx context.Context, sessionID string, rawJD string) (string, error) {
	analysis, err := o.ParseJD(ctx, sessionID, rawJD)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("JD Analysis complete. Position: %s, Level: %s, Tech Stack: %v",
		analysis.Position, analysis.Level, analysis.TechStack), nil
}

// SubmitAnswerStr implements router.InterviewServiceDelegate.
func (o *Orchestrator) SubmitAnswerStr(ctx context.Context, sessionID string, answer string) (string, error) {
	event, err := o.SubmitAnswer(ctx, sessionID, answer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", event.Data), nil
}

// GetReportStr implements router.InterviewServiceDelegate.
func (o *Orchestrator) GetReportStr(ctx context.Context, sessionID string) (string, error) {
	report, err := o.GetReport(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Interview Report - Overall Score: %.2f. Weak Areas: %v. %s",
		report.OverallScore, report.WeakAreas, report.Summary), nil
}

// GetReviewPlanStr implements router.InterviewServiceDelegate.
func (o *Orchestrator) GetReviewPlanStr(ctx context.Context, sessionID string) (string, error) {
	plan, err := o.GetReviewPlan(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Review plan has %d items for %d weak areas", len(plan.PlanItems), len(plan.WeakAreas)), nil
}

// HandleSkill implements router.SkillServiceDelegate.
func (o *Orchestrator) HandleSkill(ctx context.Context, sessionID string, subIntent string, input string, ragDocuments string) (string, error) {
	// Empty input means the user just started the skill — return welcome message.
	if input == "" || input == "start" {
		if welcome := o.skillRegistry.Welcome(subIntent); welcome != "" {
			return welcome, nil
		}
	}

	// Augment with MCP search results
	mcpDocs := o.searchMCPForSkill(ctx, subIntent, input)
	if mcpDocs != "" {
		if ragDocuments != "" {
			ragDocuments += "\n\n" + mcpDocs
		} else {
			ragDocuments = mcpDocs
		}
	}

	resp, err := o.skillRegistry.Dispatch(ctx, sessionID, subIntent, input, ragDocuments)
	if err != nil {
		return "", err
	}
	msg := resp.Message
	if resp.NextPrompt != "" {
		msg += "\n\n" + resp.NextPrompt
	}
	return msg, nil
}

// ListSkills implements router.SkillServiceDelegate.
func (o *Orchestrator) ListSkills(ctx context.Context) (map[string]string, error) {
	return o.skillRegistry.List(), nil
}

// UploadDocuments implements interaction.InterviewService.
func (o *Orchestrator) UploadDocuments(ctx context.Context, files []interaction.UploadFile) (*interaction.UploadResult, error) {
	if o.docIngestor == nil {
		return nil, fmt.Errorf("document ingestion service not available — check vector/keyword backend configuration")
	}

	result := &interaction.UploadResult{
		TotalFiles: len(files),
		Files:      make([]string, 0),
		Errors:     make([]string, 0),
	}
	for _, f := range files {
		ingestResult, err := o.docIngestor.Ingest(ctx, f.FileName, f.Content)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", f.FileName, err))
			continue
		}
		result.TotalChunks += ingestResult.Chunks
		result.Files = append(result.Files, f.FileName)
	}

	return result, nil
}

// ListDocuments implements interaction.InterviewService.
func (o *Orchestrator) ListDocuments(ctx context.Context) ([]interaction.DocInfo, error) {
	if o.docIngestor == nil {
		return []interaction.DocInfo{}, nil
	}
	entries, err := o.docIngestor.ListDocuments(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]interaction.DocInfo, len(entries))
	for i, e := range entries {
		result[i] = interaction.DocInfo{ID: e.SourceFile, SourceFile: e.SourceFile}
	}
	return result, nil
}

// StreamMessage implements interaction.InterviewService.
func (o *Orchestrator) StreamMessage(ctx context.Context, sessionID string, msg string) (*schema.StreamReader[*schema.Message], error) {
	log.Printf("[stream] StreamMessage called, casualAgent=%v, hybridRetriever=%v", o.casualAgent != nil, o.hybridRetriever != nil)

	// Use ReAct Agent for casual chat when MCP tools are available
	if o.casualAgent != nil {
		log.Printf("[stream] using agent path")
		return o.streamViaAgent(ctx, sessionID, msg)
	}

	log.Printf("[stream] using fallback LLM path")
	// Save user message so it appears in history on the next request.
	if o.memory != nil && sessionID != "" {
		o.memory.AppendConversation(ctx, sessionID, model.Message{Role: model.RoleUser, Content: msg})
	}

	// Fallback: direct LLM stream with RAG injection
	systemPrompt := "## 角色\n你是 InterviewAgent，一个专注于职业发展与面试准备的 AI 助手。\n\n## 行为准则\n- 回答简洁、友好、专业\n- 使用与用户相同的语言\n- 对于技术问题，尽量给出具体、可操作的答案\n- 不要编造信息，不确定时如实说明"

	ragDocs := ""
	if o.hybridRetriever != nil {
		docs, err := o.hybridRetriever.Retrieve(ctx, msg, retriever.WithTopK(3))
		if err == nil && len(docs) > 0 {
			ragDocs = rag.FormatDocuments(docs)
		}
	}

	var history []model.Message
	if o.memory != nil && sessionID != "" {
		history, _ = o.memory.GetConversationContext(ctx, sessionID, 6)
	}

	var messages []*schema.Message
	if o.ctxBuilder != nil {
		messages = o.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    sessionID,
			ProfileName:  "stream_fallback",
			SystemPrompt: systemPrompt,
			History:      history,
			RAGDocuments: ragDocs,
			UserInput:    msg,
		})
	} else {
		if ragDocs != "" {
			systemPrompt += "\n\n## Reference Knowledge\nUse the following reference documents to inform your answer if relevant:\n" + ragDocs
		}
		messages = []*schema.Message{schema.SystemMessage(systemPrompt)}
		for _, m := range history {
			messages = append(messages, &schema.Message{Role: schema.RoleType(m.Role), Content: m.Content})
		}
		messages = append(messages, schema.UserMessage(msg))
	}

	stream, err := o.chatModel.Stream(ctx, messages)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, fmt.Errorf("chat model returned nil stream")
	}
	return o.wrapStreamForMemory(ctx, sessionID, stream), nil
}

// wrapStreamForMemory collects streamed content and saves it as an AI response.
func (o *Orchestrator) wrapStreamForMemory(ctx context.Context, sessionID string, inner *schema.StreamReader[*schema.Message]) *schema.StreamReader[*schema.Message] {
	if o.memory == nil || sessionID == "" {
		return inner
	}
	reader, writer := schema.Pipe[*schema.Message](64)
	go func() {
		defer writer.Close()
		defer inner.Close()
		var full strings.Builder
		for {
			msg, err := inner.Recv()
			if err != nil {
				break
			}
			if msg != nil {
				full.WriteString(msg.Content)
				writer.Send(msg, nil)
			}
		}
		if full.Len() > 0 {
			o.memory.AppendConversation(ctx, sessionID, model.Message{Role: model.RoleAssistant, Content: full.String()})
			o.memory.UpdateSessionStatus(ctx, sessionID, model.PhaseActive)
		}
	}()
	return reader
}

// streamViaAgent uses the ReAct Agent (LLM + MCP tools) for streaming casual chat.
func (o *Orchestrator) streamViaAgent(ctx context.Context, sessionID string, msg string) (*schema.StreamReader[*schema.Message], error) {
	log.Printf("[stream-agent] building prompt")
	systemPrompt := `## 角色定义
你是 InterviewAgent，一个专注于职业发展与面试准备的 AI 助手。你配备了搜索工具，可以实时查询 GitHub 仓库和网络信息。

## 工作范围
- 回答技术问题、职业发展咨询、面试准备等问题
- 当用户询问具体技术信息（代码示例、文档、开源项目、事实数据）时，必须先使用搜索工具获取最新信息
- 如果用户想进行模拟面试或技能练习，引导他们使用对应功能模块

## 工具使用权限
- GitHub 搜索：查找开源项目、README、代码示例
- 网络搜索：查找教程、技术文档、最新资讯
- 重要：对具体技术问题，必须优先使用工具获取真实信息，不得凭空编造

## 边界限制
- 不得编造信息或假装知道不确定的内容——使用工具验证
- 不得以"I cannot..."回避可以通过工具解决的问题
- 不得提供医疗、法律、金融等专业建议
- 不得执行任何修改用户系统或文件的请求

## 行为准则
- 使用与用户相同的语言回复
- 回复简洁、友好、有帮助
- 技术回答应具体、可操作，引用搜索到的实际信息`

	// RAG injection
	log.Printf("[stream-agent] RAG: hybridRetriever=%v", o.hybridRetriever != nil)
	ragDocs := ""
	if o.hybridRetriever != nil {
		log.Printf("[stream-agent] calling hybridRetriever.Retrieve")
		docs, err := o.hybridRetriever.Retrieve(ctx, msg, retriever.WithTopK(3))
		log.Printf("[stream-agent] Retrieve done: err=%v, docs=%d", err, len(docs))
		if err == nil && len(docs) > 0 {
			ragDocs = rag.FormatDocuments(docs)
		}
	}

	log.Printf("[stream-agent] memory=%v session=%q", o.memory != nil, sessionID)
	var history []model.Message
	if o.memory != nil && sessionID != "" {
		history, _ = o.memory.GetConversationContext(ctx, sessionID, 6)
		log.Printf("[stream-agent] history len=%d", len(history))
	}

	var einoMsgs []*schema.Message
	if o.ctxBuilder != nil {
		einoMsgs = o.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    sessionID,
			ProfileName:  "stream_agent",
			SystemPrompt: systemPrompt,
			History:      history,
			RAGDocuments: ragDocs,
			UserInput:    msg,
		})
	} else {
		if ragDocs != "" {
			systemPrompt += "\n\n## Reference Knowledge\n" + ragDocs
		}
		einoMsgs = []*schema.Message{schema.SystemMessage(systemPrompt)}
		for _, m := range history {
			switch m.Role {
			case "user":
				einoMsgs = append(einoMsgs, schema.UserMessage(m.Content))
			case "assistant":
				einoMsgs = append(einoMsgs, schema.AssistantMessage(m.Content, nil))
			}
		}
		einoMsgs = append(einoMsgs, schema.UserMessage(msg))
	}

	// Save user message to conversation history
	if o.memory != nil && sessionID != "" {
		o.memory.AppendConversation(ctx, sessionID, model.Message{Role: model.RoleUser, Content: msg})
	}

	// Use agent.Generate (not Stream) because DeepSeek outputs text before
	// tool calls, which the default StreamToolCallChecker cannot handle.
	// Apply a 30s timeout — the agent may loop tool calls and exceed the
	// browser's fetch timeout, causing "failed to fetch" on the frontend.
	log.Printf("[stream-agent] calling agent.Generate(%d msgs)", len(einoMsgs))
	agentCtx, agentCancel := context.WithTimeout(ctx, 30*time.Second)
	defer agentCancel()
	resp, err := o.casualAgent.Generate(agentCtx, einoMsgs)
	if err != nil {
		log.Printf("[stream-agent] agent.Generate failed, falling back to direct LLM: %v", err)
		return o.streamFallback(ctx, sessionID, msg, systemPrompt, ragDocs, history)
	}
	if resp == nil {
		return nil, fmt.Errorf("agent returned nil response")
	}
	// Wrap single response as a stream for the SSE handler
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		writer.Send(resp, nil)
		writer.Close()
	}()
	return o.wrapStreamForMemory(ctx, sessionID, reader), nil
}

// streamFallback is a direct LLM stream used when the agent times out or fails.
func (o *Orchestrator) streamFallback(ctx context.Context, sessionID string, msg string, systemPrompt string, ragDocs string, history []model.Message) (*schema.StreamReader[*schema.Message], error) {
	log.Printf("[stream-fallback] using direct LLM stream for session=%s", sessionID)

	var messages []*schema.Message
	if o.ctxBuilder != nil {
		messages = o.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    sessionID,
			ProfileName:  "stream_fallback",
			SystemPrompt: systemPrompt,
			History:      history,
			RAGDocuments: ragDocs,
			UserInput:    msg,
		})
	} else {
		if ragDocs != "" {
			systemPrompt += "\n\n## Reference Knowledge\n" + ragDocs
		}
		messages = []*schema.Message{schema.SystemMessage(systemPrompt)}
		for _, m := range history {
			messages = append(messages, &schema.Message{Role: schema.RoleType(m.Role), Content: m.Content})
		}
		messages = append(messages, schema.UserMessage(msg))
	}

	stream, err := o.chatModel.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("stream fallback failed: %w", err)
	}
	if stream == nil {
		return nil, fmt.Errorf("chat model returned nil stream")
	}
	return o.wrapStreamForMemory(ctx, sessionID, stream), nil
}

// ListSessions implements interaction.InterviewService.
func (o *Orchestrator) ListSessions(ctx context.Context, userID string) ([]interaction.SessionSummary, error) {
	if o.memory == nil {
		return []interaction.SessionSummary{}, nil
	}
	ms, err := o.memory.ListSessionSummaries(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]interaction.SessionSummary, len(ms))
	for i, m := range ms {
		result[i] = interaction.SessionSummary{
			ID:           m.ID,
			Status:       m.Status,
			OverallScore: m.OverallScore,
			CreatedAt:    m.CreatedAt,
			LastMessage:  m.LastMessage,
		}
	}
	return result, nil
}

// DeleteDocument implements interaction.InterviewService.
func (o *Orchestrator) DeleteDocument(ctx context.Context, docID string) error {
	if o.docIngestor == nil {
		return fmt.Errorf("document ingestion service not available")
	}
	return o.docIngestor.DeleteDocument(ctx, docID)
}

// searchMCPForSkill augments skill practice with MCP search results.
func (o *Orchestrator) searchMCPForSkill(ctx context.Context, subIntent string, input string) string {
	var parts []string
	query := fmt.Sprintf("%s %s", subIntent, input)

	if o.githubMCP != nil {
		repos, err := o.githubMCP.SearchRepositories(ctx, query, 3)
		if err == nil && len(repos) > 0 {
			parts = append(parts, "### GitHub Repositories\n"+mcp.FormatGitHubResults(repos))
		}
	}

	if o.webMCP != nil {
		webResults, err := o.webMCP.Search(ctx, query, 3)
		if err == nil && len(webResults) > 0 {
			parts = append(parts, "### Web Results\n"+mcp.FormatWebResults(webResults))
		}
	}

	if len(parts) > 0 {
		return "## External Resources (Real-time Search)\n" + strings.Join(parts, "\n")
	}
	return ""
}

// ListSkillInfos implements interaction.InterviewService.
func (o *Orchestrator) ListSkillInfos(ctx context.Context) ([]interaction.SkillInfo, error) {
	skills := o.skillRegistry.List()
	result := make([]interaction.SkillInfo, 0, len(skills))
	for name, desc := range skills {
		result = append(result, interaction.SkillInfo{Name: name, Description: desc, Category: o.skillRegistry.Category(name)})
	}
	return result, nil
}

// ListAvailableTools implements interaction.InterviewService.
// Uses EinoBridge to dynamically discover tools from MCP servers.
func (o *Orchestrator) ListAvailableTools(ctx context.Context) ([]interaction.ToolInfo, error) {
	if o.mcpBridge == nil {
		return nil, nil
	}
	summaries := o.mcpBridge.ListToolSummaries(ctx)
	tools := make([]interaction.ToolInfo, len(summaries))
	for i, s := range summaries {
		tools[i] = interaction.ToolInfo{
			Name:        s.Name,
			Server:      s.Server,
			Description: s.Description,
		}
	}
	return tools, nil
}

// ensure we implement the delegate interfaces
var _ router.InterviewServiceDelegate = (*Orchestrator)(nil)
var _ router.SkillServiceDelegate = (*Orchestrator)(nil)
