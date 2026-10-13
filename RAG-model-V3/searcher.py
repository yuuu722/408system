"""
知识检索模块（混合检索）
1. 图存储 Provider（Neo4j / 华为云 GES / 本地扩充图谱）：子图检索，保证结构化知识点与关系
2. 全文检索 Provider（BM25 / 向量预留）：从 87 万字语料中召回原文片段
两路结果融合、去重、重排，统一为 SearchResult。
"""
from dataclasses import dataclass, field
from typing import Optional

from config import Config
from gateway import Entity

# 延迟导入工厂（providers 与本模块同级）
try:
    from providers.graph.factory import create_graph_store
    from providers.retrieval.factory import create_retriever
except Exception:  # 兼容直接以脚本方式运行的场景
    import sys
    from pathlib import Path
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from providers.graph.factory import create_graph_store
    from providers.retrieval.factory import create_retriever


@dataclass
class EvidenceItem:
    """证据条目"""
    id: str
    title: str
    content: str
    source: str  # chapter / textbook / paper
    subject: str = ""
    relevance: float = 0.0


@dataclass
class GraphNode:
    """知识图谱节点"""
    id: str
    label: str
    type: str
    subject: str = ""


@dataclass
class GraphEdge:
    """知识图谱边"""
    source: str
    target: str
    relation: str


@dataclass
class SearchResult:
    """检索结果"""
    evidence: list[EvidenceItem] = field(default_factory=list)
    graph_nodes: list[GraphNode] = field(default_factory=list)
    graph_edges: list[GraphEdge] = field(default_factory=list)
    matched_entities: list[Entity] = field(default_factory=list)
    has_result: bool = False


class KnowledgeSearcher:
    """混合知识检索器：图谱检索 + 全文检索"""

    def __init__(self, config: Config):
        self.config = config
        self.graph_store = create_graph_store(config)
        print(f"[Searcher] 使用图存储: {self.graph_store.name}")
        self.retriever = create_retriever(config)
        print(f"[Searcher] 使用全文检索: {self.retriever.name} "
              f"(可用={self.retriever.available})")

    async def search(self, query: str, entities: list[Entity], intent: str) -> SearchResult:
        """执行混合检索"""
        result = SearchResult()
        entity_names = [e.name for e in entities]

        # ---- 1. 图谱子图检索（结构化知识点 + 关系） ----
        data = await self.graph_store.search_subgraph(entity_names, intent)

        for n in data.get("nodes", []):
            result.graph_nodes.append(GraphNode(
                id=n["id"], label=n["label"],
                type=n.get("type", "concept"),
                subject=n.get("subject", "")
            ))

        for e in data.get("edges", []):
            result.graph_edges.append(GraphEdge(
                source=e["source"], target=e["target"],
                relation=e.get("relation", "")
            ))

        for ev in data.get("evidence", []):
            result.evidence.append(EvidenceItem(
                id=ev["id"], title=ev["title"],
                content=ev["content"], source=ev.get("source", ""),
                subject=ev.get("subject", ""),
                relevance=ev.get("relevance", 0.85)
            ))
            matched = next(
                (e for e in entities if e.name in ev["title"] or ev["title"] in e.name),
                None
            )
            if matched:
                result.matched_entities.append(matched)

        # ---- 2. 全文检索（BM25 召回原文片段） ----
        if self.retriever.available:
            hits = self.retriever.search(
                query, top_k=self.config.retrieval_top_k
            )
            self._merge_evidence(result, hits)

        result.has_result = len(result.evidence) > 0
        return result

    def _merge_evidence(self, result: SearchResult, hits: list, max_add: int = 3):
        """融合全文检索命中：去重 + 相关性映射（图谱证据优先）"""
        existing = "\u0001".join(ev.content for ev in result.evidence)
        added = 0
        for h in hits:
            if added >= max_add:
                break
            key = h["content"][:30]
            if key in existing:
                continue
            # BM25 相关性映射到 0.5~0.8（图谱证据 0.85 在前）
            relevance = round(0.5 + 0.3 * h.get("relevance", 0.5), 3)
            result.evidence.append(EvidenceItem(
                id=h["id"], title=h["title"], content=h["content"],
                source=h.get("source", ""), subject=h.get("subject", ""),
                relevance=relevance
            ))
            existing += "\u0001" + h["content"]
            added += 1

    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        """获取知识图谱完整数据（委托当前图存储）"""
        return await self.graph_store.get_graph_data(subject=subject)

    def close(self):
        self.graph_store.close()
        self.retriever.close()
