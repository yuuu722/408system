# Knowledge graph

> 2026-10-15 校准：本文件描述教学/语义图谱的数据边界，不表示已经完成自动图谱抽取、查询和可视化产品。当前仍以关键词检测及资源映射为主；现有学习计划和练习生成见 [当前功能](CURRENT_FEATURES.md)，不依赖完整图谱管线。

The system deliberately separates two graphs.

The teaching graph is curated: four `exam408_subjects`, tree-shaped `exam408_knowledge_points`, and typed `exam408_knowledge_point_relations`. It is global curriculum data. User state never mutates this graph; it is joined to per-user mastery and events.

The semantic graph is user-owned: `knowledge_entities` and `knowledge_relations`, with optional evidence chunks. `exam408_entity_mappings` connects extracted concepts to the teaching graph. `exam408_resource_mappings` already supports KnowledgePoint-to-Document and KnowledgePoint-to-Chunk links; phase-one API exposes the document mapping path.

`KnowledgeService` currently performs deterministic keyword detection against curated names and keywords. RAG questions use both the question and retrieved evidence, then write one `rag_question` event per detected point. LLM entity/relation extraction is intentionally not enabled yet: the schema and evidence contract are present, but automatic graph writes should wait for validation, deduplication, and replay-safe background tasks.

