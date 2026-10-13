"""
向量检索（预留）
部署 embedding 模型（本地 bge / 华为云 MaaS embedding）后启用。
当前未部署模型，标记为不可用，由工厂自动降级到 BM25。
"""
from .base import BaseRetriever


class VectorRetriever(BaseRetriever):
    """向量检索器（预留）"""

    name = "vector"

    def __init__(self, config):
        super().__init__(config)
        # TODO: 加载本地 bge 模型或接入华为云 MaaS embedding
        self.available = False

    def search(self, query: str, top_k: int = 6, subject: str = None) -> list:
        return []
