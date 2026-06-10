package skill

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
)

type KnowledgeExplainSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewKnowledgeExplainSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *KnowledgeExplainSkill {
	return &KnowledgeExplainSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *KnowledgeExplainSkill) Name() string        { return "knowledge_explain" }
func (s *KnowledgeExplainSkill) Description() string  { return "In-depth concept explanation with progressive depth levels" }
func (s *KnowledgeExplainSkill) Category() string     { return "core" }
func (s *KnowledgeExplainSkill) WelcomeMessage() string {
	return "请输入你想深入了解的技术概念（如 Go 并发、Kubernetes 架构），我将逐层讲解：入门概述→核心原理→进阶优化→前沿对比。"
}
func (s *KnowledgeExplainSkill) CanHandle(subIntent string) bool {
	return subIntent == "knowledge_explain" || subIntent == "explain" || subIntent == "concept"
}

func (s *KnowledgeExplainSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(), SkillName: s.Name(), SubIntent: subIntent,
		Round: 0, History: []string{},
		Data: map[string]any{"depth": 0, "maxDepth": 3, "topic": ""},
	}, nil
}

func (s *KnowledgeExplainSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	depth, _ := state.Data["depth"].(int)
	maxDepth, _ := state.Data["maxDepth"].(int)
	if maxDepth <= 0 { maxDepth = 3 }

	if input != "" && state.Data["topic"].(string) == "" {
		state.Data["topic"] = input
	}
	topic, _ := state.Data["topic"].(string)

	if depth >= maxDepth {
		return &SkillResponse{
			Message:    fmt.Sprintf("已讲解 %s 的深度知识（共%d层）。需要讲解其他概念吗？", topic, maxDepth),
			IsComplete: true,
		}, nil
	}

	state.Data["depth"] = depth + 1
	level := []string{"入门概述", "核心原理与实现", "进阶优化与最佳实践", "前沿发展与深度对比"}[depth]

	history := strings.Join(state.History, "\n")
	prompt := fmt.Sprintf(`## 角色定义
你是一名技术教育专家，负责逐层深入讲解技术概念。

## 工作范围
- 当前讲解主题: "%s"，第 %d 层: %s
- 按照入门概述→核心原理→进阶优化→前沿对比的递进结构进行讲解
- 使用实例辅助理解，确保内容准确、透彻

## 边界限制
- 仅讲解当前层级的主题，不要一次性输出所有深度
- 不要评价用户的理解能力，仅提供客观的知识讲解
- 如果参考资料中有相关信息，优先依据参考资料
- 不对技术选型做主观推荐，仅做客观分析

## 行为准则
- 使用与用户相同的语言
- 内容循序渐进，适合当前层级

## 此前内容
%s`, topic, depth+1, level, history)

	state.History = append(state.History, fmt.Sprintf("[Level %d] %s", depth+1, level))

	ragDocs, _ := state.Data["rag_documents"].(string)
	var msgs []*schema.Message
	if s.ctxBuilder != nil {
		msgs = s.ctxBuilder.Build(contextmanager.BuildParams{
			ProfileName:  "skill",
			SystemPrompt: prompt,
			RAGDocuments: ragDocs,
			UserInput:    "",
		})
	} else {
		if ragDocs != "" { prompt += "\n\n## 参考知识\n" + ragDocs }
		msgs = []*schema.Message{schema.SystemMessage(prompt)}
	}

	resp, err := s.chatModel.Generate(ctx, msgs)
	if err != nil { return nil, fmt.Errorf("knowledge_explain: %w", err) }

	nextPrompt := ""
	if depth+1 < maxDepth {
		nextPrompt = fmt.Sprintf("输入 'next' 进入第%d层：%s，或提问具体问题", depth+2, []string{"核心原理", "进阶优化", "前沿对比"}[depth])
	}
	return &SkillResponse{Message: resp.Content, NextPrompt: nextPrompt, IsComplete: depth+1 >= maxDepth}, nil
}
