package difficulty

// Level represents the difficulty level of interview questions.
type Level string

const (
	LevelEasy   Level = "easy"
	LevelMedium Level = "medium"
	LevelHard   Level = "hard"
)

// Levels returns all difficulty levels in ascending order.
func Levels() []Level { return []Level{LevelEasy, LevelMedium, LevelHard} }

// StateMachine manages dynamic difficulty transitions during an interview.
// It tracks consecutive correct/wrong answers and adjusts level accordingly.
type StateMachine struct {
	CurrentLevel         Level `json:"current_level"`
	ConsecutiveCorrect   int   `json:"consecutive_correct"`
	ConsecutiveWrong     int   `json:"consecutive_wrong"`
	ThresholdUp          int   `json:"threshold_up"`
	ThresholdDown        int   `json:"threshold_down"`
}

// NewStateMachine creates a new difficulty state machine starting at medium.
func NewStateMachine(thresholdUp, thresholdDown int) *StateMachine {
	if thresholdUp <= 0 {
		thresholdUp = 2
	}
	if thresholdDown <= 0 {
		thresholdDown = 2
	}
	return &StateMachine{
		CurrentLevel:  LevelMedium,
		ThresholdUp:   thresholdUp,
		ThresholdDown: thresholdDown,
	}
}

// RecordCorrect records a correct answer and adjusts difficulty if thresholds are met.
func (m *StateMachine) RecordCorrect() (Level, bool) {
	m.ConsecutiveCorrect++
	m.ConsecutiveWrong = 0

	if m.ConsecutiveCorrect >= m.ThresholdUp {
		m.levelUp()
		m.ConsecutiveCorrect = 0
		return m.CurrentLevel, true
	}
	return m.CurrentLevel, false
}

// RecordWrong records a wrong answer and adjusts difficulty if thresholds are met.
func (m *StateMachine) RecordWrong() (Level, bool) {
	m.ConsecutiveWrong++
	m.ConsecutiveCorrect = 0

	if m.ConsecutiveWrong >= m.ThresholdDown {
		m.levelDown()
		m.ConsecutiveWrong = 0
		return m.CurrentLevel, true
	}
	return m.CurrentLevel, false
}

func (m *StateMachine) levelUp() {
	switch m.CurrentLevel {
	case LevelEasy:
		m.CurrentLevel = LevelMedium
	case LevelMedium:
		m.CurrentLevel = LevelHard
	}
}

func (m *StateMachine) levelDown() {
	switch m.CurrentLevel {
	case LevelHard:
		m.CurrentLevel = LevelMedium
	case LevelMedium:
		m.CurrentLevel = LevelEasy
	}
}

// GetDifficultyDistribution returns the recommended distribution of question difficulties
// based on the current level and total question count.
func (m *StateMachine) GetDifficultyDistribution(totalQuestions int) map[Level]int {
	if totalQuestions <= 0 {
		totalQuestions = 10
	}

	switch m.CurrentLevel {
	case LevelEasy:
		return map[Level]int{
			LevelEasy:   int(float64(totalQuestions) * 0.6),
			LevelMedium: int(float64(totalQuestions) * 0.3),
			LevelHard:   totalQuestions - int(float64(totalQuestions)*0.6) - int(float64(totalQuestions)*0.3),
		}
	case LevelMedium:
		return map[Level]int{
			LevelEasy:   int(float64(totalQuestions) * 0.2),
			LevelMedium: int(float64(totalQuestions) * 0.5),
			LevelHard:   totalQuestions - int(float64(totalQuestions)*0.2) - int(float64(totalQuestions)*0.5),
		}
	case LevelHard:
		return map[Level]int{
			LevelEasy:   int(float64(totalQuestions) * 0.1),
			LevelMedium: int(float64(totalQuestions) * 0.3),
			LevelHard:   totalQuestions - int(float64(totalQuestions)*0.1) - int(float64(totalQuestions)*0.3),
		}
	}
	return nil
}

// NextQuestionDifficulty returns the target difficulty for the next question
// based on current performance.
func (m *StateMachine) NextQuestionDifficulty() Level {
	return m.CurrentLevel
}

// Reset resets the state machine to starting level.
func (m *StateMachine) Reset() {
	m.CurrentLevel = LevelMedium
	m.ConsecutiveCorrect = 0
	m.ConsecutiveWrong = 0
}
