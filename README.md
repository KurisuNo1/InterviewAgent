# InterviewAgent

基于 Go 语言 Eino 框架的多 Agent 协作面试系统，支持 Web 前端、微信小程序和 REST API 三种接入方式。

## 三层架构

```
┌──────────────────────────────────────────────────────────────┐
│  用户交互层 (Layer 1)                                         │
│  Web前端  │  微信小程序  │  REST API (Gin)  │  WebSocket      │
│                     ↕ InterviewService 接口                   │
├──────────────────────────────────────────────────────────────┤
│  Agent编排层 (Layer 2) — 基于 Eino 框架                       │
│                                                              │
│  compose.NewGraph → Setup DAG (3节点) + Interview DAG (3节点) │
│  compose.StatefulInterrupt → 每题提问后暂停等用户输入         │
│  compose.CheckPointStore (Redis) → checkpoint 持久化接口      │
│                                                              │
│  意图路由 (Host-Specialist)  │  8个可插拔 Skill 模块          │
│  Memory (Redis+MySQL)  │  ContextManager (6层上下文管理)       │
│  Agent工厂 (ReAct Agent + MCP工具)                            │
│                     ↕ capability 接口                         │
├──────────────────────────────────────────────────────────────┤
│  基础能力层 (Layer 3)                                         │
│  LLM(DeepSeek) │ Embedding(Qwen3) │ Milvus 向量库             │
│  BM25(Bleve) │ MCP(GitHub+Web) │ Redis+MySQL                 │
│  简历解析 │ 文档分块 │ JWT认证                                │
└──────────────────────────────────────────────────────────────┘
```

### Eino 框架使用方式

项目通过 **Eino 框架原生 API** 实现 Agent 编排：

| Eino API | 使用位置 | 作用 |
|----------|----------|------|
| `compose.NewGraph[*InterviewState]` | `interview/graph.go` | 构建 Setup DAG 和 Interview DAG |
| `compose.InvokableLambda` | `interview/graph.go` | 将 Agent 节点包装为 Lambda 节点 |
| `compose.AddLambdaNode` + `compose.AddEdge` | `interview/graph.go` | 连接节点形成 DAG |
| `compose.StatefulInterrupt` | `interview/graph.go` interviewer Lambda | 每题提问后暂停 Graph 执行 |
| `compose.CheckPointStore` | `capability/store/checkpoint.go` | Redis 实现的 checkpoint 存储接口 |
| `compose.GraphCompileOption` | `interview/graph.go` | 图编译选项 (MaxRunSteps, GraphName, CheckPointStore) |
| `compose.Runnable.Invoke` | `interview/runner.go` | 执行编译后的 Graph |
| `eino/components/model` | 通过 wire 注入 | DeepSeek ChatModel (OpenAI 兼容协议) |
| `eino/components/embedding` | 通过 wire 注入 | Qwen3 Embedding 向量化 |
| `eino-ext/components/tool/mcp` | `capability/mcp/` | GitHub/Web MCP 工具调用 |

### 面试执行模式

面试循环采用**直接调用节点**模式，而非 Graph Resume：

- **Setup 阶段**：通过 `setupGraph.Invoke()` 走 Eino Graph 执行 (JD分析→简历匹配→出题规划)
- **Interview 循环**：Orchestrator 直接调用 `InterviewerNode.ProcessAnswer()` / `AskCurrentQuestion()`，每轮手动保存 checkpoint 到 Redis
- **Checkpoint**：应用层手动 JSON 序列化 `InterviewState` 后写入 Redis，非 Eino 自动管理
- **中断恢复**：`StatefulInterrupt` 在 Graph 定义中存在（供未来 Graph Resume 路径使用），当前主流程通过手动 `SaveCheckpoint`/`LoadCheckpoint` 实现

### 设计原则

- **换大模型** → 只改基础层配置 (`llm.base_url` + `llm.model`)
- **加新 Agent** → 在对应 DAG 中 AddLambdaNode
- **前端接入** → 只对接 InterviewService 接口
- **上下文管理** → 所有 LLM 调用统一通过 ContextBuilder 组装 prompt

---

## 功能列表

1. **JD智能解析** — 粘贴岗位JD，AI自动提取技术栈、职级要求、核心能力项
2. **简历深度匹配** — 上传PDF/Word简历，AI分析匹配度，找出优势和短板
3. **智能出题规划** — 根据JD+简历+RAG题库检索，自动规划题目类型和难度分布
4. **多轮模拟面试** — AI面试官逐题提问，根据回答实时追问深挖，三级难度自适应
5. **实时评估打分** — 每题即时四维评分(技术准确性/回答深度/沟通表达/项目经验匹配度)
6. **个性化复习计划** — 基于薄弱点生成复习路径，MCP推荐GitHub开源学习资源
7. **意图路由** — LLM自动识别面试/技能练习/闲聊三种意图，分流到对应处理链路
8. **8个技能练习模块** — 算法编程、系统设计、行为面试(STAR)、技术测验、快速问答、知识讲解、项目亮点提炼、技术对比
9. **面试中断恢复** — Redis Checkpoint持久化，支持精确恢复到上次题目和对话状态
10. **流式输出** — SSE 实时流式返回面试官回复，提升交互体验
11. **文档上传与知识库** — 支持上传文档建立私有知识库，混合检索增强回答
12. **上下文监控** — 运行时追踪每次LLM调用的token消耗，80%/95%两级告警
13. **多种接入方式** — Web前端 + 微信小程序 + REST API + WebSocket 实时交互
14. **用户认证** — JWT登录/注册 + 微信小程序登录

---

## 各层组件

### Layer 3: 基础能力层

| 组件 | 选型 | 说明 |
|------|------|------|
| LLM | DeepSeek (OpenAI兼容) | `https://api.deepseek.com/v1`，模型 deepseek-chat |
| Embedding | Qwen3-Embedding-0.6B | 1024维向量，通过 ModelScope API |
| 向量库 | Milvus | ANN语义检索，eino-ext内置支持 |
| 关键词索引 | Bleve BM25 | 本地文件索引，混合检索关键词部分 |
| MCP工具 | GitHub + Web Search | 推荐学习资源，eino-ext MCP客户端 |
| 缓存/Checkpoint | Redis | go-redis，短期记忆+中断恢复 |
| 持久化 | MySQL | go-sql-driver，长期记忆+历史记录+用户数据 |
| 简历解析 | 自研 parser | 支持 PDF/DOCX 文本提取 |
| 文档分块 | 自研 splitter | 固定大小/递归/ Markdown 三种分块策略 |
| 认证 | JWT + 微信OAuth | 用户登录/注册/小程序登录 |

### Layer 2: Agent编排层

#### 意图路由器 (Host-Specialist模式)

所有用户请求先经过意图路由器，LLM分类为面试、技能练习或闲聊，分流到对应处理链路。

#### 面试 DAG (两个独立 Graph)

**Setup DAG (Eino Graph 执行):**
```
START → JDAnalysis → ResumeMatching → QuestionPlanning → END
```

**Interview DAG (含 Interrupt，供未来 Graph Resume 使用):**
```
START → Interviewer (StatefulInterrupt) → Evaluation → ReviewPlanning → END
```

实际面试循环中，Orchestrator 直接调用节点方法而非走 Graph Resume，每轮手动保存 checkpoint。

| Agent节点 | 职责 | 关键依赖 |
|-----------|------|----------|
| JDAnalysisNode | 解析JD文本，提取结构化要求 | LLM |
| ResumeMatchingNode | 简历与JD匹配分析 | LLM + 简历解析器 |
| QuestionPlanningNode | 基于JD+简历+RAG规划题目 | LLM + Milvus + Bleve + Embedding |
| InterviewerNode | 提问、追问决策(三层回退解析)、节奏控制 | LLM |
| EvaluationNode | 四维评分+反馈 | LLM |
| ReviewPlanningNode | 复习计划+资源推荐 | LLM + MCP GitHub |

**面试官决策解析 (三层回退):**
1. JSON 解析 — 从响应末尾提取 `{"action": "..."}` 结构化决策
2. 关键词检测 — 匹配 `NEXT_QUESTION`、`下一题`、`INTERVIEW_COMPLETE` 等信号
3. LLM 分类器 — 小参数 LLM 调用做单词语义分类

#### Skill技能系统 (8个可插拔模块)

基于统一 `Skill` 接口 (Name/Description/CanHandle/Handle/NewSession/WelcomeMessage/Category)：

| 技能 | sub_intent | 说明 |
|------|-----------|------|
| algorithm | algorithm | LeetCode风格算法练习，提示+反馈 |
| system_design | system_design | 系统设计面试引导(需求→架构→权衡) |
| behavioral | behavioral | STAR行为面试法练习 |
| tech_quiz | tech_quiz | 技术知识测验(多轮评分) |
| quick_quiz | quick_quiz | 快速问答(单轮即时反馈) |
| knowledge_explain | knowledge_explain | 知识点讲解模式 |
| project_highlight | project_highlight | 项目亮点提炼与表达优化 |
| tech_compare | tech_compare | 技术方案对比分析 |

每个技能持有独立多轮会话状态，经 Redis checkpoint 持久化，支持中断恢复。

#### Memory记忆系统

- **短期记忆 (Redis)** — 会话对话窗口(最近30轮=60条消息)，支持上下文感知
- **长期记忆 (MySQL)** — 面试历史、评估报告、复习计划、用户画像持久化
- **三层回退读取**: 内存 → Redis checkpoint → MySQL

#### ContextManager 上下文管理

六层机制应对长对话 token 膨胀和关键信息丢失：

| 层次 | 组件 | 说明 |
|------|------|------|
| 窗口分配 | `budget.go` | 总预算 32K tokens，6种场景差异化分配 |
| 记忆分层 | `hierarchy.go` | 工作记忆(LLM上下文) → 短期(Redis) → 长期(MySQL) |
| 压缩策略 | `compressor.go` | 滑动窗口+渐进摘要 / 结构化提取 / LLM摘要 |
| 上下文编排 | `builder.go` | 统一 Build() 入口，按优先级打包 prompt |
| 溢出降级 | `overflow.go` | 5级降级：裁剪工具结果→压缩历史→减半RAG→简化提示词→建议新会话 |
| 运行时监控 | `monitor.go` | 每次调用上报token消耗，80%/95%两级告警 |

#### 其他编排组件

| 组件 | 位置 | 说明 |
|------|------|------|
| Agent工厂 | `agent/factory.go` | 创建 ReAct Agent (LLM + MCP工具)，用于自主搜索场景 |
| 难度状态机 | `difficulty/difficulty.go` | 三级难度自适应，连续答对/答错达阈值自动升降 |
| 文档摄取 | `ingestion/ingestion.go` | 文档上传→分块→向量化→入库 全流程 |
| 混合检索 | `rag/fusion.go` | Milvus语义(0.7) + Bleve关键词(0.3)，LLM二阶重排序 |
| RAG评估器 | `rag/evaluation.go` | LLM评估检索质量(忠实度/相关性/完整性)，支持TopK实验 |
| 可观测性 | `observability/eino_callbacks.go` | Eino回调钩子，记录LLM/Embedding/Tool调用耗时和token |

### Layer 1: 用户交互层

| 接入方式 | 技术 | 适用场景 |
|----------|------|----------|
| Web 前端 | 原生 HTML/JS (SSE流式) | 桌面浏览器，完整功能 |
| 微信小程序 | 原生小程序框架 | 移动端面试练习 |
| REST API | Gin | API集成、第三方对接 |
| WebSocket | gorilla/websocket | 实时双向通信、事件推送 |

统一 `InterviewService` 接口，所有接入方式调用同一后端逻辑。

---

## 核心流程

```
[开始] → 用户认证 (JWT/微信登录)
              ↓
        意图路由 (LLM分类)
         ↙    ↓    ↘
   面试分流  技能练习  闲聊
       ↓
  创建会话 → JD解析 → 简历匹配 → 出题规划
                                       ↓
                               ┌──────────────┐
                               │   面试循环     │
                               │  ┌──────────┐ │
                               │  │ AI提问   │ │
                               │  │  ↓      │ │
                               │  │ 用户回答 │←── 手动 SaveCheckpoint
                               │  │  ↓      │ │    (JSON→Redis)
                               │  │ 追问?   │ │
                               │  │  ↓否    │ │
                               │  │ 评分(异步)│ │
                               │  │  ↓      │ │
                               │  │ 下一题  │ │
                               │  └──────────┘ │
                               └──────┬─────────┘
                                      ↓
                               生成评估报告 → 生成复习计划 → [结束]
```

面试循环中，Orchestrator 直接调用 InterviewerNode 节点方法，每轮回答后异步执行评估和 checkpoint 保存。Checkpoint 持久化到 Redis (默认 TTL 1小时)，支持断线重连恢复。

---

## 技术栈

| 层次 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.22+ | |
| Agent框架 | **Eino v0.9.x** | `cloudwego/eino` + `eino-ext` |
| Agent模式 | DAG + 直接节点调用 | Setup阶段走Graph，Interview循环直接调节点 |
| LLM | DeepSeek (OpenAI兼容) | 通过 Eino ChatModel 组件接入 |
| Embedding | Qwen3-Embedding-0.6B | 通过 Eino Embedder 组件接入，ModelScope API |
| 向量存储 | **Milvus** | 题库RAG语义检索 |
| 关键词索引 | **Bleve BM25** | 混合检索关键词部分 |
| 简历解析 | 自研 PDF/DOCX parser | `capability/resume/` |
| 文档分块 | 自研 splitter | 固定大小/递归/Markdown 三种策略 |
| MCP | eino-ext MCP Client | GitHub + Web Search |
| HTTP框架 | **Gin** | REST API |
| WebSocket | **gorilla/websocket** | 实时双向通信 |
| Checkpoint | **Redis** | 面试中断恢复 + 技能会话持久化 |
| Memory | **Redis** (短期) + **MySQL** (长期) | 三层回退读取 |
| 认证 | **JWT** + 微信OAuth | 用户登录/注册 |
| 配置 | Viper | YAML配置 + 环境变量展开 |

---

## 目录结构

```
InterviewAgent/
├── docker-compose.yaml               # Docker 部署 (Redis+MySQL+Milvus+etcd+minio+App)
├── Dockerfile                         # 应用容器构建
├── .env.example                       # 环境变量模板
├── .gitignore
├── go.mod / go.sum
├── README.md
├── interview_prer.md                  # 项目设计概述
├── cmd/
│   ├── server/main.go                 # HTTP+WS服务入口 (静态Web + API + WebSocket)
│   └── cli/main.go                    # CLI入口
├── config/
│   ├── config.yaml                    # 主配置文件
│   └── config.go                      # Viper加载+校验+环境变量展开
├── docker/
│   └── mysql/init.sql                 # MySQL 初始化建表
├── web/                               # Web 前端 (原生 HTML/JS/CSS)
│   ├── index.html
│   ├── css/
│   └── js/
├── miniprogram/                       # 微信小程序前端
│   ├── app.js / app.json / app.wxss
│   └── pages/
├── internal/
│   ├── app/
│   │   └── wire.go                    # 依赖注入容器 (L3→L2→L1 全量组装)
│   ├── interaction/                   # L1: 用户交互层
│   │   ├── gateway.go                 # InterviewService 接口定义
│   │   ├── rest/                      # REST API (Gin)
│   │   │   ├── router.go              # 路由注册 + 中间件
│   │   │   ├── handler.go             # 请求处理
│   │   │   ├── middleware.go          # CORS/日志/恢复 中间件
│   │   │   ├── dto.go                 # 请求/响应 DTO
│   │   │   └── auth/                  # JWT认证 + 微信登录
│   │   │       ├── jwt.go
│   │   │       └── handler.go
│   │   └── ws/                        # WebSocket
│   │       ├── hub.go                 # 连接管理Hub
│   │       ├── client.go              # 客户端连接
│   │       └── protocol.go            # 消息协议
│   ├── orchestration/                 # L2: Agent编排层
│   │   ├── orchestrator.go            # 实现 InterviewService 接口
│   │   ├── router/                    # 意图路由 (Host-Specialist)
│   │   │   ├── host.go                # 路由主机 + LLM分类
│   │   │   ├── intent.go              # 意图定义
│   │   │   ├── specialist.go          # Specialist接口
│   │   │   ├── specialists.go         # 各Specialist实现
│   │   │   └── casual_chat.go         # 闲聊处理 (ReAct Agent)
│   │   ├── interview/                 # 面试编排
│   │   │   ├── graph.go               # Setup DAG + Interview DAG 构建
│   │   │   ├── runner.go              # 节点调用 + Checkpoint 存取
│   │   │   └── nodes/                 # Agent节点实现
│   │   │       ├── state.go           # InterviewState 定义
│   │   │       ├── helpers.go         # extractJSON / safeFmt 工具
│   │   │       ├── jd_analysis.go     # JD解析节点
│   │   │       ├── resume_matching.go # 简历匹配节点
│   │   │       ├── question_planning.go # 出题规划节点
│   │   │       ├── interviewer.go     # 面试官节点 (提问+决策)
│   │   │       ├── evaluation.go      # 评估节点 (四维评分)
│   │   │       ├── review_planning.go # 复习规划节点
│   │   │       └── prompts/           # Prompt模板
│   │   │           ├── interviewer_prompt.go
│   │   │           ├── evaluation_prompt.go
│   │   │           ├── jd_analysis_prompt.go
│   │   │           ├── resume_match_prompt.go
│   │   │           ├── question_plan_prompt.go
│   │   │           └── review_plan_prompt.go
│   │   ├── skill/                     # Skill技能系统 (8个模块)
│   │   │   ├── skill.go               # Skill接口 + SkillState定义
│   │   │   ├── registry.go            # 技能注册中心 + checkpoint管理
│   │   │   ├── algorithm.go           # 算法练习
│   │   │   ├── system_design.go       # 系统设计
│   │   │   ├── behavioral.go          # 行为面试 (STAR)
│   │   │   ├── tech_quiz.go           # 技术测验
│   │   │   ├── quick_quiz.go          # 快速问答
│   │   │   ├── knowledge_explain.go   # 知识讲解
│   │   │   ├── project_highlight.go   # 项目亮点提炼
│   │   │   └── tech_compare.go        # 技术对比
│   │   ├── memory/                    # Memory记忆系统
│   │   │   ├── manager.go             # 记忆管理器 (Redis+MySQL门面)
│   │   │   ├── short_term.go          # 短期记忆 (Redis)
│   │   │   └── long_term.go           # 长期记忆 (MySQL)
│   │   ├── contextmanager/             # 上下文管理 (6层机制)
│   │   │   ├── budget.go              # Token预算分配
│   │   │   ├── builder.go             # 统一prompt构建入口
│   │   │   ├── compressor.go          # 对话压缩 (3种策略)
│   │   │   ├── hierarchy.go           # 三层记忆协调
│   │   │   ├── overflow.go            # 溢出降级 (5级策略)
│   │   │   ├── monitor.go             # 运行时token监控
│   │   │   ├── usage.go               # 使用量数据结构
│   │   │   ├── reasoning_compressor.go # 推理步骤压缩
│   │   │   └── tool_result_filter.go  # 工具结果过滤
│   │   ├── agent/                     # Agent工厂
│   │   │   ├── factory.go             # ReAct Agent 创建
│   │   │   ├── thinker.go             # 思维日志
│   │   │   ├── types.go               # Agent类型定义
│   │   │   └── memory.go              # Agent间共享内存
│   │   ├── rag/                       # RAG检索
│   │   │   ├── fusion.go              # 混合检索 + LLM重排序
│   │   │   └── evaluation.go          # RAG质量评估
│   │   ├── ingestion/                 # 文档摄取
│   │   │   └── ingestion.go           # 上传→分块→向量化
│   │   └── difficulty/                # 难度管理
│   │       └── difficulty.go          # 三级难度状态机
│   ├── capability/                    # L3: 基础能力层
│   │   ├── mcp/                       # MCP外部工具
│   │   │   ├── client.go              # MCP客户端
│   │   │   ├── manager.go             # MCP管理器
│   │   │   ├── github.go              # GitHub搜索工具
│   │   │   ├── web_search.go          # 网络搜索工具
│   │   │   ├── eino_bridge.go         # Eino-MCP桥接
│   │   │   └── eino_tool.go           # Eino工具适配
│   │   ├── vector/                    # Milvus向量库
│   │   │   ├── retriever.go           # 检索器接口
│   │   │   └── milvus.go              # Milvus实现
│   │   ├── keyword/                   # BM25关键词索引
│   │   │   ├── index.go               # 索引接口
│   │   │   └── bleve.go               # Bleve实现
│   │   ├── store/                     # 存储引擎
│   │   │   ├── redis.go               # Redis连接管理
│   │   │   ├── mysql.go               # MySQL连接管理
│   │   │   ├── checkpoint.go          # CheckpointStore (Eino接口)
│   │   │   └── user_store.go          # 用户数据存储
│   │   ├── resume/                    # 简历解析
│   │   │   └── parser.go              # PDF/DOCX文本提取
│   │   └── chunk/                     # 文档分块
│   │       ├── splitter.go            # 分块器实现
│   │       ├── eino_splitter.go       # Eino分块适配
│   │       └── splitter_test.go       # 单元测试
│   ├── model/                         # 共享数据模型
│   │   ├── interview.go               # 面试阶段/会话定义
│   │   ├── question.go                # 题目/出题计划
│   │   ├── score.go                   # 评分维度/评估结果
│   │   ├── report.go                  # 报告/复习计划
│   │   ├── message.go                 # 消息模型
│   │   ├── jd.go                      # JD分析结果
│   │   └── resume.go                  # 简历匹配结果
│   └── observability/                 # 可观测性
│       └── eino_callbacks.go          # Eino回调 (LLM/Embedding/Tool日志)
```

---

## API设计

### 认证接口

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/auth/register` | POST | 用户注册 |
| `POST /api/auth/login` | POST | 用户登录 (返回JWT) |
| `POST /api/auth/wechat-login` | POST | 微信小程序登录 |
| `GET /api/auth/me` | GET | 获取当前用户信息 |

### 面试会话

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/sessions` | POST | 创建面试会话 |
| `GET /api/sessions` | GET | 列出用户的所有会话 |
| `GET /api/sessions/:id` | GET | 获取会话信息 |
| `POST /api/sessions/:id/jd` | POST | 提交JD并解析 |
| `POST /api/sessions/:id/resume` | POST | 上传简历文件 |
| `GET /api/sessions/:id/plan` | GET | 获取出题计划 |
| `POST /api/sessions/:id/start` | POST | 开始面试(返回第一题) |
| `POST /api/sessions/:id/answer` | POST | 提交回答(返回评估+下一题/追问) |
| `POST /api/sessions/:id/answer/stream` | POST | 流式提交回答 (SSE) |
| `POST /api/sessions/:id/skip` | POST | 跳过当前题 |
| `POST /api/sessions/:id/complete` | POST | 提前结束面试 |
| `POST /api/sessions/:id/restore` | POST | 恢复中断的面试 |
| `GET /api/sessions/:id/report` | GET | 获取评估报告 |
| `GET /api/sessions/:id/review-plan` | GET | 获取复习计划 |
| `GET /api/sessions/:id/messages` | GET | 获取对话历史 |
| `GET /api/sessions/:id/context/stats` | GET | 获取单会话上下文统计 |

### 通用消息与流式

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/sessions/:id/message` | POST | 通用消息(意图路由自动分流) |
| `POST /api/sessions/:id/stream` | POST | 流式消息 (SSE实时输出) |

### 技能与工具

| 端点 | 方法 | 说明 |
|------|------|------|
| `GET /api/skills` | GET | 列出所有可用技能模块 |
| `GET /api/tools` | GET | 列出所有可用MCP工具 |

### 文档管理

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/documents/upload` | POST | 上传文档建立知识库 |
| `GET /api/documents` | GET | 列出已上传文档 |
| `DELETE /api/documents/:id` | DELETE | 删除文档 |

### 监控

| 端点 | 方法 | 说明 |
|------|------|------|
| `GET /api/context/stats` | GET | 全局上下文使用统计 |

### WebSocket

| 端点 | 说明 |
|------|------|
| `GET /ws` | WebSocket 实时交互 (事件推送) |

---

## 快速开始

### 前置依赖

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| **Docker + Docker Compose** | 一键启动所有基础服务 + 应用 | 推荐 |
| **DeepSeek API Key** | LLM 对话与推理 (deepseek-chat) | 是 |
| **DashScope API Key** | Embedding 向量化 (可选) | 推荐 |
| **Go 1.22+** | 本地编译运行 | 非 Docker 方式必须 |

### 方式一: Docker Compose 一键启动 (推荐)

```bash
# 1. 克隆项目
git clone https://github.com/KurisuNo1/InterviewAgent.git
cd InterviewAgent

# 2. 设置环境变量
cp .env.example .env
# 编辑 .env，填入你的 API Key:
#   DEEPSEEK_API_KEY=sk-xxx        (必须)
#   DASHSCOPE_API_KEY=sk-xxx      (推荐)
#   MYSQL_ROOT_PASSWORD=xxx        (可选，默认 interview123)
#   GITHUB_TOKEN=xxx               (可选，MCP GitHub工具)
#   WECHAT_APPID=xxx               (可选，微信小程序)

# 3. 启动全部服务 (Redis + MySQL + Milvus + etcd + minio + App)
docker compose up -d

# 4. 查看日志
docker compose logs -f app

# 5. 验证
curl http://localhost:8080/health
# → {"status":"ok"}
```

Docker Compose 会启动以下服务:

| 服务 | 端口 | 说明 |
|------|------|------|
| `app` | 8080 | InterviewAgent REST + WebSocket + Web前端 |
| `redis` | 6379 | Checkpoint + 短期记忆 |
| `mysql` | 3307→3306 | 长期存储 (自动建库建表) |
| `milvus` | 19530 | 向量库 (题库 RAG) |
| `etcd` | — | Milvus 元数据存储 |
| `minio` | — | Milvus 对象存储 |

### 方式二: 本地开发运行

```bash
# 1. 启动基础服务
docker compose up -d redis mysql milvus

# 2. 设置环境变量
export DEEPSEEK_API_KEY="sk-your-deepseek-key"
export DASHSCOPE_API_KEY="sk-your-dashscope-key"
export MYSQL_ROOT_PASSWORD="interview123"

# 3. 编译运行
go build -o interview-server cmd/server/main.go
./interview-server config/config.yaml

# 或直接 go run
go run cmd/server/main.go config/config.yaml
```

启动成功后会显示:
```
╔══════════════════════════════════════════╗
║   InterviewAgent Server                  ║
╠══════════════════════════════════════════╣
║  REST API:  http://localhost:8080/api    ║
║  WebSocket: ws://localhost:8080/ws       ║
║  Health:    http://localhost:8080/health ║
║  LLM:       deepseek-chat                ║
║  Web     :  http://localhost:8080        ║
╚══════════════════════════════════════════╝
```

### 验证服务

```bash
# 健康检查
curl http://localhost:8080/health

# 创建面试会话
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"jd_text": "Go后端工程师，3年经验，熟悉K8s和微服务"}'

# WebSocket 连接
wscat -c "ws://localhost:8080/ws?session_id=test-session"

# 浏览器访问 Web 前端
open http://localhost:8080
```

---

## 配置参数详解

`config/config.yaml` 完整参数说明:

### 服务器配置 `server`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `localhost` | 监听地址 |
| `port` | int | `8080` | HTTP/WS 端口 |
| `ws_path` | string | `/ws` | WebSocket 升级路径 |
| `read_timeout` | duration | `30s` | 请求读取超时 |
| `write_timeout` | duration | `60s` | 响应写入超时 |
| `jwt_secret` | string | — | JWT签名密钥 (支持 `${VAR:-default}`) |
| `jwt_expiry` | duration | `24h` | JWT过期时间 |

### LLM 配置 `llm`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `qwen` | 厂商标识 (Eino组件选择) |
| `base_url` | string | `https://api.deepseek.com/v1` | OpenAI 兼容 API 地址 |
| `api_key_env` | string | `DEEPSEEK_API_KEY` | 从该环境变量读取 API Key |
| `model` | string | `deepseek-chat` | 模型名称 |
| `temperature` | float | `0.7` | 生成温度 (0-2) |
| `max_tokens` | int | `4096` | 单次最大 Token 数 |
| `timeout` | duration | `60s` | 请求超时 |
| `max_retries` | int | `3` | 失败重试次数 (注: 当前代码中未实现重试逻辑) |

### Embedding 配置 `embedding`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `qwen` | 厂商标识 |
| `base_url` | string | `https://api-inference.modelscope.cn/v1` | ModelScope API |
| `api_key_env` | string | `DASHSCOPE_API_KEY` | 从该环境变量读取 API Key |
| `model` | string | `Qwen/Qwen3-Embedding-0.6B` | Embedding 模型 |
| `dimensions` | int | `1024` | 输出向量维度 |
| `timeout` | duration | `30s` | 请求超时 |

### 向量库 `vector_db`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | `milvus` | 向量库类型 |
| `host` | string | `localhost` | Milvus gRPC 地址 |
| `port` | int | `19530` | Milvus gRPC 端口 |
| `username` | string | `root` | 用户名 |
| `password` | string | `Milvus` | 密码 |
| `database` | string | `default` | 数据库名 |
| `collection` | string | `interview_question_bank` | 集合名称 |
| `dimension` | int | `1024` | 向量维度 (需与 Embedding 一致) |
| `index_type` | string | `IVF_FLAT` | 索引类型 |
| `metric_type` | string | `IP` | 相似度度量 (IP/L2/COSINE) |

### 关键词索引 `keyword_index`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | `bleve` | 索引引擎 |
| `index_path` | string | `./data/bleve_index` | 本地索引文件路径 |

### MCP 工具 `mcp`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `connection_timeout` | duration | `30s` | MCP Server 连接超时 |
| `servers[].name` | string | — | MCP Server 标识 |
| `servers[].command` | string | `npx` | 启动命令 |
| `servers[].args` | []string | — | 启动参数 |
| `servers[].env` | map | — | 环境变量 (支持 `${ENV}` 展开) |

### Redis `redis`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `localhost` | Redis 地址 |
| `port` | int | `6379` | Redis 端口 |
| `password` | string | `""` | 密码 (空=无密码) |
| `db` | int | `0` | 数据库编号 |
| `pool_size` | int | `10` | 连接池大小 |
| `min_idle_conns` | int | `5` | 最小空闲连接数 |
| `dial_timeout` | duration | `5s` | 连接超时 |
| `read_timeout` | duration | `3s` | 读取超时 |
| `write_timeout` | duration | `3s` | 写入超时 |

### MySQL `mysql`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `localhost` | MySQL 地址 |
| `port` | int | `3307` | MySQL 端口 |
| `user` | string | `root` | 用户名 |
| `password_env` | string | `MYSQL_ROOT_PASSWORD` | 从该环境变量读取密码 |
| `database` | string | `interview_agent` | 数据库名 (Docker自动创建) |
| `charset` | string | `utf8mb4` | 字符集 |
| `max_open_conns` | int | `25` | 最大连接数 |
| `max_idle_conns` | int | `10` | 最大空闲连接数 |
| `conn_max_lifetime` | string | `5m` | 连接最大存活时间 |

### 面试流程 `interview`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_questions` | int | `10` | 每场面试最多题目数 |
| `max_follow_ups` | int | `3` | 每题最多追问次数 |
| `time_per_question` | int | `300` | 每题建议时间 (秒) |
| `checkpoint_ttl` | duration | `3600s` | Checkpoint 有效期 |
| `difficulty_up_threshold` | int | `2` | 连续答对N题升难度 |
| `difficulty_down_threshold` | int | `2` | 连续答错N题降难度 |
| `scoring_dimensions` | array | 4个维度 | 评分维度 (名称/权重/满分) |

### 混合检索 `rag`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `vector_weight` | float | `0.7` | Milvus 语义检索权重 |
| `keyword_weight` | float | `0.3` | Bleve BM25 关键词权重 |
| `top_k` | int | `3` | 各检索器候选数 |
| `final_top_k` | int | `3` | 融合后最终题目数 |

### 记忆系统 `memory`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `short_term.max_messages` | int | `30` | 对话窗口大小 (轮数) |
| `short_term.ttl` | duration | `7200s` | 短期记忆过期时间 |
| `long_term.max_history` | int | `50` | 长期历史记录上限 |

### 上下文管理 `context`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_tokens` | int | `32768` | 总 token 预算 |
| `warning_threshold` | float | `0.80` | 警告阈值 (80%) |
| `critical_threshold` | float | `0.95` | 严重告警阈值 (95%) |
| `profiles.<name>.system_max` | int | — | 系统提示词最大 tokens |
| `profiles.<name>.working_memory` | int | — | 工作记忆区预算 |
| `profiles.<name>.rag_max` | int | — | RAG 文档最大 tokens |
| `profiles.<name>.recent_verbatim_turns` | int | `3` | 保留原文的最近对话轮数 |
| `profiles.<name>.history_max_turns` | int | — | 最多保留历史轮数 |
| `profiles.<name>.compression_threshold_turns` | int | — | 超过该轮数触发压缩 |

支持的 profile: `casual_chat`, `interview_ask`, `interview_eval`, `skill`, `stream_fallback`, `stream_agent`

### 技能模块 `skills`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | — | 技能名称 |
| `sub_intent` | string | — | 意图路由匹配的子意图 |
| `enabled` | bool | `true` | 是否启用 |

### 文档上传 `upload`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_file_size` | int | `10485760` | 最大文件大小 (10MB) |
| `chunk_size` | int | `1000` | 分块大小 (字符) |
| `chunk_overlap` | int | `200` | 分块重叠 (字符) |

### 微信小程序 `wechat`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `app_id` | string | — | 微信小程序 AppID (支持 `${WECHAT_APPID}`) |
| `app_secret` | string | — | 微信小程序 AppSecret (支持 `${WECHAT_APPSECRET}`) |

### 日志 `logging`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `level` | string | `info` | 日志级别 (debug/info/error) |
| `format` | string | `json` | 日志格式 (注: 当前使用 log.Printf，未实现JSON格式) |
| `output` | string | `stdout` | 输出目标 |
| `file_path` | string | `./logs/interview_agent.log` | 日志文件路径 |

---

## 设计决策

1. **三层分层** — 交互层/编排层/能力层，层间接口解耦，可独立替换
2. **两个独立 DAG** — Setup DAG (3节点，Graph执行) + Interview DAG (3节点，含Interrupt)。面试循环不走 Graph Resume，而是直接调用节点方法，减少框架开销
3. **应用层 Checkpoint** — 手动 JSON 序列化 `InterviewState` 存入 Redis，灵活控制保存时机；CheckpointStore 实现 Eino 接口以兼容框架，但序列化由应用层管理
4. **三层决策回退** — 面试官决策：JSON解析 → 关键词检测 → LLM分类器，确保 LLM 输出不稳定时仍能正确路由
5. **意图路由** — Host-Specialist 模式，LLM 自动分类意图，支持 `skill:name:input` 前缀绕过 LLM 分类
6. **混合检索** — Milvus 语义 (0.7) + Bleve BM25 关键词 (0.3)，LLM 二阶重排序，失败时降级为截断
7. **双层记忆 + 三层回退** — Redis 短期 + MySQL 长期；读取路径: 内存 → Redis checkpoint → MySQL
8. **8个可插拔 Skill** — 统一接口 (Name/Description/CanHandle/Handle/NewSession/WelcomeMessage/Category)，注册即可用，独立 checkpoint 持久化
9. **上下文管理六层机制** — 窗口分配/记忆分层/压缩策略/上下文编排/溢出降级/运行时监控，确保长对话不丢关键信息
10. **动态难度调整** — 三级难度状态机，连续答对/答错达阈值自动升降，影响面试官提示词和出题配比
11. **流式输出 + 异步评估** — SSE 实时流式返回回复，评估和 checkpoint 持久化在后台 goroutine 异步执行，不阻塞 HTTP 响应
12. **多种接入方式** — Web前端/微信小程序/REST API/WebSocket，统一 InterviewService 接口
