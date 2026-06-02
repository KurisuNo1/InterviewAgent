package model

// JDAnalysis represents the structured output of JD parsing.
type JDAnalysis struct {
	Position   string   `json:"position"`
	Level      string   `json:"level"`
	TechStack  []string `json:"tech_stack"`
	CoreSkills []string `json:"core_skills"`
	NiceToHave []string `json:"nice_to_have"`
	Experience int      `json:"experience_years"`
	Degree     string   `json:"degree"`
	RawText    string   `json:"raw_text,omitempty"`
	SourceURL  string   `json:"source_url,omitempty"`
}
