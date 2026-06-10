# 简历项目经历

## AI 模拟面试官（InterviewAgent）—— Go 后端开发

**2025.06 - 至今 | 个人项目 | 独立开发**

### 项目概述

基于 **Go + 字节跳动 Eino 框架** 搭建的全流程 AI 模拟面试系统。上传简历 + 输入岗位 JD → AI 自动完成完整模拟面试 → 多维评分 + 报告 + 复习计划。涵盖多 Agent 协作、Hybrid RAG 多路召回、MCP 协议集成、记忆系统、Skill 技能系统、RAG 评估体系等核心技术栈。配套 Web 前端（SPA）、REST API、WebSocket 实时通信。

**代码量 ~8000 行，Eino 框架使用率 100%。**

### 系统架构

```
用户交互层： Gin REST API / WebSocket / CLI 三通道接入
     ↓
Agent 编排层（核心）：意图路由器 → 6 个 Agent DAG 协作 + 2 个 React Agent
     ├── 面试 DAG（Eino Graph）：JD 分析 → 简历匹配 → 出题规划 → 面试官(interrupt) → 评估 → 复习计划
     ├── Skill 技能系统：4 个有状态多轮交互模块（可插拔）
     ├── Memory 系统：短期 Redis + 长期 MySQL
     └── React Agent：AI 闲聊 Agent + 面试官 Agent（LLM 自主调用 MCP 工具）
     ↓
基础能力层： DeepSeek V4 / Embedding / Milvus + Bleve 双引擎 / MCP 外部工具 / Redis + MySQL
```

### 核心技术实现

**1. Eino 框架全栈迁移与深度集成**
- 将项目自研的 ChatModel / Embedder / RAG / Document Splitter 等 **5 个模块全部迁移** 为 Eino 原生接口（`model.ToolCallingChatModel`、`embedding.Embedder`、`retriever.Retriever`、`document.Transformer`、`tool.InvokableTool`），消除 ~500 行包装代码，删除 `llm` 包、`adapters.go` 等冗余文件
- 通过 Eino `callbacks.HandlerBuilder` 统一实现 LLM / Embedding / Tool 调用的全链路可观测性日志，替代手写 `log.Printf`
- 使用 Eino `compose.Graph` + `StatefulInterrupt` 实现面试 DAG，支持暂停/恢复和多轮追问，配合 `compose.CheckPointStore` 实现断点续传

**2. 多 Agent 协作编排（6 Agent + 2 React Agent）**
- **链式 Agent（DAG）**：JD 分析 Agent → 简历匹配 Agent → 出题规划 Agent → 面试官 Agent → 评估 Agent → 复习计划 Agent，通过 Eino Graph DAG 串联协作，每个 Agent 职责单一、可独立测试
- **自主决策 Agent（React）**：AI 闲聊 Agent + 面试官 Agent 基于 Eino `react.Agent`，LLM 可根据对话上下文自主决定是否调用 GitHub / Web Search 工具获取实时信息
- 三级难度状态机：连续答对自动升级（Easy→Medium→Hard），连续答错降级，模拟真实面试官策略

**3. Hybrid RAG 多路召回 + LLM Rerank**
- 基于 Eino `flow/retriever/router` 构建向量检索（Milvus）+ BM25 关键词检索（Bleve）双路并行，内置 RRF 融合去重（k=60）
- 实现 LLM Rerank 模块，在融合后通过大模型二次精排，提升 Top-K 文档相关性
- 实现 RAG 评估体系：Faithfulness / Relevance / Completeness 三维评估 + TopK 离线调优实验

**4. MCP 协议集成与 Agent 工具调用**
- 基于 `mark3labs/mcp-go` 实现 MCP Stdio 传输层客户端，支持子进程启动 / 初始化 / 优雅关闭
- 封装 GitHub MCP + FreeSearch MCP，通过 `tool.InvokableTool` 适配为 Eino 标准 Tool 接口
- 设计 `EinoBridge` 实现 MCP 工具自动发现与动态注册，支持 API 查询可用工具列表
- 解决 DeepSeek 流式 Tool Call 兼容问题（先文本后工具调用，与默认 `StreamToolCallChecker` 不兼容）

**5. Agent 记忆系统**
- 短期记忆：Redis List + TTL + 滑动窗口，管理当前会话对话上下文
- 长期记忆：MySQL 持久化存储会话、面试报告、复习计划，支持跨会话用户历史查询

**6. Skill 有状态多轮技能系统**
- 自定义 `Skill` 接口实现有状态多轮交互（区别于无状态 Tool 调用），每个技能维护独立会话状态（Round 计数、历史记录、评分数据）
- 4 个可插拔内置技能：算法 Coding 练习（5 轮出题→审阅→进阶）、系统设计面试（6 轮需求→容量→设计→深挖）、STAR 行为面试（4 轮 S→T→A→R 评估）、技术快速测验（10 题逐题递进实时计分）
- 通过 `Registry` 统一注册与路由调度，支持 session 状态管理和自动清理

### 技术栈

| 分类 | 技术 |
|------|------|
| 语言 | Go 1.25 |
| AI 框架 | CloudWeGo Eino v0.9.2（Graph / Callbacks / React Agent / Retriever / Router） |
| LLM | DeepSeek V4（OpenAI 兼容协议） |
| Embedding | Qwen/Qwen3-Embedding-0.6B |
| 向量数据库 | Milvus（IVF_FLAT） |
| 关键词检索 | Bleve BM25 |
| MCP 协议 | mcp-go v0.43（Stdio Transport）+ GitHub MCP + FreeSearch MCP |
| 存储 | Redis + MySQL |
| Web 框架 | Gin + WebSocket（gorilla/websocket） |
| 前端 | Vanilla JS SPA（SSE 流式对话） |
| 容器化 | Docker Compose |
| 配置管理 | Viper + YAML |

### 项目难点与解决方案

- **多 Agent 状态同步**：利用 Eino Graph `StatePreHandler` 在 6 个 Agent 间传递标准化 InterviewState，避免数据耦合
- **DeepSeek 流式 Tool Call 兼容**：DeepSeek 先输出文本后工具调用，与 Eino React Agent 默认 checker 不兼容。转为使用 `agent.Generate()` 确保工具调用可靠性
- **MCP 子进程运维**：处理 npx 子进程的启动/初始化/优雅关闭，解决 npm 镜像配置、Windows 权限、Node 版本兼容等跨平台问题
- **Eino Router 稳定性**：双检索器时 Eino Router 内部并发回调有 nil pointer 风险，通过单检索器直接返回 + 双检索器 panic-safe 包装器两阶段兜底
- **Milvus 类型适配**：实现 Eino `retriever.Retriever` 接口，通过 Options 模式传递参数，在 SDK 边界处理 float64↔float32 转换

### 项目成果

- 完整覆盖面试全流程，开箱即用的 Docker Compose 一键部署
- Eino 框架使用率 100%，核心功能无自定义冗余包装层，模块化清晰
