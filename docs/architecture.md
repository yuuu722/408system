# 当前系统架构

校准：2026-10-15。功能清单见 [CURRENT_FEATURES.md](CURRENT_FEATURES.md)，调用链和逐文件索引见 [USER_AND_CODE_GUIDE.md](USER_AND_CODE_GUIDE.md)。本文件替代早期“进程内任务”描述。

## 边界与组件

系统为模块化单体：Vue SPA、Go API、复用同一二进制的独立 Go worker、PostgreSQL/pgvector，以及宿主机嵌入/解析 HTTP 服务。Compose 管理 frontend、backend、worker、postgres、redis 五个服务；宿主机 Python 进程另启。

Redis 当前不是文档权威队列，任务、租约与检查点在 PostgreSQL。上传原文件位于 uploads 私有卷，用户、业务和向量数据位于 postgres-data；复制代码不迁移这些数据。模型文件单独提供。

~~~text
Vue -> /api/v1 -> httpapi -> service/domain -> repository -> PostgreSQL/pgvector
                          |-> 原文件卷
                          |-> 本地/兼容 Embedding HTTP
                          |-> 兼容 LLM / 可选 Vision HTTP
worker -> 持久化任务 -> 页解析 HTTP -> 检查点 -> 向量批次 -> 完整版本启用
~~~

## 文档流程

Upload/Reprocess 入队 → RunDocumentJob 以短事务领取 → 90 秒租约、20 秒续租、执行代号隔离 → processRevision 保存分页/逻辑段、分块和每批 4 个向量 → 全量结构抽题 → ActivateRevision 原子切换完整活动版本。

单次处理最长 30 分钟，暂时错误最多自动 3 次；兼容检查点支持恢复。重处理或失败时仍可检索旧活动版本，但必须匹配当前嵌入空间和维度。首次处理未完成没有可检索版本。进程内兼容路径仍在源码中供较小 Store/测试使用，不能视作正常 PostgreSQL 部署路径。

## 学习与问答流程

检索先按用户、知识库、模型空间和维度筛选，再精确计算余弦距离；当前不是 HNSW 检索。相关性不足拒答，流式完整回答保存为一个问答对，来源记录版本及区域供阅读器定位。

测评快照 → 资料答案或用户确认的有效结果 → 判分变更账本/事件 → 题族最近有效结果 → 掌握度与雷达。每日计划由规则候选、用户预算和可用资源生成；不训练模型，也不构成自主 Agent。

## 安全与部署

- 认证为 bcrypt + JWT HS256；JWT_SECRET 至少 32 个非占位字符。
- 知识库、文档、对话、题目、测评与计划按认证用户隔离；文件访问需校验所有权。
- API/worker 自动通过 advisory lock 顺序应用迁移 1–8。迁移 SQL 是数据库合同。
- 通用学习事件只接受被动行为；测评结果由可信后端生成。浏览、提问、任务完成不直接增加掌握度。
- 前端 Markdown/公式渲染进行过滤，模型输出外部图片不自动加载，来源矩形缺少可靠坐标时降级。
- API/worker 使用同一个配置及上传卷；nginx 代理 /api 和流式响应。部署到公网仍需补 TLS、备份策略、监控和网络隔离。
- WeKnora 仅为开发参考，运行不依赖其源码、服务或数据；通用库与模型来源保留。
