package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/embedding"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/rag"
)

// Host is the intent router. It uses an LLM to classify user intent and
// dispatches to the appropriate specialist.
type Host struct {
	chatModel       einomodel.ToolCallingChatModel
	specialists     map[Intent]Specialist
	hybridRetriever retriever.Retriever
	embedder        embedding.Embedder
	ctxMonitor      contextmanager.ContextMonitor
}

// NewHost creates a new intent routing host.
func NewHost(chatModel einomodel.ToolCallingChatModel, hybridRetriever retriever.Retriever, embedder embedding.Embedder, ctxMonitor contextmanager.ContextMonitor) *Host {
	return &Host{
		chatModel:       chatModel,
		specialists:     make(map[Intent]Specialist),
		hybridRetriever: hybridRetriever,
		embedder:        embedder,
		ctxMonitor:      ctxMonitor,
	}
}

// Register adds a specialist for a specific intent.
func (h *Host) Register(intent Intent, specialist Specialist) {
	h.specialists[intent] = specialist
}

// Classify determines the intent of a user message.
func (h *Host) Classify(ctx context.Context, sessionID string, message string, history []model.Message) (*ClassificationResult, error) {
	prompt := []*schema.Message{
		{Role: "system", Content: intentClassificationPrompt},
	}
	start := 0
	if len(history) > 4 {
		start = len(history) - 4
	}
	for _, m := range history[start:] {
		role := string(m.Role)
		prompt = append(prompt, &schema.Message{Role: schema.RoleType(role), Content: m.Content})
	}
	prompt = append(prompt, schema.UserMessage(message))

	resp, err := h.chatModel.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("intent classification failed: %w", err)
	}

	// Report context usage to monitor
	if h.ctxMonitor != nil {
		totalTokens := 0
		for _, m := range prompt {
			totalTokens += contextmanager.EstimateTokens(m.Content)
		}
		h.ctxMonitor.RecordUsage(sessionID, "intent_classify", contextmanager.ContextUsage{
			SystemPromptTokens: contextmanager.EstimateTokens(intentClassificationPrompt),
			HistoryTokens:       totalTokens - contextmanager.EstimateTokens(intentClassificationPrompt) - contextmanager.EstimateTokens(message),
			InputTokens:         contextmanager.EstimateTokens(message),
			TotalTokens:          totalTokens,
			WindowLimit:          8192,
		})
	}

	var result ClassificationResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return &ClassificationResult{Intent: IntentCasualChat, Confidence: 0.5}, nil
	}

	return &result, nil
}

// Route classifies the message and dispatches to the matching specialist.
func (h *Host) Route(ctx context.Context, sessionID string, message string, history []model.Message) (string, error) {
	result, err := h.Classify(ctx, sessionID, message, history)
	if err != nil {
		return "", err
	}

	metadata := result.Extracted
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if result.SubIntent != "" {
		metadata["sub_intent"] = result.SubIntent
	}

	// RAG retrieval via Eino hybrid retriever
	if h.hybridRetriever != nil && (result.Intent == IntentCasualChat || result.Intent == IntentSkillPractice) {
		docs, err := h.hybridRetriever.Retrieve(ctx, message, retriever.WithTopK(3))
		if err == nil && len(docs) > 0 {
			metadata["rag_documents"] = rag.FormatDocuments(docs)
		} else if err != nil {
			log.Printf("[router] RAG search skipped: %v", err)
		}
	}

	// Pass conversation history to specialists so they can inject it into LLM context.
	if len(history) > 0 {
		historyJSON, err := json.Marshal(history)
		if err == nil {
			metadata["conversation_history"] = string(historyJSON)
		}
	}

	specialist, ok := h.specialists[result.Intent]
	if !ok {
		if chat, ok2 := h.specialists[IntentCasualChat]; ok2 {
			return chat.Handle(ctx, sessionID, message, metadata)
		}
		return "I'm not sure how to help with that. Could you try rephrasing?", nil
	}

	return specialist.Handle(ctx, sessionID, message, metadata)
}

const intentClassificationPrompt = `## 角色定义
你是一个意图分类器，专属于 AI 面试系统 InterviewAgent。你的唯一职责是对用户消息进行意图分类。

## 工作范围
分析用户消息，将其归类为三种意图之一：
1. "interview" —— 用户要开始或继续模拟面试
   - SubIntent: "create"（开始新面试）, "answer"（回答面试问题）
2. "skill_practice" —— 用户要进行技能练习
   - 核心技能: "quick_quiz"（快速测验）, "knowledge_explain"（知识讲解）,
     "project_highlight"（项目亮点提炼）, "tech_compare"（技术对比）
   - 专项训练: "algorithm"（算法练习）, "system_design"（系统设计）,
     "behavioral"（行为面试）, "tech_quiz"（技术测验）
   - "list"（浏览可用选项）
3. "casual_chat" —— 用户在进行一般闲聊或提问

## 边界限制
- 仅输出 JSON 格式的分类结果，不做任何解释或建议
- 如果无法确定意图，默认归类为 "casual_chat"，confidence 设为 0.5
- 不要替用户做决定，仅基于消息内容判断意图
- 对于模糊的消息，优先归类为 "casual_chat" 而非猜测

## 分类指引
- "快速测验" → sub_intent "quick_quiz"
- "讲解"相关 → sub_intent "knowledge_explain"
- "提炼亮点" → sub_intent "project_highlight"
- "对比"相关 → sub_intent "tech_compare"

## 输出格式
仅输出 JSON 对象，无任何其他文字：
{
  "intent": "interview|skill_practice|casual_chat",
  "confidence": 0.0-1.0,
  "sub_intent": "子类型或空字符串",
  "extracted": {}
}`

