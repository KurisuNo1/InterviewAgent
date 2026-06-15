package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"net/http"

	"github.com/KurisuNo1/InterviewAgent/config"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/keyword"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/mcp"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/vector"

	openaiembed "github.com/cloudwego/eino-ext/components/embedding/openai"
	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/rest"
	auth2 "github.com/KurisuNo1/InterviewAgent/internal/interaction/rest/auth"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/ws"
	"github.com/KurisuNo1/InterviewAgent/internal/observability"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/agent"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/ingestion"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview"
	interviewNodes "github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/memory"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/rag"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/router"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/skill"
)

type App struct {
	Config       *config.Config
	Orchestrator *orchestration.Orchestrator
	WSHub        *ws.Hub
	RESTRouter   http.Handler
	closers      []func()
}

func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

func Wire(cfg *config.Config) (*App, error) {
	ctx := context.Background()

	observability.RegisterEinoCallbacks()

	// Layer 3: Basic Capability

	// --- LLM ---
	apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("environment variable %s is not set", cfg.LLM.APIKeyEnv)
	}
	maxTokens := cfg.LLM.MaxTokens
	temp := float32(cfg.LLM.Temperature)
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      apiKey,
		Model:       cfg.LLM.Model,
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init LLM: %w", err)
	}
	log.Printf("[wire] LLM initialized (model=%s)", cfg.LLM.Model)

	// --- Embedding ---
	var embedder embedding.Embedder
	{
		embAPIKey := os.Getenv(cfg.Embedding.APIKeyEnv)
		if embAPIKey != "" {
			e, eErr := openaiembed.NewEmbedder(ctx, &openaiembed.EmbeddingConfig{
				BaseURL:    cfg.Embedding.BaseURL,
				APIKey:     embAPIKey,
				Model:      cfg.Embedding.Model,
				Dimensions: &cfg.Embedding.Dimensions,
			})
			if eErr != nil {
				log.Printf("[wire] WARNING: Embedding init failed (%v)", eErr)
			} else {
				embedder = e
				log.Printf("[wire] Embedding ready (model=%s, dimensions=%d)", cfg.Embedding.Model, cfg.Embedding.Dimensions)
			}
		} else {
			log.Printf("[wire] WARNING: Embedding API key not set")
		}
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

	// --- Milvus ---
	var vectorStore *vector.MilvusStore
	/* vectorStore, err = vector.NewMilvusStore(ctx, vector.MilvusConfig{ */
	if cfg.VectorDB.Type == "none" || cfg.VectorDB.Type == "" {
		log.Printf("[wire] Milvus skipped (type=%s)", cfg.VectorDB.Type)
		vectorStore = nil
	} else {
		milvusCtx, milvusCancel := context.WithTimeout(ctx, 5*time.Second)
		defer milvusCancel()
		vectorStore, err = vector.NewMilvusStore(milvusCtx, vector.MilvusConfig{
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
			log.Printf("[wire] WARNING: Milvus init failed (%v)", err)
			vectorStore = nil
		} else {
			log.Printf("[wire] Milvus ready (%s:%d)", cfg.VectorDB.Host, cfg.VectorDB.Port)
		}
	}

	/* if err != nil {
		log.Printf("[wire] WARNING: Milvus init failed (%v)", err)
		vectorStore = nil
	} else {
		log.Printf("[wire] Milvus ready (%s:%d)", cfg.VectorDB.Host, cfg.VectorDB.Port)
	} */

	// --- Bleve (with timeout — BoltDB lock can block indefinitely) ---
	log.Printf("[wire] Starting Bleve index (timeout 10s)...")
	var bleveIndex *keyword.BleveIndex
	bleveDone := make(chan struct{})
	go func() {
		defer close(bleveDone)
		var bErr error
		bleveIndex, bErr = keyword.NewBleveIndex(keyword.BleveConfig{IndexPath: cfg.Keyword.IndexPath})
		if bErr != nil {
			log.Printf("[wire] WARNING: Bleve init failed (%v)", bErr)
			bleveIndex = nil
		}
	}()
	select {
	case <-bleveDone:
		if bleveIndex != nil {
			log.Printf("[wire] Bleve ready (%s)", cfg.Keyword.IndexPath)
		}
	case <-time.After(10 * time.Second):
		log.Printf("[wire] WARNING: Bleve init timed out — continuing without keyword search")
		bleveIndex = nil
	}

	// --- Tool Result Filter (must be created before MCP bridge) ---
	toolResultFilter := contextmanager.NewToolResultFilter(contextmanager.DefaultToolMetas())
	mcpResultFilter := mcp.ResultFilter(func(toolName string, result string) string {
		return toolResultFilter.Filter(toolName, result)
	})

	// --- MCP ---
	var mcpManager *mcp.Manager
	var mcpBridge *mcp.EinoBridge
	var githubMCP *mcp.GitHubMCP
	var webSearchMCP *mcp.WebSearchMCP
	if len(cfg.MCP.Servers) > 0 {
		log.Printf("[wire] Starting MCP servers (this may take a while on first run)...")
		mcpServers := make([]mcp.ServerConfig, 0, len(cfg.MCP.Servers))
		for _, s := range cfg.MCP.Servers {
			mcpServers = append(mcpServers, mcp.ServerConfig{
				Name: s.Name, Command: s.Command, Args: s.Args, Env: s.Env,
			})
		}
		mcpManager = mcp.NewManager(mcpServers)
		// Use a timeout context so MCP start doesn't block indefinitely
		mcpCtx, mcpCancel := context.WithTimeout(ctx, cfg.MCP.ConnectionTimeout)
		defer mcpCancel()
		if err := mcpManager.Start(mcpCtx); err != nil {
			log.Printf("[wire] WARNING: MCP start failed (%v) — continuing without MCP tools", err)
			mcpManager = nil
		} else {
			mcpBridge = mcp.NewEinoBridge(ctx, mcpManager, observability.NewToolCallbackHandler(), mcpResultFilter)
			githubMCP = mcp.NewGitHubMCP(mcpBridge)
			webSearchMCP = mcp.NewWebSearchMCP(mcpBridge)
			log.Printf("[wire] MCP bridge ready (%d servers)", len(cfg.MCP.Servers))
		}
	} else {
		log.Printf("[wire] No MCP servers configured, skipping")
	}

	// Layer 2: Orchestration

	// --- Memory ---
	shortTerm := memory.NewShortTermMemory(redisStore, memory.ShortTermConfig{
		MaxMessages: cfg.Memory.ShortTerm.MaxMessages, TTL: cfg.Memory.ShortTerm.TTL,
	})
	longTerm := memory.NewLongTermMemory(mysqlStore, memory.LongTermConfig{
		MaxHistory: cfg.Memory.LongTerm.MaxHistory,
	})
	memoryManager := memory.NewManager(shortTerm, longTerm)
	log.Printf("[wire] Memory ready")

	// --- Context Manager ---
	conversationCompressor := contextmanager.NewConversationCompressor(chatModel)
	ctxMonitor := contextmanager.NewDefaultMonitor("data/context_stats.json")
	ctxBuilder := contextmanager.NewContextBuilder(&cfg.Context, conversationCompressor, ctxMonitor)
	memHierarchy := contextmanager.NewMemoryHierarchy(memoryManager, conversationCompressor)
	overflowHandler := contextmanager.NewOverflowHandler(chatModel)
	log.Printf("[wire] ContextBuilder + MemoryHierarchy + ContextMonitor + OverflowHandler ready")

	// --- Hybrid RAG Retriever ---
	var hybridRetriever retriever.Retriever
	if vectorStore != nil || bleveIndex != nil {
		var vr, kr retriever.Retriever
		if vectorStore != nil {
			vr = vectorStore
		}
		if bleveIndex != nil {
			kr = bleveIndex
		}
		hybridRetriever, err = rag.NewHybridRetriever(ctx, vr, kr)
		if err != nil {
			log.Printf("[wire] WARNING: hybrid retriever failed (%v)", err)
		} else {
			log.Printf("[wire] Hybrid RAG ready (vector=%v, keyword=%v)", vr != nil, kr != nil)
		}
	}

	// --- Agent Factory ---
	thinker := agent.NewConsoleThinker()
	var agentFactory *agent.AgentFactory
	if mcpBridge != nil {
		mcpTools := mcpBridge.GetAllTools()
		baseTools := make([]tool.BaseTool, len(mcpTools))
		for i, t := range mcpTools {
			baseTools[i] = t
		}
		agentFactory = agent.NewAgentFactory(chatModel, baseTools, thinker)
	} else {
		agentFactory = agent.NewAgentFactory(chatModel, nil, thinker)
	}

	// --- ReAct Agents ---
	var casualAgent, interviewerAgent, evalAgent, reviewAgent *react.Agent
	if agentFactory != nil {
		casualAgent, err = agentFactory.NewAgent(ctx, "casual_chat", nil, 3)
		if err != nil {
			log.Printf("[wire] WARNING: casual agent failed (%v)", err)
		}
		interviewerAgent, err = agentFactory.NewAgent(ctx, "interviewer", nil, 3)
		if err != nil {
			log.Printf("[wire] WARNING: interviewer agent failed (%v)", err)
		}
		evalAgent, err = agentFactory.NewAgent(ctx, "evaluator", []string{"web_search"}, 3)
		if err != nil {
			log.Printf("[wire] WARNING: evaluator agent failed (%v)", err)
		}
		reviewAgent, err = agentFactory.NewAgent(ctx, "review_planner", []string{"web_search", "github_search"}, 3)
		if err != nil {
			log.Printf("[wire] WARNING: review planner agent failed (%v)", err)
		}
	}

	// --- Intent Router ---
	host := router.NewHost(chatModel, hybridRetriever, embedder, ctxMonitor)
	host.Register(router.IntentCasualChat, router.NewCasualChatSpecialist(casualAgent, chatModel, ctxBuilder))
	log.Printf("[wire] Intent router ready")

	// --- Checkpoint Store (shared) ---
	checkpointStore := store.NewCheckpointStore(redisStore.Client(), store.CheckpointConfig{
		KeyPrefix: "ckpt:", TTL: cfg.Interview.CheckpointTTL,
	})

	// --- Skill Registry (8 skills, with checkpoint persistence) ---
	skillRegistry := skill.NewRegistry(checkpointStore)
	skillRegistry.Register(skill.NewAlgorithmSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewSystemDesignSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewBehavioralSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewTechQuizSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewQuickQuizSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewKnowledgeExplainSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewProjectHighlightSkill(chatModel, ctxBuilder))
	skillRegistry.Register(skill.NewTechCompareSkill(chatModel, ctxBuilder))
	log.Printf("[wire] Skill registry: %d skills loaded", len(skillRegistry.List()))

	// --- Document Ingestion ---
	chunkSize := cfg.Upload.ChunkSize
	chunkOverlap := cfg.Upload.ChunkOverlap
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap <= 0 {
		chunkOverlap = 200
	}
	var docIngestor *ingestion.DocumentIngestor
	if vectorStore != nil || bleveIndex != nil {
		docIngestor = ingestion.NewDocumentIngestor(chunkSize, chunkOverlap, embedder, vectorStore, bleveIndex)
		log.Printf("[wire] Ingestion service ready")
	}

	// --- RAG Evaluator ---
	_ = rag.NewRAGEvaluator(chatModel)

	// --- Interview Graph ---
	jdNode := interviewNodes.NewJDAnalysisNode(chatModel)
	resumeNode := interviewNodes.NewResumeMatchingNode(chatModel)
	questionNode := interviewNodes.NewQuestionPlanningNode(chatModel, hybridRetriever, embedder)
	interviewerNode := interviewNodes.NewInterviewerNode(chatModel, cfg.Interview.MaxFollowUps, interviewerAgent, ctxBuilder)
	evalNode := interviewNodes.NewEvaluationNode(chatModel, evalAgent, ctxBuilder)
	reviewNode := interviewNodes.NewReviewPlanningNode(chatModel, githubMCP, webSearchMCP, reviewAgent)
	nodeSet := &interview.NodeSet{
		JDAnalysis:       jdNode,
		ResumeMatching:   resumeNode,
		QuestionPlanning: questionNode,
		Interviewer:      interviewerNode,
		Evaluation:       evalNode,
		ReviewPlanning:   reviewNode,
	}
	compiledGraph, err := interview.CompileSetupGraph(ctx, nodeSet)
	if err != nil {
		return nil, fmt.Errorf("compile graph: %w", err)
	}
	runner := interview.NewRunner(compiledGraph, nodeSet, cfg.Interview.CheckpointTTL,
		cfg.Interview.DifficultyUpThreshold, cfg.Interview.DifficultyDownThreshold, checkpointStore)
	log.Printf("[wire] Eino Graph compiled")

	// --- Orchestrator ---
	orchestrator := orchestration.NewOrchestrator(host, runner, skillRegistry, memoryManager, docIngestor,
		chatModel, hybridRetriever, embedder, githubMCP, webSearchMCP, mcpBridge, casualAgent, ctxBuilder, memHierarchy, overflowHandler, ctxMonitor)
	log.Printf("[wire] Orchestrator ready")

	// Layer 1: Interaction
	userStore := store.NewUserStore(mysqlStore)
	// Seed demo user if not exists
	if exists, _ := userStore.UserExists(ctx, "demo"); !exists {
		if err := userStore.CreateUser(ctx, "demo", "demo123"); err != nil {
			log.Printf("[wire] WARNING: failed to seed demo user: %v", err)
		}
	}
	jwtManager := auth2.NewJWTManager(cfg.Server.JWTSecret, cfg.Server.JWTExpiry)
	wsHub := ws.NewHub(orchestrator)
	ginRouter := rest.NewRouter(orchestrator, jwtManager, userStore, cfg.WeChat.AppID, cfg.WeChat.AppSecret)
	log.Printf("[wire] REST + WebSocket ready")

	app := &App{Config: cfg, Orchestrator: orchestrator, WSHub: wsHub, RESTRouter: ginRouter}
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

var _ interaction.InterviewService = (*orchestration.Orchestrator)(nil)
