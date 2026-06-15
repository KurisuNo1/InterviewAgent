package nodes

import (
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/difficulty"
)

// InterviewState is the token that flows through the entire Eino Graph DAG.
// It is serialized for checkpoint persistence.
type InterviewState struct {
	SessionID string               `json:"session_id"`
	Phase     model.InterviewPhase `json:"phase"`

	// Phase outputs
	JDAnalysis   *model.JDAnalysis   `json:"jd_analysis,omitempty"`
	ResumeMatch  *model.ResumeMatch  `json:"resume_match,omitempty"`
	QuestionPlan *model.QuestionPlan `json:"question_plan,omitempty"`

	// RAG reference documents (populated by question planning, consumed by interviewer)
	RAGDocuments string `json:"rag_documents,omitempty"`

	// Interview loop state
	QuestionQueue   []model.Question `json:"question_queue"`
	CurrentQIndex   int              `json:"current_q_index"`
	CurrentFollowUp int              `json:"current_follow_up"`
	MaxFollowUps    int              `json:"max_follow_ups"`
	CurrentQuestion *model.Question  `json:"current_question,omitempty"`

	// Conversation history
	ChatHistory []model.Message `json:"chat_history"`

	// Compressed conversation state (for context window management)
	CompressedSummary    string `json:"compressed_summary,omitempty"`
	CompressedUpToRound  int    `json:"compressed_up_to_round"`

	// Results
	Answers     []model.Answer     `json:"answers,omitempty"`
	Evaluations []model.Evaluation `json:"evaluations,omitempty"`

	// Final outputs
	FinalReport *model.Report     `json:"final_report,omitempty"`
	ReviewPlan  *model.ReviewPlan `json:"review_plan,omitempty"`

	// Graph routing (set by interviewer node for branch)
	NextAction string `json:"next_action,omitempty"`

	// Dynamic difficulty
	Difficulty *difficulty.StateMachine `json:"difficulty,omitempty"`

	// Performance tracking
	StreakCorrect int `json:"streak_correct"`
	StreakWrong   int `json:"streak_wrong"`

	// Checkpoint metadata
	CheckpointID  string         `json:"checkpoint_id,omitempty"`
	InterruptData map[string]any `json:"interrupt_data"`
}

// ReleasePhaseOutputs clears large text fields that are no longer needed after
// a phase completes, freeing memory and preventing context bloat in later phases.
// Call this after each phase transition (JD parse → resume match → question plan → interview).
func (s *InterviewState) ReleasePhaseOutputs(currentPhase string) {
	switch currentPhase {
	case "jd_parsed":
		// After JD is parsed, the raw JD text in InterruptData is no longer needed
		// (structured JDAnalysis is sufficient for downstream nodes)
		if s.InterruptData != nil {
			delete(s.InterruptData, "jd_text")
		}
	case "resume_matched":
		// After resume matching, raw resume text can be released
		if s.InterruptData != nil {
			delete(s.InterruptData, "resume_text")
		}
	case "interview_started":
		// Before interview starts, we can trim RAG docs to only the most relevant parts
		// and clear the question plan's raw data that's already in the queue
		s.QuestionPlan = nil // questions are already in QuestionQueue
	}
}

// NeedsChatHistoryCompression returns true when the chat history has grown enough
// to warrant compression into the CompressedSummary field.
func (s *InterviewState) NeedsChatHistoryCompression(threshold int) bool {
	if threshold <= 0 {
		threshold = 20
	}
	return len(s.ChatHistory) > threshold && s.CompressedUpToRound < s.CurrentQIndex
}

// CompressChatHistory marks the current history as compressed, updating the summary marker.
// The actual compression is done by ContextBuilder when building prompts.
func (s *InterviewState) CompressChatHistory(summary string) {
	s.CompressedSummary = summary
	s.CompressedUpToRound = s.CurrentQIndex
}

// ChatHistoryForContext returns the appropriate history for context building:
// compressed summary + recent verbatim messages.
func (s *InterviewState) ChatHistoryForContext(recentTurns int) ([]model.Message, string) {
	if s.CompressedSummary == "" || s.CompressedUpToRound == 0 {
		return s.ChatHistory, ""
	}

	// Return last N turns verbatim + the compressed summary for older messages
	total := len(s.ChatHistory)
	keepFrom := total - recentTurns*2 // 2 messages per turn
	if keepFrom < 0 {
		keepFrom = 0
	}

	recentMsgs := s.ChatHistory[keepFrom:]
	return recentMsgs, s.CompressedSummary
}

// InterviewEvent represents a single step result in the interview loop.
type InterviewEvent struct {
	Action     string               `json:"action"`
	Response   string               `json:"response"`
	Phase      model.InterviewPhase `json:"phase"`
	IsComplete bool                 `json:"is_complete"`
}
