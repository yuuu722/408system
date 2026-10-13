"""
语义网关模块
通过 NLP Provider（LLM / 华为云 NLP / 关键词）完成意图识别和实体提取。
"""
from dataclasses import dataclass, field
from typing import Optional

from config import Config

# 延迟导入 NLP 工厂
try:
    from providers.nlp.factory import create_nlp_provider
except Exception:
    import sys
    from pathlib import Path
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from providers.nlp.factory import create_nlp_provider


@dataclass
class Entity:
    """提取的实体"""
    name: str
    type: str  # concept / chapter / subject / algorithm / structure
    aliases: list[str] = field(default_factory=list)


@dataclass
class IntentResult:
    """意图识别结果"""
    intent: str  # definition / comparison / calculation / principle / relationship / greeting
    entities: list[Entity] = field(default_factory=list)
    subject: Optional[str] = None  # ds / co / os / cn
    sub_intent: str = ""
    confidence: float = 0.0


class SemanticGateway:
    """语义网关：委托 NLP Provider 完成意图识别 + 实体提取"""

    def __init__(self, config: Config):
        self.config = config
        self.nlp_provider = create_nlp_provider(config)
        print(f"[Gateway] 使用语义理解: {self.nlp_provider.name}")

    async def analyze(self, query: str, history: list[dict]) -> IntentResult:
        """分析用户问题"""
        # 简单问候直接返回
        if self._is_greeting(query):
            return IntentResult(
                intent="greeting",
                entities=[],
                sub_intent="用户问候",
                confidence=0.95
            )

        # 委托 NLP Provider
        data = await self.nlp_provider.analyze(query, history)

        entities = [
            Entity(
                name=e.get("name", ""),
                type=e.get("type", "concept"),
                aliases=e.get("aliases", [])
            )
            for e in data.get("entities", [])
        ]

        return IntentResult(
            intent=data.get("intent", "definition"),
            entities=entities,
            subject=data.get("subject"),
            sub_intent=data.get("sub_intent", ""),
            confidence=float(data.get("confidence", 0.5) or 0.5)
        )

    def _is_greeting(self, query: str) -> bool:
        """判断是否为问候"""
        greetings = ["你好", "您好", "hi", "hello", "在吗", "早上好", "下午好", "晚上好"]
        query_lower = query.lower().strip()
        for g in greetings:
            if g in query_lower and len(query) < 20:
                return True
        return False
