# 当前数据模型

校准：2026-10-15。唯一权威结构为 backend/migrations 中顺序迁移；当前实现 1–8，本次只读核实正常库最大版本为 8。早期固定 vector(1536) 的描述已失效。

## 主要表与数据归属

| 范围 | 表 | 作用 |
|---|---|---|
| 身份 | users | 用户身份和密码哈希 |
| 知识库/原文 | knowledge_bases、documents | 用户知识库、原文路径、处理状态和 active_revision_id |
| 活动检索 | document_chunks | 当前启用分块与可变维度 vector；metadata 保存 embedding_space、版本与来源位置 |
| 任务 | document_jobs | queued/running/completed/failed、尝试次数、租约、执行代号 |
| 版本 | document_revisions、revision_pages、revision_chunks | 配置指纹、页/逻辑段、分块及向量检查点；未启用成果不进入正常检索 |
| 解析 | document_parses | 当前结构化解析快照，历史版本解析由 revision_pages 保留 |
| 教学结构 | exam408_subjects、exam408_knowledge_points、exam408_knowledge_point_relations | 四科、演示知识点和关系，尚非完整考纲 |
| 资源映射 | exam408_resource_mappings、exam408_entity_mappings | 文档/分块与知识点或语义实体的映射 |
| 语义图谱 | knowledge_entities、knowledge_relations | 已有结构，尚无完整自动抽取/查询产品流程 |
| 会话 | conversations、messages | 用户会话、问答与引用 JSON |
| 个人资源 | learning_assets | 单选题、资料和视频资源；来源、题族、答案依据等保存在 payload |
| 测评 | assessments | 题目快照与结果；不依赖题目当前版本回算旧试卷 |
| 判分账本 | assessment_result_changes | 追加保存结果变更，区分作答时间和后续确认 |
| 目标与任务 | learning_profiles、daily_plans、study_tasks | 目标问卷、按北京时间的每日预算、任务预计分钟与完成状态 |
| 生成请求 | question_generations | 练习生成幂等、额度与结果状态 |
| 学情 | exam408_learning_events、exam408_user_mastery | 有效结果事件与掌握度投影 |
| 推荐 | exam408_recommendations | 保留原推荐存储；生产 PostgreSQL 路径优先从每日任务投影推荐 |

按 user_id 或经所有者关联限定访问，不能信任客户端提供用户 ID。删除文档会删除相关版本/提取资源，既有测评题目快照保留；原文件访问还依赖 uploads 卷。数据库和文件卷须配套备份。

## 迁移顺序

| 版本 | 作用 |
|---|---|
| 000001 | 基础身份、知识库、图谱、学习和四科种子 |
| 000002 | 操作查询索引 |
| 000003 | 可变维度嵌入、模型空间，移除旧固定 HNSW 索引 |
| 000004 | 文档阶段、进度及批次数 |
| 000005 | 独立任务、会话、资源、测评和学习平台基础 |
| 000006 | 结构化解析、个人资源自动可用性和辅助判分 |
| 000007 | 文档版本、页/分块检查点、活动版本及任务租约/执行隔离 |
| 000008 | 每日计划、题族/判分账本、生成请求等学习计划增强 |

旧 ready 文档升级为 legacy 活动版本，保留旧结果；无来源映射的引用降级显示。不要修改已部署旧迁移来冒充升级。查询、恢复和迁移命令见 [使用指南](USER_AND_CODE_GUIDE.md)。
