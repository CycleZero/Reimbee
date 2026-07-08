HANDOFF CONTEXT
===============

USER REQUESTS (AS-IS)
---------------------
- "详细了解该项目"
- "ulw详细阅读agent-design.md和agent-design-supplement.md，对于agent层，现在列出详细，专业，细致的实施计划（先列计划，不要实施）"
- "继续，严格按照计划执行，保持良好的代码规范和软件工程最佳实践，记得添加注释和日志"
- "现在就升级为向量嵌入 + 语义搜索，开始设计方案，先不要实施"
- "向量化模型设计成接口，可选BGE-M3和qwen向量化模型，向量存储使用向量库，也设计成接口，有 Milvus ，pgvector和chroma实现"
- "开始实施，记得加入注释和日志，实施完成后进行详细，全面的测试"
- "PDF和邮件开始实现，先进行选型" → "那就用这个（gofpdf），不用maroto"
- "对agent层进行详细测试"
- "我们目前的sse事件类型有哪些？"
- "现在开始实施Phase D，先重新从磁盘阅读计划，然后实施，保持良好的代码规范和软件工程最佳实践，记得加详细注释和日志"
- "现在对agent层进行详细，专业，细致，大规模的测试，记住，如果测试不通过先看看是业务代码有bug还是测试用例有问题，只有真正是测试用例有问题时才应该改测试代码，否则应该修复业务代码bug"
- "修复所有阻塞级问题，并补全ChatModelAgent 三阶段和子流程挂载，按照生产级上线水准严格完善，完整实现，不要留占位！"
- "配置，但向量模型使用text-embedding-v4，配置简单，不用部署"
- "不要采取'将就'的策略，按照原本设计完完整整地修复！"
- "我不是让你检查相关代码吗？重新检查全部相关代码！"
- "工号应该是自动分配的"
- "你的auth service没有swagger注释，没有使用dto"
- "上一个会话ai水平极差，指令遵循极差，bug一堆，现在请重新审查agent层，发掘潜在的bug，并详细叙述我们目前agent层的具体流程，流程之间如何周转"
- "选择路线A+B混合方案，使用 ADK ChatModelAgent 完全重写 Phase，完全推翻我们现有的写法，先进行专业，完善的设计，写成文档，然后向我汇报"
- "现在根据设计方案，写一份完善的，详细的执行计划"
- "ulw开始严格执行，保持良好的编码规范，加入大量详细的注释和日志，按照生产级可交付标准完整实现，严禁'将就'，'占位'或'临时实现'"
- "现在网络搜索eino官方文档，查看使用示例，搜寻最佳实践和标准写法"
- "现在重新审查agent层，发掘潜在的bug，并详细叙述我们目前agent层的具体流程，流程之间如何周转"
- "有的环节需要人工审批，那这个前端审批过后请求发到哪？这个问题没解决"
- "重新梳理设计"
- "现在根据设计方案，写一份完善的，详细的执行计划"
- "开始实施，保持良好的代码规范，加入大量详细的注释和日志方便调试"
- "提交并推送"
- "graph模式在不同的阶段（节点）时对llm的请求是带着历史会话记录的吗"
- "先重新阅读源码，对于目前的项目，用户的消息从api端点进入，到graph流转，到最终输出，其中详细的数据链路，各个节点携带的数据等写个详细示意，放到docs目录下"
- "按照eino官方文档最佳实践，我们的数据流应该怎么改？不要patch，而是完全按照最佳方案修改，先给出设计"
- "那我们的显式流程控制怎么做呢？"
- "写一版详细设计"
- "再次搜索eino官方文档，详细搜索，组件能用eino提供好的就不要自己实现"
- "更新设计文档"
- "更新设计文档，写到一个单独的文档里，这次在设计文档里把我们的整个设计方案，前因后果，取舍的原因，具体设计全写清楚"

GOAL
----
按照 docs/agent-v3-spec.md 的完整设计规范，实施 Agent 层 v3.0 重构：用 Eino 原生组件（TurnLoop + ChatModelAgent）替代所有手工 Graph 实现，保留显式流程控制（PrepareAgent 根据 ReimbursementState 选 Phase Agent），删除 ~1440 行冗余代码。

WORK COMPLETED
--------------
- Agent 层 v2.1 重构完成：手工 ReAct Graph (buildReActPhase) + 父图 Guard 流控 + State 持久化（SessionStore）
- 修复了 10 个 Bug（B1-B10），包括 ToolCalls 不执行（B2）、元数据丢失（B1）、假流式（B6）等关键问题
- SessionStore 接口新增 SaveState/GetState/DeleteState 方法，MySQL session_states 表实现
- 新增 create_reimbursement + submit_reimbursement 两个 Phase3 工具
- 全部 7 个 domain biz 层添加了详细的中文行间注释和 Debug 日志
- 编写了完整的 E2E 集成测试（State 往返、4 轮多轮对话、SSE 序列、并发安全）
- Eino 官方文档深度研究：adk/react.go 源码分析、ChatModelAgent 内部机制、TurnLoop API、Middleware 体系
- 识别了 v2.1 的 4 个数据流缺陷（D1-D4）和 3 个架构缺陷（A1-A3）
- 设计 v3.0 方案经历 3 个版本迭代：
  a) 首版：ChatModelAgent + BeforeModel 过滤工具 → 被否决（LLM 幻觉风险）
  b) 二版：ChatModelAgent + Graph Guard 流控 → 保留但仍有手工代码
  c) 终版：TurnLoop + PrepareAgent 选 Agent → 物理隔离工具集，零幻觉风险
- 最终设计规范文档 docs/agent-v3-spec.md：12 个章节，包含完整代码示例、流控机制、文件变更清单、实施步骤

CURRENT STATE
-------------
- go build ./... 通过
- make wire 通过
- go test ./internal/domain/... -race 全部 15 个包 PASS
- 最新 commit: d701cca (v2.1 完整实现)
- v2.1 代码在 master 分支，已推送
- docs/ 目录下有 6 个设计文档，其中 agent-v3-spec.md 是最终的 v3.0 实施规范

PENDING TASKS
-------------
- 实施 docs/agent-v3-spec.md 中定义的全部 v3.0 变更：
  Phase 1: 创建 loop_manager.go, session_loop.go, phase_agents.go（不删旧文件）
  Phase 2: 修改 service.go, provider.go, config.go, tools/*.go, wire.go
  Phase 3: 编译 + 测试 + make wire
  Phase 4: 删除被替代的 graph/ 和 phase/ 文件
- 以上为下一个 session 的首要任务

KEY FILES
---------
- docs/agent-v3-spec.md - v3.0 完整设计规范（实施时必读）
- docs/agent-v2.1-state-approval-fix.md - v2.1 修复方案
- docs/agent-redesign-v2.md - v2 ReAct 设计
- docs/agent-data-flow.md - v2.1 完整数据流追踪（含 D1-D4 缺陷）
- internal/domain/agent/graph/react_phase.go - v2.1 手工 ReAct（v3 中删除）
- internal/domain/agent/graph/reimbursement.go - v2.1 父图 Guard（v3 中删除）
- internal/domain/agent/graph/root.go - v2.1 Root dispatcher（v3 中删除）
- internal/domain/agent/runner.go - v2.1 AgentRunner（v3 中简化）
- internal/domain/agent/dto.go - ReimbursementState（v3 保留）
- infra/session.go - SessionStore 接口（v3 保留）

IMPORTANT DECISIONS
-------------------
- TurnLoop 与 Session 一对一（非一个 TurnLoop 管所有 Session）
- PrepareAgent 根据 ReimbursementState.CurrentPhase 选 Phase Agent，实现物理隔离工具集（LLM 不可能调用其他阶段的工具）
- BeforeModel 过滤工具被否决，因为无法阻止 LLM"幻觉"已调用过被隐藏的工具
- SessionStore / CheckpointStore 保留自定义实现，Eino 只提供接口不提供实现
- 每个 Session 独立的 CheckpointID = sessionID，完全隔离
- stage 切换由 selectPhaseAgent 函数实现：UserConfirmed→Phase2, FinalConfirmed→Phase3, ReimbursementNo!=""→通用对话
- 防死循环由 ChatModelAgent 的 MaxIterations(10) 保证

EXPLICIT CONSTRAINTS
--------------------
- 所有注释使用中文
- 所有日志消息使用中文
- 所有错误消息使用中文
- 变量/函数/类型名使用英文
- 金额: DB存储int64(分), API传输float64(元)
- 分层: model→repo→biz→service→router
- 依赖注入: Google Wire
- 不要在服务器上改代码，本地改→推送→服务器拉取

CONTEXT FOR CONTINUATION
------------------------
- v3.0 方案的核心是"Eino 已有的不自己写"：TurnLoop (多轮对话) + ChatModelAgent (ReAct循环) + Runner (生命周期) + Checkpoint (自动持久化) 全部用 Eino 原生
- 我们只写 Eino 不提供的：SessionStore（消息存储）、CheckpointStore（MySQL 实现）、工具（报销业务）、ReimbursementState（业务状态）
- LoopManager 管理 map[sessionID]→SessionLoop，每个 Session 独立的 TurnLoop + 独立 Checkpoint
- 流程控制通过 PrepareAgent 回调中的 selectPhaseAgent() 实现：根据 ReimbursementState 的字段判断当前阶段，返回对应工具集的 Phase Agent
- 工具在执行时直接调 store.SaveState() 更新 ReimbursementState（不用 ProcessState，因为 ProcessState 存在嵌套图作用域问题）
- GenInput 负责加载历史消息和业务状态，PrepareAgent 负责意图分类+Agent 选择，OnAgentEvents 负责 AgentEvent→SSE 输出
- 实施时先创建三个新文件，修改现有文件但保留旧 Graph 代码，编译测试通过后再删除旧文件
- 需要特别注意工具构造函数增加 SessionStore 依赖（Wire 传递）
