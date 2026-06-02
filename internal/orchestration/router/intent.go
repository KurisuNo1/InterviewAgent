package router

// Intent represents the classified user intent.
type Intent string

const (
	IntentInterview     Intent = "interview"
	IntentSkillPractice Intent = "skill_practice"
	IntentCasualChat    Intent = "casual_chat"
)

// ClassificationResult is the output of intent classification.
type ClassificationResult struct {
	Intent     Intent            `json:"intent"`
	Confidence float64           `json:"confidence"`
	SubIntent  string            `json:"sub_intent,omitempty"`
	Extracted  map[string]string `json:"extracted,omitempty"`
}
