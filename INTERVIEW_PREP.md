# InterviewAgent 简历项目总结 & 面试问答准备

---

## 一、简历项目描述

### 版本 A：标准版（适合篇幅有限的简历，约 6-8 行）

> **InterviewAgent — AI 模拟面试官系统** | Go 后端开发
>
> 基于 Go + Gin 开发的 AI 面试模拟平台，采用六边形架构将系统解耦为交互层（REST / WebSocket / CLI）、编排层（业务逻辑、意图路由、DAG 流程引擎）和能力层（LLM / Embedding / 向量库 / 全文检索 / MCP / 存储）。核心覆盖面试全流程：LLM 驱动的 JD 解析与简历匹配置信度评估、基于字节开源 Eino 框架构建的 DAG 图执行引擎串联 7 个处理节点（JD 分析 → 简历匹配 → 出题规划 → 面试对话 → 答案评估 → 报告生成 → 复习规划），利用 StatefulInterrupt 机制将流程状态序列化至 Redis 实现任意节点的中断恢复，使多轮面试流程具备断点续传能力。设计难度自适应状态机（Easy / Medium / Hard 三态，基于连续答对/答错计数自动升降），影响出题分布比例并动态调整追问策略。自研三阶段 RAG 混合检索引擎：① 通义千问 Embedding + Milvus IVF_FLAT 向量索引做语义近邻搜索，② Bleve（Go 原生嵌入式引擎，零外部依赖）做 BM25 关键词全文搜索，③ RRF 倒数秩融合算法（k=60）合并两路异构检索结果，再由 LLM 对候选集进行相关性重排序，配合 Markdown 结构分块、递归分隔符分块、固定长度分块三种策略实现文档智能切分。评分体系按技术准确性(40%)、回答深度(25%)、沟通表达(20%)、项目匹配度(15%)四个维度加权计算，支持用户自定义维度与权重。交互侧提供 15 个 REST API 端点、SSE 流式逐 token 对话（Gin Flusher 强制推送 + 前端 Fetch ReadableStream + TextDecoder 增量解析）、WebSocket 面试事件实时同步（单会话支持多客户端）。会话短时记忆存 Redis（List + TTL 自动过期），长时记忆落 MySQL（会话历史、评估报告、复习计划），所有外部依赖均实现优雅降级——Milvus 不可用时回退纯关键词搜索，Bleve 不可用时回退纯向量搜索，MCP 工具不可用时不影响核心面试流程。

### 版本 B：详细版（适合项目经验需要展开描述的简历，按功能模块写）

> **InterviewAgent — AI 模拟面试官系统** | Go + Gin + Eino | 个人项目
>
> 从零构建了一个完整的 AI 面试模拟平台，涵盖 JD 解析、简历匹配、智能出题、多轮自适应面试、自动评分、复习计划生成的全流程。项目核心亮点：
>
> **LLM 编排引擎** — 基于字节跳动开源的 Eino 框架构建 DAG 图执行引擎，将面试流程抽象为有向无环图（SetupGraph + InterviewGraph），节点包含 JD 分析、简历匹配、出题规划、面试对话、答案评估、报告生成、复习规划共 7 个处理节点。利用 Eino 的 StatefulInterrupt 机制实现在需要用户输入时自动挂起并序列化状态到 Redis，用户响应后从检查点恢复继续执行，解决了多轮对话流程中状态管理与中断恢复的难题。
>
> **RAG 混合检索引擎** — 自主设计了三阶段检索架构：① 向量语义搜索（通义千问 Embedding → Milvus IVF_FLAT 索引，ANN 近邻检索）② BM25 关键词搜索（Bleve 嵌入式全文引擎，无外部依赖）③ RRF（Reciprocal Rank Fusion，k=60）融合两路异构分数并进行 LLM 相关性重排序。支持 Markdown 结构分块、递归分隔符分块、固定长度分块三种策略，文档摄入时自动选择最优策略。配合 RAG 质量评估器（忠诚度/相关性/完整性三维度）持续优化检索效果。
>
> **多轮自适应面试** — 实现难度自适应状态机（Easy/Medium/Hard 三态转换，连续答对 2 次升难度、答错 2 次降难度），难度影响出题计划中的题目分布比例。面试官节点支持追问策略（follow-up / next_question / complete 三种决策），评估节点在 4 个维度（技术准确性 40%、回答深度 25%、沟通表达 20%、项目匹配度 15%）上打分，加权计算面试总分。
>
> **多通道交互与流式输出** — REST API 提供 15 个端点覆盖全流程；SSE（Server-Sent Events）实现聊天流式逐 token 输出，前后端完整实现（后端 Gin + Flusher 强制推送，前端 Fetch ReadableStream + TextDecoder 增量解析）；WebSocket 通道实现面试事件实时推送（出题/评估/报告），支持单会话多客户端同步；同时提供 Cobra CLI 工具支持命令行操作。
>
> **架构设计** — 六边形架构（端口与适配器），交互层（REST / WebSocket / CLI）通过接口依赖编排层，能力层（LLM / Embedding / Vector / Keyword / MCP / Memory）以适配器模式接入，各组件可独立替换。手动依赖注入（app/wire.go），无反射无框架魔法。所有外部服务（Milvus、Bleve、MCP）均实现优雅降级——不可用时自动回退，系统核心流程不受影响。

### 版本 C：英文版（外企简历）

> **InterviewAgent — AI-Powered Mock Interview Platform** | Go + Gin + Eino
>
> Built a full-stack AI interview simulation platform from scratch, covering JD parsing, resume matching, adaptive multi-turn interviewing, multi-dimensional auto-scoring, and personalized review plan generation.
>
> **LLM Orchestration**: Designed a DAG-based execution engine using the Eino framework (ByteDance open-source) with 7 processing nodes. Leveraged StatefulInterrupt for checkpoint-based state persistence to Redis, enabling pause-resume across multi-turn conversations.
>
> **Hybrid RAG Engine**: Implemented a three-stage retrieval pipeline — ① semantic search via Qwen3 Embedding + Milvus ANN (IVF_FLAT index), ② BM25 keyword search via Bleve (embedded, zero-dependency), ③ RRF fusion (k=60) with LLM reranking. Supports three chunking strategies (Markdown structure-aware, recursive separator, fixed-length) with automatic selection per file type.
>
> **Adaptive Interview Engine**: Difficulty state machine (Easy/Medium/Hard) with configurable thresholds drives dynamic question distribution. Interviewer node supports follow-up strategies; evaluation node scores across 4 weighted dimensions.
>
> **Multi-Channel Interaction**: 15 REST endpoints + SSE streaming (token-by-token, end-to-end from Gin Flusher to Fetch ReadableStream) + WebSocket real-time events + Cobra CLI. Hexagonal architecture with manual DI, all external services gracefully degrade on failure.

---

## 二、核心技术栈

| 层级 | 技术 |
|------|------|
| **语言** | Go 1.25 |
| **HTTP 框架** | Gin v1.12（路由、中间件、SSE） |
| **实时通信** | Gorilla WebSocket + SSE（Server-Sent Events） |
| **LLM 编排** | CloudWeGo eino（DAG 图执行引擎，类似 LangChain 的 Go 实现） |
| **LLM 接入** | DeepSeek API（OpenAI 兼容协议），通义千问 embedding |
| **向量数据库** | Milvus（IVF_FLAT 索引，ANN 语义搜索） |
| **全文检索** | Bleve（Go 原生 BM25 引擎，零外部依赖） |
| **RAG 融合** | RRF（Reciprocal Rank Fusion）+ LLM Rerank |
| **关系型存储** | MySQL 8.0（长时记忆：会话历史、报告、复习计划） |
| **缓存/会话** | Redis 7（短时记忆 + 检查点持久化，AOF 模式） |
| **配置管理** | Viper（YAML + 环境变量） |
| **CLI** | Cobra |
| **前端** | Vanilla JS SPA（无框架，原生 Fetch + SSE + WebSocket） |
| **容器化** | Docker Compose（6 服务：App / MySQL / Redis / Milvus / etcd / MinIO） |
| **设计模式** | 六边形架构、依赖注入、策略模式、发布-订阅、状态机、门面模式、适配器模式 |

---

## 三、核心功能清单

1. **JD 解析**：LLM 将职位描述提取为结构化字段（岗位、级别、技术栈、核心技能）
2. **简历解析与匹配**：支持 PDF/TXT，LLM 对比简历与 JD 输出差距分析
3. **智能出题计划**：结合 JD 要求、简历差距、RAG 知识库生成 5-10 题，覆盖不同难度和考察维度
4. **多轮自适应面试**：基于追问策略、难度自适应（状态机：简单/中等/困难动态调整）
5. **多维度自动评分**：技术准确性(40%)、回答深度(25%)、沟通表达(20%)、项目匹配(15%)，每项 1-10 分
6. **面试报告与复习计划**：综合评估 + 个性化学习路径（含通过 MCP 获取的外部学习资源）
7. **技能专项练习**：算法/系统设计/行为面试/技术问答四种模式
8. **知识库管理**：文档上传 → 分块 → 嵌入 → 双索引（向量+关键词）→ RAG 增强问答
9. **意图路由**：LLM 自动分类用户意图（面试/技能练习/闲聊），分发至对应处理器
10. **会话持久化**：Redis 检查点 + MySQL 长时记忆，支持中断恢复
11. **优雅降级**：Milvus/Bleve/MCP 等外部服务不可用时自动降级运行

---

## 四、高频面试问题与回答

### 4.1 架构设计类

#### Q1: 项目的整体架构是怎样的？为什么选择六边形架构？

**回答**：

项目分为三层：
- **交互层**（REST Handler / WebSocket Hub / CLI）：负责接收外部请求，转换为内部调用
- **编排层**（Orchestrator / IntentRouter / InterviewRunner）：核心业务逻辑，不依赖任何传输协议
- **能力层**（LLM / Embedding / Vector / Keyword / MCP / Memory）：与外部系统交互的适配器

核心接口定义在 `internal/interaction/gateway.go`，编排层实现该接口，交互层依赖接口而非实现。

这样设计的好处：
- 同一套业务逻辑同时服务 REST API、WebSocket、CLI 三种客户端，无需重复代码
- 能力层各组件可独立替换（如从 Milvus 切换到 Qdrant 只需新建适配器）
- 测试时可以 mock 接口层，不依赖真实外部服务

依赖注入在 `internal/app/wire.go` 中手动完成：先创建能力层 → 传入编排层 → 传入交互层 → 返回组装好的 App 结构体。

#### Q2: 为什么使用 Eino 而不是直接调用 OpenAI SDK？

**回答**：

Eino 是字节跳动开源的 Go 语言 LLM 应用框架，提供了三个核心能力：

1. **DAG 图执行引擎**：面试流程（JD解析→简历匹配→出题→面试→评估→报告）是一个有向无环图，Eino 的 `compose.NewGraph` 可以声明式定义节点和边，自动处理依赖顺序、条件分支、状态流转
2. **检查点/中断机制**：面试是多轮对话，每轮需要等待用户输入。Eino 的 `StatefulInterrupt` 允许在图执行的任意节点暂停，将状态序列化到 Redis，用户响应后从检查点恢复继续执行
3. **组件抽象**：`ChatModel`、`Embedder` 等接口统一了不同提供商的差异，切换模型只需更换实现

直接使用 OpenAI SDK 需要自己实现状态管理、流程编排、中断恢复，代码量会大很多。

---

### 4.2 Go 语言与并发

#### Q3: 事件总线（eventBus）是如何实现的？为什么不用 channel 直接广播？

**回答**：

实现在 `internal/orchestration/orchestrator.go`，核心设计：

```go
type eventBus struct {
    mu          sync.RWMutex
    subscribers map[string]map[chan *InterviewEvent]struct{}
}
```

- 每个 session 维护一个 `map[chan]*InterviewEvent` 集合（支持多个 WebSocket 客户端同时订阅同一会话）
- `subscribe(sessionID)` 创建容量 64 的缓冲 channel，注册到对应 session 的集合中
- `publish(sessionID, event)` 使用 **非阻塞发送**（select + default），防止慢消费者阻塞发布者
- `unsubscribe(sessionID, ch)` 从集合中移除并 close channel

关键设计考量：
- **sync.RWMutex**：subscribe/unsubscribe 获取写锁，publish 获取读锁（读多写少）
- **非阻塞发送**：如果消费者 channel 满了，直接丢弃事件（面试场景可以容忍少量事件丢失，但不能阻塞面试流程）
- **context 取消自动清理**：goroutine 监听 ctx.Done()，客户端断开时自动清理订阅

#### Q4: SSE 流式输出是如何实现的？Flusher 的作用是什么？

**回答**：

SSE 实现在 `internal/interaction/rest/handler.go:233-265`：

1. 设置响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`（禁用 nginx 缓冲）
2. 调用 `c.Writer.(http.Flusher)` 获取 flusher 接口
3. 从 orchestrator 拿到 `<-chan *ChatChunk`，循环读取每个 chunk
4. 调用 `c.SSEvent("chunk", chunk.Content)` 写入 SSE 格式数据
5. **立即调用 `flusher.Flush()`**——这是关键。HTTP 响应通常有缓冲，不 flush 的话数据会积压在缓冲区，客户端看不到逐 token 输出。Flush 强制将缓冲数据推送到客户端

Gin 的 `c.SSEvent` 底层使用 `gin-contrib/sse` 库，输出的 SSE 格式为：
```
event:chunk
data:响应内容

```

前端使用 Fetch API 的 `ReadableStream` + `TextDecoder` 以流式模式逐行解析，通过 `{stream: true}` 选项处理跨 chunk 的 UTF-8 多字节字符边界。

#### Q5: 项目中哪些地方使用了 goroutine？有没有考虑 goroutine 泄漏？

**回答**：

项目中 goroutine 的使用场景：

1. **ChatStream goroutine** (`deepseek.go:98`)：从 StreamReader 读取 token，发送到 channel。在 `io.EOF` 或 error 时 return 并 close channel → 不会泄漏
2. **eventBus 订阅 goroutine** (`orchestrator.go`)：监听 ctx.Done()，触发时取消订阅并清理 → 不会泄漏
3. **WebSocket readPump/writePump** (`ws/client.go`)：每个连接两个 goroutine，连接关闭时 return → 不会泄漏
4. **forwardEvents goroutine** (`ws/hub.go:119`)：将订约事件转发到 WebSocket client，监听 ctx.Done 退出 → 不会泄漏
5. **MCP 连接 goroutine**：每个 MCP server 一个连接 goroutine

防泄漏措施：
- 所有 goroutine 都有退出条件（context 取消、channel 关闭、io.EOF）
- channel 都有明确的 close 时机（defer close 或 unsubscribe 时关闭）
- 无界 goroutine 创建都绑定到连接/会话生命周期

---

### 4.3 LLM/AI 相关

#### Q6: LLM 的 system prompt 是如何设计的？如何保证输出格式稳定？

**回答**：

以面试官节点为例（`internal/orchestration/interview/nodes/interviewer.go`），system prompt 包含：

1. **角色定义**：明确 LLM 的身份（专业面试官）、面试风格（友好但严格）
2. **上下文注入**：JD 分析结果、简历匹配结果、当前问题、难度等级
3. **行为约束**：追问策略、不可透露答案、保持对话自然
4. **输出格式**：要求以特定格式输出，如 `ACTION: follow_up|next_question|complete` + `CONTENT: ...`

保证输出稳定性的手段：
- **JSON Mode**：需要结构化输出时，在 prompt 中明确要求 JSON 格式，并给出字段说明
- **兜底解析**：`nodes/helpers.go` 中的 `extractJSON` 函数按优先级尝试多种解析策略：````json 代码块 → 任意代码块 → 大括号边界匹配
- **意图路由失败兜底**：当 LLM 输出的 JSON 解析失败时，默认路由到 casual_chat，不会返回错误给用户
- **有限状态约束**：面试流程中通过 state.Phase 限制当前可执行的操作

#### Q7: DeepSeek 和通义千问 embedding 的 API 是 OpenAI 兼容的，为什么还要用 eino 封装？

**回答**：

虽然 API 协议兼容，但 eino 提供了额外价值：

1. **统一消息格式**：eino 的 `schema.Message` 统一了多模态内容（文本/图片/音频），不论后端提供商
2. **回调机制**：eino 支持在模型调用前后注入回调，方便添加日志、监控、限流
3. **流式处理统一**：不同提供商的 streaming 响应细节不同（如 DeepSeek 有 `reasoning_content` 字段），eino 的 `StreamReader` + `streamMessageBuilder` 统一处理这些差异
4. **工具调用抽象**：eino 的 `ToolCallingChatModel` 接口统一了不同提供商的 function calling 实现

实际项目中，DeepSeek 的 streaming 响应可能包含 `reasoning_content`（思维链），eino 的 `streamMessageBuilder` 会将其提取到 `msg.ReasoningContent` 字段，不影响 `msg.Content` 的正常流转。

---

### 4.4 数据库与存储

#### Q8: 为什么短时记忆用 Redis，长时记忆用 MySQL？各自的数据结构是什么？

**回答**：

**短时记忆（Redis）**：
- 数据结构：`LIST`，key 为 `session:{id}:messages`
- 存储最近 N 条对话消息（配置为 30 条），每条是 JSON 序列化的 `{role, content, timestamp}`
- TTL 7200 秒（2 小时），过期自动清理
- 使用 Redis 的原因：面试对话需要极低延迟读写（每轮回答后立即追加），Redis 内存操作微秒级响应

**长时记忆（MySQL）**：
- 表结构：`sessions`、`messages`、`reports`、`review_plans`
- 存储完整的会话历史、评估报告、复习计划
- 使用 MySQL 的原因：数据需要持久化、支持复杂查询（按用户查历史会话）、数据量大（Redis 内存有限）

**写入策略**：
- 每条消息同时写 Redis（追加到 List）和 MySQL（异步批量插入）
- 读取时优先从 Redis 读取最近的消息，冷数据回退 MySQL
- 检查点数据（interview state）写入 Redis，TTL 1 小时，支持中断恢复

#### Q9: Milvus 的索引类型为什么选择 IVF_FLAT？它的工作原理是什么？

**回答**：

IVF_FLAT（Inverted File with Flat compression）：
- **IVF** 部分：使用 k-means 聚类将向量空间划分为 N 个簇（cell），搜索时只检索与查询向量最近的 `nprobe` 个簇
- **FLAT** 部分：在选中的簇内进行暴力精确搜索，不做向量压缩

选择 IVF_FLAT 的原因：
1. 知识库文档量级在万级以内，不需要 HNSW 等复杂图索引
2. IVF_FLAT 在簇内做精确搜索，召回率高于量化压缩方法（如 IVF_SQ8、IVF_PQ）
3. 配置简单，参数少（只需调整 `nlist` 和 `nprobe`）

性能估算：假设 10,000 个向量，`nlist=100`，`nprobe=5`，每次搜索只需计算约 500 次距离（5% 的数据），比暴力搜索快 20 倍，同时保持高召回率。

---

### 4.5 RAG 系统

#### Q10: 混合检索（Hybrid Search）是如何实现的？RRF 融合的公式是什么？

**回答**：

混合检索流程在 `internal/orchestration/rag/fusion.go` 中实现：

**步骤**：
1. **向量搜索**：用户问题 → embedding → Milvus ANN 搜索 → Top-10 语义相似文档
2. **关键词搜索**：用户问题 → Bleve BM25 全文搜索 → Top-10 关键词匹配文档
3. **RRF 融合**：将两路结果合并排序
4. **LLM 重排序**（可选）：使用 LLM 对 Top-K 候选做最终排序

**RRF 公式**：
```
score(d) = Σ 1 / (k + rank_i(d))
```
其中 k=60（平滑常数），`rank_i(d)` 是文档 d 在第 i 个搜索结果中的排名（从 1 开始）。

举例：某文档在向量搜索结果排第 2，在关键词搜索结果排第 5：
```
score = 1/(60+2) + 1/(60+5) = 1/62 + 1/65 ≈ 0.0161 + 0.0154 = 0.0315
```

为什么用 RRF 而不是线性加权：
- 向量相似度和 BM25 分数的量纲不同，无法直接加权求和
- RRF 只依赖排名，与分数量纲无关，天然适合异构检索结果融合
- 超参数少（只有 k），无需调优各检索源的权重

#### Q11: 文档是如何分块的？为什么需要多种分块策略？

**回答**：

分块实现在 `internal/capability/chunk/splitter.go`，提供三种策略：

| 策略 | 算法 | 适用文件 |
|------|------|---------|
| **Markdown** | 按 h1-h3 标题结构切分，保留层级路径 | `.md`, `.markdown` |
| **递归** | 按分隔符优先级递归切分：`\n\n` → `\n` → `。` → `. ` → `，` → `, ` → ` ` | 通用文本 |
| **固定长度** | 固定 chunk_size（默认 1000 字符）+ overlap（默认 200） | `.txt`, `.log` |

**为什么需要多种策略**：
- Markdown 文档有天然的标题结构，按标题切分可以保持每个 chunk 的语义完整性（一个标题下的内容是一个独立的知识点）
- 普通文本没有结构标记，递归策略优先在自然断点（段落、句子）处切分，避免把一句话切成两半
- 日志等无结构文本使用固定长度切分更高效

关键实现细节：递归切分时，如果某个段落仍然超过 chunk_size，会用下一个级别的分隔符继续切分，直到所有分隔符用完后进行硬切分（按字符数截断）。chunk_overlap 保证相邻 chunk 有重叠内容，避免关键信息落在边界被割裂。

#### Q12: 如何处理 RAG 检索质量不高的情况？

**回答**：

项目中有多层质量保障机制：

1. **混合检索**：向量搜索（语义匹配）+ 关键词搜索（精确匹配），互补各自盲区
2. **RRF 融合**：将两路结果按排名融合，避免单一检索源的偏差
3. **LLM 重排序**：对于候选文档，使用 LLM 做最终相关性判断，过滤掉看起来相关但实际无关的文档
4. **优雅降级**：Milvus 不可用时，回退为仅关键词搜索；embedding 不可用时同样回退
5. **RAG 评估器**（`rag/evaluation.go`）：从忠诚度（faithfulness）、相关性（relevance）、完整性（completeness）三个维度自动评估 RAG 质量，支持 Top-K 实验寻找最优参数
6. **System prompt 注入**：RAG 文档以"Reference Knowledge"形式注入 prompt，并提示 LLM"如果参考文档不相关，请忽略"

---

### 4.6 系统设计

#### Q13: 面试流程中的难度自适应是如何实现的？

**回答**：

实现在 `internal/orchestration/interview/difficulty/difficulty.go`，是一个简单的**状态机**：

```
                 连续答对>=2题
     Easy ────────────────────────→ Medium
       ↑                              │
       │         连续答错>=2题          │ 连续答对>=2题
       │                              ↓
     Medium ←─────────────────────── Hard
                连续答错>=2题
```

状态转换规则：
- 连续答对/答错计数在每次评估后更新
- 阈值可配置（默认 2 题）
- 难度影响出题计划中的题目分布：
  - Easy 阶段：60% 简单题、30% 中等题、10% 困难题
  - Medium 阶段：30% 简单题、50% 中等题、20% 困难题
  - Hard 阶段：10% 简单题、30% 中等题、60% 困难题

设计考虑：这里没用更复杂的算法（如 ELO），因为面试场景中题目数量有限（5-10 题），简单状态机已经足够有效且行为可解释。

#### Q14: 系统如何支持水平扩展？有哪些瓶颈？

**回答**：

当前架构的扩展性分析：

**可水平扩展的部分**：
- **无状态 HTTP 服务**：Gin 应用本身无状态，可以通过增加实例 + 负载均衡横向扩展
- **Milvus**：支持分布式部署（proxy + query node + data node），可独立扩展

**瓶颈与解决方案**：
- **WebSocket 连接**：有状态（每个连接绑定到特定实例）。解决方案：使用 Redis Pub/Sub 作为跨实例消息总线，或使用一致性哈希路由 WebSocket 连接到固定实例
- **Redis**：单机部署。解决方案：Redis Cluster 或 Sentinel 高可用
- **MySQL**：单机部署。解决方案：读写分离（主从复制），或分库分表（按 user_id 哈希）
- **LLM API 调用**：受 API 速率限制。解决方案：本地请求队列 + 令牌桶限流 + 重试机制

当前定位是个人项目/小团队使用，单机部署足以支撑数百并发用户。若需要大规模部署，核心改造点在于 WebSocket 层的跨实例消息路由。

#### Q15: 为什么选择 Bleve 而不是 Elasticsearch？

**回答**：

1. **零依赖**：Bleve 是纯 Go 实现的全文检索引擎，编译为单一二进制文件，不需要额外部署 Java 运行时或外部服务
2. **嵌入式**：Bleve 以库的形式嵌入应用进程，通过文件系统持久化索引，部署运维成本极低
3. **足够用**：知识库文档量级在万级以内，Bleve 的 BM25 性能完全够用，不需要 ES 的分布式能力
4. **降低复杂度**：项目的 Docker Compose 已经包含 6 个服务（App、MySQL、Redis、Milvus、etcd、MinIO），再加一个 ES 会显著增加资源消耗和运维复杂度

如果未来文档量增长到百万级，可以考虑切换到 ES 或用 Bleve 的分布式方案。切换成本低——因为有适配器层（`keyword.Index` 接口），只需实现新的 adapter。

---

### 4.7 项目难点与思考

#### Q16: 这个项目中最有挑战的技术点是什么？

**回答**：

**最有挑战的是面试流程的状态管理**：

面试是一个多轮、多阶段的对话流程，涉及：
- JD 解析 → 简历匹配 → 出题计划 → 多轮问答（含追问）→ 评估 → 报告 → 复习计划
- 每个阶段有前置依赖，不能跳过
- 用户可以在任意时刻断开连接，之后恢复继续
- 同一道题可能有多次追问（follow-up），需要判断何时进入下一题

解决方案是用 **Eino DAG 图引擎 + Redis 检查点**：
1. 将面试流程定义为有向无环图（SetupGraph + InterviewGraph），Eino 自动管理节点执行顺序和状态流转
2. 在需要用户输入的节点（如 `InterviewerNode.AskQuestion`）设置 `StatefulInterrupt`，Eino 自动将当前状态序列化到 Redis
3. 用户提交回答后，从 Redis 恢复状态，DAG 从中断点继续执行
4. 如果用户直接关闭页面，下次打开时可以调用 restore 端点恢复

这个方案的关键权衡：DAG 图比手写状态机更结构化、更易扩展（加一个新阶段只需加一个节点），但引入了 Eino 框架的学习成本和抽象开销。

#### Q17: 如果让你重新设计这个项目，你会做哪些改进？

**回答**：

1. **前端框架**：当前使用 Vanilla JS，代码组织靠 IIFE + 全局变量。团队协作或功能增多后会难以维护。会考虑用 React/Vue + TypeScript 重写
2. **API 网关**：当前 Gin 直接暴露给前端，缺少统一的认证、限流、日志中间件。会加一层 API Gateway（如 kong / traefik）
3. **可观测性**：当前只有基本的日志输出。会加入 OpenTelemetry 分布式追踪（面试流程跨多个 LLM 调用，排查问题需要 trace）+ Prometheus 指标（LLM 调用延迟、RAG 检索耗时、错误率）
4. **异步任务队列**：评估和报告生成可以异步处理（不需要阻塞用户等待）。会引入消息队列（如 Redis Streams 或 RabbitMQ），评估完成后通过 WebSocket 推送通知
5. **A/B 测试框架**：不同 prompt 策略、不同 RAG 参数的效果对比，需要一个实验框架来量化评估
6. **流式输出体验**：面试场景下，目前是非流式输出（等 LLM 完全生成后才展示）。改为流式输出可以大幅改善体验——面试者不用等待 3-5 秒看面试官"正在输入..."

#### Q18: 如何保证 LLM 输出的安全性（不输出不当内容）？

**回答**：

当前项目主要依赖以下手段：

1. **System Prompt 约束**：在 system prompt 中明确角色边界（"你是专业面试官"），限定回答范围
2. **输出校验**：结构化输出（如评分 JSON）通过 schema 验证，格式不对会触发重试或降级
3. **意图路由隔离**：LLM 先分类用户意图，不同意图走不同处理流程，避免面试场景下回答无关问题

可以进一步增强的：
- 内容过滤层：在 LLM 输出后增加一道检测，过滤敏感词
- 用户输入预处理：对明显恶意输入直接拒绝，不发送给 LLM
- 速率限制：防止滥用 API

---

### 4.8 简历/面试技巧类

#### Q19: 简历中这个项目应该放在什么位置？如何突出亮点？

**回答**：

- **位置**：放在"项目经验"板块的第一或第二个，如果是应届生/Golang 方向，建议放第一位
- **突出数字**：如果能加上量化数据更好，如"支持 4 种面试题型、15 个 REST API 端点、3 种分块策略、混合检索召回率提升 X%"
- **关键词匹配**：确保 JD 中的关键词（Go、微服务、LLM、RAG、MySQL、Redis、Docker）出现在项目描述中
- **区分度**：大多数简历的项目是 CRUD，你的项目涉及 LLM 编排、向量检索、SSE 流式、混合搜索，技术深度明显更高

#### Q20: 面试时如果被问到"你为什么做这个项目"该怎么回答？

**回答建议**：

"我做这个项目的出发点是想深入理解 LLM 应用开发的全链路——不只是调 API，而是从 prompt 工程、RAG 检索、流式输出、状态管理到系统架构设计都亲自动手实践。同时面试是 LLM 的一个很好的应用场景，因为它天然是多轮对话 + 领域知识 + 结构化评估的结合。通过这个项目，我对 Go 语言的并发模型、六边形架构的实践、向量数据库的选型和使用都有了比较深入的理解。"

---

## 五、代码重点文件速查

| 想看什么 | 文件路径 |
|---------|---------|
| 整体架构组装 | `internal/app/wire.go` |
| 服务接口定义 | `internal/interaction/gateway.go` |
| 业务逻辑核心 | `internal/orchestration/orchestrator.go` |
| 面试 DAG 图定义 | `internal/orchestration/interview/graph.go` |
| 面试官节点实现 | `internal/orchestration/interview/nodes/interviewer.go` |
| LLM 适配器 | `internal/capability/llm/deepseek.go` |
| 混合检索 + RRF | `internal/orchestration/rag/fusion.go` |
| 文档分块策略 | `internal/capability/chunk/splitter.go` |
| 文档摄取流程 | `internal/orchestration/ingestion/ingestion.go` |
| SSE 流式实现 | `internal/interaction/rest/handler.go:233-265` |
| WebSocket Hub | `internal/interaction/ws/hub.go` |
| 记忆系统 | `internal/orchestration/memory/manager.go` |
| 难度状态机 | `internal/orchestration/interview/difficulty/difficulty.go` |
| 前端 SSE 解析 | `web/js/app.js:789-821` |
