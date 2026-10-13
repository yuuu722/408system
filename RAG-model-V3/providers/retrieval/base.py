"""
全文检索抽象接口
检索器返回统一的命中列表：
[{"id", "title", "content", "source", "subject", "page", "relevance"}]
"""
from typing import Optional


class BaseRetriever:
    """检索器基类"""

    name: str = "base"

    def __init__(self, config):
        self.config = config
        self.available: bool = False

    def search(self, query: str, top_k: int = 6, subject: Optional[str] = None) -> list:
        raise NotImplementedError

    def close(self):
        pass
