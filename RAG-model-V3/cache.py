"""
缓存模块
内存缓存 + 可选 Redis
"""
import time
import hashlib
import json
from typing import Optional, Any


class ResponseCache:
    """回答缓存"""

    def __init__(self, ttl: int = 86400, max_entries: int = 500):
        self.ttl = ttl
        self.max_entries = max_entries
        self._cache: dict[str, dict] = {}
        self._redis_client = None

        # 尝试连接 Redis
        self._init_redis()

    def _init_redis(self):
        """初始化 Redis 连接（可选）"""
        try:
            import redis
            self._redis_client = redis.Redis(
                host="localhost",
                port=6379,
                db=0,
                decode_responses=True
            )
            self._redis_client.ping()
            print("[Cache] Redis 连接成功")
        except Exception:
            print("[Cache] Redis 不可用，使用内存缓存")
            self._redis_client = None

    def make_key(self, query: str, history: list[dict]) -> str:
        """生成缓存键"""
        # 基于问题和历史生成哈希
        content = json.dumps({
            "q": query,
            "h": history[-3:]  # 只取最近3轮
        }, ensure_ascii=False, sort_keys=True)
        return hashlib.sha256(content.encode()).hexdigest()[:32]

    def get(self, key: str) -> Optional[Any]:
        """获取缓存"""
        # 先查 Redis
        if self._redis_client:
            try:
                data = self._redis_client.get(key)
                if data:
                    return json.loads(data)
            except Exception:
                pass

        # 查内存缓存
        if key in self._cache:
            entry = self._cache[key]
            if time.time() - entry["time"] < self.ttl:
                return entry["data"]
            else:
                del self._cache[key]

        return None

    def set(self, key: str, data: Any):
        """设置缓存"""
        # 写入 Redis
        if self._redis_client:
            try:
                self._redis_client.setex(
                    key,
                    self.ttl,
                    json.dumps(data, ensure_ascii=False)
                )
            except Exception:
                pass

        # 写入内存缓存
        if len(self._cache) >= self.max_entries:
            # 淘汰最旧的
            oldest = min(self._cache.keys(), key=lambda k: self._cache[k]["time"])
            del self._cache[oldest]

        self._cache[key] = {
            "data": data,
            "time": time.time()
        }

    def clear(self):
        """清空缓存"""
        self._cache.clear()
        if self._redis_client:
            try:
                self._redis_client.flushdb()
            except Exception:
                pass
