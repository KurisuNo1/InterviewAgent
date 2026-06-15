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

type TechCompareSkill struct {
	chatModel  einomodel.ToolCallingChatModel
	ctxBuilder *contextmanager.ContextBuilder
}

func NewTechCompareSkill(chatModel einomodel.ToolCallingChatModel, ctxBuilder *contextmanager.ContextBuilder) *TechCompareSkill {
	return &TechCompareSkill{chatModel: chatModel, ctxBuilder: ctxBuilder}
}

func (s *TechCompareSkill) Name() string        { return "tech_compare" }
func (s *TechCompareSkill) Description() string { return "Structured comparison of technologies, frameworks, or tools" }
func (s *TechCompareSkill) Category() string    { return "core" }
func (s *TechCompareSkill) WelcomeMessage() string {
	return "请列出你想对比的两项技术（如 Go vs Python、React vs Vue），我将从性能、生态、学习曲线、适用场景四个维度进行对比。"
}
func (s *TechCompareSkill) CanHandle(subIntent string) bool {
	return subIntent == "tech_compare" || subIntent == "compare" || subIntent == "vs"
}

func (s *TechCompareSkill) NewSession(ctx context.Context, subIntent string) (*SkillState, error) {
	return &SkillState{
		SessionID: uuid.New().String(), SkillName: s.Name(), SubIntent: subIntent,
		Round: 0, History: []string{},
		Data: map[string]any{"rounds": 4, "maxRounds": 4},
	}, nil
}

func (s *TechCompareSkill) Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error) {
	state.Round++
	maxRounds, _ := state.Data["rounds"].(int)

	if input != "" {
		state.History = append(state.History, input)
	}
	if len(state.History) >= 2 && maxRounds >= state.Round {
		return s.compare(ctx, state)
	}
	if state.Round >= maxRounds {
		return &SkillResponse{Message: s.buildSummary(state), IsComplete: true}, nil
	}

	if state.Round == 1 {
		return &SkillResponse{CaptureInput: true,
			Message: "请列出你想对比的两项技术（例如：Go vs Python、React vs Vue）。"}, nil
	}
	return &SkillResponse{CaptureInput: true,
		Message: "现在请输入技术B的信息，或者指定你想了解的对比维度（如性能、生态、学习曲线）。"}, nil
}

func (s *TechCompareSkill) compare(ctx context.Context, state *SkillState) (*SkillResponse, error) {
	dimensions := []string{"性能与效率", "生态系统与社区", "学习曲线", "适用场景"}
	dimIdx := (state.Round - 2) % len(dimensions)
	dim := dimensions[dimIdx]

	hist := strings.Join(state.History, "; ")
	prompt := fmt.Sprintf(`## 角色定义
你是一个技术对比分析专家，负责对两项技术进行多维度客观比较。

## 工作范围
- 当前对比维度: "%s"
- 用户输入: %s
- 四个对比维度：性能与效率、生态系统与社区、学习曲线、适用场景

## 边界限制
- 仅做客观对比，不做主观推荐或优劣评判
- 尽可能引用具体数据（版本、基准测试、市场份额等）
- 不使用过时信息；如果无法确定某项数据，明确标注信息来源
- 不要贬低或过度抬高任何一项技术

## 行为准则
- 使用与用户相同的语言
- 对比应平衡、实事求是
`, dim, hist)

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
	if err != nil { return nil, fmt.Errorf("tech_compare: %w", err) }

	state.Data[dim] = resp.Content
	maxRounds, _ := state.Data["maxRounds"].(int)
	if maxRounds <= 0 { maxRounds = 4 }
	complete := state.Round >= maxRounds
	return &SkillResponse{Message: fmt.Sprintf("【%s】\n%s", dim, resp.Content), IsComplete: complete,
		NextPrompt: "按 Enter 继续下一维度对比"}, nil
}

func (s *TechCompareSkill) buildSummary(state *SkillState) string {
	var sb strings.Builder
	sb.WriteString("## 技术对比总结\n")
	for _, d := range []string{"性能与效率", "生态系统与社区", "学习曲线", "适用场景"} {
		if v, ok := state.Data[d].(string); ok { sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", d, v)) }
	}
	return sb.String()
}
