"""
知识检索模块
Neo4j 图查询 + 向量语义检索
"""
import json
from dataclasses import dataclass, field
from typing import Optional
from pathlib import Path

from config import Config
from gateway import Entity, IntentResult


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
    """知识检索器：图谱查询 + 向量检索"""

    def __init__(self, config: Config):
        self.config = config
        self.neo4j_driver = None
        self.sample_data = self._load_sample_data()

        # 尝试连接 Neo4j
        self._init_neo4j()

    def _init_neo4j(self):
        """初始化 Neo4j 连接"""
        try:
            from neo4j import GraphDatabase
            self.neo4j_driver = GraphDatabase.driver(
                self.config.neo4j_uri,
                auth=(self.config.neo4j_user, self.config.neo4j_password)
            )
            # 测试连接
            self.neo4j_driver.verify_connectivity()
            print("[Searcher] Neo4j 连接成功")
        except Exception as e:
            print(f"[Searcher] Neo4j 连接失败，使用示例数据: {e}")
            self.neo4j_driver = None

    def _load_sample_data(self) -> dict:
        """加载示例知识点数据"""
        sample_path = Path(self.config.sample_data_path) / "knowledge_points.json"
        if sample_path.exists():
            with open(sample_path, "r", encoding="utf-8") as f:
                return json.load(f)
        return self._default_sample_data()

    def _default_sample_data(self) -> dict:
        """默认示例数据"""
        return {
            "subjects": {
                "ds": "数据结构",
                "co": "计算机组成原理",
                "os": "操作系统",
                "cn": "计算机网络"
            },
            "knowledge_points": [
                {
                    "id": "ds_array",
                    "name": "数组",
                    "subject": "ds",
                    "chapter": "第一章 线性表",
                    "definition": "数组是按一定顺序排列的相同类型元素的集合，在内存中占据连续的存储空间。",
                    "properties": "随机访问O(1)，插入删除O(n)，空间连续",
                    "relations": [
                        {"target": "ds_linkedlist", "type": "对比"},
                        {"target": "ds_stack", "type": "前驱"}
                    ]
                },
                {
                    "id": "ds_linkedlist",
                    "name": "链表",
                    "subject": "ds",
                    "chapter": "第一章 线性表",
                    "definition": "链表是通过指针将一组零散的内存块串联起来使用的数据结构。",
                    "properties": "插入删除O(1)，随机访问O(n)，空间不连续",
                    "relations": [
                        {"target": "ds_array", "type": "对比"},
                        {"target": "ds_stack", "type": "前驱"}
                    ]
                },
                {
                    "id": "ds_stack",
                    "name": "栈",
                    "subject": "ds",
                    "chapter": "第二章 栈和队列",
                    "definition": "栈是只允许在一端进行插入或删除操作的线性表，遵循后进先出（LIFO）原则。",
                    "properties": "后进先出，栈顶操作O(1)",
                    "relations": [
                        {"target": "ds_queue", "type": "对比"},
                        {"target": "ds_array", "type": "实现"}
                    ]
                },
                {
                    "id": "ds_queue",
                    "name": "队列",
                    "subject": "ds",
                    "chapter": "第二章 栈和队列",
                    "definition": "队列是只允许在一端进行插入，在另一端进行删除的线性表，遵循先进先出（FIFO）原则。",
                    "properties": "先进先出，队头队尾操作O(1)",
                    "relations": [
                        {"target": "ds_stack", "type": "对比"}
                    ]
                },
                {
                    "id": "os_process",
                    "name": "进程",
                    "subject": "os",
                    "chapter": "第二章 进程管理",
                    "definition": "进程是程序的一次执行过程，是系统进行资源分配和调度的基本单位。",
                    "properties": "动态性、并发性、独立性、异步性",
                    "relations": [
                        {"target": "os_thread", "type": "对比"},
                        {"target": "os_schedule", "type": "关联"}
                    ]
                }
            ]
        }

    async def search(self, query: str, entities: list[Entity], intent: str) -> SearchResult:
        """执行检索"""
        result = SearchResult()

        # 如果有 Neo4j，从图谱查询
        if self.neo4j_driver:
            await self._search_from_neo4j(query, entities, result)
        else:
            # 否则从示例数据查询
            self._search_from_sample(query, entities, result)

        result.has_result = len(result.evidence) > 0
        return result

    async def _search_from_neo4j(self, query: str, entities: list[Entity], result: SearchResult):
        """从 Neo4j 图谱检索"""
        if not entities:
            return

        try:
            with self.neo4j_driver.session(database=self.config.neo4j_database) as session:
                for entity in entities:
                    # 查询节点
                    cypher = """
                    MATCH (n:KnowledgePoint)
                    WHERE n.name CONTAINS $name OR n.aliases CONTAINS $name
                    OPTIONAL MATCH (n)-[r]-(m)
                    RETURN n, type(r) as rel_type, m
                    LIMIT 20
                    """
                    records = session.run(cypher, name=entity.name)

                    for record in records:
                        node = record["n"]
                        result.graph_nodes.append(GraphNode(
                            id=node.element_id,
                            label=node.get("name", ""),
                            type=node.get("type", "concept"),
                            subject=node.get("subject", "")
                        ))

                        if record["m"]:
                            result.graph_nodes.append(GraphNode(
                                id=record["m"].element_id,
                                label=record["m"].get("name", ""),
                                type=record["m"].get("type", "concept"),
                                subject=record["m"].get("subject", "")
                            ))
                            result.graph_edges.append(GraphEdge(
                                source=node.element_id,
                                target=record["m"].element_id,
                                relation=record["rel_type"]
                            ))

                        # 添加证据
                        result.evidence.append(EvidenceItem(
                            id=node.element_id,
                            title=node.get("name", ""),
                            content=node.get("definition", ""),
                            source=node.get("chapter", ""),
                            subject=node.get("subject", ""),
                            relevance=0.9
                        ))

        except Exception as e:
            print(f"[Searcher] Neo4j 查询失败: {e}")

    def _search_from_sample(self, query: str, entities: list[Entity], result: SearchResult):
        """从示例数据检索（降级方案）"""
        kps = self.sample_data.get("knowledge_points", [])

        # 匹配实体
        for entity in entities:
            for kp in kps:
                if self._match_entity(entity.name, kp):
                    result.matched_entities.append(entity)
                    result.evidence.append(EvidenceItem(
                        id=kp["id"],
                        title=kp["name"],
                        content=f"{kp['definition']}\n\n特性：{kp.get('properties', '')}",
                        source=kp.get("chapter", ""),
                        subject=kp.get("subject", ""),
                        relevance=0.85
                    ))

                    # 添加图谱节点和边
                    result.graph_nodes.append(GraphNode(
                        id=kp["id"],
                        label=kp["name"],
                        type="concept",
                        subject=kp.get("subject", "")
                    ))

                    for rel in kp.get("relations", []):
                        target_kp = next((k for k in kps if k["id"] == rel["target"]), None)
                        if target_kp:
                            result.graph_nodes.append(GraphNode(
                                id=target_kp["id"],
                                label=target_kp["name"],
                                type="concept",
                                subject=target_kp.get("subject", "")
                            ))
                            result.graph_edges.append(GraphEdge(
                                source=kp["id"],
                                target=target_kp["id"],
                                relation=rel["type"]
                            ))

    def _match_entity(self, entity_name: str, kp: dict) -> bool:
        """匹配实体和知识点"""
        name = entity_name.lower()
        if name in kp["name"].lower():
            return True
        if "alias" in kp:
            for alias in kp["alias"]:
                if name in alias.lower():
                    return True
        return False

    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        """获取知识图谱完整数据"""
        nodes = []
        edges = []

        kps = self.sample_data.get("knowledge_points", [])
        for kp in kps:
            if subject and kp.get("subject") != subject:
                continue
            nodes.append({
                "id": kp["id"],
                "label": kp["name"],
                "type": "concept",
                "subject": kp.get("subject", "")
            })
            for rel in kp.get("relations", []):
                edges.append({
                    "source": kp["id"],
                    "target": rel["target"],
                    "relation": rel["type"]
                })

        return {"nodes": nodes, "edges": edges}
