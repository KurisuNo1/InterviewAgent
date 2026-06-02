package model

// Report represents the final interview evaluation report.
type Report struct {
	SessionID      string             `json:"session_id"`
	OverallScore   float64            `json:"overall_score"`
	DimensionScore map[string]float64 `json:"dimension_score"`
	Evaluations    []Evaluation       `json:"evaluations"`
	Highlights     []string           `json:"highlights"`
	WeakAreas      []string           `json:"weak_areas"`
	Summary        string             `json:"summary"`
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
