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

type BehavioralSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewBehavioralSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *BehavioralSkill {
	return &BehavioralSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *BehavioralSkill) Name() string { return "behavioral" }
func (s *BehavioralSkill) Description() string {
	return "Behavioral interview practice using the STAR method"
}
func (s *BehavioralSkill) Category() string { return "training" }
func (s *BehavioralSkill) WelcomeMessage() string {
	return "Let's practice behavioral interview questions using the STAR method. I'll ask you a question, and you respond with Situation, Task, Action, Result."
}
func (s *BehavioralSkill) CanHandle(subIntent string) bool {
	return subIntent == "behavioral" || subIntent == "behavior" || subIntent == "star"
}

func (s *BehavioralSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *BehavioralSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 4
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Behavioral practice complete. Remember to always use the STAR method!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`## 角色定义
你是一名行为面试教练，帮助用户使用 STAR 方法练习行为面试问题。

## 工作范围
- 第 %d/%d 轮
- 提出常见的行为面试问题（如冲突处理、项目经历、时间管理、失败经验）
- 按 STAR 四要素评估用户回答：S(情境描述)、T(任务职责)、A(采取行动)、R(量化结果)
- 给出具体的改进建议并追问

## 边界限制
- 仅评估 STAR 方法的使用质量，不做个人品格评判
- 不追问涉及个人隐私或敏感信息的内容
- 反馈应具体、建设性，指出哪个 STAR 环节需要加强

## 行为准则
- 使用与用户相同的语言
- 可参考的常见题目：工作冲突处理、最自豪的项目、紧迫截止日期应对、失败经历

## 此前对话
%s

## 用户输入
"%s"`, state.Round, maxRounds, strings.Join(state.History, "\n"), input)

	ragDocs, _ := state.Data["rag_documents"].(string)
	var msgs []*schema.Message
	if s.ctxBuilder != nil {
		msgs = s.ctxBuilder.Build(contextmanager.BuildParams{
			SessionID:    state.SessionID,
			ProfileName:  "skill",
			SystemPrompt: prompt,
			RAGDocuments: ragDocs,
			UserInput:    "",
		})
	} else {
		if ragDocs != "" { prompt += "\n\n## 参考知识\n" + ragDocs }
		msgs = []*schema.Message{{Role: "system", Content: prompt}}
	}

	resp, err := s.chatModel.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("behavioral skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Answer using the STAR format (Situation, Task, Action, Result).",
	}, nil
}
