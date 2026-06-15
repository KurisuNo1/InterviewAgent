# InterviewAgent 项目技术全景

## 一、项目定位

InterviewAgent 是一个 **AI 驱动的模拟面试系统**，支持 JD 解析、简历匹配、智能出题、多轮追问、自动评分、复习计划生成，以及 8 种技能练习模块。提供 REST API + WebSocket + 微信小程序 + CLI 四种交互方式。

**核心技术栈**: Go + CloudWeGo Eino (LLM 编排框架) + Milvus (向量库) + Bleve (关键词检索引擎) + Redis + MySQL + MCP 协议

---

## 二、三层架构

```
Layer 1: Interaction   ← REST / WebSocket / CLI / 微信小程序
    │ 调用 InterviewService 接口
    ▼
Layer 2: Orchestration ← Orchestrator (中心协调器) + Intent Router + Skill Registry + Interview Runner
    │ 使用 L3 能力
    ▼
Layer 3: Capability    ← LLM / Embedding / Milvus / Bleve / Redis / MySQL / MCP / Resume Parser
```

层级间完全解耦：Layer 1 只依赖 `InterviewService` 接口，Layer 2 不感知 HTTP/WS 协议细节，Layer 3 可独立替换实现。

---

## 三、依赖注入 (wire.go)

`app.Wire()` 是唯一的组装入口，按依赖顺序逐层构建：

```
Config → LLM/Embedding → Redis/MySQL → Milvus/Bleve → MCP Manager → EinoBridge
    → Memory Manager → Context Builder → RAG Retriever → Agent Factory
    → Skill Registry → Interview Graph Nodes → Graph Compilation → Runner
    → Intent Router → Orchestrator → REST Router + WS Hub
```

所有组件创建后注入到 `Orchestrator`，后者持有全部依赖并实现 `InterviewService` 接口。

---

## 四、核心数据模型

### InterviewState（图中流转的唯一状态令牌）

```go
type InterviewState struct {
    SessionID, Phase              // 会话标识 + 当前阶段
    JDAnalysis                    // JD 解析结果
    ResumeMatch                   // 简历匹配结果
    QuestionPlan                  // 出题计划
    QuestionQueue, CurrentQIndex  // 题目队列 + 当前位置
    CurrentQuestion, CurrentFollowUp, MaxFollowUps  // 当前题目 + 追问计数
    ChatHistory                   // 完整对话历史
    Answers, Evaluations          // 用户回答 + 评分结果
    FinalReport, ReviewPlan       // 最终报告 + 复习计划
    NextAction                    // follow_up / next_question / complete
    Difficulty                    // 难度状态机
    StreakCorrect, StreakWrong    // 连续正确/错误计数
}
```

### 评分维度

| 维度 | 权重 | 说明 |
|------|------|------|
| technical_accuracy | 0.40 | 技术准确性 |
| answer_depth | 0.25 | 回答深度 |
| communication | 0.20 | 沟通表达 |
| project_experience | 0.15 | 项目经验匹配度 |

---

## 五、MCP 工具集成

### 架构层次

```
config.yaml                     → 定义 MCP Server (github, web_search)
Manager (manager.go)            → 通过 stdio 子进程启动 Server，管理连接生命周期
EinoBridge (eino_bridge.go)     → 启动时 ListTools() 发现所有工具，包装为 EinoTool
EinoTool (eino_tool.go)        → 实现 Eino InvokableTool 接口，含 OnStart/OnEnd/OnError 回调
ResultFilter (tool_result_filter.go) → 在结果进入 LLM 上下文前裁剪（按工具类型分策略）
AgentFactory (agent/factory.go) → 给不同 Agent 分配不同工具子集
```

### 两个 MCP Server

| Server | 启动方式 | 环境变量 | 用途 |
|--------|---------|---------|------|
| github | `npx @modelcontextprotocol/server-github` | `GITHUB_TOKEN` | 搜索开源仓库 |
| web_search | `npx freesearch-mcpserver` | — | 网络搜索 (Brave Search) |

### Agent 工具权限分配

| Agent | 工具 | maxSteps |
|-------|------|----------|
| casualAgent | GitHub + Web Search (全部) | 10 |
| interviewerAgent | 无 | 8 |
| evalAgent | Web Search | 5 |
| reviewAgent | GitHub + Web Search | 8 |

---

## 六、面试流程详解

### 6.1 Setup 阶段（一次性执行）

```
Graph DAG: START → JD Analysis → Resume Matching → Question Planning → END
```

| 步骤 | Node | 输入 | 输出 |
|------|------|------|------|
| JD 解析 | `JDAnalysisNode` | 原始 JD 文本 | 结构化岗位需求 (职位/级别/技术栈/核心技能等) |
| 简历匹配 | `ResumeMatchingNode` | 简历文本 + JD 分析 | 匹配分数 + 优势/差距 |
| 出题计划 | `QuestionPlanningNode` | JD + 简历 + RAG 检索 | 5-10 道题 (含难度分布、类别、参考答案、评分点) |

### 6.2 Interview 阶段（循环执行）

```
Graph DAG: START → Interviewer (interrupt/resume) → Evaluation → Review Planning → END
```

**每一轮的流程**:

```
1. AskCurrentQuestion: 根据 CurrentQIndex 取题 → LLM 生成自然语言提问 → 返回给用户
2. 用户回答后 SubmitAnswer:
   ├─ ProcessAnswer (LLM 决策): 评估回答质量，决定 follow_up / next_question / complete
   ├─ 如果 follow_up: 生成追问，等待下一轮回答
   ├─ 如果 next_question: AskCurrentQuestion 出下一题
   └─ 如果 complete: 触发 Evaluation + ReviewPlanning
3. EvaluateAnswer (异步后台执行):
   ├─ 4 维度评分 (technical_accuracy 0.4, answer_depth 0.25, communication 0.2, project_experience 0.15)
   ├─ 难度状态机调整 (连续正确 2 次加难度，连续错误 2 次降难度)
   └─ 保存 checkpoint
```

**Interviewer 的决策机制**:

LLM 在每个回答末尾输出 JSON 决策块：
```json
{"action": "follow_up", "reason": "回答涉及了基本概念但缺少实际应用场景的例子，需要追问"}
{"action": "next_question", "reason": "回答全面且深入，可以进入下一题"}
{"action": "complete", "reason": "所有题目已完成"}
```

代码有 JSON 解析失败的 fallback：关键词匹配 `follow_up`/`next_question`/`complete`。

### 6.3 完成阶段

```
Evaluation (评分) → buildReportFromState (汇总) → ReviewPlanning (复习计划 + 资源搜索)
```

报告包含：总分、各维度均分、亮点、薄弱项、逐题点评、总体建议
复习计划包含：薄弱项分析、学习计划项 (优先级+预估时长)、学习资源链接

---

## 七、Intent Router（意图路由）

### 两级路由设计

**第一级（硬编码前缀匹配）** — 在 `Orchestrator.HandleMessage()`:
```
消息格式 "skill:算法:输入" → 直接走 SkillPractice，跳过 LLM 分类
```

**第二级（LLM 分类）** — `Host.Route()`:
```
用户消息 → Host.Classify() → LLM 分类 prompt → JSON 解析 → 得到 Intent
    ├─ interview      → InterviewSpecialist      → Orchestrator delegate 方法
    ├─ skill_practice → SkillPracticeSpecialist  → Registry.Dispatch → 具体 Skill
    └─ casual_chat    → CasualChatSpecialist     → ReAct Agent (MCP 工具) 或直接 LLM
```

### Host → Specialist 策略模式

```
Host (调度器) 持有 map[Intent]Specialist
    │
    ├─ classify (LLM) → intent
    ├─ 注入 RAG + history → metadata
    └─ specialists[intent].Handle(ctx, sessionID, input, metadata)
```

Specialist 只做分发，不持有业务逻辑。`InterviewSpecialist` 和 `SkillPracticeSpecialist` 通过 Delegate 接口回抛给 Orchestrator 执行。

---

## 八、Skill 系统（8 种技能练习）

### Skill 接口

```go
type Skill interface {
    Name() string
    Description() string
    Category() string                // "core"(核心技能) / "专项训练"
    WelcomeMessage() string          // 首次进入的欢迎语
    CanHandle(subIntent string) bool
    Handle(ctx, sessionID, subIntent, input, ragDocuments string) (*SkillResponse, error)
    NewSession(sessionID, subIntent string) *SkillState
}
```

### 8 种技能

| 技能 | 分类 | 说明 |
|------|------|------|
| Algorithm | 专项训练 | 算法练习，模拟 LeetCode 风格 |
| SystemDesign | 专项训练 | 系统设计面试练习 |
| Behavioral | 专项训练 | 行为面试 (STAR 方法论) |
| TechQuiz | 专项训练 | 技术知识测验 |
| QuickQuiz | 核心技能 | 快速知识点测验 |
| KnowledgeExplain | 核心技能 | 技术概念讲解 |
| ProjectHighlight | 核心技能 | 项目亮点提炼 |
| TechCompare | 核心技能 | 技术方案对比分析 |

### Skill 增强机制

Skill 执行时会调用 `searchMCPForSkill()` 注入实时搜索结果：
```
HandleSkill → searchMCPForSkill → GitHubMCP.SearchRepositories + WebSearchMCP.Search
            → 拼接为 Markdown → 作为 RAG 文档注入 LLM 上下文
```

---

## 九、Context Management（上下文管理）

### 分层压缩策略 (slidingWindow)

```
最近 N 轮: verbatim（原样保留）
中间轮次: structuredExtract（结构化提取关键句，截断 200/300 字符）
更早轮次: summarizeTurns（合并为 "Covered topics: ..."）
```

### Context Profiles（6 种场景各有独立预算）

| Profile | SystemMax | WorkingMemory | RAGMax | 压缩阈值 |
|---------|-----------|---------------|--------|---------|
| casual_chat | 1024 | 24576 | 4096 | 12 turns |
| interview_ask | 2048 | 16384 | 4096 | 8 turns |
| interview_eval | 3072 | 8192 | 4096 | 6 turns |
| skill | 2048 | 20480 | 4096 | 10 turns |
| stream_agent | 1024 | 24576 | 4096 | 12 turns |
| stream_fallback | 1024 | 24576 | 4096 | 12 turns |

### ContextBuilder 组装流程

```
BuildParams (SystemPrompt, History, RAGDocuments, UserInput)
    ↓
1. SystemPrompt 裁剪到 SystemMax
2. Conversation History 滑窗压缩到 HistoryMaxTurns
3. RAG Docs 裁剪到 RAGMax
4. 按 System → History(compressed) → RAG → User 顺序组装
5. ContextMonitor 记录 token 用量
    ↓
返回 []*schema.Message (可直接喂给 LLM)
```

### OverflowHandler（上下文溢出分级降级）

```
Level 1: 裁剪 Tool Results
Level 2: 压缩对话历史
Level 3: 减少 RAG 文档
Level 4: 简化 System Prompt
Level 5: 建议新建会话
```

### Token 监控

- ContextMonitor 追踪每会话 + 全局的 token 用量
- 80% 警告，95% 严重
- 持久化到 `data/context_stats.json`

---

## 十、Memory 系统（双层记忆）

### Short-Term (Redis)

```
Key: conv:{sessionID}
结构: List (LPush + LTrim)
TTL: 配置指定 (默认 7200s)
容量: MaxMessages (默认 30 条)
```

### Long-Term (MySQL)

四张表：`interview_sessions`、`interview_results`、`review_plans`、`chat_messages`
全量持久化，支持 `INSERT IGNORE`/`ON DUPLICATE KEY UPDATE` 幂等写入

### MemoryHierarchy（三层协调）

```
Layer 0: Working Memory (per-call assembled by ContextBuilder)
Layer 1: Short-Term (Redis, hot data)
Layer 2: Long-Term (MySQL, cold archive + summaries)
```

---

## 十一、RAG 系统

### Hybrid Retriever (混合检索)

```
用户查询
    ├─ Milvus (向量检索, 语义匹配, weight=0.7)
    └─ Bleve (关键词检索, BM25, weight=0.3)
    ↓
RRF (Reciprocal Rank Fusion) 融合
    ↓
Top-K 结果 (默认 finalTopK=5)
```

### Document Ingestion (文档摄入流水线)

```
PDF/文本 → ResumeParser 解析 → Chunk 分块 (1000字符, 200重叠)
    → Embedding (Qwen3-Embedding-0.6B, 1024维)
    → 并发写入 Milvus + Bleve
```

### RAG Evaluation

每次检索后用 LLM 评估三个维度：Faithfulness（忠实度）、Relevance（相关性）、Completeness（完整性）

---

## 十二、难度自适应系统

### 状态机

```
起始: medium
连续正确 ≥2: 升一级 (easy→medium, medium→hard)
连续错误 ≥2: 降一级 (hard→medium, medium→easy)
```

### 出题分布随难度变化

| 当前难度 | Easy | Medium | Hard |
|----------|------|--------|------|
| Easy | 60% | 30% | 10% |
| Medium | 20% | 50% | 30% |
| Hard | 10% | 30% | 60% |

---

## 十三、可观测性

### Eino Callbacks（全局注册）

```
RegisterEinoCallbacks()  ← 在 wire.go 启动时调用一次
    ├─ ChatModel: OnStart (记录 prompt 长度 + 模型名) / OnEnd (记录 token 用量 + 耗时) / OnError
    ├─ Embedding: OnStart / OnEnd / OnError
    └─ Tool: OnStart / OnEnd (记录参数 + 结果预览) / OnError
```

日志级别三档：None / Error / Info / Debug，运行时可通过 `SetLevel()` 切换

### Agent Thinker（预留，尚未接入）

```go
type ThinkLogger interface {
    Log(agentName, phase, content string)   // phase: plan/execute/observe/reflect/output
    GetTraces(agentName string) []ThinkLog
    AllTraces() []ThinkLog
}
```

当前 `ConsoleThinker` 已创建并注入 `AgentFactory`，但 `Log()` 方法未被调用。设计意图是记录 ReAct Agent 每轮推理步骤，未来可用于：
- 推理链的语义压缩（替代原始 tool_call 消息）
- 调试 Agent 决策路径
- 提供前端 "思考过程" 展示

---

## 十四、Checkpoint / 容错

- Redis-backed `CheckpointStore`：实现 Eino 的 `compose.CheckPointStore` 接口
- 每个步骤后自动保存 `InterviewState` 的 JSON 快照
- TTL 过期机制（默认 3600s）
- 三层 fallback 读取：内存状态 → Redis checkpoint → MySQL 长存

---

## 十五、Casual Chat 中的 ReAct Agent 流程

```
用户消息
    ↓
系统指令:
  "你是 InterviewAgent，配有搜索工具。
   对技术问题，必须先用工具查实，不得编造。
   GitHub: 找开源项目、README、代码示例
   Web: 找教程、文档、最新资讯"
    ↓
ReAct 循环 (maxSteps=10):
  Round 1: LLM 判断 → tool_call(web_search_search) → Tool Result (可能被 ResultFilter 裁剪)
  Round 2: LLM 看到累积上下文 → tool_call(github_search_repositories) → Tool Result
  Round N: LLM 认为信息足够 → 生成最终回复
    ↓
如果 Agent 30s 超时 → fallback 到直接 LLM (stream_fallback)
```

---

## 十六、关键文件索引

| 文件 | 行数 | 职责 |
|------|------|------|
| `internal/app/wire.go` | ~414 | DI 组装，唯一入口 |
| `internal/orchestration/orchestrator.go` | ~1354 | 中心协调器，实现 InterviewService |
| `internal/orchestration/interview/runner.go` | ~287 | 面试阶段执行 |
| `internal/orchestration/interview/graph.go` | ~222 | Eino 图编译 |
| `internal/orchestration/interview/nodes/interviewer.go` | ~471 | Q&A 循环 + LLM 决策 |
| `internal/orchestration/interview/nodes/evaluation.go` | ~175 | 多维度评分 |
| `internal/orchestration/interview/nodes/review_planning.go` | ~431 | 报告生成 + 资源搜索 |
| `internal/orchestration/contextmanager/builder.go` | ~226 | Prompt 组装 + 预算控制 |
| `internal/orchestration/contextmanager/compressor.go` | ~389 | 对话压缩 |
| `internal/orchestration/router/host.go` | ~167 | LLM 意图分类 + 分发 |
| `internal/capability/mcp/manager.go` | ~187 | MCP Server 生命周期 |
| `internal/capability/mcp/eino_bridge.go` | ~143 | MCP → Eino 工具桥接 |
| `internal/interaction/gateway.go` | ~137 | InterviewService 接口 |
| `internal/observability/eino_callbacks.go` | ~311 | Eino 全链路回调 |

---

## 十七、启动流程总览

```
1. config.Load("config.yaml")           → 解析配置
2. observability.RegisterEinoCallbacks() → 注册全局回调
3. LLM/Embedding 初始化                  → DeepSeek API + Qwen Embedding
4. Redis + MySQL 连接                    → 存储层
5. Milvus + Bleve 初始化                 → 向量 + 关键词检索引擎
6. MCP Manager.Start()                  → 启动 GitHub + Web Search 子进程
7. EinoBridge.discoverTools()           → 发现 MCP 工具，包装为 EinoTool
8. Memory + ContextBuilder 初始化        → 记忆 + 上下文管理
9. Hybrid RAG Retriever 组装             → 向量 + 关键词混合检索
10. AgentFactory 创建                    → 4 个 ReAct Agent (不同工具权限)
11. Skill Registry 注册 8 种技能         → 含 checkpoint 持久化
12. Interview Graph 编译                 → Eino DAG 编译
13. Orchestrator 组装                    → 注入全部依赖
14. REST Router + WS Hub 启动            → 开始接受请求
```
