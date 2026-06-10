## InterviewAgent — 基于 Go + Eino 的 AI 模拟面试系统

2025年06月-至今 &nbsp; 技术栈: Go 1.22 + Eino v0.9.x + DeepSeek API + Milvus + Bleve BM25 + Redis + MySQL + Gin + WebSocket + Cobra + MCP + Docker

**项目概述**: 独立开发的端到端 AI 面试平台，覆盖 JD 解析、简历匹配、混合检索出题、追问式多轮面试、四维评估和复习计划生成全流程，核心解决 LLM 面试流程不可控、长对话 token 膨胀、出题相关性差三个问题。

**Graph DAG 编排与中断恢复**: 针对 LLM 自由主持面试流程不可控的问题，基于 Eino 框架构建 6 节点固定拓扑 DAG，通过 Branch 条件路由显式控制追问/下一题/结束三条分支，追问上限与总题数可配置。利用 Interrupt/Resume 机制每题提问后挂起，将完整状态序列化至 Redis Checkpoint，支持断线或换设备后 1 小时内精确恢复。

**四层上下文管理**: 针对 40+ 轮对话撑爆 32K token 窗口的问题，设计了窗口分配-记忆分层-压缩策略-上下文编排四层体系。实现基于 UTF-8 字节/字符比的语种自适应 token 估算，定义 6 种 Context Profile 按调用场景差异化分配预算，通过 Spend/Reserve 两级预算接口控制优先级。三种压缩策略按场景选用：面试提问用滑动窗口保留近 3 轮原文、中间轮次结构化提取、更早合并为主题摘要，长对话 token 占用降低约 30%；评估节点用结构化提取将 Q&A 压至 500 字符内；Session 结束时异步调用 LLM 生成摘要归档。所有 LLM 调用通过 ContextBuilder.Build() 统一入口组装 prompt，调用方仅声明场景身份无需关注内部压缩逻辑。

**混合 RAG 与意图路由**: 构建 Milvus 语义检索(0.7) + Bleve BM25 关键词检索(0.3)混合管道，通过 RRF 融合取 Top-5 作为 LLM 出题参考，解决出题偏离岗位技术栈的问题。采用 Host-Specialist 模式统一路由面试、8 个可插拔 Skill 和闲聊三种意图，新增 Skill 约 100 行代码。

**双层记忆与三层架构**: Redis List 维护 30 轮短期对话窗口，MySQL 归档全量历史与 LLM 摘要；新面试自动加载近 3 次薄弱领域注入 system prompt，形成追踪闭环。三层解耦架构通过 Eino 标准组件接口依赖基础能力，更换 LLM 厂商仅需修改 YAML 配置两行，MCP 工具故障自动降级不阻塞主流程。
