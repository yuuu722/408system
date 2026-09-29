"""
知识图谱初始化模块
创建 Neo4j 索引、导入示例知识点数据
"""
import json
from pathlib import Path
from typing import Optional

# 项目根目录
PROJECT_ROOT = Path(__file__).resolve().parent.parent


class GraphInitializer:
    """知识图谱初始化器"""

    def __init__(self, uri: str, user: str, password: str, database: str = "neo4j"):
        self.uri = uri
        self.user = user
        self.password = password
        self.database = database
        self.driver = None

    def connect(self):
        """连接 Neo4j"""
        from neo4j import GraphDatabase
        self.driver = GraphDatabase.driver(self.uri, auth=(self.user, self.password))
        self.driver.verify_connectivity()
        print("[KG Init] Neo4j 连接成功")

    def close(self):
        """关闭连接"""
        if self.driver:
            self.driver.close()

    def create_constraints(self):
        """创建约束和索引"""
        statements = [
            "CREATE CONSTRAINT knowledge_point_id IF NOT EXISTS FOR (n:KnowledgePoint) REQUIRE n.id IS UNIQUE",
            "CREATE CONSTRAINT chapter_id IF NOT EXISTS FOR (n:Chapter) REQUIRE n.id IS UNIQUE",
            "CREATE CONSTRAINT subject_id IF NOT EXISTS FOR (n:Subject) REQUIRE n.id IS UNIQUE",
            "CREATE INDEX kp_name IF NOT EXISTS FOR (n:KnowledgePoint) ON (n.name)",
            "CREATE INDEX kp_subject IF NOT EXISTS FOR (n:KnowledgePoint) ON (n.subject)",
        ]

        with self.driver.session(database=self.database) as session:
            for stmt in statements:
                session.run(stmt)
        print("[KG Init] 约束和索引创建完成")

    def import_subjects(self):
        """导入科目"""
        subjects = [
            {"id": "ds", "name": "数据结构", "code": "DS"},
            {"id": "co", "name": "计算机组成原理", "code": "CO"},
            {"id": "os", "name": "操作系统", "code": "OS"},
            {"id": "cn", "name": "计算机网络", "code": "CN"},
        ]

        cypher = """
        MERGE (s:Subject {id: $id})
        SET s.name = $name, s.code = $code
        """

        with self.driver.session(database=self.database) as session:
            for s in subjects:
                session.run(cypher, **s)
        print(f"[KG Init] 导入 {len(subjects)} 个科目")

    def import_sample_knowledge(self):
        """导入示例知识点"""
        sample_path = PROJECT_ROOT / "data" / "sample" / "knowledge_points.json"

        if not sample_path.exists():
            print(f"[KG Init] 示例数据文件不存在: {sample_path}")
            return

        with open(sample_path, "r", encoding="utf-8") as f:
            data = json.load(f)

        kps = data.get("knowledge_points", [])

        # 导入知识点节点
        node_cypher = """
        MERGE (kp:KnowledgePoint {id: $id})
        SET kp.name = $name,
            kp.subject = $subject,
            kp.chapter = $chapter,
            kp.definition = $definition,
            kp.properties = $properties,
            kp.type = "concept"
        """

        with self.driver.session(database=self.database) as session:
            for kp in kps:
                session.run(node_cypher, **kp)

            # 导入关系
            for kp in kps:
                for rel in kp.get("relations", []):
                    rel_cypher = """
                    MATCH (a:KnowledgePoint {id: $from_id}), (b:KnowledgePoint {id: $to_id})
                    MERGE (a)-[r:RELATES {type: $rel_type}]->(b)
                    """
                    session.run(rel_cypher, {
                        "from_id": kp["id"],
                        "to_id": rel["target"],
                        "rel_type": rel["type"]
                    })

        print(f"[KG Init] 导入 {len(kps)} 个知识点")

    def get_stats(self) -> dict:
        """获取图谱统计信息"""
        stats = {}
        with self.driver.session(database=self.database) as session:
            result = session.run("MATCH (n) RETURN labels(n) AS label, count(n) AS count")
            for record in result:
                label = record["label"][0] if record["label"] else "Unknown"
                stats[label] = record["count"]

            result = session.run("MATCH ()-[r]->() RETURN count(r) AS count")
            stats["relationships"] = result.single()["count"]

        return stats


def main():
    """主函数"""
    import os
    from dotenv import load_dotenv

    env_path = PROJECT_ROOT / ".env"
    if env_path.exists():
        load_dotenv(env_path)

    initializer = GraphInitializer(
        uri=os.getenv("NEO4J_URI", "bolt://localhost:7687"),
        user=os.getenv("NEO4J_USER", "neo4j"),
        password=os.getenv("NEO4J_PASSWORD", ""),
        database=os.getenv("NEO4J_DATABASE", "neo4j")
    )

    try:
        initializer.connect()
        initializer.create_constraints()
        initializer.import_subjects()
        initializer.import_sample_knowledge()

        stats = initializer.get_stats()
        print(f"[KG Init] 图谱统计: {stats}")
        print("[KG Init] 初始化完成！")

    except Exception as e:
        print(f"[KG Init] 初始化失败: {e}")
    finally:
        initializer.close()


if __name__ == "__main__":
    main()
