package model

// Report represents the final interview evaluation report.
type Report struct {
	SessionID       string             `json:"session_id"`
	OverallScore    float64            `json:"overall_score"`
	Score100        float64            `json:"score_100"`
	Grade           string             `json:"grade"`
	DimensionScore  map[string]float64 `json:"dimension_score"`
	Evaluations     []Evaluation       `json:"evaluations"`
	Highlights      []string           `json:"highlights"`
	WeakAreas       []string           `json:"weak_areas"`
	Summary         string             `json:"summary"`
	QuestionReviews []string           `json:"question_reviews,omitempty"` // per-question detailed reviews
	OverallAdvice   string             `json:"overall_advice,omitempty"`   // dimension commentary
}

// ReviewPlan represents a personalized study plan.
type ReviewPlan struct {
	SessionID string       `json:"session_id"`
	WeakAreas []string     `json:"weak_areas"`
	PlanItems []ReviewItem `json:"plan_items"`
	Resources []Resource   `json:"resources"`
}

// ReviewItem represents a single study item in the review plan.
type ReviewItem struct {
	Topic          string  `json:"topic"`
	Priority       string  `json:"priority"`
	EstimatedHours float64 `json:"estimated_hours"`
	Description    string  `json:"description"`
}

// Resource represents a learning resource recommended for review.
type Resource struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Source      string `json:"source"`
}
