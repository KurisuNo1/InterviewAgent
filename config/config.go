package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the top-level application configuration.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	LLM       LLMConfig       `mapstructure:"llm"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	VectorDB  VectorDBConfig  `mapstructure:"vector_db"`
	Keyword   KeywordConfig   `mapstructure:"keyword_index"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Redis     RedisConfig     `mapstructure:"redis"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	Interview InterviewConfig `mapstructure:"interview"`
	RAG       RAGConfig       `mapstructure:"rag"`
	Memory    MemoryConfig    `mapstructure:"memory"`
	Context   ContextConfig   `mapstructure:"context"`
	Skills    []SkillConfig   `mapstructure:"skills"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Upload    UploadConfig    `mapstructure:"upload"`
	WeChat    WeChatConfig    `mapstructure:"wechat"`
}

// ServerConfig holds HTTP/WebSocket server settings.
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	WSPath       string        `mapstructure:"ws_path"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	JWTSecret    string        `mapstructure:"jwt_secret"`
	JWTExpiry    time.Duration `mapstructure:"jwt_expiry"`
}

// LLMConfig holds LLM provider settings.
type LLMConfig struct {
	Provider    string        `mapstructure:"provider"`
	BaseURL     string        `mapstructure:"base_url"`
	APIKeyEnv   string        `mapstructure:"api_key_env"`
	Model       string        `mapstructure:"model"`
	Temperature float64       `mapstructure:"temperature"`
	MaxTokens   int           `mapstructure:"max_tokens"`
	Timeout     time.Duration `mapstructure:"timeout"`
	MaxRetries  int           `mapstructure:"max_retries"`
}

// EmbeddingConfig holds embedding provider settings.
type EmbeddingConfig struct {
	Provider   string        `mapstructure:"provider"`
	BaseURL    string        `mapstructure:"base_url"`
	APIKeyEnv  string        `mapstructure:"api_key_env"`
	Model      string        `mapstructure:"model"`
	Dimensions int           `mapstructure:"dimensions"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

// VectorDBConfig holds vector database settings.
type VectorDBConfig struct {
	Type       string `mapstructure:"type"`
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	Username   string `mapstructure:"username"`
	Password   string `mapstructure:"password"`
	Database   string `mapstructure:"database"`
	Collection string `mapstructure:"collection"`
	Dimension  int    `mapstructure:"dimension"`
	IndexType  string `mapstructure:"index_type"`
	MetricType string `mapstructure:"metric_type"`
}

// KeywordConfig holds keyword index settings.
type KeywordConfig struct {
	Type      string `mapstructure:"type"`
	IndexPath string `mapstructure:"index_path"`
}

// MCPConfig holds MCP server configuration.
type MCPConfig struct {
	ConnectionTimeout time.Duration    `mapstructure:"connection_timeout"`
	Servers           []MCPServerEntry `mapstructure:"servers"`
}

// MCPServerEntry defines a single MCP server.
type MCPServerEntry struct {
	Name    string            `mapstructure:"name"`
	Command string            `mapstructure:"command"`
	Args    []string          `mapstructure:"args"`
	Env     map[string]string `mapstructure:"env"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// MySQLConfig holds MySQL connection settings.
type MySQLConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	PasswordEnv     string `mapstructure:"password_env"`
	Database        string `mapstructure:"database"`
	Charset         string `mapstructure:"charset"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"`
}

// InterviewConfig holds interview flow settings.
type InterviewConfig struct {
	MaxQuestions         int                `mapstructure:"max_questions"`
	MaxFollowUps         int                `mapstructure:"max_follow_ups"`
	TimePerQuestion      int                `mapstructure:"time_per_question"`
	ScoringDimensions    []ScoringDimConfig `mapstructure:"scoring_dimensions"`
	CheckpointTTL        time.Duration      `mapstructure:"checkpoint_ttl"`
	DifficultyUpThreshold   int             `mapstructure:"difficulty_up_threshold"`
	DifficultyDownThreshold int             `mapstructure:"difficulty_down_threshold"`
}

// ScoringDimConfig defines a single scoring dimension.
type ScoringDimConfig struct {
	Name        string  `mapstructure:"name"`
	Weight      float64 `mapstructure:"weight"`
	Description string  `mapstructure:"description"`
	MaxScore    float64 `mapstructure:"max_score"`
}

// RAGConfig holds hybrid search settings.
type RAGConfig struct {
	VectorWeight  float64 `mapstructure:"vector_weight"`
	KeywordWeight float64 `mapstructure:"keyword_weight"`
	TopK          int     `mapstructure:"top_k"`
	FinalTopK     int     `mapstructure:"final_top_k"`
}

// MemoryConfig holds memory system settings.
type MemoryConfig struct {
	ShortTerm ShortTermMemConfig `mapstructure:"short_term"`
	LongTerm  LongTermMemConfig  `mapstructure:"long_term"`
}

// ShortTermMemConfig holds short-term memory settings.
type ShortTermMemConfig struct {
	MaxMessages int           `mapstructure:"max_messages"`
	TTL         time.Duration `mapstructure:"ttl"`
}

// LongTermMemConfig holds long-term memory settings.
type LongTermMemConfig struct {
	MaxHistory int `mapstructure:"max_history"`
}

// ContextConfig holds LLM context window management settings.
type ContextConfig struct {
	MaxTokens          int                       `mapstructure:"max_tokens"`
	WarningThreshold   float64                   `mapstructure:"warning_threshold"`
	CriticalThreshold  float64                   `mapstructure:"critical_threshold"`
	Profiles           map[string]ContextProfile `mapstructure:"profiles"`
}

// ContextProfile defines token allocation for a specific LLM call path.
type ContextProfile struct {
	SystemMax           int `mapstructure:"system_max"`
	WorkingMemory       int `mapstructure:"working_memory"`
	RAGMax              int `mapstructure:"rag_max"`
	RecentVerbatimTurns int `mapstructure:"recent_verbatim_turns"`
	HistoryMaxTurns     int `mapstructure:"history_max_turns"`
	CompressionThreshold int `mapstructure:"compression_threshold_turns"`
}

// SkillConfig defines a single skill module.
type SkillConfig struct {
	Name      string `mapstructure:"name"`
	SubIntent string `mapstructure:"sub_intent"`
	Enabled   bool   `mapstructure:"enabled"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

// UploadConfig holds document upload settings.
type UploadConfig struct {
	MaxFileSize  int64 `mapstructure:"max_file_size"`
	ChunkSize    int   `mapstructure:"chunk_size"`
	ChunkOverlap int   `mapstructure:"chunk_overlap"`
}

// WeChatConfig holds WeChat Mini-Program login settings.
type WeChatConfig struct {
	AppID     string `mapstructure:"app_id"`
	AppSecret string `mapstructure:"app_secret"`
}

// LoadEnv reads a .env file (if present) and sets environment variables.
// Lines starting with # are treated as comments. Empty lines are skipped.
func LoadEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip inline comment (e.g. "value  # comment" -> "value")
		if idx := strings.Index(val, " #"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		if val == "" {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// Load reads and parses the configuration file.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand ${ENV_VAR} placeholders in string config values
	cfg.expandEnvVars()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars replaces ${VAR} and ${VAR:-default} patterns with environment variable values.
func (c *Config) expandEnvVars() {
	c.WeChat.AppID = expandEnv(c.WeChat.AppID)
	c.WeChat.AppSecret = expandEnv(c.WeChat.AppSecret)
	c.Server.JWTSecret = expandEnv(c.Server.JWTSecret)
}

// expandEnv replaces ${VAR} and ${VAR:-default} patterns in s with env var values.
func expandEnv(s string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	return os.Expand(s, func(key string) string {
		// Handle ${VAR:-default} syntax
		if idx := strings.Index(key, ":-"); idx >= 0 {
			varName := key[:idx]
			defaultVal := key[idx+2:]
			if val := os.Getenv(varName); val != "" {
				return val
			}
			return defaultVal
		}
		return os.Getenv(key)
	})
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port must be set")
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("llm.base_url must be set")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model must be set")
	}
	if c.Interview.MaxQuestions <= 0 {
		return fmt.Errorf("interview.max_questions must be positive")
	}
	return nil
}
