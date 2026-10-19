# 408 考研智能学习助手

408 考研智能学习助手：支持公共/个人知识库、资料解析与向量检索、带引用 AI 答疑、测评判分、薄弱点分析、每日规则计划和知识图谱查看。内置四科 12 个目录、38 个主题的讲解、例题及原创基础练习，新用户无需上传即可学习和测评。图谱支持共享目录、资料匹配以及可选模型候选的证据校验；内容不是完整官方考纲或完整教材。规则学习策略不是训练模型，RAG 不是自主 Agent。

## 交接阅读入口

- [当前实现功能与限制](docs/CURRENT_FEATURES.md)：以当前代码为准的产品能力、模型费用及验收边界。
- [使用与代码交接指南](docs/USER_AND_CODE_GUIDE.md)：交付清单、新电脑安装、三个终端启动、数据备份恢复、操作流程、排错与逐文件函数索引。
- [中文配置模板](.env.example)：哪些必填、哪些是宿主机配置、哪些被 Compose 覆盖。
- [工作进度](docs/WORK_PROGRESS.md)：当前快照与单独保留的历史证据。
- [公共知识库与图谱改造说明](docs/PUBLIC_KB_ADMIN_ROADMAP.md)：首个管理员初始化、公共资料上传、检索和图谱验收步骤。

## 公共知识库与管理员

管理员使用独立工作台维护公共知识库、上传/替换/删除资料、查看连线图谱并管理多用户；普通用户默认只读公共学习资源。上传默认200 MiB，Docker内置PDF文字提取和中文/英文OCR；本地词法向量无需模型Key。升级现有 `.env` 的具体步骤见 [知识库与管理员指南](docs/KNOWLEDGE_ADMIN_GUIDE.md)。

## 启动

1. 安装 Docker Desktop（Linux 容器），新环境复制 .env.example 为 .env，替换至少 32 字符 JWT_SECRET 并配置模型；不要覆盖已有 .env。
2. 使用本地嵌入时，按 [嵌入服务说明](services/local_embedding/README.md) 启动宿主机 8090；完整 PDF/DOCX 结构解析还需按 [解析说明](services/document_parser/README.md) 启动 8091。Compose 不启动这两个 Python 服务。
3. 在项目根目录运行 docker compose up -d --build。后端及 worker 自动顺序执行未应用迁移；当前迁移为 1–15。已有数据升级先备份数据库与上传卷。
4. 打开 http://localhost:5173，后端健康检查 http://localhost:8080/healthz。

详细可复制 PowerShell 命令见使用指南。直接运行 Go/Python 不会自动读取根目录 .env；不要照抄只运行 go run 而未导入配置的旧步骤。

## 使用流程

系统自动建立公共课程与基础图谱，worker 索引内置讲义。部署者初始化首个管理员后可维护目录、课程和公共资料；普通用户注册后即可学习、测评并检索公共资料，也可选择加入个人知识库。图谱模型抽取需显式设置 GRAPH_EXTRACTION_ENABLED=true 并配置正常模型。之后查看解析和警告 → 带引用答疑 → 检查抽取题目/做测评 → 确认 AI 辅助判分 → 查看薄弱点与生成每日任务。资料不完整时不会自动变为可用题目；没有可用资源时计划可以为空。

默认正常模型模式要求答疑所需 LLM/Embedding 配置齐全。AI_DEMO_MODE=true 只提供演示向量和检索片段预览，不是 AI 回答。当前本地 OCR/text2vec 不消耗云端模型额度；文本答疑、参考解析和生成练习可能收费。

## 开发与验证

前端 Vue 3/TypeScript/Vite，后端 Go/PostgreSQL/pgvector；独立 worker 使用 PostgreSQL 持久化任务。Redis 在 Compose 中保留，但不是文档任务权威队列。代码目录与函数详见使用指南第 8 节。

基础命令：node scripts/check-independence.mjs；backend 下 go test -count=1 ./...、go vet ./...；frontend 下 npm ci、npm test、npm run build。数据库集成测试需显式设置 TEST_DATABASE_URL，否则可能跳过。Python 离线回归和真实链路测试的区别见使用指南。

## 独立实现与来源

WeKnora 仅作开发设计参考，不是构建、测试、运行或部署依赖。项目不要求交付其源码。开发前读 [AGENTS.md](AGENTS.md)，能力归属见 [independence.md](docs/independence.md)，参考记录见 [weknora-reference.md](docs/weknora-reference.md)。通用开源依赖和模型保留各自许可证；独立性脚本仅检查显式耦合，不证明原创性。
