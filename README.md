# 408 考研智能学习助手

408 考研智能学习助手：支持公共与个人知识库、资料解析与向量检索、Neo4j 知识图谱、图谱与文档证据组合的 KAG 问答、题库审核、测评和学习反馈。公共教材与题目由管理员上传维护，个人资料仅所有者可管理；不内置完整官方教材或题库。规则学习策略不是训练模型，KAG 检索路径不是通用符号证明。

## 交接阅读入口

- [Neo4j / KAG / 个人知识库升级与启动](docs/NEO4J_KAG_AND_CONTEXT.md)：当前推荐部署、旧图谱迁移、前提分块和实际能力边界。

- [当前实现功能与限制](docs/CURRENT_FEATURES.md)：以当前代码为准的产品能力、模型费用及验收边界。
- [使用与代码交接指南](docs/USER_AND_CODE_GUIDE.md)：交付清单、新电脑安装、三个终端启动、数据备份恢复、操作流程、排错与逐文件函数索引。
- [中文配置模板](.env.example)：哪些必填、哪些是宿主机配置、哪些被 Compose 覆盖。
- [工作进度](docs/WORK_PROGRESS.md)：当前快照与单独保留的历史证据。
- [公共知识库与图谱改造说明](docs/PUBLIC_KB_ADMIN_ROADMAP.md)：首个管理员初始化、公共资料上传、检索和图谱验收步骤。

## 公共知识库与管理员

管理员使用独立工作台维护公共知识库、上传/替换/删除资料、查看连线图谱并管理多用户；普通用户只读公共学习资源，也可建立和维护自己的个人知识库。上传默认200 MiB，Docker内置PDF文字提取和中文/英文OCR；本地词法向量无需模型Key。升级现有 `.env` 的具体步骤见 [知识库与管理员指南](docs/KNOWLEDGE_ADMIN_GUIDE.md)。

## 启动

1. 启动 Docker Desktop；已有 `.env` 不要覆盖。
2. 在项目 PowerShell 终端运行 `powershell -ExecutionPolicy Bypass -File .\scripts\start-kag.ps1`。脚本生成缺少的本地密钥，保留现有配置，并构建启动全部服务。
3. 打开 http://localhost:5173。管理员可在资料管理点击“升级本库分块与索引”，为旧资料建立带章节和前提的新索引。当前迁移为 1–20。
4. 回答仍需要可用模型；已有 Ollama 配置可以继续使用。内置文字解析/OCR 和本地词法向量无需额外启动 Python 服务，独立 Python 解析/text2vec 是可选方案。

## 使用流程

管理员维护公共目录、资料与题目；普通用户可建立个人库、上传资料，并在问答中明确选择知识库。资料可用于知识库、题库或两者，自动抽取题目先审核。图谱在 Neo4j 中浏览和遍历，原文证据及 pgvector 索引保存在 PostgreSQL；来源构建记录和同步日志用于恢复。KAG 问答执行问题规划、最多两跳图查询、文档检索和前提校验，正文后统一列出来源；未命中图谱时明确标记仅使用文档证据。

详见 [当前升级说明](docs/NEO4J_KAG_AND_CONTEXT.md) 和 [资料分类与题库审核](docs/RESOURCE_CATALOG_AND_REVIEW.md)。切勿用 `docker compose down -v` 清空原有数据卷。资料/OCR、图谱关系和模型组织仍需核验，不宣称已解决所有教材逻辑推理问题。

## 开发与验证

前端 Vue 3/TypeScript/Vite，后端 Go/PostgreSQL/pgvector，Neo4j 保存查询图；独立 worker 使用 PostgreSQL 持久化任务。Redis 在 Compose 中保留，但不是文档任务权威队列。代码目录与函数详见使用指南第 8 节。

基础命令：node scripts/check-independence.mjs；backend 下 go test -count=1 ./...、go vet ./...；frontend 下 npm ci、npm test、npm run build。数据库集成测试需显式设置 TEST_DATABASE_URL，否则可能跳过。Python 离线回归和真实链路测试的区别见使用指南。

## 独立实现与来源

WeKnora 仅作开发设计参考，不是构建、测试、运行或部署依赖。项目不要求交付其源码。开发前读 [AGENTS.md](AGENTS.md)，能力归属见 [independence.md](docs/independence.md)，参考记录见 [weknora-reference.md](docs/weknora-reference.md)。通用开源依赖和模型保留各自许可证；独立性脚本仅检查显式耦合，不证明原创性。

资料分类、教材优先问答、题目解析关联和成员配额的操作流程见 [知识库使用流程](docs/LIBRARY_WORKFLOW.md)。
