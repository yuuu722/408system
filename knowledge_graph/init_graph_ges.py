"""
GES 图引擎建图脚本
将 408 知识点样例（knowledge_points.json）通过 Cypher 批量写入华为云 GES。

使用前准备（在 GES 控制台完成）：
  1. 创建图（持久化版/内存版均可），获取「图访问地址 SERVER_URL」和图名称
  2. 在图元数据中创建 label「KnowledgePoint」，属性：
       name(string)、subject(string)、chapter(string)、definition(string)
  3. 构建 Cypher 所需索引（点/边 label 索引）：
       POST /ges/v1.0/{project_id}/graphs/{graph_name}/indices
       {"indexName":"cypher_vertex_index","indexType":"GlobalCompositeVertexIndex","hasLabel":"true","indexProperty":[]}
       {"indexName":"cypher_edge_index","indexType":"GlobalCompositeEdgeIndex","hasLabel":"true","indexProperty":[]}
  4. .env 中填好 HUAWEI_ACCESS_KEY / HUAWEI_SECRET_KEY / HUAWEI_PROJECT_ID
     以及 HUAWEI_GES_GRAPH_ID / HUAWEI_GES_ENDPOINT

运行：
    python init_graph_ges.py
"""
import json
import os
import sys
from pathlib import Path

# 定位配置
BACKEND_DIR = Path(__file__).resolve().parent.parent / "RAG-model-V3"
sys.path.insert(0, str(BACKEND_DIR))

from config import Config  # noqa: E402


def build_statements(kps: list) -> list:
    """根据知识点构造 Cypher 语句列表"""
    statements = []

    # 1. 建点（_ID_ 用稳定 ID；GES 不支持 merge，只能 create）
    for i, kp in enumerate(kps):
        kp_id = kp.get("id") or f"kp_{i:03d}"
        name = kp["name"].replace("'", "\\'")
        subject = kp.get("subject", "").replace("'", "\\'")
        chapter = kp.get("chapter", "").replace("'", "\\'")
        definition = kp.get("definition", "").replace("'", "\\'")
        cypher = (
            f"CREATE (n:KnowledgePoint{{_ID_:'{kp_id}', name:'{name}', "
            f"subject:'{subject}', chapter:'{chapter}', definition:'{definition}'}})"
        )
        statements.append(cypher)

    # 2. 建边（用 id() 匹配稳定 ID）
    for i, kp in enumerate(kps):
        src = kp.get("id") or f"kp_{i:03d}"
        for rel in kp.get("relations", []):
            tgt = rel.get("target", "")
            rtype = rel.get("type", "related")
            if not tgt:
                continue
            cypher = (
                f"MATCH (n:KnowledgePoint),(m:KnowledgePoint) "
                f"WHERE id(n)='{src}' AND id(m)='{tgt}' "
                f"CREATE (n)-[:{rtype}]->(m)"
            )
            statements.append(cypher)

    return statements


def execute(config: Config, statements: list, batch: int = 20):
    """分批调用 GES Cypher API"""
    import requests
    from huaweicloudsdkcore.signer.signer import Signer
    from huaweicloudsdkcore.auth.credentials import BasicCredentials

    project_id = config.hw_ges_project_id or config.hw_project_id
    server_url = config.hw_ges_endpoint.replace("https://", "").replace("http://", "").strip("/")
    graph = config.hw_ges_graph_id

    creds = BasicCredentials(config.hw_ak, config.hw_sk, project_id)
    signer = Signer(creds)

    url = (
        f"https://{server_url}/ges/v1.0/{project_id}/graphs/{graph}/action"
        f"?action_id=execute-cypher-query"
    )

    for start in range(0, len(statements), batch):
        chunk = statements[start:start + batch]
        body = json.dumps(
            {"statements": [{"statement": c, "resultDataContents": ["row"]} for c in chunk]},
            ensure_ascii=False
        ).encode("utf-8")
        req = requests.Request(
            "POST", url, data=body,
            headers={"Content-Type": "application/json", "Host": server_url.split(":")[0]}
        )
        req = signer.sign(req)
        with requests.Session() as s:
            resp = s.send(req, timeout=60)
        print(f"批次 {start//batch + 1}: HTTP {resp.status_code}")
        if resp.status_code != 200:
            print(resp.text[:500])
        else:
            errors = resp.json().get("errors", [])
            if errors:
                print("  errors:", errors)


def main():
    data_path = Path(__file__).resolve().parent.parent / "data" / "sample" / "knowledge_points.json"
    if not data_path.exists():
        print(f"未找到知识点文件: {data_path}")
        return

    with open(data_path, "r", encoding="utf-8") as f:
        kps = json.load(f)

    statements = build_statements(kps)
    print(f"共生成 {len(statements)} 条 Cypher（{len(kps)} 个点）")

    config = Config()
    if not config.hw_ak or not (config.hw_ges_project_id or config.hw_project_id):
        print("华为云凭证未配置，请先在 .env 填写，然后重新运行。")
        return

    execute(config, statements)
    print("GES 建图完成。")


if __name__ == "__main__":
    main()
