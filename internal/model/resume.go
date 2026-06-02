package model

// ResumeMatch represents the matching result between a resume and JD.
type ResumeMatch struct {
	OverallScore    float64            `json:"overall_score"`
	DimensionScores map[string]float64 `json:"dimension_scores"`
	Strengths       []string           `json:"strengths"`
	Gaps            []string           `json:"gaps"`
	ResumeSummary   string             `json:"resume_summary"`
}
