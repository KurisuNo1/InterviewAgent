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

type TechQuizSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewTechQuizSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *TechQuizSkill {
	return &TechQuizSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *TechQuizSkill) Name() string        { return "tech_quiz" }
func (s *TechQuizSkill) Description() string { return "Rapid-fire technical knowledge quiz on specific tech stacks" }
func (s *TechQuizSkill) Category() string    { return "training" }
func (s *TechQuizSkill) WelcomeMessage() string {
	return "Let's test your technical knowledge! Tell me your tech stack, and I'll quiz you with 10 progressively harder questions."
}
func (s *TechQuizSkill) CanHandle(subIntent string) bool {
	return subIntent == "tech_quiz" || subIntent == "knowledge"
}

func (s *TechQuizSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(),
		SkillName: s.Name(),
		SubIntent: subIntent,
		Round:     0,
		History:   []string{},
		Data: map[string]any{
			"score": 0,
			"total": 0,
		},
	}, nil
}

func (s *TechQuizSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	state.History = append(state.History, fmt.Sprintf("[Q%d] User: %s", state.Round, input))

	totalQuestions := 10
	if state.Round > totalQuestions {
		score, _ := state.Data["score"].(int)
		return &SkillResponse{
			Message:    fmt.Sprintf("Quiz complete! Your score: %d/%d", score, totalQuestions),
			IsComplete: true,
		}, nil
	}

	prompt := fmt.Sprintf(`## 角色定义
你是一名技术测验官，负责对用户进行技术知识测试。

## 工作范围
- 第 %d/%d 题
- 每次出一道单选题或简答题，收到回答后判断 [CORRECT] 或 [INCORRECT] 并解释原因
- 第1题从简单热身题开始，后续题目难度逐步递增
- 题目应覆盖用户技术栈相关的多个领域

## 边界限制
- 不出与技术栈无关的题目
- 不在题目中暗示正确答案
- 评分仅基于答案正确性

## 行为准则
- 使用与用户相同的语言
- 回复格式：[CORRECT]/[INCORRECT] - 解释\nScore: X/%d\n\nNext question: [题目]

## 此前对话
%s

## 用户输入
"%s"`, state.Round, totalQuestions, totalQuestions, strings.Join(state.History, "\n"), input)

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
		if ragDocs != "" { prompt += "\n\n## 参考知识\n使用以下参考资料出题:\n" + ragDocs }
		msgs = []*schema.Message{{Role: "system", Content: prompt}}
	}

	resp, err := s.chatModel.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("tech quiz skill error: %w", err)
	}

	response := resp.Content
	total, _ := state.Data["total"].(int)
	total++
	score, _ := state.Data["score"].(int)
	if strings.Contains(response, "[CORRECT]") {
		score++
	}
	state.Data["score"] = score
	state.Data["total"] = total

	return &SkillResponse{
		Message:    response,
		IsComplete: false,
		NextPrompt: fmt.Sprintf("Your answer? (Question %d of %d, Score: %d/%d)", state.Round, totalQuestions, score, totalQuestions),
	}, nil
}
