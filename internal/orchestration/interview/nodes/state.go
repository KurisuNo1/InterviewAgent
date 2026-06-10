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

// InterviewEvent represents a single step result in the interview loop.
type InterviewEvent struct {
	Action     string               `json:"action"`
	Response   string               `json:"response"`
	Phase      model.InterviewPhase `json:"phase"`
	IsComplete bool                 `json:"is_complete"`
}
