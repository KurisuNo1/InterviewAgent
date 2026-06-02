package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// Host is the intent router. It uses an LLM to classify user intent and
// dispatches to the appropriate specialist.
type Host struct {
	chatModel   llm.ChatModel
	specialists map[Intent]Specialist
}

// NewHost creates a new intent routing host.
func NewHost(chatModel llm.ChatModel) *Host {
	return &Host{
		chatModel:   chatModel,
		specialists: make(map[Intent]Specialist),
	}
}

// Register adds a specialist for a specific intent.
func (h *Host) Register(intent Intent, specialist Specialist) {
	h.specialists[intent] = specialist
}

// Classify determines the intent of a user message.
func (h *Host) Classify(ctx context.Context, sessionID string, message string) (*ClassificationResult, error) {
	prompt := []llm.Message{
		{Role: "system", Content: intentClassificationPrompt},
		{Role: "user", Content: message},
	}

	resp, err := h.chatModel.Chat(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("intent classification failed: %w", err)
	}

	var result ClassificationResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// Fallback: treat as casual chat if classification fails
		return &ClassificationResult{
			Intent:     IntentCasualChat,
			Confidence: 0.5,
		}, nil
	}

	return &result, nil
}

// Route classifies the message and dispatches to the matching specialist.
func (h *Host) Route(ctx context.Context, sessionID string, message string, history []model.Message) (string, error) {
	result, err := h.Classify(ctx, sessionID, message)
	if err != nil {
		return "", err
	}

	specialist, ok := h.specialists[result.Intent]
	if !ok {
		// Fall back to casual chat
		if chat, ok2 := h.specialists[IntentCasualChat]; ok2 {
			return chat.Handle(ctx, sessionID, message, result.Extracted)
		}
		return "I'm not sure how to help with that. Could you try rephrasing?", nil
	}

	return specialist.Handle(ctx, sessionID, message, result.Extracted)
}

const intentClassificationPrompt = `You are an intent classifier for an AI interview system.
Analyze the user's message and classify it into one of three intents:

1. "interview" - User wants to start or continue a job interview session.
   - SubIntent: "create" (new interview), "answer" (responding to a question)
2. "skill_practice" - User wants to practice specific skills.
   - SubIntent: "algorithm", "system_design", "behavioral", "tech_quiz"
3. "casual_chat" - User is just chatting or asking general questions.

Respond with ONLY a JSON object (no markdown, no extra text):
{
  "intent": "<interview|skill_practice|casual_chat>",
  "confidence": <0.0-1.0>,
  "sub_intent": "<specific subtype>",
  "extracted": {"key": "value"}
}`
