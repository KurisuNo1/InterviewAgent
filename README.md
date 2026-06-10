# InterviewAgent

基于 Go 语言 Eino 框架的多 Agent 协作面试系统。

## 三层架构

```
┌──────────────────────────────────────────────────────────────┐
│  用户交互层 (Layer 1)                                         │
│  CLI (Cobra)  │  WebSocket  │  REST API (Gin)                │
│                     ↕ InterviewService 接口                   │
├──────────────────────────────────────────────────────────────┤
│  Agent编排层 (Layer 2) — 基于 Eino 框架                       │
│                                                              │
│  compose.NewGraph → 6 Agent Lambda 节点 + Branch 条件路由     │
│  compose.StatefulInterrupt → 每题提问后暂停等用户输入         │
│  compose.ResumeWithData → 用户回答后恢复 Graph 执行            │
│  compose.CheckPointStore (Redis) → 自动 checkpoint 持久化     │
│                                                              │
│  意图路由 + 4个 Skill 模块  │  Memory (Redis+MySQL)           │
│                     ↕ capability 接口                         │
├──────────────────────────────────────────────────────────────┤
│  基础能力层 (Layer 3)                                         │
│  LLM(DeepSeek v4) │ Embedding(text-embedding-v3) │ Milvus    │
│  BM25(Bleve) │ MCP(GitHub+Web) │ Redis+MySQL                 │
└──────────────────────────────────────────────────────────────┘
```

### Eino 框架使用方式

项目通过 **Eino 框架原生 API** 实现 Agent 编排，而非自行实现：

| Eino API | 使用位置 | 作用 |
|----------|----------|------|
| `compose.NewGraph[*InterviewState]` | `interview/graph.go` | 构建 7 节点 DAG Graph |
| `compose.InvokableLambda` | `interview/graph.go` | 将 6 个 Agent 包装为 Lambda 节点 |
| `compose.AddLambdaNode` + `compose.AddEdge` | `interview/graph.go` | 连接节点形成 setup 链 |
| `compose.NewGraphBranch` + `AddBranch` | `interview/graph.go` | 面试追问/下一题/完成的条件路由 |
| `compose.StatefulInterrupt` | `interview/graph.go` interviewer Lambda | 每题提问后暂停 Graph |
| `compose.ResumeWithData` | `interview/runner.go` | 用户提交回答后恢复执行 |
| `compose.CheckPointStore` | `capability/store/checkpoint.go` | Redis 实现的 checkpoint 持久化 |
| `compose.Runnable.Invoke` | `interview/runner.go` | 执行/恢复编译后的 Graph |
| `compose.ExtractInterruptInfo` | `interview/runner.go` | 提取中断信息(问题内容) |
| `eino-ext/components/model/openai` | `capability/llm/` | DeepSeek ChatModel 调用 |
| `eino-ext/components/embedding/openai` | `capability/embedding/` | text-embedding-v3 向量化 |
| `eino-ext/components/tool/mcp` | `capability/mcp/` | GitHub/Web MCP 工具调用 |

### 设计原则

- **换大模型** → 只改基础层配置 (eino-ext openai BaseURL + Model)
- **加新 Agent** → 在 Graph 中 AddLambdaNode + 注册到 Branch
- **前端接入** → 只对接 InterviewService 接口
- **Graph 执行** → Eino 编译后自动管理: 节点调度、状态流转、checkpoint

---

## 功能列表

1. **JD智能解析** — 粘贴岗位JD或招聘链接，AI自动提取技术栈、职级要求、核心能力项
2. **简历深度匹配** — 上传PDF/Word简历，AI分析匹配度，找出优势和短板
3. **智能出题规划** — 根据JD+简历+RAG题库检索，自动规划题目类型和难度分布
4. **多轮模拟面试** — AI面试官逐题提问，根据回答实时追问深挖，模拟真实面试节奏
5. **实时评估打分** — 每题即时评分(四维度: 技术准确性/回答深度/沟通表达/项目匹配)，面试结束生成多维度评估报告
6. **个性化复习计划** — 基于薄弱点生成复习路径，MCP推荐GitHub开源学习资源
7. **意图路由** — 自动识别面试/技能练习/闲聊三种意图，分流到对应处理链路
8. **技能练习** — 4个可插拔练习模块: 算法编程、系统设计、行为面试(STAR)、技术快问快答
9. **三种接入方式** — CLI命令行(本地调试) + WebSocket(前端实时交互) + REST API(API集成)
10. **面试中断恢复** — Redis Checkpoint持久化，支持精确恢复到上次题目和对话状态

---

## 各层组件

### Layer 3: 基础能力层

| 组件 | 选型 | 说明 |
|------|------|------|
| LLM | DeepSeek v4 (OpenAI兼容) | `https://api.deepseek.com/v1`，模型 deepseek-chat |
| Embedding | text-embedding-v3 | 1024维向量，同为OpenAI兼容API |
| 向量库 | Milvus | ANN语义检索，eino-ext内置支持 |
| 关键词索引 | Bleve BM25 | 本地文件索引，混合检索关键词部分 |
| MCP工具 | GitHub + Web Search | 推荐学习资源，eino-ext MCP客户端 |
| 缓存/Checkpoint | Redis | go-redis，短期记忆+中断恢复 |
| 持久化 | MySQL | go-sql-driver，长期记忆+历史记录 |

### Layer 2: Agent编排层

#### 意图路由器 (Host-Specialist模式)
所有用户请求先经过意图路由器，智能判断是面试、技能练习还是闲聊，分流到对应处理链路。

#### 面试DAG (6个Agent节点)
```
JDAnalysisNode → ResumeMatchingNode → QuestionPlanningNode
                                            ↓
ReviewPlanningNode ← EvaluationNode ← InterviewerNode
                                            ↕ (Interrupt/Resume)
                                         用户回答
```

| Agent节点 | 职责 | 关键依赖 |
|-----------|------|----------|
| JDAnalysisNode | 解析JD文本，提取结构化要求 | LLM |
| ResumeMatchingNode | 简历与JD匹配分析 | LLM |
| QuestionPlanningNode | 基于JD+简历+RAG规划题目 | LLM + Milvus + Bleve + Embedding |
| InterviewerNode | 提问、追问决策、节奏控制 | LLM + Checkpoint |
| EvaluationNode | 四维评分+反馈 | LLM |
| ReviewPlanningNode | 复习计划+资源推荐 | LLM + MCP GitHub |

#### Skill技能系统 (4个可插拔模块)
- **AlgorithmPractice** — LeetCode风格算法练习，提示+反馈
- **SystemDesignPractice** — 系统设计面试引导(需求→架构→权衡)
- **BehavioralPractice** — STAR行为面试法练习
- **TechKnowledgeQuiz** — 技术知识快速问答(10题/轮)

#### Memory记忆系统
- **短期记忆 (Redis)** — 会话对话窗口(最近N条消息)，支持上下文感知
- **长期记忆 (MySQL)** — 面试历史、评估报告、复习计划持久化

### Layer 1: 用户交互层

| 接入方式 | 技术 | 适用场景 |
|----------|------|----------|
| CLI | Cobra | 本地调试、自动化脚本 |
| REST API | Gin | API集成、第三方对接 |
| WebSocket | gorilla/websocket | 前端实时交互、流式输出 |

统一 `InterviewService` 接口，三种接入方式调用同一后端逻辑。

---

## 核心流程

```
[开始] → 意图路由 → 面试分流
                        ↓
                  JD解析 → 简历匹配 → 出题规划
                                            ↓
                                    ┌──────────────┐
                                    │   面试循环     │
                                    │  ┌──────────┐ │
                                    │  │ 提问     │ │
                                    │  │  ↓      │ │
                                    │  │ 用户回答 │←── Interrupt/Resume
                                    │  │  ↓      │ │    (Checkpoint→Redis)
                                    │  │ 追问?   │ │
                                    │  │  ↓否    │ │
                                    │  │ 评分    │ │
                                    │  │  ↓      │ │
                                    │  │ 下一题  │ │
                                    │  └──────────┘ │
                                    └──────┬─────────┘
                                           ↓
                                    生成评估报告 → 生成复习计划 → [结束]
```

面试循环利用 Eino 的 **Interrupt/Resume** 机制，在每题用户回答后暂停等待输入，Checkpoint 持久化保证中断后可恢复。

---

## 技术栈

| 层次 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.22+ | |
| Agent框架 | **Eino v0.9.x** | `cloudwego/eino` + `eino-ext` |
| Agent模式 | Graph DAG + Interrupt/Resume | 固定DAG编排，每题中断 |
| LLM | DeepSeek v4 (OpenAI兼容) | 通过 eino-ext ChatModel 组件接入 |
| Embedding | text-embedding-v3 | 通过 eino-ext Embedder 组件接入 |
| 向量存储 | **Milvus** | 题库RAG语义检索 |
| 关键词索引 | **Bleve BM25** | 混合检索关键词部分 |
| 文档解析 | Eino PDF Parser + DOCX Parser | 简历解析 |
| MCP | eino-ext MCP Client | GitHub + Web Search |
| HTTP框架 | **Gin** | REST API |
| WebSocket | **gorilla/websocket** | 实时双向通信 |
| CLI | **Cobra** | 命令行工具 |
| Checkpoint | **Redis** | 面试中断恢复 |
| Memory | **Redis** (短期) + **MySQL** (长期) | 双层记忆 |
| 配置 | Viper | YAML配置管理 |

---

## 目录结构

```
InterviewAgent/
├── docker-compose.yaml               # Docker 一键部署 (Redis+MySQL+Milvus+App)
├── Dockerfile                         # 应用容器构建
├── .env.example                       # 环境变量模板
├── .gitignore
├── cmd/
│   ├── server/main.go                 # HTTP+WS服务入口 (依赖注入)
│   └── cli/main.go                    # CLI入口
├── config/
│   ├── config.yaml                    # 主配置文件
│   └── config.go                      # Viper加载+校验
├── docker/
│   └── mysql/init.sql                 # MySQL 初始化建表
├── internal/
│   ├── app/
│   │   └── wire.go                    # 依赖注入容器 (L3→L2→L1 全量组装)
│   ├── interaction/                   # L1: 用户交互层
│   │   ├── gateway.go                 # InterviewService 接口定义
│   │   ├── rest/                      # REST API (Gin)
│   │   │   ├── router.go, handler.go, middleware.go, dto.go
│   │   ├── ws/                        # WebSocket
│   │   │   ├── hub.go, client.go, protocol.go
│   │   └── cli/                       # CLI (Cobra)
│   │       ├── root.go, interview.go, skill.go, format.go
│   ├── orchestration/                 # L2: Agent编排层
│   │   ├── orchestrator.go            # 实现 InterviewService
│   │   ├── router/                    # 意图路由
│   │   │   ├── intent.go, host.go, specialist.go
│   │   ├── interview/                 # 面试DAG编排
│   │   │   ├── graph.go               # Graph构建 (6节点)
│   │   │   ├── runner.go              # invoke/resume/checkpoint
│   │   │   └── nodes/                 # 6个Agent节点
│   │   │       ├── state.go           # InterviewState
│   │   │       ├── helpers.go         # JSON提取等工具
│   │   │       ├── jd_analysis.go     # Agent 1: JD解析
│   │   │       ├── resume_matching.go # Agent 2: 简历匹配
│   │   │       ├── question_planning.go # Agent 3: 出题规划
│   │   │       ├── interviewer.go     # Agent 4: 面试官
│   │   │       ├── evaluation.go      # Agent 5: 评分
│   │   │       ├── review_planning.go # Agent 6: 复习规划
│   │   │       └── prompts/           # Prompt模板
│   │   ├── skill/                     # Skill技能系统
│   │   │   ├── skill.go, registry.go
│   │   │   ├── algorithm.go, system_design.go
│   │   │   ├── behavioral.go, tech_quiz.go
│   │   ├── memory/                    # Memory记忆系统
│   │   │   ├── manager.go, short_term.go, long_term.go
│   │   └── contextmanager/             # 上下文管理
│   │       ├── budget.go, builder.go, compressor.go, hierarchy.go
│   ├── capability/                    # L3: 基础能力层
│   │   ├── llm/                       # LLM (DeepSeek)
│   │   │   ├── chat_model.go, deepseek.go
│   │   ├── embedding/                 # Embedding
│   │   │   ├── embedder.go, embedder_impl.go
│   │   ├── vector/                    # Milvus向量库
│   │   │   ├── retriever.go, milvus.go
│   │   ├── keyword/                   # BM25关键词索引
│   │   │   ├── index.go, bleve.go
│   │   ├── mcp/                       # MCP外部工具
│   │   │   ├── client.go, manager.go, github.go, web_search.go
│   │   └── store/                     # 存储引擎
│   │       ├── redis.go, mysql.go
│   └── model/                         # 共享数据模型
│       ├── jd.go, resume.go, question.go
│       ├── interview.go, score.go, report.go, message.go
├── go.mod / go.sum
└── README.md
```

---

## API设计

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/sessions` | POST | 创建面试会话 |
| `GET /api/sessions/:id` | GET | 获取会话信息 |
| `POST /api/sessions/:id/jd` | POST | 提交JD并解析 |
| `POST /api/sessions/:id/resume` | POST | 上传简历 |
| `GET /api/sessions/:id/plan` | GET | 获取出题计划 |
| `POST /api/sessions/:id/start` | POST | 开始面试 |
| `POST /api/sessions/:id/answer` | POST | 提交当前题回答 |
| `POST /api/sessions/:id/skip` | POST | 跳过当前题 |
| `GET /api/sessions/:id/report` | GET | 获取评估报告 |
| `GET /api/sessions/:id/review-plan` | GET | 获取复习计划 |
| `POST /api/sessions/:id/resume` | POST | 恢复中断的面试 |
| `GET /ws?session_id=xxx` | WebSocket | 实时面试交互 |

---

## 快速开始

### 前置依赖

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| **Docker + Docker Compose** | 一键启动所有基础服务 + 应用 | 推荐 |
| **DeepSeek API Key** | LLM 对话与推理 (deepseek-chat) | 是 |
| **DashScope API Key** | Embedding 向量化 | 推荐 (无则 LLM 直接出题) |
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

# 3. 启动全部服务 (Redis + MySQL + Milvus + 应用)
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
| `app` | 8080 | InterviewAgent REST + WebSocket |
| `redis` | 6379 | Checkpoint + 短期记忆 |
| `mysql` | 3306 | 长期存储 (自动建库建表) |
| `milvus` | 19530 | 向量库 (题库 RAG) |
| `etcd` | — | Milvus 元数据 |
| `minio` | — | Milvus 对象存储 |

### 方式二: 本地开发运行

```bash
# 1. 启动基础服务 (任选其一)
# Docker 启动基础服务(不含 app):
docker compose up -d redis mysql milvus

# 或手动分别启动:
redis-server
mysqld
# Milvus 需要 etcd + minio，建议 docker compose 启动

# 2. 设置环境变量
export DEEPSEEK_API_KEY="sk-your-deepseek-key"
export DASHSCOPE_API_KEY="sk-your-dashscope-key"
export MYSQL_PASSWORD="interview123"

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
║  REST API:  http://0.0.0.0:8080/api      ║
║  WebSocket: ws://0.0.0.0:8080/ws         ║
║  Health:    http://0.0.0.0:8080/health   ║
║  LLM:       deepseek-chat                ║
╚══════════════════════════════════════════╝
[wire] DeepSeek LLM initialized (model=deepseek-chat)
[wire] Redis connected (localhost:6379)
[wire] MySQL connected (localhost:3306/interview_agent)
[wire] Milvus connected (localhost:19530, collection=interview_question_bank)
[wire] Memory system ready
[wire] Interview Graph ready
[wire] Application fully wired
```

# 或直接 go run
go run cmd/server/main.go config/config.yaml
```

### 5. CLI 模式

```bash
go build -o interview-cli cmd/cli/main.go
./interview-cli config/config.yaml

# 创建面试会话
./interview-cli create "5年Go后端，熟悉微服务..."

# 开始面试
./interview-cli start <session-id>

# 提交回答
./interview-cli answer <session-id> "我的回答是..."
```

### 6. 验证服务

```bash
# REST API 健康检查
curl http://localhost:8080/api/sessions -X POST \
  -H "Content-Type: application/json" \
  -d '{"jd_text": "Go后端工程师，3年经验，熟悉K8s和微服务"}'

# WebSocket 连接
wscat -c "ws://localhost:8080/ws?session_id=test-session"
```

---

## 配置参数详解

`config/config.yaml` 完整参数说明:

### 服务器配置 `server`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `0.0.0.0` | 监听地址 |
| `port` | int | `8080` | HTTP/WS 端口 |
| `ws_path` | string | `/ws` | WebSocket 升级路径 |
| `read_timeout` | duration | `30s` | 请求读取超时 |
| `write_timeout` | duration | `60s` | 响应写入超时 |

### LLM 配置 `llm`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `deepseek` | 厂商标识 |
| `base_url` | string | `https://api.deepseek.com/v1` | OpenAI 兼容 API 地址 |
| `api_key_env` | string | `DEEPSEEK_API_KEY` | 从该环境变量读取 API Key |
| `model` | string | `deepseek-chat` | 模型名称 |
| `temperature` | float | `0.7` | 生成温度 (0-2) |
| `max_tokens` | int | `4096` | 单次最大 Token 数 |
| `timeout` | duration | `60s` | 请求超时 |
| `max_retries` | int | `3` | 失败重试次数 |

### Embedding 配置 `embedding`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `qwen` | 厂商标识 (DeepSeek 无 Embedding API，用 DashScope) |
| `base_url` | string | `https://dashscope.aliyuncs.com/compatible-mode/v1` | OpenAI 兼容 Embedding API |
| `api_key_env` | string | `DASHSCOPE_API_KEY` | 从该环境变量读取 API Key |
| `model` | string | `text-embedding-v3` | Embedding 模型 |
| `dimensions` | int | `1024` | 输出向量维度 |
| `timeout` | duration | `30s` | 请求超时 |

### 向量库 `vector_db`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | `milvus` | 向量库类型 |
| `host` | string | `localhost` | Milvus gRPC 地址 |
| `port` | int | `19530` | Milvus gRPC 端口 |
| `username` | string | `""` | 用户名 (空=无认证) |
| `password` | string | `""` | 密码 |
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
| `dial_timeout` | duration | `5s` | 连接超时 |
| `read_timeout` | duration | `3s` | 读取超时 |
| `write_timeout` | duration | `3s` | 写入超时 |

### MySQL `mysql`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `localhost` | MySQL 地址 |
| `port` | int | `3306` | MySQL 端口 |
| `user` | string | `root` | 用户名 |
| `password_env` | string | `MYSQL_PASSWORD` | 从该环境变量读取密码 |
| `database` | string | `interview_agent` | 数据库名 (需手动创建) |
| `charset` | string | `utf8mb4` | 字符集 |
| `max_open_conns` | int | `25` | 最大连接数 |
| `max_idle_conns` | int | `10` | 最大空闲连接数 |
| `conn_max_lifetime` | duration | `5m` | 连接最大存活时间 |

### 面试流程 `interview`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_questions` | int | `10` | 每场面试最多题目数 |
| `max_follow_ups` | int | `3` | 每题最多追问次数 |
| `time_per_question` | int | `300` | 每题建议时间 (秒) |
| `checkpoint_ttl` | duration | `3600s` | Checkpoint 有效期 (超时未恢复则清除) |
| `scoring_dimensions` | array | 4个维度 | 评分维度定义 (名称/权重/满分) |

### 混合检索 `rag`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `vector_weight` | float | `0.7` | Milvus 语义检索权重 |
| `keyword_weight` | float | `0.3` | Bleve BM25 关键词权重 |
| `top_k` | int | `10` | 各检索器候选数 |
| `final_top_k` | int | `5` | 融合后最终题目数 |

### 记忆系统 `memory`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `short_term.max_messages` | int | `30` | 对话窗口大小 |
| `short_term.ttl` | duration | `7200s` | 短期记忆过期时间 |
| `long_term.max_history` | int | `50` | 长期历史记录上限 |

### 上下文管理 `context`

LLM 上下文窗口管理，实现窗口分配、记忆分层、压缩策略和上下文编排四层机制。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_tokens` | int | `32768` | 总 token 预算 |
| `profiles.<name>.system_max` | int | — | 该路径的系统提示词最大 tokens |
| `profiles.<name>.working_memory` | int | — | 工作记忆区 budget |
| `profiles.<name>.rag_max` | int | — | RAG 文档最大 tokens |
| `profiles.<name>.recent_verbatim_turns` | int | `3` | 保留原文的最近对话轮数 |
| `profiles.<name>.history_max_turns` | int | — | 最多保留多少轮历史 |
| `profiles.<name>.compression_threshold_turns` | int | — | 超过该轮数触发压缩 |

支持的 profile: `casual_chat`, `interview_ask`, `interview_eval`, `skill`, `stream_fallback`, `stream_agent`

#### 架构设计

```
┌──────────────────────────────────────────────────────┐
│                   ContextBuilder                      │
│  (所有 LLM 调用的统一入口)                             │
│                                                      │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │ 窗口分配  │  │ 记忆分层  │  │     压缩策略       │  │
│  │ (Budget) │  │(Hierarchy)│  │  (Compressor)     │  │
│  └──────────┘  └──────────┘  └───────────────────┘  │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │              上下文编排 (Orchestration)       │    │
│  │  每个 graph step 独立的 context profile       │    │
│  │  优先级打包: system > state > recent > RAG > old │  │
│  └──────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

#### 窗口分配

总预算 32K tokens，按调用路径差异化分配:

| 调用路径 | System | 工作记忆 | RAG | 预留 |
|----------|:------:|:------:|:---:|:----:|
| 闲聊 (chat) | 1K | 24K | 4K | 3K |
| 面试-提问 | 2K | 16K | 4K | 10K |
| 面试-评估 | 3K | 8K | 4K | 17K |
| 技能练习 | 2K | 20K | 4K | 6K |

优先级打包规则（budget 不足时从低到高丢弃）:
1. System Prompt + Current User Input (绝不丢弃)
2. 当前题目上下文 (Question, ScoringPoints)
3. 最近 3 轮对话 (verbatim)
4. RAG 参考文档
5. 更早的对话历史 (compress 或 drop)

#### 记忆分层

三层模型:

```
Layer 0: Working Memory (LLM 上下文内)
  ├── 当前轮: 问题 + 回答 (verbatim)
  ├── 近期历史: 最近 3 轮 verbatim + 4-8 轮摘要
  ├── 活跃状态: position, level, techStack
  └── RAG 片段: 与当前问题相关的参考文档
  容量: 16K tokens | 生命周期: 每次 LLM 调用时重新组装

Layer 1: Short-Term (Redis)
  ├── 完整近期消息 (最多 30 轮 = 60 条)
  ├── 原始文本, 未压缩
  └── TTL: 24h | 操作: LPush + LTrim to 60

Layer 2: Long-Term (MySQL)
  ├── 全部消息 (完整归档)
  ├── Session 摘要 (面试结束后 LLM 生成)
  ├── 历史面试报告 (Report + ReviewPlan)
  └── 用户画像: 薄弱领域、进步曲线
```

转换规则:
- 短期→工作: 每次 LLM 调用前，ContextBuilder 从 Redis 拉取并按 budget 压缩后打包
- 长期→工作: 新 session 创建时加载用户最近 3 次面试的 weak_areas
- 压缩触发: 工作记忆超过 budget → 旧轮次从 verbatim 降级为摘要

#### 压缩策略

三种策略按场景选择:

| 策略 | 适用场景 | 方法 |
|------|---------|------|
| A: 滑动窗口+渐进摘要 | 面试提问 | 最近3轮原文 + 4-7轮提取关键信息 + 更早合并为摘要 |
| B: 结构化提取 | 面试评估 | Q&A转结构化字段(qs/akp/sc/wd)，3x压缩率 |
| C: LLM 摘要 | 长对话/Session结束 | 异步调LLM将旧轮次总结为200字 |

压缩率: 20轮对话 ~8000 tokens → 压缩后 ~5800 tokens (约30%压缩)

#### 上下文编排

每个 Graph Step 独立的 Context Profile:

| Graph Step | System | History | RAG | State Context |
|------------|--------|---------|-----|---------------|
| JD Analysis | JD prompt | 无 | 无 | JD text |
| Resume Match | Match prompt | 无 | 无 | JD + Resume |
| Question Plan | Plan prompt | 无 | 3 docs | JD + Resume |
| Interview (提问) | Interview prompt | 压缩8轮 | 3 docs | position/level/stack |
| Interview (评估) | Eval prompt | 最后3轮 | 搜索参考 | Q + answer |
| Review Plan | Review prompt | 无 | 无 | Evaluations |
| Casual Chat | Chat prompt | 压缩历史 | 3 docs | 无 |

所有 LLM 调用统一通过 `ContextBuilder.Build()` 组装 prompt，在内部完成 token 计数、压缩和打包。

#### 文件结构

```
internal/orchestration/contextmanager/
├── budget.go       # TokenBudget + EstimateTokens + Profile
├── builder.go      # ContextBuilder (统一 prompt 构建入口)
├── compressor.go   # ConversationCompressor (3种压缩策略)
└── hierarchy.go    # MemoryHierarchy (三层记忆协调)
```

### 技能模块 `skills`

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | — | 技能名称 |
| `sub_intent` | string | — | 意图路由匹配的子意图 |
| `enabled` | bool | `true` | 是否启用 |

---

## 设计决策

1. **三层分层** — 交互层/编排层/能力层，层间接口解耦，可独立替换
2. **Eino Graph DAG 编排** — 使用 `compose.NewGraph` 构建 7 节点 DAG + Branch，非 Supervisor 动态调度
3. **Eino Interrupt/Resume** — `compose.StatefulInterrupt` + `compose.ResumeWithData` 实现人机协同
4. **Eino CheckpointStore** — 实现 2 方法接口 (Get/Set)，Redis 持久化，Eino 自动管理序列化
5. **意图路由** — Host-Specialist 模式，LLM 自动分类意图
6. **混合检索** — Milvus 语义 (0.7) + Bleve BM25 关键词 (0.3)
7. **双层记忆** — Redis 短期对话窗口 + MySQL 长期历史
8. **4个可插拔Skill** — 统一接口，注册即可用
9. **三种接入方式** — CLI/REST/WebSocket，统一 InterviewService 接口
10. **eino-ext 组件** — ChatModel(DeepSeek) + Embedding + MCP 均通过 Eino 官方扩展接入

## skills
/run 能做什么

  1. 启动应用并验证改动是否生效
  刚才我们就是这样用的——构建、启动服务器、调 API 验证报告和 DB 写入是否正常。比只看代码可靠得多。

  2. 端到端流程测试
  不只看单个函数，而是模拟用户操作链路：创建会话 → 上传 JD → 解析简历 → 面试答题 →
  查看报告。能发现代码审查发现不了的问题（如刚才发现的多个进程占用端口、skip 到结束不触发完成）。

  3. 发现运行时问题
  - 依赖连接是否正常（Redis/MySQL/Milvus 连接状态一目了然）
  - 配置是否正确（刚才发现 MCP 工具无法启动但不影响主流程）
  - 竞态和并发问题（多进程抢占端口就是典型）

  4. 验证 DB 持久化
  API 返回 200 不代表数据落库了。刚才我们直接查 MySQL 确认了 interview_results 和 review_plans 的写入情况。

  什么时候用

  ┌───────────────────┬─────────────────────────────────┐
  │       场景        │              示例               │
  ├───────────────────┼─────────────────────────────────┤
  │ 修了一个 bug 后   │ /run 确认修复真的生效了         │
  ├───────────────────┼─────────────────────────────────┤
  │ 加了一个新 API 后 │ /run 调接口验证输入输出         │
  ├───────────────────┼─────────────────────────────────┤
  │ 改动了配置        │ /run 确认服务能启动、依赖能连通 │
  ├───────────────────┼─────────────────────────────────┤
  │ PR 提交前最后检查 │ /run 跑一遍核心流程防止回归     │
  ├───────────────────┼─────────────────────────────────┤
  │ 排查线上问题时    │ /run 本地复现问题链路           │
  └───────────────────┴─────────────────────────────────┘

  局限性

  - 需要本地有运行环境（Docker、Go、依赖服务等）——刚才 Docker 已有 MySQL/Redis/Milvus，所以可以直接跑
  - 不会自动写测试，是手动驱动验证
  - 复杂 UI 交互需要 Playwright 模式，纯 API 项目用 curl 就够了

  简单说：每次改完代码后用 /run 验证一下，能省掉很多"明明改了为什么还报错"的调试时间。