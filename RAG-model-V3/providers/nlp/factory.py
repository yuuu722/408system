"""
语义理解工厂
根据 config.nlp_provider 创建对应的 NLP Provider，外部服务不可用时自动降级。
"""
from .base import NLPProvider
from .keyword_nlp import KeywordNLP


def create_nlp_provider(config) -> NLPProvider:
    """创建 NLP Provider"""
    provider = (config.nlp_provider or "auto").lower().strip()

    if provider == "keyword":
        return KeywordNLP(config)

    if provider == "llm":
        from .llm_nlp import LLMNLP
        nlp = LLMNLP(config)
        if nlp.available:
            return nlp
        print("[NLP] LLM 不可用，降级到关键词")
        return KeywordNLP(config)

    if provider == "huawei_nlp":
        from .huawei_nlp import HuaweiNLP
        nlp = HuaweiNLP(config)
        if nlp.available:
            return nlp
        print("[NLP] 华为云 NLP 不可用，降级到关键词")
        return KeywordNLP(config)

    # auto：有 LLM 用 LLM，否则关键词
    from .llm_nlp import LLMNLP
    nlp = LLMNLP(config)
    if nlp.available:
        return nlp
    return KeywordNLP(config)
