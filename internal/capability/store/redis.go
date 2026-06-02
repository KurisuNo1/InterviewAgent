package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps Redis operations.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	GetJSON(ctx context.Context, key string, dest any) error
	LPush(ctx context.Context, key string, values ...string) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LTrim(ctx context.Context, key string, start, stop int64) error
}

// RedisStore implements RedisClient using go-redis.
type RedisStore struct {
	client *redis.Client
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Host         string
	Port         int
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Client returns the underlying Redis client for use with Eino CheckpointStore etc.
func (r *RedisStore) Client() *redis.Client {
	return r.client
}

// NewRedisStore creates a new Redis client wrapper.
func NewRedisStore(cfg RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// Get retrieves a string value.
func (r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Set stores a string value with TTL.
func (r *RedisStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Del removes one or more keys.
func (r *RedisStore) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Expire sets a TTL on an existing key.
func (r *RedisStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// SetJSON stores a value as JSON with TTL.
func (r *RedisStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// GetJSON retrieves a JSON value.
func (r *RedisStore) GetJSON(ctx context.Context, key string, dest any) error {
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// LPush prepends values to a list.
func (r *RedisStore) LPush(ctx context.Context, key string, values ...string) error {
	vals := make([]interface{}, len(values))
	for i, v := range values {
		vals[i] = v
	}
	return r.client.LPush(ctx, key, vals...).Err()
}

// LRange retrieves a range of list elements.
func (r *RedisStore) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

// LTrim trims a list to the specified range.
func (r *RedisStore) LTrim(ctx context.Context, key string, start, stop int64) error {
	return r.client.LTrim(ctx, key, start, stop).Err()
}
