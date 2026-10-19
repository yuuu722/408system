> 公共只读权限、独立管理员工作台、大文件上传、Docker内置PDF/OCR和无需Key的本地词法索引已更新，请先按 [迁移15升级步骤](KNOWLEDGE_ADMIN_GUIDE.md) 设置已有 `.env` 并重建镜像。下文历史三终端/模型方案可选，不是当前基础上传和图谱的前置条件。

# 使用与代码交接指南

更新：2026-10-15。适合接手部署、日常使用和继续开发的同学。功能范围见 [当前功能](CURRENT_FEATURES.md)，配置逐项说明见 [环境变量示例](../.env.example)。本文命令为交接操作说明，本次文档整理没有执行安装、恢复或付费模型测试。

## 1. 阅读顺序与交付内容

先看本文和 CURRENT_FEATURES.md，再按模块读 architecture.md、data-model.md、rag.md、recommendation.md。开发前读 AGENTS.md 和 independence.md。WORK_PROGRESS.md 保存历史证据；明确标注历史的段落不代表当前运行状态。

交付整个项目目录，包括 backend、frontend、services、scripts、docs、compose.yaml、根目录 .dockerignore/.env.example/.gitignore、README、AGENTS 和 Makefile；继续开发时保留 .git。不要遗漏 scripts，前端 Docker 构建需要其中的许可证生成脚本。

可排除 node_modules、dist、.npm-cache、api.exe、Python 虚拟环境、构建缓存和临时日志。真实 .env 单独安全交付或由接手人填写自己的凭据。模型和数据另交付：

- text2vec 完整模型目录，含分词器、配置、权重和许可证。可放在新电脑 `D:\models\text2vec-base-chinese`；无需复制其所在的其他业务项目。
- `.cache/paddle` 模型缓存建议一起交付，避免解析模型重新下载。
- 如需保留账号、资料和学习记录，必须配套导出 PostgreSQL 数据库与 uploads 数据卷；这些不在代码包中。当前没有为本次交接新制作完整数据备份。
- 不需要 WeKnora 或其他项目源码才能构建、运行；模型的通用开源许可证仍需保留。

## 2. 新电脑环境与安装

推荐先沿用 Windows + Docker Desktop（Linux 容器）、Python 3.11、uv。Docker 构建前后端不要求宿主机安装 Go/Node；宿主机开发才需要 Go 1.25+、Node 24/npm。当前 Compose 使用 `env_file.required`，需要支持该语法的较新 Compose。

以下假设项目放在 `D:\school\408-assistant`，换目录时同步修改命令。先启动 Docker Desktop，确认 `docker info`、`docker compose version` 正常。

```powershell
Set-Location 'D:\school\408-assistant'
if (!(Test-Path .env)) { Copy-Item .env.example .env }
notepad .env
```

必须替换 JWT_SECRET（至少 32 字符），配置实际 LLM 地址/模型/Key。复现本地嵌入时，将 EMBEDDING_* 切换为 8090、text2vec-base-chinese-windowed-v1、768 维，设置本地令牌；配置解析地址 8091 和解析令牌。视觉配置可留空。不要重复定义同名变量。

### 2.1 解析 Python 环境

```powershell
Set-Location 'D:\school\408-assistant'
uv venv .cache/parser-venv --python 3.11
uv pip install --python .cache/parser-venv/Scripts/python.exe -r services/document_parser/requirements-windows.lock.txt
```

这是既有 Windows 依赖快照；新电脑安装和真实 OCR 需重新验证。缓存放回 `.cache/paddle`，服务启动时使用下文指定的缓存目录。

### 2.2 嵌入 Python 环境

当前服务需要 torch、sentence-transformers、numpy。旧机器复用了已有 Conda 环境，不能依赖那个绝对路径或直接复制虚拟环境。2026-10-15 只读查询到的关键版本如下，可作为新环境安装起点；并非全量锁文件，也尚未在全新电脑验证。

```powershell
Set-Location 'D:\school\408-assistant'
uv venv .cache/embedding-venv --python 3.11
uv pip install --python .cache/embedding-venv/Scripts/python.exe torch==2.14.0 sentence-transformers==6.0.1 numpy==2.4.6 transformers==5.17.0 tokenizers==0.23.2 safetensors==0.8.0 huggingface-hub==1.31.0
```

复制 text2vec 模型到新位置。服务只加载本地文件，不会自动下载模型。若版本无法安装，应先检查平台和 Python 版本，不要把任意升级后的环境称为已验收环境。后续交付仍应补完整依赖锁定和干净环境验证。

## 3. 启动与停止

两个 Python 服务在宿主机各占一个终端，Compose 管理另外五个容器。下列简易令牌读取适用于本项目 `KEY=value` 格式的单行、不带包裹引号的值；不是完整 dotenv 解析器。

### 终端一：本地嵌入 8090

```powershell
Set-Location 'D:\school\408-assistant'
$embeddingLine = Get-Content .env | Where-Object { $_ -match '^EMBEDDING_API_KEY=' }
$env:LOCAL_EMBEDDING_API_KEY = $embeddingLine.Substring($embeddingLine.IndexOf('=') + 1).Trim()
& 'D:\school\408-assistant\.cache\embedding-venv\Scripts\python.exe' services/local_embedding/server.py --model-path 'D:\models\text2vec-base-chinese'
```

### 终端二：结构化解析 8091

```powershell
Set-Location 'D:\school\408-assistant'
$parserLine = Get-Content .env | Where-Object { $_ -match '^DOCUMENT_PARSER_TOKEN=' }
$env:PARSER_TOKEN = $parserLine.Substring($parserLine.IndexOf('=') + 1).Trim()
$env:PARSER_DEVICE = 'cpu'
$env:PADDLE_PDX_CACHE_HOME = Join-Path (Get-Location) '.cache/paddle'
$env:PADDLE_PDX_MODEL_SOURCE = 'BOS'
$env:DISABLE_MODEL_SOURCE_CHECK = 'True'
& 'D:\school\408-assistant\.cache\parser-venv\Scripts\python.exe' services/document_parser/server.py
```

### 终端三：前后端与依赖

如果需要恢复旧数据，先完成下一节的恢复，再运行应用；全新空库直接执行：

```powershell
Set-Location 'D:\school\408-assistant'
docker compose up -d --build
docker compose ps
curl.exe --max-time 5 http://localhost:8080/healthz
curl.exe --max-time 5 -I http://localhost:5173
curl.exe --max-time 5 http://localhost:8090/healthz
curl.exe --max-time 5 http://localhost:8091/healthz
docker compose exec -T postgres psql -U exam408 -d exam408 -c 'SELECT max(version) FROM schema_migrations;'
```

API 应返回 `{"status":"ok"}`，前端 HTTP 200，本地嵌入为 768 维，数据库应为迁移 15（公共知识库管理版本）。解析 `layout_loaded=false` 表示 HTTP 已启动但模型尚未在首次复杂页处理时加载，不等于故障。配置完整不等于真实模型供应商已经连通。

打开 http://localhost:5173 。只改 Python 源码需重启对应 Python 服务；Go/前端改动需重新构建镜像，不能只重启旧容器。

停止时运行 `docker compose stop`，两个 Python 终端分别 Ctrl+C。不要使用 `docker compose down -v`，它会删除业务数据卷。切换旧版本程序前需确认迁移兼容，不建议在迁移 8 数据库上直接运行旧 worker。

## 4. 数据交接与恢复操作

以下是人工执行步骤，没有为当前机器制作新备份。建议先等待在途解析结束，再暂停业务写入，在同一停写窗口备份数据库与原文件。历史 `.cache/*.dump` 只是历史快照，不保证包含当前资料。

### 4.1 原机器备份

先启动所需容器、确保数据目录存在，然后停止前端/API/worker，保留数据库：

```powershell
Set-Location 'D:\school\408-assistant'
New-Item -ItemType Directory -Force output/handoff
docker compose stop frontend backend worker
docker compose exec -T postgres pg_dump -U exam408 -d exam408 -Fc -f /tmp/exam408-handoff.dump
docker compose cp postgres:/tmp/exam408-handoff.dump ./output/handoff/database.dump
docker compose run --rm --no-deps --user root --entrypoint tar backend -czf /data/uploads/handoff-uploads.tar.gz --exclude=./handoff-uploads.tar.gz -C /data/uploads .
docker compose cp backend:/data/uploads/handoff-uploads.tar.gz ./output/handoff/uploads.tar.gz
docker compose run --rm --no-deps --user root --entrypoint rm backend /data/uploads/handoff-uploads.tar.gz
Get-FileHash output/handoff/database.dump,output/handoff/uploads.tar.gz -Algorithm SHA256
```

数据库导出采用容器内文件再 `compose cp`，避免 PowerShell 二进制重定向损坏。上传归档临时写在同一个卷内并排除自身，导出后删除临时归档。确认命令成功后交付两个文件和哈希；本命令示例尚未实际执行验收。如需恢复原机器服务，执行 `docker compose up -d`。

### 4.2 新机器恢复（仅用于确认没有需要保留数据的新环境）

以下步骤会写入目标数据库/上传卷。先配置好 .env，将备份放到 output/handoff；不要对已有重要数据的环境直接操作。

```powershell
Set-Location 'D:\school\408-assistant'
docker compose build backend worker frontend
docker compose up -d postgres redis
docker compose ps
docker compose create backend
docker compose cp ./output/handoff/database.dump postgres:/tmp/exam408-handoff.dump
docker compose exec -T postgres pg_restore -U exam408 -d exam408 --clean --if-exists --no-owner --no-acl --exit-on-error /tmp/exam408-handoff.dump
docker compose cp ./output/handoff/uploads.tar.gz backend:/data/uploads/handoff-uploads.tar.gz
docker compose run --rm --no-deps --user root --entrypoint sh backend -c 'tar -xzf /data/uploads/handoff-uploads.tar.gz -C /data/uploads && chown -R app:app /data/uploads && rm /data/uploads/handoff-uploads.tar.gz'
docker compose up -d
```

先确认数据库健康再运行 pg_restore，任何一步报错先处理，不继续覆盖写入。随后启动/检查两个本地服务及第 3 节健康接口。使用原账号登录，抽查资料原文件可打开、解析结果可读、历史测评和对话存在、当前模型空间可检索。更换模型空间后旧向量不会自动兼容，是否重处理由资料所有者决定。

Redis 当前不是文档任务权威存储；核心恢复对象是数据库与 uploads。完整恢复示例尚未在另一台干净机器演练，接手验收时应把这一项单独记录。

## 5. 学生如何使用

1. 注册/登录，在“学习资料”创建知识库并上传资料。大教材建议先拆章；等待处理完成，可随时查看解析结果和警告。
2. 在“知识体系”查看知识点，在资料页关联知识点。关联文档不会自动修正所有抽题匹配问题。
3. 在“AI 学习助手”选知识库提问。点击引用检查原文，历史会话可恢复、改名或删除。低相关资料可能拒答。
4. 在“题目与资源”查看自动提取题目；可手工补齐知识点、选项、来源，或管理教材和视频链接。下载 JSON 模板后按格式导入，答案索引从 0 开始，未知为 -1。
5. 在测评页面填写目标、日期、每日分钟数和偏好，开始诊断/练习/新题复测。交卷后查看错题和解析；无资料答案时可请求 AI 参考结果，按自己判断确认正确/错误或不确定撤回。
6. 查看四科雷达、掌握度和薄弱点，在“学习任务”主动生成每日计划，完成后标记。修改当天已冻结的时间预算会在次日生效。
7. 需要补练时主动点击生成练习，可选知识库、知识点和父题。默认资料依据不足不生成；明确允许无依据后生成的题会标注来源限制。

不要把 AI 推断当成资料标准答案；生成题默认不计学情。题目/推荐为空可能只是还没有完整且匹配知识点的个人资源。

## 6. 常见问题与检查入口

| 现象 | 检查与处理 |
|---|---|
| 一直在 5% 解析 | 阶段百分比只分配 5%–15%；CPU OCR 很慢，先看 parser 日志和页检查点，不反复上传 |
| 文档失败 | 看错误提示、解析服务、模型令牌和向量维度；修复后点继续处理，兼容检查点可复用 |
| AI 不可用 | 检查 LLM/Embedding 的地址、模型、Key 和本地 8090；状态接口只检测配置，不验证供应商连通 |
| Docker 访问本地模型失败 | 容器使用 host.docker.internal，直接运行 Go 使用 127.0.0.1；检查服务监听和防火墙 |
| 原文没有矩形高亮 | 可能是旧资料无映射、裁切页或旋转基准不一致；系统会降级显示，不伪造坐标 |
| 新电脑登录/资料丢失 | 检查是否恢复了配套数据库和 uploads；只复制代码无法迁移用户数据 |
| JSON 导入中途失败 | 按提示核对已成功条数再修正；导入不是整批原子事务 |
| Docker 权限错误 | 确认 Docker Desktop Linux 引擎运行，当前终端有 Docker 访问权限 |

```powershell
docker compose logs --tail=100 backend worker
docker compose logs --tail=60 postgres
```

宿主机 Python 前台终端显示解析/嵌入输出；后台日志文件名由启动方式决定，不是所有机器都会有旧机器的 .cache 日志。

## 7. 开发验证与关键调用链

只修改文档无需重跑收费业务或处理真实教材。修改代码时按范围验证：

```powershell
Set-Location 'D:\school\408-assistant'
node scripts/check-independence.mjs
Set-Location backend
# 可选：设置 TEST_DATABASE_URL 后，集成测试会创建/删除随机隔离库，连接账号需有建库权限。
# $env:TEST_DATABASE_URL='postgres://exam408:exam408@localhost:5432/exam408?sslmode=disable'
& 'C:\Program Files\Go\bin\go.exe' test -count=1 ./...
& 'C:\Program Files\Go\bin\go.exe' vet ./...
Set-Location ../frontend
npm ci
npm test
npm run build
Set-Location ..
& 'D:\school\408-assistant\.cache\parser-venv\Scripts\python.exe' -m unittest discover -s services/document_parser -p test_server.py
```

Go 路径按实际安装调整。不设 TEST_DATABASE_URL 时数据库测试可能跳过，不能报告数据库验收通过。两个 `scripts/check-*.py` 真实链路脚本可能调用供应商产生费用，不能当普通离线测试自动执行。

关键调用链：

- 上传：ResourcesView → learningApi.upload → httpapi.uploadDocument → DocumentService.Upload → EnqueueDocument → worker.RunDocumentJob → Process/processRevision → ParsePageBatch → 保存页/分块 → Embed → SaveRevisionVectors → ExtractQuestions → ActivateRevision。
- 答疑：AssistantView → streamChat → chatStream → RAGService.ChatStream → contextualQuestion/Embed/SearchChunks → 相关性筛选 → AnswerStream → SaveTurn；Sources 进入 CitationReader。
- 学情：AssessmentView → SubmitAssessment/Grade → syncAssessmentEvents；AI 辅助结果走 Infer/SaveAIAnswer，确认走 ConfirmAnswer → 替换本次事件与重算 → Radar/ListMastery。
- 推荐：TasksView 或 WeaknessView → GenerateTasks → PlanCandidates → daily_plans/study_tasks → PlanRecommendations。旧加权公式只作为不具备计划能力的 Store 回退路径。

## 8. 文件与函数索引

以下索引覆盖 149 个项目自有后端、前端 src、服务、脚本、迁移、测试、配置和文档文件。列出具名函数/方法和文件职责；不把依赖目录、构建产物、上传资料、缓存和匿名回调逐一列为产品源码。函数名来自当前文件声明，具体参数与分支以源码为准。同一文件中的函数共享该行描述的职责；下文另解释容易混淆的关键函数。测试文件若只有 test(...) 匿名注册，表中标为无具名函数，不代表没有测试。

<!-- FILE_INDEX_START -->

### 根目录与构建配置

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [.dockerignore](../.dockerignore) | 项目根前端构建上下文白名单，排除依赖/产物。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [.env.example](../.env.example) | 带中文注释的配置示例，真实密钥不写入。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [.gitignore](../.gitignore) | 排除密钥、环境、上传和构建缓存。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [AGENTS.md](../AGENTS.md) | 开发、独立性及可选参考源码约束。 | 无具名函数；说明文档。 |
| [Makefile](../Makefile) | 常用构建/验证命令入口。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [README.md](../README.md) | 项目阅读入口与快速流程。 | 无具名函数；说明文档。 |
| [backend/.dockerignore](../backend/.dockerignore) | 排除后端构建上下文的本地无关文件。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/Dockerfile](../backend/Dockerfile) | Go API/worker 多阶段镜像构建，共用一个二进制。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/go.mod](../backend/go.mod) | Go 模块、语言版本和依赖声明。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/go.sum](../backend/go.sum) | Go 依赖校验记录。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/storage/uploads/.gitkeep](../backend/storage/uploads/.gitkeep) | 保留宿主机开发上传目录；不是 Docker uploads 卷的数据备份。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [compose.yaml](../compose.yaml) | 五个容器、环境覆盖、端口、数据卷和依赖编排。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/.dockerignore](../frontend/.dockerignore) | 保留旧 frontend 独立上下文排除规则；当前 Compose 根上下文使用根 .dockerignore。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/Dockerfile](../frontend/Dockerfile) | 在项目根上下文构建前端及许可证，交付 nginx 静态镜像。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/index.html](../frontend/index.html) | SPA HTML 入口。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/nginx.conf](../frontend/nginx.conf) | 静态资源、SPA 回退、API 和 SSE 反向代理/上传上限。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/package-lock.json](../frontend/package-lock.json) | npm 依赖版本锁。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/package.json](../frontend/package.json) | npm 依赖与开发、测试、类型检查和构建脚本。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/tsconfig.json](../frontend/tsconfig.json) | TypeScript 编译/检查配置。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/vite.config.ts](../frontend/vite.config.ts) | Vue 构建、别名、开发端口和 /api 代理。 | 无具名函数；声明、配置、数据或顶层脚本。 |

### 后端配置、领域和入口

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [backend/cmd/api/main.go](../backend/cmd/api/main.go) | 组装配置、数据库、模型和路由；APP_ROLE 选择 API 或独立 worker；处理退出信号。 | `main` |
| [backend/internal/config/capabilities.go](../backend/internal/config/capabilities.go) | 公开答疑模式与配置完整性，不探测真实供应商。 | `Config.AssistantStatus` |
| [backend/internal/config/config.go](../backend/internal/config/config.go) | 后端配置读取、默认值和启动校验。 | `Load`、`env` |
| [backend/internal/config/config_test.go](../backend/internal/config/config_test.go) | 配置合法性/JWT 等安全边界回归。 | `setRequiredEnvironment`、`TestAssistantStatusModes`、`TestLoadRequiresJWTSecret`、`TestLoadRejectsPlaceholderJWTSecret`、`TestLoadAcceptsStrongJWTSecret` |
| [backend/internal/config/embedding.go](../backend/internal/config/embedding.go) | 计算端点、模型、维度、演示模式的向量空间指纹。 | `Config.EmbeddingSpace` |
| [backend/internal/config/embedding_test.go](../backend/internal/config/embedding_test.go) | 模型空间和维度配置回归。 | `TestEmbeddingSpaceIsolation`、`TestLocalEmbeddingDimension` |
| [backend/internal/domain/conversation.go](../backend/internal/domain/conversation.go) | 会话、消息和流式模型合同。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/internal/domain/errors.go](../backend/internal/domain/errors.go) | 带公开状态码的请求错误。 | `RequestError.Error`、`RequestError.PublicStatus` |
| [backend/internal/domain/interfaces.go](../backend/internal/domain/interfaces.go) | Store、Parser、Embedding、LLM 等依赖接口与错误合同。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/internal/domain/learning.go](../backend/internal/domain/learning.go) | 个人资源、测评、问卷、任务和学习平台接口结构。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/internal/domain/models.go](../backend/internal/domain/models.go) | 用户、知识库、文档、分块、来源、学习和推荐等基础数据结构。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/internal/domain/planner.go](../backend/internal/domain/planner.go) | 题族身份和规则计划候选：有效作答、复习间隔、难度、偏好与时长。 | `Family`、`PlanCandidates` |
| [backend/internal/domain/planner_test.go](../backend/internal/domain/planner_test.go) | 规则复习、难度/偏好/预计时间候选回归。 | `TestPlannerEvidenceAndDurations` |
| [backend/internal/domain/resource_status.go](../backend/internal/domain/resource_status.go) | 根据内容完整性和来源计算资源 ready/unavailable 及原因。 | `PrepareAsset` |
| [backend/internal/domain/structure.go](../backend/internal/domain/structure.go) | 结构化页面、来源区域、版本及检查点数据结构。 | 无具名函数；声明、配置、数据或顶层脚本。 |

### 后端 HTTP 与服务

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [backend/internal/httpapi/browser_acceptance_test.go](../backend/internal/httpapi/browser_acceptance_test.go) | 显式开关的一次性浏览器验收环境与本地模型替身。 | `browserParser.Parse`、`browserParser.ParsePages`、`TestBrowserAcceptance` |
| [backend/internal/httpapi/foundation.go](../backend/internal/httpapi/foundation.go) | 资源、问卷、测评、雷达、计划、会话路由；认证文件下载和流式回答。 | `Server.foundationRoutes`、`Server.reply`、`Server.input`、`Server.saveAsset`、`Server.documentFile`、`Server.chatStream` |
| [backend/internal/httpapi/foundation_integration_test.go](../backend/internal/httpapi/foundation_integration_test.go) | 隔离 PostgreSQL 下测评、资源、会话、流式和学习接口集成验收。 | `syntheticEmbedding.Embed`、`TestFoundationDatabaseAndHTTP` |
| [backend/internal/httpapi/model_policy_test.go](../backend/internal/httpapi/model_policy_test.go) | 模型配置与演示模式边界回归。 | `TestMissingModelsRejectChatBeforeProcessing` |
| [backend/internal/httpapi/personal.go](../backend/internal/httpapi/personal.go) | 生成练习、用户确认、参考答案和版本结构读取路由；使用匿名处理函数。 | `Server.personalRoutes` |
| [backend/internal/httpapi/server.go](../backend/internal/httpapi/server.go) | 基础路由、认证/CORS/请求 ID/异常恢复；账号、知识库、文档、知识点、学情和同步问答处理。 | `Server.Handler`、`write`、`decode`、`uid`、`Server.fail`、`Server.auth`、`Server.cors`、`Server.requestID`、`Server.recover`、`publicUser`、`Server.register`、`Server.login`、`Server.me`、`Server.createKB`、`Server.listKB`、`Server.deleteKB`、`Server.listDocuments`、`fileHeader`、`Server.uploadDocument`、`Server.deleteDocument`、`Server.reprocessDocument`、`Server.mapDocument`、`Server.subjects`、`Server.points`、`Server.point`、`Server.event`、`Server.mastery`、`Server.weaknesses`、`Server.recommendations`、`Server.generateRecommendations`、`Server.dashboard`、`Server.chat` |
| [backend/internal/service/asset_validation.go](../backend/internal/service/asset_validation.go) | 校验资源和问卷输入字段。 | `ValidateAsset`、`ValidateProfile` |
| [backend/internal/service/auth.go](../backend/internal/service/auth.go) | 注册、登录、bcrypt 和 JWT 签发/解析。 | `AuthService.Register`、`AuthService.Login`、`AuthService.Token`、`AuthService.Parse` |
| [backend/internal/service/availability.go](../backend/internal/service/availability.go) | 公开且可标注重试性的供应商错误。 | `PublicError.Temporary`、`PublicError.Error`、`unavailable`、`providerFailure` |
| [backend/internal/service/document.go](../backend/internal/service/document.go) | 上传校验/保存/入队、处理分发、重处理与删除；保留兼容 Store 的旧处理路径。 | `Parser.Parse`、`ChunkText`、`DocumentService.Upload`、`DocumentService.Process`、`DocumentService.fail`、`DocumentService.Reprocess`、`DocumentService.progress`、`DocumentService.start`、`DocumentService.Delete`、`DocumentService.logError` |
| [backend/internal/service/document_progress_test.go](../backend/internal/service/document_progress_test.go) | 文档批次与进度/失败回归。 | `progressParser.Parse`、`progressEmbedder.Embed`、`TestDocumentProgressSuccessAndFailure` |
| [backend/internal/service/document_revision.go](../backend/internal/service/document_revision.go) | 配置/文件指纹、可恢复版本处理、向量校验、规范块文本与来源分块。 | `digest`、`fileDigest`、`DocumentService.processRevision`、`validateVectors`、`CanonicalPage`、`MappedChunks` |
| [backend/internal/service/evaluation_test.go](../backend/internal/service/evaluation_test.go) | 固定合成检索相关性回归，不代表教材质量。 | `TestSyntheticEvidenceRegression` |
| [backend/internal/service/foundation_test.go](../backend/internal/service/foundation_test.go) | 基础资源/测评规则与模型流协议回归。 | `TestDOCXAndQualityGate`、`TestRealSSEParsingAndHistory`、`TestResourceReadinessRequiresCompleteContent` |
| [backend/internal/service/learning.go](../backend/internal/service/learning.go) | 关键词知识点识别、可信/被动学习事件校验、优先每日计划的推荐入口。 | `KnowledgeService.Detect`、`LearningService.RecordUserEvent`、`LearningService.Record`、`RecommendationService.Generate` |
| [backend/internal/service/model_policy_test.go](../backend/internal/service/model_policy_test.go) | 模型配置与演示模式边界回归。 | `TestProvidersRequireExplicitDemo`、`TestProviderFailuresNeverFallbackOrLeakResponse`、`TestInvalidAndEmptyModelResponsesAreErrors`、`forbiddenLLM.Answer`、`TestRAGRejectsUnrelatedAndLowScoreEvidenceWithoutCallingLLM` |
| [backend/internal/service/pages.go](../backend/internal/service/pages.go) | 本地 TXT/Markdown/PDF/DOCX 解析与正文质量检测。 | `ValidBody`、`Parser.ParsePages` |
| [backend/internal/service/parser_transport.go](../backend/internal/service/parser_transport.go) | 独立结构服务的指纹、认证传输、分页协议与超时/响应校验。 | `Parser.Fingerprint`、`Parser.remotePages`、`Parser.ParsePageBatch`、`Parser.remoteBatch` |
| [backend/internal/service/parser_transport_test.go](../backend/internal/service/parser_transport_test.go) | 远程分页解析 HTTP 合同回归。 | `TestParserBatchTransport` |
| [backend/internal/service/personal_test.go](../backend/internal/service/personal_test.go) | 抽题、来源、AI 辅助判分及个人资源回归。 | `TestConfiguredParserKeepsMarkdownLocal`、`TestVisionStreamSendsImages`、`TestExtractQuestionsAndMissingFigure`、`TestQuestionContinuationAndAnswerPageFailures`、`TestQuestionAIProtocolAndUncertainty` |
| [backend/internal/service/providers.go](../backend/internal/service/providers.go) | 演示向量、OpenAI 兼容嵌入和文本回答适配。 | `OpenAIEmbedding.Embed`、`DeterministicEmbedding.Embed`、`OpenAILLM.Answer` |
| [backend/internal/service/question_ai.go](../backend/internal/service/question_ai.go) | 交卷后按文本/可选视觉模型取得 AI 参考答案。 | `QuestionAI.Infer` |
| [backend/internal/service/question_extract.go](../backend/internal/service/question_extract.go) | 规则抽取编号单选题、资料答案、知识点匹配和题目身份。 | `ExtractQuestions` |
| [backend/internal/service/question_generation.go](../backend/internal/service/question_generation.go) | 验证所有权、次数、资料依据和生成响应，保存新题/变式来源。 | `QuestionGenerator.Generate`、`pointEvidenceRelated` |
| [backend/internal/service/rag.go](../backend/internal/service/rag.go) | 同步/流式 RAG 编排、引用构造、图像证据、事件及完整问答保存。 | `RAGService.Chat`、`RAGService.ChatStream`、`RAGService.figureEvidence`、`RAGService.warn`、`citationPage` |
| [backend/internal/service/relevance.go](../backend/internal/service/relevance.go) | 保守的词面证据相关性检查。 | `evidenceRelated` |
| [backend/internal/service/service_test.go](../backend/internal/service/service_test.go) | 认证、分块、RAG 等服务基础回归。 | `TestAuthRegisterLoginAndJWT`、`TestKnowledgeBaseAndUserIsolation`、`TestDocumentChunkingPreservesOverlap`、`TestVectorRetrievalOrdersAndIsolates`、`TestVectorRetrievalExcludesNonReadyDocuments`、`TestKnowledgePointDetectionAndRelation`、`TestLearningMasteryWeaknessAndRecommendation`、`TestPublicLearningEventsCannotForgeOutcomes`、`TestRAGReturnsCitationAndLearningEvent`、`fixedEmbedding.Embed`、`fixedLLM.Answer` |
| [backend/internal/service/source_mapping_test.go](../backend/internal/service/source_mapping_test.go) | 规范文本和分块来源映射回归。 | `TestCanonicalSourceMapping` |
| [backend/internal/service/stream.go](../backend/internal/service/stream.go) | 模型 SSE 和图像输入适配、有限历史及短追问上下文。 | `OpenAILLM.AnswerStream`、`OpenAILLM.answerWithImages`、`boundedHistory`、`contextualQuestion` |
| [backend/internal/service/testdata/synthetic_relevance.json](../backend/internal/service/testdata/synthetic_relevance.json) | 合成词面相关性回归案例，不是用户教材数据。 | 无具名函数；声明、配置、数据或顶层脚本。 |

### 后端数据库、迁移与测试替身

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [backend/internal/repository/conversations.go](../backend/internal/repository/conversations.go) | 会话查询、改名删除、锁和问答对事务保存。 | `Postgres.ListConversations`、`Postgres.ConversationMessages`、`Postgres.RenameConversation`、`Postgres.DeleteConversation`、`Postgres.LockConversation`、`Postgres.SaveTurn` |
| [backend/internal/repository/document_jobs.go](../backend/internal/repository/document_jobs.go) | 持久化入队/领取、租约续期、超时重试和执行隔离。 | `Postgres.EnqueueDocument`、`Postgres.RunDocumentJob`、`Postgres.executionTx`、`retryableDocumentError` |
| [backend/internal/repository/document_revisions.go](../backend/internal/repository/document_revisions.go) | 页/分块/向量检查点、版本状态、任务进度和活动版本原子切换。 | `Postgres.BeginRevision`、`Postgres.SaveRevisionPage`、`Postgres.RevisionPages`、`Postgres.CompleteRevisionParse`、`Postgres.SaveRevisionChunks`、`Postgres.PendingRevisionChunks`、`Postgres.SaveRevisionVectors`、`Postgres.RevisionCounts`、`Postgres.RevisionProgress`、`Postgres.ActivateRevision`、`Postgres.revisionTx` |
| [backend/internal/repository/document_revisions_test.go](../backend/internal/repository/document_revisions_test.go) | 数据库检查点恢复、版本/租约隔离、预算/生成与可选受控压力测试。 | `pageParser.Parse`、`pageParser.Fingerprint`、`pageParser.ParsePageBatch`、`zeroCostEmbedding.Embed`、`countEmbedding.Embed`、`TestRevisionRecoveryAndFencing` |
| [backend/internal/repository/document_structure.go](../backend/internal/repository/document_structure.go) | 解析快照和抽题资源保存，当前/指定历史版本结构读取。 | `Postgres.SaveDocumentStructure`、`saveDocumentStructureTx`、`Postgres.DocumentStructure`、`Postgres.RevisionStructure` |
| [backend/internal/repository/generation.go](../backend/internal/repository/generation.go) | 生成请求次数预留、幂等、失败与完成结果持久化。 | `Postgres.ReserveGeneration`、`Postgres.FailGeneration`、`Postgres.CompleteGeneration` |
| [backend/internal/repository/learning_platform.go](../backend/internal/repository/learning_platform.go) | 个人资源、问卷、测评快照/判分、四科雷达和任务完成。 | `Postgres.Profile`、`Postgres.SaveProfile`、`scanAsset`、`Postgres.Assets`、`Postgres.SaveAsset`、`Postgres.Asset`、`Postgres.DeleteAsset`、`RedactAssessment`、`Postgres.CreateAssessment`、`scanAssessment`、`Postgres.Assessments`、`Grade`、`Postgres.SubmitAssessment`、`Postgres.Radar`、`Postgres.Tasks`、`Postgres.CompleteTask` |
| [backend/internal/repository/personal.go](../backend/internal/repository/personal.go) | AI 结果、用户确认/撤回、判分变更账本和题族学情重算。 | `syncAssessmentEvents`、`recount`、`Postgres.Assessment`、`Postgres.ConfirmAnswer`、`Postgres.SaveAIAnswer`、`Postgres.updateResult`、`Postgres.RefreshAssetAvailability` |
| [backend/internal/repository/plans.go](../backend/internal/repository/plans.go) | 北京时间每日预算、计划任务事务生成和推荐投影。 | `planDate`、`Postgres.DailyPlan`、`Postgres.GenerateTasks`、`Postgres.PlanRecommendations` |
| [backend/internal/repository/postgres.go](../backend/internal/repository/postgres.go) | 数据库连接、基础 CRUD、模型空间精确检索、事件/旧投影、知识点和看板查询。 | `Open`、`id`、`nf`、`Postgres.CreateUser`、`scanUser`、`Postgres.UserByLogin`、`Postgres.UserByID`、`Postgres.CreateKnowledgeBase`、`scanKB`、`Postgres.KnowledgeBase`、`Postgres.ListKnowledgeBases`、`Postgres.DeleteKnowledgeBase`、`Postgres.CreateDocument`、`scanDoc`、`Postgres.Document`、`Postgres.ListDocuments`、`Postgres.UpdateDocumentStatus`、`Postgres.DeleteDocument`、`Postgres.UpdateDocumentProgress`、`vectorLiteral`、`Postgres.ReplaceChunks`、`Postgres.SearchChunks`、`Postgres.ListSubjects`、`Postgres.ListKnowledgePoints`、`Postgres.KnowledgePoint`、`Postgres.PointRelations`、`Postgres.MapDocumentToPoint`、`Postgres.KnowledgePointOverview`、`Postgres.CreateConversation`、`Postgres.SaveMessage`、`Postgres.CreateLearningEvent`、`deltas`、`Postgres.ApplyMastery`、`Postgres.ListMastery`、`Postgres.ReplaceRecommendations`、`Postgres.ListRecommendations`、`Postgres.Dashboard` |
| [backend/internal/testutil/store.go](../backend/internal/testutil/store.go) | 测试内存 Store 替身；不是生产数据库或实际效果证据。 | `NewStore`、`newid`、`Store.CreateUser`、`Store.UserByLogin`、`Store.UserByID`、`Store.CreateKnowledgeBase`、`Store.ListKnowledgeBases`、`Store.KnowledgeBase`、`Store.DeleteKnowledgeBase`、`Store.CreateDocument`、`Store.ListDocuments`、`Store.Document`、`Store.UpdateDocumentStatus`、`Store.DeleteDocument`、`Store.UpdateDocumentProgress`、`Store.ReplaceChunks`、`cosine`、`Store.SearchChunks`、`Store.ListSubjects`、`Store.ListKnowledgePoints`、`Store.KnowledgePoint`、`Store.PointRelations`、`Store.MapDocumentToPoint`、`Store.KnowledgePointOverview`、`Store.CreateConversation`、`Store.SaveMessage`、`Store.CreateLearningEvent`、`Store.ApplyMastery`、`Store.ListMastery`、`Store.ReplaceRecommendations`、`Store.ListRecommendations`、`Store.Dashboard` |
| [backend/migrations/000001_initial.down.sql](../backend/migrations/000001_initial.down.sql) | 版本 000001 的 回滚 SQL（历史版本；不要直接对现有业务库执行）。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000001_initial.up.sql](../backend/migrations/000001_initial.up.sql) | 版本 000001 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000002_operational_indexes.down.sql](../backend/migrations/000002_operational_indexes.down.sql) | 版本 000002 的 回滚 SQL（历史版本；不要直接对现有业务库执行）。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000002_operational_indexes.up.sql](../backend/migrations/000002_operational_indexes.up.sql) | 版本 000002 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000003_embedding_spaces.up.sql](../backend/migrations/000003_embedding_spaces.up.sql) | 版本 000003 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000004_document_progress.up.sql](../backend/migrations/000004_document_progress.up.sql) | 版本 000004 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000005_learning_foundation.up.sql](../backend/migrations/000005_learning_foundation.up.sql) | 版本 000005 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000006_personal_resources.up.sql](../backend/migrations/000006_personal_resources.up.sql) | 版本 000006 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000007_document_revisions.up.sql](../backend/migrations/000007_document_revisions.up.sql) | 版本 000007 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/000008_learning_plans.up.sql](../backend/migrations/000008_learning_plans.up.sql) | 版本 000008 的 结构/数据升级 SQL；作用见 data-model.md。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [backend/migrations/embed.go](../backend/migrations/embed.go) | 内嵌有序迁移、启动锁和旧数据库版本登记。 | `Up` |
| [backend/migrations/revisions_test.go](../backend/migrations/revisions_test.go) | 旧数据库升级与迁移结构回归。 | `TestUpgradeKeepsLegacyActiveRevision` |

### 前端页面、组件与 API

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [frontend/src/App.vue](../frontend/src/App.vue) | 应用根组件与路由出口。 | 无具名函数；以组件模板/响应式回调为入口。 |
| [frontend/src/api/client.ts](../frontend/src/api/client.ts) | Axios 实例、认证请求/响应拦截及通用错误消息。 | `messageOf` |
| [frontend/src/api/errors.ts](../frontend/src/api/errors.ts) | 将服务端错误转换成可读消息。 | `errorMessage` |
| [frontend/src/api/foundation.ts](../frontend/src/api/foundation.ts) | 资源、测评、计划、会话、确认与生成 API；认证原文件打开和 SSE 事件读取。 | `openDocument`、`streamChat`、`assets`、`saveAsset`、`profile`、`assessments`、`start`、`submit`、`confirm`、`infer`、`structure`、`radar`、`tasks`、`dailyPlan`、`generatePractice`、`conversations`、`messages`、`removeAsset`、`saveProfile`、`generate`、`complete`、`rename`、`removeConversation` |
| [frontend/src/api/learning.ts](../frontend/src/api/learning.ts) | 知识库/上传/知识点/看板/答疑/推荐 API 封装。 | `list`、`assistantStatus`、`dashboard`、`createBase`、`upload`、`point`、`generateRecommendations`、`chat`、`bases`、`documents`、`removeDocument`、`reprocess`、`mapPoint`、`subjects`、`points`、`viewPoint`、`weaknesses`、`recommendations` |
| [frontend/src/api/normalize.ts](../frontend/src/api/normalize.ts) | 空数组和看板响应标准化。 | `asList`、`normalizeDashboard` |
| [frontend/src/api/types.ts](../frontend/src/api/types.ts) | 知识库、文档、知识点、引用、学情、看板等 TypeScript 合同。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/src/components/AppLayout.vue](../frontend/src/components/AppLayout.vue) | 主导航、用户信息、退出和页面框架。 | `logout` |
| [frontend/src/components/CitationReader.vue](../frontend/src/components/CitationReader.vue) | 加载版本结构和原文件、PDF.js 绘页、缩放旋转/区域叠加与文本降级。 | `area`、`cleanup`、`paint`、`onKey` |
| [frontend/src/components/GeneratePractice.vue](../frontend/src/components/GeneratePractice.vue) | 选择资料/知识点/父题，显式生成练习及无依据确认，复用请求 ID。 | `generate` |
| [frontend/src/components/SafeAnswer.vue](../frontend/src/components/SafeAnswer.vue) | 显示安全回答 HTML，并转发真实引用点击。 | `click` |
| [frontend/src/lib/answer.ts](../frontend/src/lib/answer.ts) | Markdown/表格/KaTeX 渲染、清洗与有效引用按钮。 | `renderAnswer` |
| [frontend/src/lib/sourceGeometry.ts](../frontend/src/lib/sourceGeometry.ts) | 校验归一化矩形，并按阅读器旋转变换坐标。 | `rotateBox`、`validBox` |
| [frontend/src/main.ts](../frontend/src/main.ts) | 挂载 Vue、Pinia、Router 和全局样式。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/src/router/index.ts](../frontend/src/router/index.ts) | 异步页面路由及登录守卫。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/src/stores/auth.ts](../frontend/src/stores/auth.ts) | 登录、注册、用户加载、退出和本地令牌状态。 | `login`、`register`、`load`、`state`、`logout` |
| [frontend/src/styles.css](../frontend/src/styles.css) | 全局样式、布局与通用控件外观。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/src/views/AssessmentView.vue](../frontend/src/views/AssessmentView.vue) | 问卷、诊断/练习/复测、交卷、历史、四科雷达与 AI 确认/撤回。 | `xy`、`load`、`action`、`start`、`resume`、`submit`、`infer`、`confirmAnswer`、`answerLetter`、`saveProfile` |
| [frontend/src/views/AssetsView.vue](../frontend/src/views/AssetsView.vue) | 资源编辑、完整性反馈、JSON 模板/导入与原文查看。 | `blank`、`load`、`edit`、`action`、`save`、`remove`、`importFile`、`template` |
| [frontend/src/views/AssistantView.vue](../frontend/src/views/AssistantView.vue) | 多轮聊天、历史切换/重命名/删除、流式处理与引用阅读入口。 | `load`、`reset`、`selectConversation`、`rename`、`remove`、`onEnter`、`source`、`send` |
| [frontend/src/views/DashboardView.vue](../frontend/src/views/DashboardView.vue) | 学习概览、四科知识点覆盖与薄弱点/建议展示。 | `load` |
| [frontend/src/views/KnowledgeView.vue](../frontend/src/views/KnowledgeView.vue) | 知识点分科、详情和关联资料，记录被动查看。 | `load`、`open` |
| [frontend/src/views/LoginView.vue](../frontend/src/views/LoginView.vue) | 注册/登录表单与提交状态。 | `switchMode`、`submit` |
| [frontend/src/views/ResourcesView.vue](../frontend/src/views/ResourcesView.vue) | 知识库、上传、状态筛选、轮询、解析结果、原文、关联/继续/删除。 | `inspectStructure`、`loadDocs`、`selectBase`、`refresh`、`create`、`upload`、`documentAction`、`progressOf` |
| [frontend/src/views/SettingsView.vue](../frontend/src/views/SettingsView.vue) | 个人账号/系统设置说明页面。 | 无具名函数；以组件模板/响应式回调为入口。 |
| [frontend/src/views/TasksView.vue](../frontend/src/views/TasksView.vue) | 每日预算和学习任务，生成计划及完成标记。 | `action` |
| [frontend/src/views/WeaknessView.vue](../frontend/src/views/WeaknessView.vue) | 加载薄弱点与建议，主动生成推荐。 | `load`、`generate` |

### 前端测试与静态文件

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [frontend/public/favicon.svg](../frontend/public/favicon.svg) | 站点图标资产。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/public/third-party-notices.txt](../frontend/public/third-party-notices.txt) | 构建生成的第三方库许可证与来源说明，不手工改写以替代生成脚本。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/tests/answer.test.mjs](../frontend/tests/answer.test.mjs) | 回答清洗、引用与来源坐标等前端回归。 | `parsed` |
| [frontend/tests/normalize.test.mjs](../frontend/tests/normalize.test.mjs) | 空列表/看板标准化回归。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [frontend/tests/routes.test.mjs](../frontend/tests/routes.test.mjs) | 页面路由和入口回归。 | 无具名函数；声明、配置、数据或顶层脚本。 |

### 本地服务与辅助脚本

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [scripts/check-independence.mjs](../scripts/check-independence.mjs) | 扫描项目显式参考耦合；不是代码原创性或视觉相似性审计。 | `walk` |
| [scripts/check-local-embedding.py](../scripts/check-local-embedding.py) | 真实本地嵌入/RAG 链路验收，创建临时数据并可能调用收费文本模型。 | `request`、`pdf_bytes`、`main` |
| [scripts/check-personal-resources.py](../scripts/check-personal-resources.py) | 真实合成资料 OCR/抽题/参考解析/确认验收，可能调用收费模型。 | `call` |
| [scripts/update-frontend-notices.mjs](../scripts/update-frontend-notices.mjs) | 构建时收集第三方依赖版本和许可证到 public/third-party-notices.txt。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [services/document_parser/README.md](../services/document_parser/README.md) | 解析安装、协议、模型与格式限制说明。 | 无具名函数；说明文档。 |
| [services/document_parser/requirements-windows.lock.txt](../services/document_parser/requirements-windows.lock.txt) | 既有 Windows Python 解析环境全量包版本快照。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [services/document_parser/requirements.txt](../services/document_parser/requirements.txt) | 解析服务直接依赖与版本约束。 | 无具名函数；声明、配置、数据或顶层脚本。 |
| [services/document_parser/server.py](../services/document_parser/server.py) | PDF/DOCX 分页结构接口，延迟加载 PaddleOCR，图片/公式/表格和警告保留。 | `png`、`box`、`structure`、`parse_pdf`、`parse_docx`、`append_text`、`append_image`、`health`、`parse` |
| [services/document_parser/test_server.py](../services/document_parser/test_server.py) | 替换模型的离线 DOCX/图片/分页解析回归。 | `docx`、`test_failed_image_keeps_original`、`test_image_text_keeps_document_order_and_unique_ids`、`test_unreferenced_media_is_not_inserted`、`test_table_ignores_xml_indentation`、`test_missing_image_warns_and_keeps_text`、`test_native_formula_still_warns`、`test_logical_batch_http_and_auth`、`test_pdf_batch_recognizes_only_requested_page` |
| [services/local_embedding/README.md](../services/local_embedding/README.md) | 嵌入协议、模型文件、环境与启动说明。 | 无具名函数；说明文档。 |
| [services/local_embedding/server.py](../services/local_embedding/server.py) | 本地加载 text2vec；分窗平均归一化、768 维检查、令牌保护的 HTTP 嵌入接口。 | `main`、`log_message`、`reply`、`do_GET`、`do_POST` |

### 文档目录

| 文件 | 功能/职责 | 具名函数、方法或入口 |
|---|---|---|
| [docs/CURRENT_FEATURES.md](../docs/CURRENT_FEATURES.md) | 当前功能/实现限制/费用与验收边界。 | 无具名函数；说明文档。 |
| [docs/DATA_AUDIT.md](../docs/DATA_AUDIT.md) | 早期资料抽样，不是当前数据清单。 | 无具名函数；说明文档。 |
| [docs/ENGINEERING_FOUNDATION.md](../docs/ENGINEERING_FOUNDATION.md) | 历史工程基线，非当前功能说明。 | 无具名函数；说明文档。 |
| [docs/HIGHLIGHT_PLAN.md](../docs/HIGHLIGHT_PLAN.md) | 历史高亮设计讨论。 | 无具名函数；说明文档。 |
| [docs/LEARNING_STRATEGY_RESEARCH.md](../docs/LEARNING_STRATEGY_RESEARCH.md) | 候选算法调研，不代表已安装/训练。 | 无具名函数；说明文档。 |
| [docs/PERSONAL_RESOURCES.md](../docs/PERSONAL_RESOURCES.md) | 个人资料、自动抽题、判分来源与历史验收。 | 无具名函数；说明文档。 |
| [docs/TASKS_3_5_IMPLEMENTATION.md](../docs/TASKS_3_5_IMPLEMENTATION.md) | 首版版本恢复、阅读器、计划/生成工程证据和限制。 | 无具名函数；说明文档。 |
| [docs/TASK_QUEUE.md](../docs/TASK_QUEUE.md) | 已实施队列与按日期保留的讨论。 | 无具名函数；说明文档。 |
| [docs/USER_AND_CODE_GUIDE.md](../docs/USER_AND_CODE_GUIDE.md) | 安装、备份恢复、使用、排错与本文件索引。 | 无具名函数；说明文档。 |
| [docs/WORK_PROGRESS.md](../docs/WORK_PROGRESS.md) | 当前状态和单独保留的历史证据。 | 无具名函数；说明文档。 |
| [docs/architecture.md](../docs/architecture.md) | 当前组件和主流程架构。 | 无具名函数；说明文档。 |
| [docs/data-model.md](../docs/data-model.md) | 当前表、数据归属与迁移 1–8。 | 无具名函数；说明文档。 |
| [docs/frontend-migration.md](../docs/frontend-migration.md) | 历史前端外观来源与迁移证据。 | 无具名函数；说明文档。 |
| [docs/independence.md](../docs/independence.md) | 实现归属、独立性规则与历史贡献记录。 | 无具名函数；说明文档。 |
| [docs/knowledge-graph.md](../docs/knowledge-graph.md) | 教学/语义图谱结构与未完成闭环边界。 | 无具名函数；说明文档。 |
| [docs/rag.md](../docs/rag.md) | 当前解析、模型空间检索和多轮流式问答合同。 | 无具名函数；说明文档。 |
| [docs/recommendation.md](../docs/recommendation.md) | 当前题族学情和每日规则计划。 | 无具名函数；说明文档。 |
| [docs/weknora-reference.md](../docs/weknora-reference.md) | 开发参考的思想和独立实现留痕。 | 无具名函数；说明文档。 |

<!-- FILE_INDEX_END -->

### 关键函数如何分工

| 函数/方法 | 具体职责 |
|---|---|
| Config.Load / AssistantStatus / EmbeddingSpace | 读取并校验配置 / 输出公开可用模式 / 计算模型空间标识 |
| DocumentService.Upload / Reprocess / Delete | 保存并入队 / 重新入队 / 按所有者删除原文件和关联数据 |
| RunDocumentJob / executionTx | 领取、续租、重试任务 / 校验执行代号与租约，阻止迟到执行写入 |
| processRevision / CanonicalPage / MappedChunks | 检查点流程 / 统一块文本和码点范围 / 将来源区域映射到分块 |
| BeginRevision / SaveRevisionPage / SaveRevisionVectors / ActivateRevision | 开始/复用版本 / 保存页 / 保存向量批次 / 原子启用完整版本 |
| Parser.ParsePageBatch / remoteBatch | 分页解析选择 / 以认证 multipart 请求独立解析服务 |
| ExtractQuestions / PrepareAsset / ValidateAsset | 规则抽取单选题 / 计算自动可用性 / 拒绝非法资源字段 |
| SearchChunks / evidenceRelated | 筛选所属用户及兼容向量并精确检索 / 保守词面相关性过滤 |
| Chat / ChatStream / AnswerStream / SaveTurn | RAG 同步/流式流程 / 模型 SSE 适配 / 完整问答对原子保存 |
| QuestionAI.Infer / SaveAIAnswer / ConfirmAnswer | 获取参考答案 / 保存 AI 结果 / 保存用户自评决定 |
| Grade / syncAssessmentEvents / Radar | 按来源区分判分 / 替换本次学情贡献、记录修改、重算 / 按题族最近有效结果统计四科 |
| Family / PlanCandidates / GenerateTasks | 确定题族 / 计算候选优先级和规则间隔 / 在每日预算内持久化任务 |
| QuestionGenerator.Generate / ReserveGeneration / CompleteGeneration | 验证依据并生成题 / 预留幂等与次数额度 / 保存生成题与结果 |
| renderAnswer / rotateBox / validBox / CitationReader.paint | 安全富文本 / 来源矩形旋转及校验 / 渲染 PDF 原页和证据框 |

文档中的版本、运行端口和验收记录均有日期。后续代码变更后应同步维护本索引和 CURRENT_FEATURES.md，避免将历史状态再次当作当前合同。
