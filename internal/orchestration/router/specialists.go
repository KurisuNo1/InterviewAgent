package router

import (
	"context"
	"fmt"
)

// InterviewServiceDelegate is the minimal interface an interview specialist needs
// from the orchestrator to handle interview-related messages.
type InterviewServiceDelegate interface {
	CreateSessionStr(ctx context.Context, userID, jdText string) (string, error)
	ParseJDStr(ctx context.Context, sessionID string, rawJD string) (string, error)
	SubmitAnswerStr(ctx context.Context, sessionID string, answer string) (string, error)
	GetReportStr(ctx context.Context, sessionID string) (string, error)
	GetReviewPlanStr(ctx context.Context, sessionID string) (string, error)
}

// SkillServiceDelegate is the minimal interface for skill-related messages.
type SkillServiceDelegate interface {
	HandleSkill(ctx context.Context, sessionID string, subIntent string, input string) (string, error)
	ListSkills(ctx context.Context) ([]string, error)
}

// InterviewSpecialist handles interview intent messages.
type InterviewSpecialist struct {
	service InterviewServiceDelegate
}

// NewInterviewSpecialist creates a new interview specialist.
func NewInterviewSpecialist(service InterviewServiceDelegate) *InterviewSpecialist {
	return &InterviewSpecialist{service: service}
}

func (s *InterviewSpecialist) Name() string        { return "interview" }
func (s *InterviewSpecialist) Description() string  { return "Handles interview session operations" }
func (s *InterviewSpecialist) CanHandle(intent Intent, subIntent string) bool {
	return intent == IntentInterview
}

func (s *InterviewSpecialist) Handle(ctx context.Context, sessionID string, input string, metadata map[string]string) (string, error) {
	subIntent := ""
	if metadata != nil {
		subIntent = metadata["sub_intent"]
	}

	switch subIntent {
	case "create":
		jdText := ""
		if metadata != nil {
			jdText = metadata["jd_text"]
		}
		sessionID, err := s.service.CreateSessionStr(ctx, "", jdText)
		if err != nil {
			return "", fmt.Errorf("failed to create session: %w", err)
		}
		return fmt.Sprintf("Interview session created: %s. You can now paste a job description to get started.", sessionID), nil

	case "answer":
		if sessionID == "" {
			return "Please provide your session ID. You can create a new session by saying 'start a new interview'.", nil
		}
		response, err := s.service.SubmitAnswerStr(ctx, sessionID, input)
		if err != nil {
			return "", fmt.Errorf("failed to process answer: %w", err)
		}
		return response, nil

	default:
		return `I can help you with:
- Create a new interview session: just tell me the job description
- Answer interview questions: type your answer directly
- Get your report: ask for "my report"
Which would you like to do?`, nil
	}
}

// SkillPracticeSpecialist handles skill_practice intent messages.
type SkillPracticeSpecialist struct {
	service SkillServiceDelegate
}

// NewSkillPracticeSpecialist creates a new skill practice specialist.
func NewSkillPracticeSpecialist(service SkillServiceDelegate) *SkillPracticeSpecialist {
	return &SkillPracticeSpecialist{service: service}
}

func (s *SkillPracticeSpecialist) Name() string        { return "skill_practice" }
func (s *SkillPracticeSpecialist) Description() string  { return "Handles skill practice sessions" }
func (s *SkillPracticeSpecialist) CanHandle(intent Intent, subIntent string) bool {
	return intent == IntentSkillPractice
}

func (s *SkillPracticeSpecialist) Handle(ctx context.Context, sessionID string, input string, metadata map[string]string) (string, error) {
	subIntent := ""
	if metadata != nil {
		subIntent = metadata["sub_intent"]
	}
	if subIntent == "" {
		subIntent = "list"
	}

	if subIntent == "list" {
		skills, err := s.service.ListSkills(ctx)
		if err != nil {
			return "", err
		}
		msg := "Available practice modules:\n"
		for _, name := range skills {
			msg += fmt.Sprintf("  - %s\n", name)
		}
		msg += "\nType the name of a module to start. For example: 'algorithm' or 'system_design'"
		return msg, nil
	}

	response, err := s.service.HandleSkill(ctx, sessionID, subIntent, input)
	if err != nil {
		return "", fmt.Errorf("skill practice failed: %w", err)
	}
	return response, nil
}
