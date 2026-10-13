"""
BM25 关键词检索（轻量自实现，中文用 jieba 分词）
作为向量检索的补充与降级：向量模型不可用时单独使用，正常时与向量结果融合。
"""
import math
import pickle
from collections import Counter
from pathlib import Path


STOPWORDS = {
    "的", "了", "是", "在", "和", "与", "或", "等", "也", "都", "就", "及", "为",
    "对", "由", "从", "到", "把", "被", "让", "使", "这", "那", "之", "其", "以",
    "吗", "呢", "吧", "啊", "什么", "怎么", "如何", "哪些", "请问", "一下", "中",
    "上", "下", "可以", "可能", "需要", "进行", "通过", "一个", "一种",
}


def tokenize(text: str) -> list[str]:
    """中文分词 + 过滤停用词"""
    tokens = []
    try:
        import jieba
        raw = jieba.lcut(text)
    except ImportError:
        # 无 jieba 时退化为字符级 2-gram
        raw = [text[i:i + 2] for i in range(len(text) - 1)]
    for t in raw:
        t = t.strip()
        if len(t) >= 2 and t not in STOPWORDS and not t.isdigit():
            tokens.append(t)
        elif len(t) == 1 and t.isalpha() and t not in STOPWORDS:
            tokens.append(t)
    return tokens


class BM25Index:
    """BM25 索引"""

    def __init__(self, k1: float = 1.5, b: float = 0.75):
        self.k1 = k1
        self.b = b
        self.records: list[dict] = []
        self.doc_tokens: list[list[str]] = []
        self.doc_lens: list[int] = []
        self.avgdl = 0.0
        self.idf: dict[str, float] = {}
        self.tf: list[Counter] = []

    def build(self, chunks: list):
        self.records = [{"id": c.id, "text": c.text, "metadata": c.metadata} for c in chunks]
        self.doc_tokens = [tokenize(c.text) for c in chunks]
        self.doc_lens = [len(t) for t in self.doc_tokens]
        self.avgdl = sum(self.doc_lens) / max(len(self.doc_lens), 1)
        self.tf = [Counter(t) for t in self.doc_tokens]

        n = len(self.records)
        df = Counter()
        for tokens in self.doc_tokens:
            for term in set(tokens):
                df[term] += 1
        self.idf = {
            term: math.log(1 + (n - freq + 0.5) / (freq + 0.5))
            for term, freq in df.items()
        }
        print(f"[BM25] 构建完成，共 {n} 个文档，词典 {len(self.idf)} 词")

    def _score(self, q_tokens: list[str], idx: int) -> float:
        score = 0.0
        tf = self.tf[idx]
        dl = self.doc_lens[idx]
        norm = 1 - self.b + self.b * dl / max(self.avgdl, 1)
        for term in q_tokens:
            if term not in self.idf:
                continue
            f = tf.get(term, 0)
            if f == 0:
                continue
            score += self.idf[term] * (f * (self.k1 + 1)) / (f + self.k1 * norm)
        return score

    def search(self, query: str, k: int = 5) -> list[dict]:
        if not self.records:
            return []
        q_tokens = tokenize(query)
        scored = [
            (self._score(q_tokens, i), i)
            for i in range(len(self.records))
        ]
        scored = [(s, i) for s, i in scored if s > 0]
        scored.sort(key=lambda x: x[0], reverse=True)

        results = []
        max_score = scored[0][0] if scored else 1
        for score, idx in scored[:k]:
            rec = dict(self.records[idx])
            rec["score"] = float(score / max_score)  # 归一化到 0~1
            results.append(rec)
        return results

    def save(self, directory: str):
        d = Path(directory)
        d.mkdir(parents=True, exist_ok=True)
        with open(d / "bm25.pkl", "wb") as f:
            pickle.dump(self, f)

    def load(self, directory: str) -> bool:
        p = Path(directory) / "bm25.pkl"
        if not p.exists():
            return False
        try:
            obj = pickle.loads(p.read_bytes())
            self.__dict__.update(obj.__dict__)
            return True
        except Exception as e:
            print(f"[BM25] 加载失败: {e}")
            return False
