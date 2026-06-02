package ws

import "encoding/json"

// WSMessage is the incoming WebSocket message envelope.
type WSMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// WSEvent is the outgoing WebSocket event envelope.
type WSEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id"`
	Data      interface{} `json:"data"`
	Streaming bool        `json:"streaming"`
}

// Message type constants.
const (
	TypeStart    = "start"
	TypeAnswer   = "answer"
	TypeSkip     = "skip"
	TypeResume   = "resume"
	TypeQuestion = "question"
	TypeEval     = "evaluation"
	TypeReport   = "report"
	TypeReview   = "review_plan"
	TypeError    = "error"
	TypePing     = "ping"
	TypePong     = "pong"
)
