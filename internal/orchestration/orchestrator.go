package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/resume"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/memory"
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
) *Orchestrator {
	o := &Orchestrator{
		intentRouter:    intentRouter,
		interviewRunner: interviewRunner,
		skillRegistry:   skillRegistry,
		memory:          memoryManager,
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

func (o *Orchestrator) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	state, err := o.getState(sessionID)
	if err != nil {
		return nil, err
	}
	return &model.Session{ID: sessionID, Status: state.Phase}, nil
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
	state, err := o.getState(sessionID)
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
	state, err := o.getState(sessionID)
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
	state, err := o.getState(sessionID)
	if err != nil {
		return nil, err
	}
	if state.QuestionPlan == nil {
		return nil, fmt.Errorf("question plan not yet generated")
	}
	return state.QuestionPlan, nil
}

func (o *Orchestrator) StartInterview(ctx context.Context, sessionID string) (*interaction.InterviewEvent, error) {
	state, err := o.getState(sessionID)
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
				if err := o.interviewRunner.Graph().ReviewPlanning.Execute(ctx, state); err != nil {
					log.Printf("Warning: review planning on complete failed: %v", err)
				}
			}
			if o.memory != nil && state.FinalReport != nil {
				o.memory.UpdateSessionStatus(ctx, sessionID, model.PhaseCompleted)
				o.memory.SaveInterviewResult(ctx, state.FinalReport, state.ReviewPlan)
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

	state, err := o.getState(sessionID)
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
		// Persist final results to database immediately when interview ends
		if o.memory != nil && state.FinalReport != nil {
			if err := o.memory.UpdateSessionStatus(ctx, sessionID, model.PhaseCompleted); err != nil {
				log.Printf("Warning: failed to update session status: %v", err)
			}
			if err := o.memory.SaveInterviewResult(ctx, state.FinalReport, state.ReviewPlan); err != nil {
				log.Printf("Warning: failed to persist interview result on complete: %v", err)
			}
		}
	}

	// Publish event to WebSocket subscribers
	o.events.publish(sessionID, result)

	// Save checkpoint after each answer
	o.setState(sessionID, state)
	if err := o.interviewRunner.SaveCheckpoint(ctx, state); err != nil {
		log.Printf("Warning: checkpoint save failed for session %s: %v", sessionID, err)
	}

	return result, nil
}

func (o *Orchestrator) SkipQuestion(ctx context.Context, sessionID string) (*interaction.InterviewEvent, error) {
	state, err := o.getState(sessionID)
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
	if err != nil {
		// Fall back to checkpoint
		state, err = o.interviewRunner.LoadCheckpoint(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		// Sync back to memory
		o.setState(sessionID, state)
	}

	// Try to build report if not yet generated but evaluations exist
	if state.FinalReport == nil && len(state.Evaluations) > 0 {
		state.FinalReport = o.buildReportFromState(state)
		o.setState(sessionID, state)
	}

	if state.FinalReport == nil {
		return nil, fmt.Errorf("report not yet generated — please complete the interview first")
	}

	// Persist to long-term memory
	if o.memory != nil {
		if err := o.memory.SaveInterviewResult(ctx, state.FinalReport, state.ReviewPlan); err != nil {
			log.Printf("Warning: failed to save interview result: %v", err)
		}
	}

	return state.FinalReport, nil
}

func (o *Orchestrator) GetReviewPlan(ctx context.Context, sessionID string) (*model.ReviewPlan, error) {
	// Try in-memory state first
	state, err := o.getState(sessionID)
	if err != nil {
		// Fall back to checkpoint
		state, err = o.interviewRunner.LoadCheckpoint(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		o.setState(sessionID, state)
	}

	if state.ReviewPlan != nil {
		return state.ReviewPlan, nil
	}

	// Try to generate review plan on-the-fly if evaluations exist
	if len(state.Evaluations) > 0 {
		if state.FinalReport == nil {
			state.FinalReport = o.buildReportFromState(state)
		}
		if err := o.interviewRunner.Graph().ReviewPlanning.Execute(ctx, state); err != nil {
			log.Printf("Warning: on-the-fly review planning failed: %v", err)
			return nil, fmt.Errorf("review plan not yet available")
		}
		o.setState(sessionID, state)
		// Persist to database after on-the-fly generation
		if o.memory != nil {
			if err := o.memory.SaveInterviewResult(ctx, state.FinalReport, state.ReviewPlan); err != nil {
				log.Printf("Warning: failed to persist review plan: %v", err)
			}
		}
		return state.ReviewPlan, nil
	}

	return nil, fmt.Errorf("review plan not yet generated — please complete the interview first")
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

	return &model.Report{
		SessionID:      state.SessionID,
		OverallScore:   overallScore,
		DimensionScore: dimensionAvg,
		Evaluations:    state.Evaluations,
		Highlights:     highlights,
		WeakAreas:      weakAreas,
		Summary:        fmt.Sprintf("Interview completed. Overall score: %.2f. %d areas need improvement.", overallScore, len(weakAreas)),
	}
}

func (o *Orchestrator) HandleMessage(ctx context.Context, sessionID string, msg string) (*interaction.MessageResponse, error) {
	var history []model.Message
	if o.memory != nil && sessionID != "" {
		history, _ = o.memory.GetConversationContext(ctx, sessionID, 10)
	}

	response, err := o.intentRouter.Route(ctx, sessionID, msg, history)
	if err != nil {
		return &interaction.MessageResponse{
			Intent: "error",
			Reply:  fmt.Sprintf("Sorry, I encountered an error: %v", err),
		}, err
	}

	result, _ := o.intentRouter.Classify(ctx, sessionID, msg)

	if o.memory != nil && sessionID != "" {
		o.memory.AppendConversation(ctx, sessionID, model.Message{
			Role:    model.RoleAssistant,
			Content: response,
		})
	}

	return &interaction.MessageResponse{
		Intent: string(result.Intent),
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
func (o *Orchestrator) HandleSkill(ctx context.Context, sessionID string, subIntent string, input string) (string, error) {
	resp, err := o.skillRegistry.Dispatch(ctx, sessionID, subIntent, input)
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
func (o *Orchestrator) ListSkills(ctx context.Context) ([]string, error) {
	skills := o.skillRegistry.List()
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	return names, nil
}

// ensure we implement the delegate interfaces
var _ router.InterviewServiceDelegate = (*Orchestrator)(nil)
var _ router.SkillServiceDelegate = (*Orchestrator)(nil)
