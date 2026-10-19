# WeKnora reference audit

> 交接说明：本文保留开发参考与独立实现留痕，不是安装依赖清单。接手部署不需要参考项目源码。当前组件、调用链和文件职责见 [使用指南](USER_AND_CODE_GUIDE.md)，本文各日期条目仅代表对应开发时点。

2026-09-30 处理进度：复查检索接口定义，沿用模型调用与业务状态分离的组织思路；独立设计文档进度字段、4 块批次计数及页面轮询，不复制参考项目任务实现。

2026-09-30 本地嵌入：再次查看 `internal/types/interfaces/retriever.go`，参考检索接口与模型维度相互独立的组织方式。独立实现 HTTP 嵌入适配、配置指纹与可变维度存储；没有复制其代码、路由或 schema。

2026-09-30：重新查看参考项目的 `internal/types/interfaces/retriever.go`，仅参考将检索契约与调用方分离的思想。本项目自行定义模型状态接口、显式演示开关、安全错误与证据门槛；未复制参考代码或增加运行依赖。

Reference checkout inspected during initial design: `D:\school\WeKnora-main`. The table records design ideas, not source-code reuse or an originality certification. This local checkout is optional for delivery: it is not a build, test, deployment, or runtime dependency. See [the independent implementation policy](independence.md).

As of 2026-09-28, reference use is restricted to architecture, engineering organization, RAG ideas, and knowledge-base workflows. Do not fork, copy whole modules, or reuse WeKnora branding, UI, or logos. Shared use of Vue, Pinia, or TDesign is use of general-purpose libraries, not permission to copy reference pages.

| Capability | Inspected WeKnora source | Design idea independently applied in 408 Assistant | Not adopted and why |
|---|---|---|---|
| Layering / composition | `internal/router/router.go`, `internal/handler/knowledgebase.go`, `internal/application/service/knowledgebase.go`, `internal/application/repository/knowledgebase.go`, `cmd/server/main.go` | Transport → service → repository contracts with composition in `cmd/api` | Large dependency container, tenant/organization composition, API-key capability matrix are beyond a personal MVP |
| Document parser contract | `internal/types/interfaces/document_parser.go`, `internal/application/service/knowledge_process.go`, `internal/application/service/knowledge_post_process.go` | Parser interface and explicit document lifecycle | Remote Python docreader, parser engine discovery, advanced post-processing and subtask orchestration are deferred |
| Chunking | `internal/infrastructure/chunker/splitter.go`, `internal/types/interfaces/chunk.go`, `internal/application/repository/chunk.go` | Ordered chunks, overlap, batch embeddings, transactional document replacement | Parent/summary/image/FAQ chunk variants and revision history are unnecessary in phase one |
| Retrieval abstraction | `internal/types/interfaces/retriever.go`, `internal/application/service/retriever/registry.go` | Provider/store interfaces, owner-scoped retrieval, cosine top-k | Driver registry, OpenSearch, Milvus, Qdrant, Weaviate and multi-process engine hydration were deliberately omitted |
| Access control | `internal/middleware/kb_access.go`, `internal/router/rbac.go`, repository methods carrying `tenantID` | Auth context supplies user ID; each owned lookup filters it; isolation tests | Organizations, shared KBs, role hierarchy, API-key scopes and enterprise RBAC are not phase-one product needs |
| Migrations | `migrations/versioned/000000_init.up.sql`, `migrations/versioned/000002_embeddings.up.sql`, `internal/database/migration_sqlite_test.go` | Numbered up/down SQL, embedded startup application, pgvector schema | Multi-database migration branches and SQLite compatibility are omitted to keep PostgreSQL authoritative |
| Frontend | `frontend/src/router/index.ts`, `frontend/src/stores/auth.ts`, `frontend/src/views/knowledge/KnowledgeBaseList.vue`, `frontend/src/views/chat/index.vue` | Vue Router auth guard, Pinia auth state, lazy page routes, KB/resource/chat separation, TDesign | Organization, Agent, artifact, system-admin, IM and marketplace screens are unrelated to 408 learning |
| Deployment/config | `docker-compose.yml`, `.env.example`, `internal/config/config.go` | Compose services, backend-only secrets, local storage volume, provider configuration | WeKnora's optional vector stores, Neo4j, MinIO, Langfuse, SearXNG, sandbox and MCP services are not needed for the MVP |

The resulting code follows the useful architectural boundaries while making 408 syllabus, mastery, weakness, and recommendation first-class rather than attaching them to a generic knowledge platform.

2026-10-13：实施前复查 internal/types/interfaces/retriever.go 和 document_parser.go。参考解析结果与检索接口解耦的组织思想，独立设计 408 页级文本合同、原文权限、PostgreSQL 任务行锁/重试、会话完成对保存；没有复制参考项目模块、路由、表结构或提示词，也未增加参考目录运行依赖。

2026-10-14：实施前重新阅读 internal/types/interfaces/document_parser.go 与 retriever.go，仅参考解析引擎和业务检索分离的边界。独立设计 HTTP 文件解析、页块/坐标数据、自有题目提取与判分来源合同；不采用其 gRPC schema、引擎发现和项目模块。模型/依赖安装与运行不读取参考目录。

2026-10-15：修复解析前复查上述两个接口，沿用解析与检索解耦的组织思想。跨页警告传播、异常答案页跳过、DOCX 图片关系顺序/原图保留以及合成回归由本项目独立实现；未复制参考模块、路由、schema 或提示词，测试不读取参考目录。

2026-10-15 任务 3–5：实施前再次读取 document_parser.go 与 retriever.go，参考解析引擎边界和检索结果来源解耦思想；自有短事务租约、版本检查点、来源区域、学习规则与生成题接口均独立实现，不采用其模块、schema 或提示词。参考仓库仍不参与构建、测试、部署与运行。
