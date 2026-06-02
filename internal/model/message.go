package model

// Role represents the role of a message sender.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single chat message.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}
