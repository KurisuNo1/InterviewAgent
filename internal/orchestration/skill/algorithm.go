package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/contextmanager"
)

type AlgorithmSkill struct {
	chatModel  model.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewAlgorithmSkill(chatModel model.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *AlgorithmSkill {
	return &AlgorithmSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *AlgorithmSkill) Name() string        { return "algorithm" }
func (s *AlgorithmSkill) Description() string { return "LeetCode-style algorithm coding practice" }
func (s *AlgorithmSkill) Category() string    { return "training" }
func (s *AlgorithmSkill) WelcomeMessage() string {
	return "Let's practice algorithms! Tell me your preferred difficulty (easy/medium/hard) or a specific topic, and I'll give you a coding problem."
}
func (s *AlgorithmSkill) CanHandle(subIntent string) bool {
	return subIntent == "algorithm" || subIntent == "algo" || subIntent == "coding"
}

func (s *AlgorithmSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data:      make(map[string]any),
	}, nil
}

func (s *AlgorithmSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Round %d] User: %s", state.Round, input))

	maxRounds := 5
	if state.Round >= maxRounds {
		return &SkillResponse{
			Message:    "Practice session complete. Review your solutions and try more problems!",
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`## 角色定义
你是一名算法编程教练，负责对用户进行算法编程训练。

## 工作范围
- 第 %d/%d 轮练习
- 首轮：根据用户水平出一道合适的编程题
- 后续轮：评审用户提交的解法并给出反馈
- 解法正确：给出更难的变体题或进入新题目
- 解法需要改进：给出提示(hint)，不直接给出完整答案
- 不允许直接写出完整解法，必须引导用户自己思考

## 边界限制
- 不出与当前技术水平严重不匹配的题目
- 不直接泄露答案，始终通过提示引导
- 不评价用户的智力或学习能力

## 行为准则
- 使用与用户相同的语言回复
- 反馈简洁、有针对性

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
		msgs = []*schema.Message{schema.SystemMessage(prompt)}
	}

	resp, err := s.chatModel.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("algorithm skill error: %w", err)
	}

	return &SkillResponse{
		Message:    resp.Content,
		IsComplete: false,
		NextPrompt: "Type your solution or 'hint' for a hint, 'skip' to move on.",
	}, nil
}
