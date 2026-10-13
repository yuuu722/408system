"""
FAISS 向量库
使用内积索引（向量已归一化，内积即余弦相似度），支持构建、检索、持久化。
"""
import json
from pathlib import Path

import numpy as np


class VectorStore:
    """FAISS 向量库"""

    def __init__(self, dim: int = 512):
        self.dim = dim
        self.index = None
        self.records: list[dict] = []  # 与向量位置一一对应

    def build(self, chunks: list, embeddings: np.ndarray):
        import faiss
        self.index = faiss.IndexFlatIP(self.dim)
        self.index.add(embeddings.astype("float32"))
        self.records = [
            {"id": c.id, "text": c.text, "metadata": c.metadata}
            for c in chunks
        ]
        print(f"[VectorStore] 构建完成，共 {len(self.records)} 个向量")

    def search(self, query_embedding: np.ndarray, k: int = 5) -> list[dict]:
        if self.index is None or not self.records:
            return []
        k = min(k, len(self.records))
        scores, indices = self.index.search(query_embedding.astype("float32"), k)
        results = []
        for score, idx in zip(scores[0], indices[0]):
            if idx < 0:
                continue
            rec = dict(self.records[idx])
            rec["score"] = float(score)
            results.append(rec)
        return results

    def save(self, directory: str):
        import faiss
        d = Path(directory)
        d.mkdir(parents=True, exist_ok=True)
        faiss.write_index(self.index, str(d / "faiss.index"))
        with open(d / "chunks.json", "w", encoding="utf-8") as f:
            json.dump(self.records, f, ensure_ascii=False, indent=2)
        print(f"[VectorStore] 已保存到 {d}")

    def load(self, directory: str) -> bool:
        import faiss
        d = Path(directory)
        index_path = d / "faiss.index"
        chunks_path = d / "chunks.json"
        if not index_path.exists() or not chunks_path.exists():
            return False
        try:
            self.index = faiss.read_index(str(index_path))
            with open(chunks_path, "r", encoding="utf-8") as f:
                self.records = json.load(f)
            print(f"[VectorStore] 已加载 {len(self.records)} 个向量")
            return True
        except Exception as e:
            print(f"[VectorStore] 加载失败: {e}")
            return False
