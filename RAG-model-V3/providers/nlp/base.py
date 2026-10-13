"""
语义理解抽象接口
所有 NLP 实现返回统一的字典结构（由 gateway 组装为 IntentResult）：

{
  "intent": "definition",
  "subject": "ds" 或 None,
  "entities": [{"name", "type", "aliases": []}],
  "sub_intent": "",
  "confidence": 0.8
}
"""


class NLPProvider:
    """语义理解基类"""

    name: str = "base"

    def __init__(self, config):
        self.config = config
        self.available: bool = False

    async def analyze(self, query: str, history: list) -> dict:
        """意图识别 + 实体抽取"""
        raise NotImplementedError

    @staticmethod
    def default_result(query: str) -> dict:
        return {
            "intent": "definition",
            "subject": None,
            "entities": [{"name": query, "type": "concept", "aliases": []}],
            "sub_intent": "默认处理",
            "confidence": 0.3,
        }
