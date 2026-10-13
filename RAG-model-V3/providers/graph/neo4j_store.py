"""
Neo4j 图存储
封装 Neo4j 连接与 Cypher 查询，兼容现有图谱结构
"""
from typing import Optional

from .base import GraphStore


class Neo4jGraphStore(GraphStore):
    """Neo4j 图存储"""

    name = "neo4j"

    def __init__(self, config):
        super().__init__()
        self.config = config
        self.driver = None
        self._connect()

    def _connect(self):
        """初始化 Neo4j 连接，失败则 available=False"""
        try:
            from neo4j import GraphDatabase
            self.driver = GraphDatabase.driver(
                self.config.neo4j_uri,
                auth=(self.config.neo4j_user, self.config.neo4j_password)
            )
            self.driver.verify_connectivity()
            self.available = True
            print("[GraphStore] Neo4j 连接成功")
        except Exception as e:
            print(f"[GraphStore] Neo4j 连接失败: {e}")
            self.driver = None
            self.available = False

    async def search_subgraph(self, entity_names: list[str], intent: str = "") -> dict:
        result = self.empty_result()
        if not self.available or not entity_names:
            return result

        cypher = """
        MATCH (n:KnowledgePoint)
        WHERE n.name CONTAINS $name OR n.aliases CONTAINS $name
        OPTIONAL MATCH (n)-[r]-(m)
        RETURN n, type(r) as rel_type, m
        LIMIT 20
        """
        seen_nodes = set()
        seen_evidence = set()

        try:
            with self.driver.session(database=self.config.neo4j_database) as session:
                for name in entity_names:
                    records = session.run(cypher, name=name)
                    for record in records:
                        n = record["n"]
                        self._add_node(n, result, seen_nodes)
                        if n.element_id not in seen_evidence:
                            seen_evidence.add(n.element_id)
                            result["evidence"].append({
                                "id": n.element_id,
                                "title": n.get("name", ""),
                                "content": n.get("definition", ""),
                                "source": n.get("chapter", ""),
                                "subject": n.get("subject", ""),
                                "relevance": 0.9
                            })
                        m = record["m"]
                        if m:
                            self._add_node(m, result, seen_nodes)
                            edge_key = (n.element_id, m.element_id, record["rel_type"])
                            if edge_key not in seen_nodes:
                                result["edges"].append({
                                    "source": n.element_id,
                                    "target": m.element_id,
                                    "relation": record["rel_type"]
                                })
        except Exception as e:
            print(f"[GraphStore] Neo4j 查询失败: {e}")

        return result

    def _add_node(self, node, result: dict, seen: set):
        if node.element_id in seen:
            return
        seen.add(node.element_id)
        result["nodes"].append({
            "id": node.element_id,
            "label": node.get("name", ""),
            "type": node.get("type", "concept"),
            "subject": node.get("subject", "")
        })

    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        result = self.empty_result()
        if not self.available:
            return result

        cypher = """
        MATCH (n:KnowledgePoint)
        OPTIONAL MATCH (n)-[r]->(m:KnowledgePoint)
        WHERE ($subject IS NULL OR n.subject = $subject)
        RETURN n, r, m
        """
        seen_nodes = set()
        try:
            with self.driver.session(database=self.config.neo4j_database) as session:
                records = session.run(cypher, subject=subject)
                for record in records:
                    n = record["n"]
                    self._add_node(n, result, seen_nodes)
                    if record["r"] and record["m"]:
                        self._add_node(record["m"], result, seen_nodes)
                        result["edges"].append({
                            "source": n.element_id,
                            "target": record["m"].element_id,
                            "relation": record["r"].type
                        })
        except Exception as e:
            print(f"[GraphStore] Neo4j 全图查询失败: {e}")
        return result

    def close(self):
        if self.driver:
            self.driver.close()
