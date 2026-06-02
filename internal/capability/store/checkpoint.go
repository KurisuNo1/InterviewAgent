package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CheckpointStore implements Eino's compose.CheckPointStore interface backed by Redis.
// The interface contract is:
//
//	Get(ctx, checkPointID string) ([]byte, bool, error)
//	Set(ctx, checkPointID string, checkPoint []byte) error
type CheckpointStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
}

// CheckpointConfig holds configuration for the checkpoint store.
type CheckpointConfig struct {
	KeyPrefix string
	TTL       time.Duration
}

// NewCheckpointStore creates a Redis-backed Eino CheckPointStore.
func NewCheckpointStore(client *redis.Client, cfg CheckpointConfig) *CheckpointStore {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "ckpt:"
	}
	return &CheckpointStore{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		ttl:       cfg.TTL,
	}
}

// Get retrieves checkpoint data. Returns (nil, false, nil) if not found.
func (s *CheckpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	key := s.keyPrefix + checkPointID
	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("checkpoint get failed: %w", err)
	}
	return data, true, nil
}

// Set persists checkpoint data with TTL.
func (s *CheckpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	key := s.keyPrefix + checkPointID
	return s.client.Set(ctx, key, checkPoint, s.ttl).Err()
}
