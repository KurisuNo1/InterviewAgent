package model

// ScoreDimension represents a single scoring dimension.
type ScoreDimension struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	MaxScore    float64 `json:"max_score"`
	Weight      float64 `json:"weight"`
}

// Evaluation represents the scoring result for a single question.
type Evaluation struct {
	QuestionID   string           `json:"question_id"`
	Dimensions   []ScoreDimension `json:"dimensions"`
	TotalScore   float64          `json:"total_score"`
	Feedback     string           `json:"feedback"`
	IsCorrect    bool             `json:"is_correct"`
	Praise       string           `json:"praise,omitempty"`     // specific strengths in this answer
	Issues       string           `json:"issues,omitempty"`     // specific problems or gaps
	Improvement  string           `json:"improvement,omitempty"` // actionable advice to improve
	KeyTakeaway  string           `json:"key_takeaway,omitempty"` // single most important learning point
}
