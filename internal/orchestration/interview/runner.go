package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/google/uuid"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/difficulty"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
)

const checkpointKeyPrefix = "checkpoint:"

type Runner struct {
	setupGraph        Runnable
	nodes             *NodeSet
	checkpointTTL     time.Duration
	diffUpThreshold   int
	diffDownThreshold int
	checkpointStore   compose.CheckPointStore
}

func NewRunner(setupGraph Runnable, ns *NodeSet, checkpointTTL time.Duration,
	diffUp, diffDown int, checkpointStore compose.CheckPointStore) *Runner {
	return &Runner{
		setupGraph:        setupGraph,
		nodes:             ns,
		checkpointTTL:     checkpointTTL,
		diffUpThreshold:   diffUp,
		diffDownThreshold: diffDown,
		checkpointStore:   checkpointStore,
	}
}

func (r *Runner) Graph() *NodeSet { return r.nodes }

func (r *Runner) NewState(sessionID string) *nodes.InterviewState {
	return &nodes.InterviewState{
		SessionID:     sessionID,
		Phase:         model.PhaseCreated,
		InterruptData: make(map[string]any),
		Difficulty:    difficulty.NewStateMachine(r.diffUpThreshold, r.diffDownThreshold),
	}
}

// RunSetup executes JD → Resume → Questions via the Eino Setup DAG.
func (r *Runner) RunSetup(ctx context.Context, state *nodes.InterviewState) error {
	_, err := r.setupGraph.Invoke(ctx, state)
	if err != nil {
		return fmt.Errorf("setup graph failed: %w", err)
	}
	state.Phase = model.PhaseInterviewing
	log.Printf("[Runner] Setup complete: %d questions planned (session=%s)", len(state.QuestionQueue), state.SessionID)
	return nil
}

// AskCurrentQuestion calls the Interviewer node directly.
func (r *Runner) AskCurrentQuestion(ctx context.Context, state *nodes.InterviewState) (string, error) {
	return r.nodes.Interviewer.AskQuestion(ctx, state)
}

// ProcessAnswer calls Interviewer → Evaluation nodes directly.
func (r *Runner) ProcessAnswer(ctx context.Context, state *nodes.InterviewState, answer string) (action string, response string, err error) {
	action, response, err = r.nodes.Interviewer.ProcessAnswer(ctx, state, answer)
	if err != nil {
		return "", "", err
	}
	return action, response, nil
}

// EvaluateAnswer runs evaluation and review planning in the background.
// Should be called after ProcessAnswer has returned the response to the user.
func (r *Runner) EvaluateAnswer(ctx context.Context, state *nodes.InterviewState) {
	if state.NextAction == "next_question" || state.NextAction == "complete" ||
		state.Phase == model.PhaseCompleted || state.CurrentQIndex >= len(state.QuestionQueue) {
		if evalErr := r.nodes.Evaluation.Execute(ctx, state); evalErr != nil {
			log.Printf("[Runner] Evaluation warning: %v", evalErr)
		} else {
			r.adjustDifficulty(state)
		}
	}

	if state.NextAction == "complete" || state.Phase == model.PhaseCompleted || state.CurrentQIndex >= len(state.QuestionQueue) {
		if planErr := r.nodes.ReviewPlanning.ExecuteLightweight(ctx, state); planErr != nil {
			log.Printf("[Runner] Review planning warning: %v", planErr)
		}
	}
}

func (r *Runner) adjustDifficulty(state *nodes.InterviewState) {
	if state.Difficulty == nil || len(state.Evaluations) == 0 {
		return
	}

	lastEval := state.Evaluations[len(state.Evaluations)-1]
	isCorrect := lastEval.TotalScore >= 6.0

	if isCorrect {
		level, changed := state.Difficulty.RecordCorrect()
		if changed {
			log.Printf("[Runner] Difficulty increased to %s (session=%s)", level, state.SessionID)
		}
		state.StreakCorrect++
		state.StreakWrong = 0
	} else {
		level, changed := state.Difficulty.RecordWrong()
		if changed {
			log.Printf("[Runner] Difficulty decreased to %s (session=%s)", level, state.SessionID)
		}
		state.StreakCorrect = 0
		state.StreakWrong++
	}
}

// CreateState creates a new state with a checkpoint ID.
func (r *Runner) CreateState(sessionID string) *nodes.InterviewState {
	state := r.NewState(sessionID)
	state.CheckpointID = uuid.New().String()
	return state
}

// SaveCheckpoint persists the current state to the checkpoint store.
func (r *Runner) SaveCheckpoint(ctx context.Context, state *nodes.InterviewState) error {
	if r.checkpointStore == nil {
		return nil
	}
	if state == nil {
		return fmt.Errorf("cannot save nil state")
	}

	key := checkpointKeyPrefix + state.SessionID
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint state: %w", err)
	}

	if err := r.checkpointStore.Set(ctx, key, data); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	log.Printf("[Runner] Checkpoint saved for session %s (key=%s, ttl=%v)", state.SessionID, key, r.checkpointTTL)
	return nil
}

// LoadCheckpoint retrieves a previously saved state from the checkpoint store.
func (r *Runner) LoadCheckpoint(ctx context.Context, sessionID string) (*nodes.InterviewState, error) {
	if r.checkpointStore == nil {
		return nil, fmt.Errorf("checkpoint store not available")
	}

	key := checkpointKeyPrefix + sessionID
	data, found, err := r.checkpointStore.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint for session %s: %w", sessionID, err)
	}

	if !found || data == nil {
		return nil, fmt.Errorf("no checkpoint found for session %s", sessionID)
	}

	var state nodes.InterviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	// Ensure maps are initialized (nil maps can result from omitempty in older checkpoints)
	if state.InterruptData == nil {
		state.InterruptData = make(map[string]any)
	}

	log.Printf("[Runner] Checkpoint loaded for session %s (phase=%s, q=%d/%d)",
		sessionID, state.Phase, state.CurrentQIndex+1, len(state.QuestionQueue))
	return &state, nil
}

// GetReport retrieves the report from a saved checkpoint.
func (r *Runner) GetReport(ctx context.Context, sessionID string) (*nodes.InterviewState, error) {
	state, err := r.LoadCheckpoint(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if state.FinalReport == nil {
		// Try to build from evaluations if report wasn't generated yet
		if len(state.Evaluations) > 0 {
			state.FinalReport = buildReportFromEvals(state)
			return state, nil
		}
		return nil, fmt.Errorf("no report available for session %s", sessionID)
	}

	return state, nil
}

func buildReportFromEvals(state *nodes.InterviewState) *model.Report {
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

func (r *Runner) InvokeInterview(ctx context.Context, sessionID string, resumeAnswer string, state *nodes.InterviewState) (string, string, bool, error) {
	if resumeAnswer != "" {
		action, response, err := r.ProcessAnswer(ctx, state, resumeAnswer)
		isComplete := action == "complete" || state.Phase == model.PhaseCompleted || state.CurrentQIndex >= len(state.QuestionQueue)
		return action, response, isComplete, err
	}
	question, err := r.AskCurrentQuestion(ctx, state)
	return "question", question, false, err
}

func (r *Runner) InvokeSetup(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return nil, r.RunSetup(ctx, state)
}

var _ compose.CheckPointStore = nil // ensure import
