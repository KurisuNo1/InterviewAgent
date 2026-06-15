package router

import (
	"context"
	"encoding/json"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
)

// CasualChatSpecialist handles casual conversation using an Eino ReAct Agent.
// The agent autonomously decides when to call MCP tools to enrich responses.
// Falls back to direct LLM when agent is unavailable.
type CasualChatSpecialist struct {
	agent      *react.Agent
	chatModel  einomodel.ToolCallingChatModel // fallback when agent is nil
	ctxBuilder *contextmanager.ContextBuilder
}

func NewCasualChatSpecialist(agent *react.Agent, chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *CasualChatSpecialist {
	return &CasualChatSpecialist{agent: agent, chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (h *CasualChatSpecialist) Name() string       { return "casual_chat" }
func (h *CasualChatSpecialist) Description() string { return "Handles casual conversation with agentic MCP tool calling" }
func (h *CasualChatSpecialist) CanHandle(intent Intent, subIntent string) bool {
	return intent == IntentCasualChat
}

func (h *CasualChatSpecialist) Handle(ctx context.Context, sessionID string, input string, metadata map[string]string) (string, error) {
	systemPrompt := `## 角色定义
	你是 InterviewAgent，一个专注于职业发展和面试准备的 AI 助手。

	## 工作范围
	- 回答关于技术、职业发展、面试技巧等方面的问题
	- 如果用户询问具体技术信息、开源项目、教程或时效性内容，使用搜索工具获取实时信息
	- 如果用户想开始模拟面试或技能练习，引导他们使用对应功能

	## 边界限制
	- 不要编造信息；对于不确定的内容，使用搜索工具验证或如实说明不确定
	- 不要提供医疗、法律、金融等专业建议
	- 不要执行任何可能修改用户系统或文件的请求
	- 你有搜索 GitHub 仓库和网络的权限，用于查找技术文档和项目信息`

	if h.agent != nil {
		systemPrompt += `
	- 你已接入搜索工具，可以查询 GitHub 仓库和网络信息；需要具体信息时请主动使用工具`
	}
	systemPrompt += `

	## 行为准则
	- 使用与用户相同的语言回复
	- 保持友好、简洁、有帮助
	- 如果涉及面试相关建议，应专业且务实`

	// Parse conversation history from metadata
	var history []model.Message
	if historyJSON, ok := metadata["conversation_history"]; ok && historyJSON != "" {
		json.Unmarshal([]byte(historyJSON), &history)
	}

	ragDocs := ""
	if d, ok := metadata["rag_documents"]; ok {
		ragDocs = d
	}

	var messages []*schema.Message
	if h.ctxBuilder != nil {
		messages = h.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    sessionID,
			ProfileName:  "casual_chat",
			SystemPrompt: systemPrompt,
			History:      history,
			RAGDocuments: ragDocs,
			UserInput:    input,
		})
	} else {
		if ragDocs != "" {
			systemPrompt += "\n\n## Reference Knowledge\n" + ragDocs
		}
		messages = []*schema.Message{
			schema.SystemMessage(systemPrompt),
		}
		for _, m := range history {
			switch m.Role {
			case model.RoleUser:
				messages = append(messages, schema.UserMessage(m.Content))
			case model.RoleAssistant:
				messages = append(messages, schema.AssistantMessage(m.Content, nil))
			}
		}
		messages = append(messages, schema.UserMessage(input))
	}

	if h.agent != nil {
		resp, err := h.agent.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("agent generate failed: %w", err)
		}
		if resp != nil {
			return resp.Content, nil
		}
		return "", fmt.Errorf("agent returned nil response")
	}

	resp, err := h.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("casual chat failed: %w", err)
	}
	if resp != nil {
		return resp.Content, nil
	}
	return "", fmt.Errorf("chat model returned nil response")
}

var _ Specialist = (*CasualChatSpecialist)(nil)
