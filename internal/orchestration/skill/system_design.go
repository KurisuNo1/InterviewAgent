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

type SystemDesignSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewSystemDesignSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *SystemDesignSkill {
	return &SystemDesignSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *SystemDesignSkill) Name() string        { return "system_design" }
func (s *SystemDesignSkill) Description() string { return "System design interview practice" }
func (s *SystemDesignSkill) Category() string    { return "training" }
func (s *SystemDesignSkill) WelcomeMessage() string {
	return "Let's practice system design! What system would you like to design? (e.g., URL shortener, chat app, news feed)"
}
func (s *SystemDesignSkill) CanHandle(subIntent string) bool {
	return subIntent == "system_design" || subIntent == "design" || subIntent == "architecture"
}

func (s *SystemDesignSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *SystemDesignSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 6
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Design discussion complete. Great work thinking through the architecture!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`## 角色定义
你是一名系统设计面试官，引导用户完成系统设计练习。

## 工作范围
- 第 %d/%d 轮
- 按照以下阶段引导用户：需求澄清→容量估算→高层设计→组件深入→权衡与瓶颈
- 针对用户的设计选择提出追问，给出建设性反馈
- 不做最终正确/错误的绝对判断，而是引导思考和讨论

## 边界限制
- 不做超出用户所提供信息的假设
- 不直接给出现成答案，始终引导用户自己思考
- 不做主观的架构推荐，仅分析各类方案的权重

## 行为准则
- 使用与用户相同的语言

## 此前对话
%s

## 用户输入
"%s"`, state.Round, maxRounds, strings.Join(state.History, "\n"), input)

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
		msgs = []*schema.Message{{Role: "system", Content: prompt}}
	}

	resp, err := s.chatModel.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("system design skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Describe your design approach or ask for clarification.",
	}, nil
}
