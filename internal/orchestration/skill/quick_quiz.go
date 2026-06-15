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

// QuickQuizSkill provides rapid-fire knowledge assessment during interviews.
type QuickQuizSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewQuickQuizSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *QuickQuizSkill {
	return &QuickQuizSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *QuickQuizSkill) Name() string        { return "quick_quiz" }
func (s *QuickQuizSkill) Description() string  { return "Rapid knowledge assessment on a given topic (3-5 quick questions)" }
func (s *QuickQuizSkill) Category() string     { return "core" }
func (s *QuickQuizSkill) WelcomeMessage() string {
	return "请输入你想测验的技术主题，我将出 5 道题测试你的知识水平，并给出评分。"
}
func (s *QuickQuizSkill) CanHandle(subIntent string) bool {
	return subIntent == "quick_quiz" || subIntent == "rapid"
}

func (s *QuickQuizSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(), SkillName: s.Name(), SubIntent: subIntent,
		Round: 0, History: []string{},
		Data: map[string]any{"score": 0, "total": 0, "maxRounds": 5},
	}, nil
}

func (s *QuickQuizSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	maxRounds, _ := state.Data["maxRounds"].(int)
	if maxRounds <= 0 { maxRounds = 5 }
	if state.Round > maxRounds {
		score, _ := state.Data["score"].(int)
		return &SkillResponse{Message: fmt.Sprintf("测验结束！得分：%d/%d", score, maxRounds), IsComplete: true}, nil
	}

	topic := ""
	if v, ok := state.Data["topic"].(string); ok { topic = v }
	if input != "" && topic == "" { topic = input }

	prompt := fmt.Sprintf(`## 角色定义
你是一个快速测验评估官，负责对候选人进行指定主题的知识水平测试。

## 工作范围
- 针对主题 "%s" 出题，当前第 %d/%d 轮
- 每次只出一道简洁的题目，格式：[Q%d] 题目内容
- 收到回答后判断正确与否并简要解释

## 边界限制
- 题目应覆盖该主题的核心知识点，难度逐步递增
- 不要在题目中暗示答案
- 评分仅基于答案正确性，不评价候选人个人

## 行为准则
- 使用与用户相同的语言
- 出题简洁明了
`, topic, state.Round, maxRounds, state.Round)

	history := strings.Join(state.History, "\n")
	if history != "" { prompt += "\n## 此前对话\n" + history }
	state.History = append(state.History, fmt.Sprintf("[Q%d] A: %s", state.Round, input))
	state.Data["topic"] = topic

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
	if err != nil { return nil, fmt.Errorf("quick_quiz: %w", err) }

	if strings.Contains(resp.Content, "Correct") || strings.Contains(resp.Content, "正确") {
		score, _ := state.Data["score"].(int); state.Data["score"] = score + 1
	}
	state.Data["total"] = state.Round

	return &SkillResponse{Message: resp.Content, NextPrompt: fmt.Sprintf("回答第 %d/%d 题", state.Round+1, maxRounds)}, nil
}
