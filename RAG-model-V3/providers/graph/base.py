"""
图存储抽象接口
所有图存储实现（Neo4j / 华为云 GES / 示例数据）返回统一的字典结构：

{
  "nodes": [{"id", "label", "type", "subject"}],
  "edges": [{"source", "target", "relation"}],
  "evidence": [{"id", "title", "content", "source", "subject", "relevance"}],
}
"""
from typing import Optional


class GraphStore:
    """图存储基类"""

    name: str = "base"

    def __init__(self):
        self.available: bool = False

    async def search_subgraph(self, entity_names: list[str], intent: str = "") -> dict:
        """
        根据实体名检索子图（节点 + 边 + 证据）
        :param entity_names: 实体名称列表
        :param intent: 意图类型
        """
        raise NotImplementedError

    async def get_graph_data(self, subject: Optional[str] = None) -> dict:
        """获取整张图（可按科目过滤）"""
        raise NotImplementedError

    def close(self):
        """释放连接"""
        pass

    @staticmethod
    def empty_result() -> dict:
        return {"nodes": [], "edges": [], "evidence": []}
