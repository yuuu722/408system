"""
华为云 GES 图引擎存储
通过 GES 业务面「Cypher 操作 API」查询，使用 IAM AK/SK 签名。

依赖（可选，仅在使用 GES 时安装）：
    pip install huaweicloudsdkcore requests

Cypher 操作 API：
    POST {SERVER_URL}/ges/v1.0/{project_id}/graphs/{graph_name}/action?action_id=execute-cypher-query
    body: {"statements": [{"statement": "...", "resultDataContents": ["row"]}]}
    返回: {"results": [{"columns": [...], "data": [{"row": [...]}]}], "errors": []}

SERVER_URL 为图的访问地址（公网/内网访问地址），从 GES 控制台图详情获取，
通过 HUAWEI_GES_ENDPOINT 配置。图中 label/属性名需与 Neo4j 建图保持一致。
"""
import json
from typing import Optional

from .base import GraphStore


class GESGraphStore(GraphStore):
    """华为云 GES 图存储"""

    name = "huawei_ges"

    def __init__(self, config):
        super().__init__()
        self.config = config
        self.project_id = config.hw_ges_project_id or config.hw_project_id
        self.graph_id = config.hw_ges_graph_id
        self.server_url = (config.hw_ges_endpoint or "").replace("https://", "").replace("http://", "").strip("/")
        self._signer = None
        self._connect()

    # ---------- 初始化 ----------
    def _connect(self):
        missing = []
        if not self.config.hw_ak:
            missing.append("HUAWEI_ACCESS_KEY")
        if not self.config.hw_sk:
            missing.append("HUAWEI_SECRET_KEY")
        if not self.project_id:
            missing.append("HUAWEI_PROJECT_ID")
        if not self.graph_id:
            missing.append("HUAWEI_GES_GRAPH_ID")
        if not self.server_url:
            missing.append("HUAWEI_GES_ENDPOINT(图访问地址)")
        if missing:
            print(f"[GraphStore] GES 不可用，缺少: {', '.join(missing)}")
            return

        try:
            from huaweicloudsdkcore.signer.signer import Signer
            from huaweicloudsdkcore.auth.credentials import BasicCredentials
            creds = BasicCredentials(self.config.hw_ak, self.config.hw_sk, self.project_id)
            self._signer = Signer(creds)
            self.available = True
            print("[GraphStore] 华为云 GES 客户端就绪")
        except Exception as e:
            print(f"[GraphStore] 华为云签名组件初始化失败（请 pip install huaweicloudsdkcore）: {e}")

    # ---------- 底层请求 ----------
    def _cypher_query(self, cypher: str) -> list:
        """执行 Cypher，返回按列名建立的 dict 列表"""
        import requests

        url = (
            f"https://{self.server_url}/ges/v1.0/{self.project_id}/graphs/"
            f"{self.graph_id}/action?action_id=execute-cypher-query"
        )
        payload = {"statements": [{"statement": cypher, "resultDataContents": ["row"]}]}
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")

        req = requests.Request(
            "POST", url, data=body,
            headers={"Content-Type": "application/json", "Host": self.server_url.split(":")[0]}
        )
        req = self._signer.sign(req)
        with requests.Session() as s:
            resp = s.send(req, timeout=30)

        if resp.status_code != 200:
            print(f"[GraphStore] GES HTTP {resp.status_code}: {resp.text[:500]}")
            return []

        data = resp.json()
        for err in data.get("errors", []) or []:
            print(f"[GraphStore] GES 错误: {err}")

        rows = []
        for block in data.get("results", []) or []:
            columns = block.get("columns", [])
            for item in block.get("data", []) or []:
                rows.append(dict(zip(columns, item.get("row", []))))
        return rows

    @staticmethod
    def _escape(text: str) -> str:
        return (text or "").replace("\\", "\\\\").replace("'", "\\'")

    # ---------- 检索 ----------
    async def search_subgraph(self, entity_names: list[str], intent: str = "") -> dict:
        result = self.empty_result()
        if not self.available:
            return result

        seen_nodes = set()
        seen_evidence = set()
        seen_edges = set()

        for name in entity_names:
            safe = self._escape(name)
            cypher = (
                "MATCH (n:KnowledgePoint) "
                f"WHERE n.name CONTAINS '{safe}' "
                "OPTIONAL MATCH (n)-[r]-(m) "
                "RETURN id(n) AS n_id, n.name AS n_name, n.definition AS n_def, "
                "n.subject AS n_sub, n.chapter AS n_ch, type(r) AS r_type, "
                "id(m) AS m_id, m.name AS m_name, m.definition AS m_def, "
                "m.subject AS m_sub, m.chapter AS m_ch LIMIT 20"
            )
            for d in self._cypher_query(cypher):
                n_id = str(d.get("n_id"))
                if n_id and n_id not in seen_nodes:
                    seen_nodes.add(n_id)
                    result["nodes"].append({
                        "id": n_id, "label": d.get("n_name") or n_id,
                        "type": "concept", "subject": d.get("n_sub") or ""
                    })
                if n_id and n_id not in seen_evidence:
                    seen_evidence.add(n_id)
                    result["evidence"].append({
                        "id": n_id, "title": d.get("n_name", ""),
                        "content": d.get("n_def", ""),
                        "source": d.get("n_ch", ""),
                        "subject": d.get("n_sub", ""), "relevance": 0.9
                    })

                m_id = d.get("m_id")
                if m_id is not None and n_id:
                    m_id = str(m_id)
                    if m_id not in seen_nodes:
                        seen_nodes.add(m_id)
                        result["nodes"].append({
                            "id": m_id, "label": d.get("m_name") or m_id,
                            "type": "concept", "subject": d.get("m_sub") or ""
                        })
                    edge = (n_id, m_id, d.get("r_type") or "")
                    if edge not in seen_edges:
                        seen_edges.add(edge)
                        result["edges"].append({
                            "source": n_id, "target": m_id,
                            "relation": d.get("r_type") or ""
                        })
        return result

    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        result = self.empty_result()
        if not self.available:
            return result

        where = f"WHERE n.subject = '{self._escape(subject)}' " if subject else ""
        cypher = (
            "MATCH (n:KnowledgePoint) "
            "OPTIONAL MATCH (n)-[r]->(m:KnowledgePoint) "
            f"{where}"
            "RETURN id(n) AS n_id, n.name AS n_name, n.subject AS n_sub, "
            "type(r) AS r_type, id(m) AS m_id, m.name AS m_name, m.subject AS m_sub"
        )
        seen_nodes = set()
        seen_edges = set()
        for d in self._cypher_query(cypher):
            n_id = str(d.get("n_id"))
            if n_id and n_id not in seen_nodes:
                seen_nodes.add(n_id)
                result["nodes"].append({
                    "id": n_id, "label": d.get("n_name") or n_id,
                    "type": "concept", "subject": d.get("n_sub") or ""
                })
            m_id = d.get("m_id")
            if m_id is not None and n_id:
                m_id = str(m_id)
                if m_id not in seen_nodes:
                    seen_nodes.add(m_id)
                    result["nodes"].append({
                        "id": m_id, "label": d.get("m_name") or m_id,
                        "type": "concept", "subject": d.get("m_sub") or ""
                    })
                edge = (n_id, m_id, d.get("r_type") or "")
                if edge not in seen_edges:
                    seen_edges.add(edge)
                    result["edges"].append({
                        "source": n_id, "target": m_id,
                        "relation": d.get("r_type") or ""
                    })
        return result

    def close(self):
        pass
