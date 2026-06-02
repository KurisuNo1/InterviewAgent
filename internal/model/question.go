package model

// Question represents a single interview question.
type Question struct {
	ID            string   `json:"id"`
	Content       string   `json:"content"`
	Category      string   `json:"category"`
	Difficulty    string   `json:"difficulty"`
	Tags          []string `json:"tags"`
	ReferenceAns  string   `json:"reference_answer,omitempty"`
	ScoringPoints []string `json:"scoring_points,omitempty"`
	Source        string   `json:"source"`
}

// QuestionPlan represents the planned question distribution.
type QuestionPlan struct {
	TotalQuestions int                `json:"total_questions"`
	Categories     []QuestionCategory `json:"categories"`
	Questions      []Question         `json:"questions"`
}

// QuestionCategory represents a category with its question count and difficulty distribution.
type QuestionCategory struct {
	Name      string  `json:"name"`
	Count     int     `json:"count"`
	EasyPct   float64 `json:"easy_pct"`
	MediumPct float64 `json:"medium_pct"`
	HardPct   float64 `json:"hard_pct"`
}

// Answer represents a user's answer to a question.
type Answer struct {
	QuestionID string   `json:"question_id"`
	Content    string   `json:"content"`
	FollowUps  []string `json:"follow_ups,omitempty"`
}
