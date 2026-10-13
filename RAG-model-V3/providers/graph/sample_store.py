"""
本地图存储（默认方案）
读取 data/knowledge/knowledge_points.json（4 Word 构建的扩充图谱），
回退到 data/sample/knowledge_points.json。无需任何外部服务。

检索：实体同义词扩展 + 匹配打分（精确 > 别名 > 子串）+ 排序，避免宽泛误召回。
"""
import json
from pathlib import Path
from typing import Optional

from .base import GraphStore

# 共享实体扩展
try:
    from providers.synonyms import expand_entity
except Exception:
    import sys as _sys
    _sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
    from synonyms import expand_entity


class SampleGraphStore(GraphStore):
    """本地图存储"""

    name = "sample"

    def __init__(self, config):
        super().__init__()
        self.config = config
        self.data = self._load_data(config)
        self.available = True
        self._node_index = {}
        for kp in self.data.get("knowledge_points", []):
            self._node_index[kp["id"]] = kp

    # ---------- 数据加载 ----------
    def _load_data(self, config) -> dict:
        # 优先扩充版图谱
        kg_path = Path(config.knowledge_data_path) / "knowledge_points.json"
        if kg_path.exists():
            with open(kg_path, "r", encoding="utf-8") as f:
                return json.load(f)
        # 回退示例数据
        sample_path = Path(config.sample_data_path) / "knowledge_points.json"
        if sample_path.exists():
            with open(sample_path, "r", encoding="utf-8") as f:
                return json.load(f)
        return self._default_data()

    def _default_data(self) -> dict:
        return {
            "subjects": {
                "ds": "数据结构", "co": "计算机组成原理",
                "os": "操作系统", "cn": "计算机网络"
            },
            "knowledge_points": []
        }

    # ---------- 检索 ----------
    async def search_subgraph(self, entity_names: list[str], intent: str = "") -> dict:
        result = self.empty_result()
        kps = self.data.get("knowledge_points", [])
        scored = {}  # kp_id -> best_score

        # 1. 构建待匹配词（原实体 + 同义词扩展）
        terms = []
        for name in entity_names:
            name = name.strip()
            if not name:
                continue
            terms.append((name, 1.0))
            for syn in expand_entity(name):
                terms.append((syn, 0.85))

        # 2. 匹配打分
        for term, base in terms:
            tlow = term.lower()
            for kp in kps:
                s = self._match_score(tlow, kp)
                if s > 0:
                    total = base * s
                    if kp["id"] not in scored or total > scored[kp["id"]]:
                        scored[kp["id"]] = total

        # 3. 排序取 top
        ranked = sorted(scored.items(), key=lambda x: x[1], reverse=True)
        top_ids = [kid for kid, s in ranked if s >= 0.5][:6]

        seen_nodes = set()
        for kid in top_ids:
            kp = self._node_index[kid]
            self._add_node(kp, result, seen_nodes)

            content = kp.get("definition", "")
            props = kp.get("properties", "")
            if props:
                content = f"{content}\n\n特性：{props}"
            result["evidence"].append({
                "id": kp["id"],
                "title": kp["name"],
                "content": content,
                "source": kp.get("section") or kp.get("chapter", ""),
                "subject": kp.get("subject", ""),
                "relevance": round(scored[kid], 3),
            })

            # 关联节点（1 跳，只加节点与边，丰富图谱展示）
            for rel in kp.get("relations", []):
                target = self._node_index.get(rel["target"])
                if target:
                    self._add_node(target, result, seen_nodes)
                    edge = {"source": kid, "target": rel["target"], "relation": rel["type"]}
                    if edge not in result["edges"]:
                        result["edges"].append(edge)

        return result

    def _add_node(self, kp: dict, result: dict, seen: set):
        if kp["id"] in seen:
            return
        seen.add(kp["id"])
        result["nodes"].append({
            "id": kp["id"], "label": kp["name"],
            "type": "concept", "subject": kp.get("subject", "")
        })

    # ---------- 整图 ----------
    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        result = self.empty_result()
        seen = set()
        for kp in self.data.get("knowledge_points", []):
            if subject and kp.get("subject") != subject:
                continue
            self._add_node(kp, result, seen)
            for rel in kp.get("relations", []):
                if self._node_index.get(rel["target"]):
                    result["edges"].append({
                        "source": kp["id"], "target": rel["target"],
                        "relation": rel["type"]
                    })
        return result

    # ---------- 匹配打分 ----------
    def _match_score(self, tlow: str, kp: dict) -> float:
        name = kp["name"].lower()
        if tlow == name:
            return 1.0
        for alias in kp.get("aliases", []):
            if tlow == alias.lower():
                return 0.95
        # 子串匹配（要求至少 2 字，避免单字误命中）
        if len(tlow) >= 2:
            if tlow in name:
                return 0.65
            if len(name) >= 2 and name in tlow:
                return 0.6
        return 0
