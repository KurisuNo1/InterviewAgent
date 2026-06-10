package observability

import (
	"context"
	"log"

	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	einoUtils "github.com/cloudwego/eino/utils/callbacks"
)

// Level controls log verbosity.
type Level int

const (
	LevelNone  Level = iota
	LevelError
	LevelInfo
	LevelDebug
)

var currentLevel = LevelInfo

// SetLevel changes the observability log level at runtime.
func SetLevel(l Level) { currentLevel = l }

// RegisterEinoCallbacks registers global Eino callback handlers for LLM and Embedding observability.
// Must be called once BEFORE any graph execution, during application startup.
func RegisterEinoCallbacks() {
	handler := einoUtils.NewHandlerHelper().
		ChatModel(&einoUtils.ModelCallbackHandler{
			OnStart: onChatModelStart,
			OnEnd:   onChatModelEnd,
			OnError: onChatModelError,
		}).
		Embedding(&einoUtils.EmbeddingCallbackHandler{
			OnStart: onEmbeddingStart,
			OnEnd:   onEmbeddingEnd,
			OnError: onEmbeddingError,
		}).
		Tool(&einoUtils.ToolCallbackHandler{
			OnStart: onToolStart,
			OnEnd:   onToolEnd,
			OnError: onToolError,
		}).
		Handler()

	callbacks.AppendGlobalHandlers(handler)
	log.Println("[obs] Eino global callbacks registered (ChatModel + Embedding)")
}

// ---------- ChatModel callbacks ----------

type llmTiming struct {
	start time.Time
	name  string
	model string
}

func onChatModelStart(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
	if currentLevel < LevelInfo {
		return ctx
	}
	nodeName := ""
	modelName := ""
	if info != nil {
		nodeName = info.Name
	}
	if input != nil && input.Config != nil {
		modelName = input.Config.Model
	}
	msgCount := 0
	totalLen := 0
	if input != nil {
		msgCount = len(input.Messages)
		for _, m := range input.Messages {
			totalLen += len(m.Content)
		}
	}

	if currentLevel >= LevelDebug {
		preview := ""
		if input != nil && len(input.Messages) > 0 {
			last := input.Messages[len(input.Messages)-1]
			preview = last.Content
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
		}
		log.Printf("[EINO-LLM] → node=%s model=%s msgs=%d total_len=%d preview=%q",
			nodeName, modelName, msgCount, totalLen, preview)
	} else {
		log.Printf("[EINO-LLM] → node=%s model=%s msgs=%d total_len=%d",
			nodeName, modelName, msgCount, totalLen)
	}

	return context.WithValue(ctx, &llmTimingKey{}, &llmTiming{
		start: time.Now(),
		name:  nodeName,
		model: modelName,
	})
}

func onChatModelEnd(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
	if currentLevel < LevelInfo {
		return ctx
	}
	nodeName := ""
	modelName := ""
	if info != nil {
		nodeName = info.Name
	}
	respLen := 0
	tokens := &model.TokenUsage{}
	if output != nil {
		if output.Message != nil {
			respLen = len(output.Message.Content)
		}
		if output.TokenUsage != nil {
			tokens = output.TokenUsage
		}
		if output.Config != nil && modelName == "" {
			modelName = output.Config.Model
		}
	}

	t, _ := ctx.Value(&llmTimingKey{}).(*llmTiming)
	duration := time.Duration(0)
	if t != nil {
		duration = time.Since(t.start)
		modelName = t.model
	}

	if currentLevel >= LevelDebug {
		preview := ""
		if output != nil && output.Message != nil {
			preview = output.Message.Content
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
		}
		log.Printf("[EINO-LLM] ← node=%s model=%s resp_len=%d prompt_t=%d comp_t=%d total_t=%d duration=%v resp=%q",
			nodeName, modelName, respLen, tokens.PromptTokens, tokens.CompletionTokens, tokens.TotalTokens, duration, preview)
	} else {
		log.Printf("[EINO-LLM] ← node=%s model=%s resp_len=%d prompt_t=%d comp_t=%d total_t=%d duration=%v",
			nodeName, modelName, respLen, tokens.PromptTokens, tokens.CompletionTokens, tokens.TotalTokens, duration)
	}

	return ctx
}

func onChatModelError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	nodeName := ""
	if info != nil {
		nodeName = info.Name
	}
	t, _ := ctx.Value(&llmTimingKey{}).(*llmTiming)
	duration := time.Duration(0)
	if t != nil {
		duration = time.Since(t.start)
	}
	log.Printf("[EINO-LLM] ✗ node=%s error=%v duration=%v", nodeName, err, duration)
	return ctx
}

// ---------- Embedding callbacks ----------

func onEmbeddingStart(ctx context.Context, info *callbacks.RunInfo, input *embedding.CallbackInput) context.Context {
	if currentLevel < LevelDebug {
		return ctx
	}
	nodeName := ""
	if info != nil {
		nodeName = info.Name
	}
	textLen := 0
	if input != nil {
		textLen = len(input.Texts)
	}
	log.Printf("[EINO-EMBED] → node=%s texts=%d", nodeName, textLen)
	return context.WithValue(ctx, &embedTimingKey{}, time.Now())
}

func onEmbeddingEnd(ctx context.Context, info *callbacks.RunInfo, output *embedding.CallbackOutput) context.Context {
	if currentLevel < LevelDebug {
		return ctx
	}
	nodeName := ""
	if info != nil {
		nodeName = info.Name
	}
	vecCount := 0
	if output != nil && output.Embeddings != nil {
		vecCount = len(output.Embeddings)
	}
	start, _ := ctx.Value(&embedTimingKey{}).(time.Time)
	duration := time.Since(start)
	log.Printf("[EINO-EMBED] ← node=%s vectors=%d duration=%v", nodeName, vecCount, duration)
	return ctx
}

func onEmbeddingError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	nodeName := ""
	if info != nil {
		nodeName = info.Name
	}
	log.Printf("[EINO-EMBED] ✗ node=%s error=%v", nodeName, err)
	return ctx
}

// context keys for passing timing between OnStart/OnEnd
type llmTimingKey struct{}
type embedTimingKey struct{}

// ---------- Tool callbacks (MCP tools) ----------

func onToolStart(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
	if currentLevel < LevelInfo {
		return ctx
	}
	toolName := ""
	if info != nil {
		toolName = info.Name
	}
	argsPreview := ""
	if input != nil && currentLevel >= LevelDebug {
		argsPreview = input.ArgumentsInJSON
		if len(argsPreview) > 120 {
			argsPreview = argsPreview[:120] + "..."
		}
	}
	log.Printf("[EINO-TOOL] → name=%s args=%s", toolName, argsPreview)
	return context.WithValue(ctx, &toolTimingKey{}, time.Now())
}

func onToolEnd(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
	if currentLevel < LevelInfo {
		return ctx
	}
	toolName := ""
	if info != nil {
		toolName = info.Name
	}
	respLen := 0
	if output != nil {
		respLen = len(output.Response)
	}
	start, _ := ctx.Value(&toolTimingKey{}).(time.Time)
	duration := time.Since(start)

	if currentLevel >= LevelDebug {
		preview := ""
		if output != nil {
			preview = output.Response
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
		}
		log.Printf("[EINO-TOOL] ← name=%s result_len=%d duration=%v result=%q",
			toolName, respLen, duration, preview)
	} else {
		log.Printf("[EINO-TOOL] ← name=%s result_len=%d duration=%v",
			toolName, respLen, duration)
	}
	return ctx
}

func onToolError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	toolName := ""
	if info != nil {
		toolName = info.Name
	}
	log.Printf("[EINO-TOOL] ✗ name=%s error=%v", toolName, err)
	return ctx
}

type toolTimingKey struct{}

// NewToolCallbackHandler builds a standalone callbacks.Handler for MCP tool invocations.
// Unlike global handlers (which only fire inside Eino graphs), this handler is meant to
// be called explicitly by the EinoTool.InvokableRun method so that every MCP call —
// whether triggered by the graph or by direct code — goes through Eino's callback system.
func NewToolCallbackHandler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			toolInput := tool.ConvCallbackInput(input)
			return onToolStart(ctx, info, toolInput)
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			toolOutput := tool.ConvCallbackOutput(output)
			return onToolEnd(ctx, info, toolOutput)
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			return onToolError(ctx, info, err)
		}).
		Build()
}

// Discard stream helpers — Eino callbacks require stream handler functions even if unused
func init() {
	_ = schema.StreamReader[any]{}
}
