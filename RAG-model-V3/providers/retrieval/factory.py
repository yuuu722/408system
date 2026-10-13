"""
全文检索器工厂
auto / bm25：BM25 离线检索；vector：向量检索（预留）；none：关闭全文检索
"""
from .base import BaseRetriever
from .bm25_retriever import BM25Retriever
from .vector_retriever import VectorRetriever


def create_retriever(config) -> BaseRetriever:
    provider = (config.retrieval_provider or "auto").lower().strip()

    if provider == "none":
        r = BaseRetriever(config)
        r.available = False
        return r

    if provider == "vector":
        r = VectorRetriever(config)
        if r.available:
            return r
        # 向量模型未就绪，降级到 BM25
        bm25 = BM25Retriever(config)
        if bm25.available:
            return bm25
        return r

    # auto / bm25
    bm25 = BM25Retriever(config)
    if bm25.available:
        return bm25
    return VectorRetriever(config)
