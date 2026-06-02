package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"net/http"

	"github.com/KurisuNo1/InterviewAgent/config"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/embedding"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/keyword"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/llm"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/mcp"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/vector"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/rest"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/ws"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview"
	interviewNodes "github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/memory"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/rag"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/router"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/skill"
)

// App holds all wired-up components for the running application.
type App struct {
	Config       *config.Config
	Orchestrator *orchestration.Orchestrator
	WSHub        *ws.Hub
	RESTRouter   http.Handler // *gin.Engine via rest.NewRouter
	closers      []func()     // cleanup functions called on shutdown
}

// Close releases all resources acquired during wiring.
func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

// Wire builds the full dependency tree: L3 capabilities → L2 orchestration → L1 interaction.
// Optional dependencies (Milvus, MCP) are gracefully degraded if unavailable.
func Wire(cfg *config.Config) (*App, error) {
	ctx := context.Background()

	// ═══════════════════════════════════════════
	// Layer 3: Basic Capability
	// ═══════════════════════════════════════════

	// --- LLM (DeepSeek v4 via OpenAI-compatible API) ---
	chatModel, err := llm.NewDeepSeekChatModel(ctx, llm.DeepSeekConfig{
		BaseURL:     cfg.LLM.BaseURL,
		APIKeyEnv:   cfg.LLM.APIKeyEnv,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: float32(cfg.LLM.Temperature),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init DeepSeek LLM: %w", err)
	}
	log.Printf("[wire] DeepSeek LLM initialized (model=%s)", cfg.LLM.Model)

	// --- Embedding ---
	var embedder *embedding.OpenAIEmbedder
	embedder, err = embedding.NewOpenAIEmbedder(ctx, embedding.OpenAIEmbeddingConfig{
		BaseURL:    cfg.Embedding.BaseURL,
		APIKeyEnv:  cfg.Embedding.APIKeyEnv,
		Model:      cfg.Embedding.Model,
		Dimensions: cfg.Embedding.Dimensions,
	})
	if err != nil {
		log.Printf("[wire] WARNING: Embedding init failed (%v), RAG vector search disabled", err)
		embedder = nil
	} else {
		log.Printf("[wire] Embedding initialized (model=%s, dim=%d)", cfg.Embedding.Model, cfg.Embedding.Dimensions)
	}

	// --- Redis ---
	redisStore, err := store.NewRedisStore(store.RedisConfig{
		Host:         cfg.Redis.Host,
		Port:         cfg.Redis.Port,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init Redis: %w", err)
	}
	log.Printf("[wire] Redis connected (%s:%d)", cfg.Redis.Host, cfg.Redis.Port)

	// --- MySQL ---
	mysqlStore, err := store.NewMySQLStore(store.MySQLConfig{
		Host:            cfg.MySQL.Host,
		Port:            cfg.MySQL.Port,
		User:            cfg.MySQL.User,
		PasswordEnv:     cfg.MySQL.PasswordEnv,
		Database:        cfg.MySQL.Database,
		Charset:         cfg.MySQL.Charset,
		MaxOpenConns:    cfg.MySQL.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.MaxIdleConns,
		ConnMaxLifetime: parseDuration(cfg.MySQL.ConnMaxLifetime),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init MySQL: %w", err)
	}
	log.Printf("[wire] MySQL connected (%s:%d/%s)", cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)

	// --- Milvus (optional) ---
	var vectorStore *vector.MilvusStore
	vectorStore, err = vector.NewMilvusStore(ctx, vector.MilvusConfig{
		Host:       cfg.VectorDB.Host,
		Port:       cfg.VectorDB.Port,
		Username:   cfg.VectorDB.Username,
		Password:   cfg.VectorDB.Password,
		Database:   cfg.VectorDB.Database,
		Collection: cfg.VectorDB.Collection,
		Dimension:  cfg.VectorDB.Dimension,
		IndexType:  cfg.VectorDB.IndexType,
		MetricType: cfg.VectorDB.MetricType,
	})
	if err != nil {
		log.Printf("[wire] WARNING: Milvus init failed (%v), RAG vector search disabled", err)
		vectorStore = nil
	} else {
		log.Printf("[wire] Milvus connected (%s:%d, collection=%s)", cfg.VectorDB.Host, cfg.VectorDB.Port, cfg.VectorDB.Collection)
	}

	// --- Bleve BM25 ---
	bleveIndex, err := keyword.NewBleveIndex(keyword.BleveConfig{
		IndexPath: cfg.Keyword.IndexPath,
	})
	if err != nil {
		log.Printf("[wire] WARNING: Bleve init failed (%v), keyword search disabled", err)
		bleveIndex = nil
	} else {
		log.Printf("[wire] Bleve BM25 index ready (%s)", cfg.Keyword.IndexPath)
	}

	// --- MCP (optional) ---
	var mcpManager *mcp.Manager
	var githubMCP *mcp.GitHubMCP
	var webSearchMCP *mcp.WebSearchMCP
	if len(cfg.MCP.Servers) > 0 {
		mcpServers := make([]mcp.ServerConfig, 0, len(cfg.MCP.Servers))
		for _, s := range cfg.MCP.Servers {
			mcpServers = append(mcpServers, mcp.ServerConfig{
				Name:    s.Name,
				Command: s.Command,
				Args:    s.Args,
				Env:     s.Env,
			})
		}
		mcpManager = mcp.NewManager(mcpServers)
		if err := mcpManager.Start(ctx); err != nil {
			log.Printf("[wire] WARNING: MCP manager start failed (%v), MCP tools disabled", err)
			mcpManager = nil
		} else {
			githubMCP = mcp.NewGitHubMCP(mcpManager)
			webSearchMCP = mcp.NewWebSearchMCP(mcpManager)
			log.Printf("[wire] MCP manager started (%d servers)", len(cfg.MCP.Servers))
		}
	}

	// ═══════════════════════════════════════════
	// Layer 2: Orchestration
	// ═══════════════════════════════════════════

	// --- Memory ---
	shortTerm := memory.NewShortTermMemory(redisStore, memory.ShortTermConfig{
		MaxMessages: cfg.Memory.ShortTerm.MaxMessages,
		TTL:         cfg.Memory.ShortTerm.TTL,
	})
	longTerm := memory.NewLongTermMemory(mysqlStore, memory.LongTermConfig{
		MaxHistory: cfg.Memory.LongTerm.MaxHistory,
	})
	memoryManager := memory.NewManager(shortTerm, longTerm)
	log.Printf("[wire] Memory system ready (short=%d msgs, long=%d records)",
		cfg.Memory.ShortTerm.MaxMessages, cfg.Memory.LongTerm.MaxHistory)

	// --- Intent Router ---
	host := router.NewHost(chatModel)
	host.Register(router.IntentCasualChat, &casualChatSpecialist{})
	log.Printf("[wire] Intent router ready")

	// --- Skill Registry ---
	skillRegistry := skill.NewRegistry()
	if isSkillEnabled(cfg.Skills, "algorithm") {
		skillRegistry.Register(skill.NewAlgorithmSkill(chatModel))
	}
	if isSkillEnabled(cfg.Skills, "system_design") {
		skillRegistry.Register(skill.NewSystemDesignSkill(chatModel))
	}
	if isSkillEnabled(cfg.Skills, "behavioral") {
		skillRegistry.Register(skill.NewBehavioralSkill(chatModel))
	}
	if isSkillEnabled(cfg.Skills, "tech_quiz") {
		skillRegistry.Register(skill.NewTechQuizSkill(chatModel))
	}
	log.Printf("[wire] Skill registry: %d skills loaded", len(skillRegistry.List()))

	// --- Hybrid RAG Searcher ---
	hybridSearcher := rag.NewHybridSearcher(
		rag.NewVectorRetrieverAdapter(vectorStore),
		rag.NewKeywordSearcherAdapter(bleveIndex),
		chatModel,
		60,       // RRF k constant
		true,     // enable LLM rerank
	)
	log.Printf("[wire] Hybrid RAG searcher ready (RRF fusion + LLM rerank)")

	// --- RAG Evaluator ---
	_ = rag.NewRAGEvaluator(chatModel) // available for offline evaluation
	log.Printf("[wire] RAG evaluator ready (faithfulness/relevance/completeness)")

	// --- Interview Graph (6 agent nodes) ---
	jdNode := interviewNodes.NewJDAnalysisNode(chatModel)
	resumeNode := interviewNodes.NewResumeMatchingNode(chatModel)
	questionNode := interviewNodes.NewQuestionPlanningNode(chatModel, hybridSearcher, embedder)
	interviewerNode := interviewNodes.NewInterviewerNode(chatModel, cfg.Interview.MaxFollowUps)
	evalNode := interviewNodes.NewEvaluationNode(chatModel)
	reviewNode := interviewNodes.NewReviewPlanningNode(chatModel, githubMCP, webSearchMCP)

	// Build NodeSet for Eino Graph compilation
	nodeSet := &interview.NodeSet{
		JDAnalysis:      jdNode,
		ResumeMatching:  resumeNode,
		QuestionPlanning: questionNode,
		Interviewer:     interviewerNode,
		Evaluation:      evalNode,
		ReviewPlanning:  reviewNode,
	}

	// --- Checkpoint Store ---
	checkpointStore := store.NewCheckpointStore(redisStore.Client(), store.CheckpointConfig{
		KeyPrefix: "ckpt:",
		TTL:       cfg.Interview.CheckpointTTL,
	})
	log.Printf("[wire] Checkpoint store ready (Redis, ttl=%v)", cfg.Interview.CheckpointTTL)

	// Compile the Eino Setup DAG (linear pipeline, no checkpoints needed)
	compiledGraph, err := interview.CompileSetupGraph(ctx, nodeSet)
	if err != nil {
		return nil, fmt.Errorf("failed to compile Eino graph: %w", err)
	}

	// Create the Runner with the compiled Eino Graph
	runner := interview.NewRunner(compiledGraph, nodeSet, cfg.Interview.CheckpointTTL,
		cfg.Interview.DifficultyUpThreshold, cfg.Interview.DifficultyDownThreshold, checkpointStore)
	log.Printf("[wire] Eino Graph compiled (max_questions=%d, max_follow_ups=%d)",
		cfg.Interview.MaxQuestions, cfg.Interview.MaxFollowUps)

	// --- Orchestrator ---
	orchestrator := orchestration.NewOrchestrator(host, runner, skillRegistry, memoryManager)
	log.Printf("[wire] Orchestrator ready")

	// ═══════════════════════════════════════════
	// Layer 1: Interaction
	// ═══════════════════════════════════════════
	wsHub := ws.NewHub(orchestrator)
	ginRouter := rest.NewRouter(orchestrator)

	log.Printf("[wire] Application fully wired")
	log.Printf("[wire] REST API ready, WebSocket ready")

	app := &App{
		Config:       cfg,
		Orchestrator: orchestrator,
		WSHub:        wsHub,
		RESTRouter:   ginRouter,
	}

	// Register cleanup functions (executed in reverse order on shutdown)
	app.closers = append(app.closers, func() { wsHub.Close() })
	app.closers = append(app.closers, func() { mysqlStore.Close() })
	app.closers = append(app.closers, func() { redisStore.Client().Close() })
	if vectorStore != nil {
		app.closers = append(app.closers, func() { vectorStore.Close() })
	}
	if bleveIndex != nil {
		app.closers = append(app.closers, func() { bleveIndex.Close() })
	}
	if mcpManager != nil {
		app.closers = append(app.closers, func() { mcpManager.Close() })
	}

	return app, nil
}

// isSkillEnabled checks if a skill is present and enabled in config.
func isSkillEnabled(skills []config.SkillConfig, name string) bool {
	for _, s := range skills {
		if s.Name == name && s.Enabled {
			return true
		}
	}
	return false
}

// parseDuration safely parses a duration string, returning a default on failure.
func parseDuration(s string) time.Duration {
	if s == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// casualChatSpecialist provides a simple fallback for non-interview conversation.
type casualChatSpecialist struct{}

func (h *casualChatSpecialist) Name() string        { return "casual_chat" }
func (h *casualChatSpecialist) Description() string { return "Handles casual conversation" }
func (h *casualChatSpecialist) CanHandle(intent router.Intent, subIntent string) bool {
	return intent == router.IntentCasualChat
}
func (h *casualChatSpecialist) Handle(ctx context.Context, sessionID string, input string, metadata map[string]string) (string, error) {
	return "Hello! I'm the InterviewAgent. I can help you with:\n" +
		"- Technical interviews (create a session with a JD)\n" +
		"- Skill practice (algorithm, system design, behavioral, tech quiz)\n" +
		"- Review plans based on interview performance\n\n" +
		"What would you like to do?", nil
}

var _ router.Specialist = (*casualChatSpecialist)(nil)
var _ interaction.InterviewService = (*orchestration.Orchestrator)(nil)
