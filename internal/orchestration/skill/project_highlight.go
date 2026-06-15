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

type ProjectHighlightSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewProjectHighlightSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *ProjectHighlightSkill {
	return &ProjectHighlightSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *ProjectHighlightSkill) Name() string        { return "project_highlight" }
func (s *ProjectHighlightSkill) Description() string  { return "Extract and refine project highlights for interview storytelling" }
func (s *ProjectHighlightSkill) Category() string     { return "core" }
func (s *ProjectHighlightSkill) WelcomeMessage() string {
	return "请描述你参与过的一个项目，我将分4个阶段帮你提炼面试亮点：项目描述→技术分析→成果量化→面试故事。"
}
func (s *ProjectHighlightSkill) CanHandle(subIntent string) bool {
	return subIntent == "project_highlight" || subIntent == "project" || subIntent == "highlight"
}

func (s *ProjectHighlightSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(), SkillName: s.Name(), SubIntent: subIntent,
		Round: 0, History: []string{},
		Data: map[string]any{"phase": 0, "phases": []string{"project_desc", "tech_analysis", "impact_metrics", "interview_story"}},
	}, nil
}

func (s *ProjectHighlightSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	phase := state.Data["phase"].(int)
	phases := state.Data["phases"].([]string)
	if phase >= len(phases) {
		return &SkillResponse{Message: "项目亮点提炼完成！所有维度已覆盖。", IsComplete: true}, nil
	}

	currentPhase := phases[phase]
	state.Data["phase"] = phase + 1

	phasePrompts := map[string]string{
		"project_desc":    "让用户描述项目背景、规模和角色。输出结构化总结。",
		"tech_analysis":   "分析技术栈选型理由、架构设计亮点、技术难点。",
		"impact_metrics":  "引导用户量化项目成果（性能提升、用户增长等），输出STAR格式亮点。",
		"interview_story": "将前面内容整合为面试可用的2分钟项目介绍，突出个人贡献。",
	}

	history := strings.Join(state.History, "\n")
	prompt := fmt.Sprintf(`## 角色定义
你是一名技术面试职业教练，专门帮助候选人提炼和优化项目经验描述。

## 工作范围
- 阶段 %d/%d: %s
- 当前目标: %s
- 引导用户完成4个阶段：项目描述→技术分析→成果量化→面试故事
- 输出需要具体、可操作，能直接用于面试场景

## 边界限制
- 仅对用户提供的项目信息进行分析和提炼，不得虚构用户未提及的细节
- 不要对用户的职业前景做预判性评价
- 输出的STAR格式故事应基于事实，不应夸大或捏造成果

## 行为准则
- 使用与用户相同的语言
- 引导用户提供更多细节，而非替用户编造

## 用户已提供内容
%s`,
		phase+1, len(phases), currentPhase, phasePrompts[currentPhase], history+"; "+input)

	state.History = append(state.History, fmt.Sprintf("[Phase %d: %s] input: %s", phase+1, currentPhase, input))

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
	if err != nil { return nil, fmt.Errorf("project_highlight: %w", err) }

	return &SkillResponse{
		Message:    resp.Content,
		NextPrompt: fmt.Sprintf("阶段 %d/%d 完成。继续描述项目细节或按 Enter 进入下一阶段", phase+1, len(phases)),
		IsComplete: phase+1 >= len(phases),
	}, nil
}
