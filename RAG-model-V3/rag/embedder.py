"""
向量化器
使用本地 BGE（BAAI/bge-small-zh-v1.5）通过 sentence-transformers 生成向量。
首次加载会自动下载模型（约 100MB），之后完全离线。
模型不可用时返回 None，由 BM25 关键词检索兜底。
"""
import numpy as np


# BGE 中文检索的查询前缀（提升检索效果）
QUERY_INSTRUCTION = "为这个句子生成表示以用于检索相关文章："


class Embedder:
    """BGE 向量化器（单例懒加载）"""

    def __init__(self, model_name: str = "BAAI/bge-small-zh-v1.5"):
        self.model_name = model_name
        self._model = None
        self.available = False
        self._tried = False

    def _ensure_model(self):
        if self._tried:
            return
        self._tried = True
        try:
            from sentence_transformers import SentenceTransformer
            self._model = SentenceTransformer(self.model_name)
            self.available = True
            print(f"[Embedder] 模型加载成功: {self.model_name}")
        except Exception as e:
            print(f"[Embedder] 模型加载失败（将降级为 BM25）: {e}")

    def encode(self, texts: list[str], is_query: bool = False,
               batch_size: int = 32) -> "np.ndarray | None":
        self._ensure_model()
        if not self.available:
            return None

        if is_query:
            texts = [QUERY_INSTRUCTION + t for t in texts]

        emb = self._model.encode(
            texts,
            batch_size=batch_size,
            normalize_embeddings=True,  # 归一化后内积即余弦相似度
            show_progress_bar=len(texts) > 100,
            convert_to_numpy=True,
        )
        return np.asarray(emb, dtype="float32")
