"""
配置管理模块
从环境变量或 .env 文件读取配置
"""
import os
from pathlib import Path
from dataclasses import dataclass, field
from dotenv import load_dotenv

# 项目根目录
PROJECT_ROOT = Path(__file__).resolve().parent.parent
ENV_PATH = PROJECT_ROOT / ".env"

# 加载 .env 文件
if ENV_PATH.exists():
    load_dotenv(ENV_PATH)


@dataclass
class Config:
    """系统配置"""
    # LLM 配置
    llm_api_key: str = field(default_factory=lambda: os.getenv("LLM_API_KEY", ""))
    llm_base_url: str = field(default_factory=lambda: os.getenv("LLM_BASE_URL", "https://api.deepseek.com/v1"))
    llm_model: str = field(default_factory=lambda: os.getenv("LLM_MODEL", "deepseek-chat"))
    llm_temperature: float = field(default_factory=lambda: float(os.getenv("LLM_TEMPERATURE", "0.1")))
    llm_max_tokens: int = field(default_factory=lambda: int(os.getenv("LLM_MAX_TOKENS", "2048")))
    llm_timeout: int = field(default_factory=lambda: int(os.getenv("LLM_TIMEOUT", "60")))

    # Neo4j 配置
    neo4j_uri: str = field(default_factory=lambda: os.getenv("NEO4J_URI", "bolt://localhost:7687"))
    neo4j_user: str = field(default_factory=lambda: os.getenv("NEO4J_USER", "neo4j"))
    neo4j_password: str = field(default_factory=lambda: os.getenv("NEO4J_PASSWORD", ""))
    neo4j_database: str = field(default_factory=lambda: os.getenv("NEO4J_DATABASE", "neo4j"))

    # 向量配置
    embedding_model: str = field(default_factory=lambda: os.getenv("EMBEDDING_MODEL", "BAAI/bge-small-zh-v1.5"))
    vector_dimension: int = field(default_factory=lambda: int(os.getenv("VECTOR_DIMENSION", "512")))
    vector_index_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "vector_indices"))

    # 缓存配置
    cache_ttl: int = field(default_factory=lambda: int(os.getenv("CACHE_TTL", "86400")))
    cache_max_entries: int = field(default_factory=lambda: int(os.getenv("CACHE_MAX_ENTRIES", "500")))

    # 系统配置
    max_history_rounds: int = field(default_factory=lambda: int(os.getenv("MAX_HISTORY_ROUNDS", "5")))
    knowledge_version: str = field(default_factory=lambda: os.getenv("KNOWLEDGE_VERSION", "v1.0"))

    # 数据路径
    data_dir: str = field(default_factory=lambda: str(PROJECT_ROOT / "data"))
    sample_data_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "data" / "sample"))

    def validate(self) -> bool:
        """验证必要配置"""
        errors = []
        if not self.llm_api_key:
            errors.append("LLM_API_KEY 未配置")
        if not self.neo4j_password:
            errors.append("NEO4J_PASSWORD 未配置")

        if errors:
            print("[Config] 配置警告:")
            for e in errors:
                print(f"  - {e}")
            return False
        return True


_config_instance = None


def get_config() -> Config:
    """获取配置单例"""
    global _config_instance
    if _config_instance is None:
        _config_instance = Config()
        _config_instance.validate()
    return _config_instance
