"""
BM25 全文检索（jieba 中文分词 + rank_bm25）
纯离线、无需模型、无需联网，适合中文考研语料。
"""
import json
from pathlib import Path

import jieba
from rank_bm25 import BM25Okapi

from .base import BaseRetriever

# 共享同义词（providers 与 retrieval 同级）
try:
    from providers.synonyms import QUERY_SYNONYMS
except Exception:
    import sys as _sys
    from pathlib import Path as _Path
    _sys.path.insert(0, str(_Path(__file__).resolve().parent.parent))
    from synonyms import QUERY_SYNONYMS

# 简单停用词
STOPWORDS = set(
    "的 了 和 是 与 及 或 在 为 对 中 以 等 一 个 上 也 就 都 而 被 把 这 那 "
    "什么 怎么 如何 吗 呢 啊 吧 请 问 一下 有 之 其 于 由 从 并 但 可 能".split()
)


def tokenize(text: str) -> list:
    tokens = [t.strip() for t in jieba.lcut(text) if t.strip()]
    return [t for t in tokens if t not in STOPWORDS and len(t) > 0]


def expand_tokens(tokens: list) -> list:
    """同义词扩展：保留原词，补充教材用词"""
    expanded = list(tokens)
    for t in tokens:
        for syn in QUERY_SYNONYMS.get(t, []):
            if syn not in expanded:
                expanded.append(syn)
        low = t.lower()
        for syn in QUERY_SYNONYMS.get(low, []):
            if syn not in expanded:
                expanded.append(syn)
    return expanded


class BM25Retriever(BaseRetriever):
    """BM25 全文检索器"""

    name = "bm25"

    def __init__(self, config):
        super().__init__(config)
        self.chunks = []
        self.bm25 = None
        self._load()

    def _load(self):
        path = Path(self.config.corpus_data_path) / "chunks.json"
        if not path.exists():
            print(f"[Retrieval] 语料不存在: {path}")
            return
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        self.chunks = data.get("chunks", [])
        jieba.setLogLevel(60)
        corpus_tokens = [tokenize(c["text"]) for c in self.chunks]
        self.bm25 = BM25Okapi(corpus_tokens)
        self.available = True
        print(f"[Retrieval] BM25 索引就绪: {len(self.chunks)} 块")

    def search(self, query: str, top_k: int = 6, subject: str = None) -> list:
        if not self.available:
            return []
        q_tokens = expand_tokens(tokenize(query))
        if not q_tokens:
            return []
        scores = self.bm25.get_scores(q_tokens)
        max_score = max(scores) if len(scores) else 0
        if max_score <= 0:
            return []

        indexed = list(enumerate(scores))
        if subject:
            indexed = [(i, s) for i, s in indexed if self.chunks[i]["subject"] == subject]
        indexed.sort(key=lambda x: x[1], reverse=True)

        results = []
        for idx, score in indexed[:top_k]:
            c = self.chunks[idx]
            relevance = round(score / max_score, 3)
            if relevance < 0.15:
                continue
            results.append({
                "id": c["id"],
                "title": self._make_title(c),
                "content": c["text"],
                "source": f"{c['source']} 第{c['page']}页",
                "subject": c["subject"],
                "page": c["page"],
                "relevance": relevance,
            })
        return results

    def _make_title(self, c: dict) -> str:
        first = c["text"].split("\n")[0]
        return first[:24]
