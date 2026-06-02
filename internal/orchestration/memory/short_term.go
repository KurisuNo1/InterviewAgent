package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// ShortTermMemory manages conversation messages in Redis.
type ShortTermMemory struct {
	redis     store.RedisClient
	maxMsgs   int
	ttl       time.Duration
	keyPrefix string
}

// ShortTermConfig holds configuration for short-term memory.
type ShortTermConfig struct {
	MaxMessages int
	TTL         time.Duration
	KeyPrefix   string
}

// NewShortTermMemory creates a new short-term memory store.
func NewShortTermMemory(redis store.RedisClient, cfg ShortTermConfig) *ShortTermMemory {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "conv:"
	}
	return &ShortTermMemory{
		redis:     redis,
		maxMsgs:   cfg.MaxMessages,
		ttl:       cfg.TTL,
		keyPrefix: cfg.KeyPrefix,
	}
}

func (s *ShortTermMemory) sessionKey(sessionID string) string {
	return s.keyPrefix + sessionID
}

// Append adds a message to the conversation history.
func (s *ShortTermMemory) Append(ctx context.Context, sessionID string, msg model.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	key := s.sessionKey(sessionID)
	if err := s.redis.LPush(ctx, key, string(data)); err != nil {
		return err
	}
	if err := s.redis.LTrim(ctx, key, 0, int64(s.maxMsgs-1)); err != nil {
		return err
	}
	if s.ttl > 0 {
		return s.redis.Expire(ctx, key, s.ttl)
	}
	return nil
}

// GetRecent returns the last N messages for a session.
func (s *ShortTermMemory) GetRecent(ctx context.Context, sessionID string, n int) ([]model.Message, error) {
	key := s.sessionKey(sessionID)
	raw, err := s.redis.LRange(ctx, key, 0, int64(n-1))
	if err != nil {
		return nil, err
	}

	msgs := make([]model.Message, 0, len(raw))
	// Redis list returns in reverse order (newest first), so reverse
	for i := len(raw) - 1; i >= 0; i-- {
		var msg model.Message
		if err := json.Unmarshal([]byte(raw[i]), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Clear removes all messages for a session.
func (s *ShortTermMemory) Clear(ctx context.Context, sessionID string) error {
	return s.redis.Del(ctx, s.sessionKey(sessionID))
}
