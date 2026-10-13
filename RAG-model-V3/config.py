"""
配置管理模块
从环境变量或 .env 文件读取配置

支持三套可插拔 Provider（通过 .env 一键切换）：
- LLM 大模型：DeepSeek / 华为云 MaaS(盘古) / 昇腾 MindIE / 通用 OpenAI
- 图存储：Neo4j / 华为云 GES / 示例数据（降级）
- 语义理解：LLM / 华为云 NLP / 关键词（降级）
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


# ---------- 华为云常量（可被环境变量覆盖） ----------
# 华为云 MaaS（模型即服务，OpenAI 兼容）
HW_MAAS_BASE_URL = "https://api.modelarts-maas.com/openai/v1"
# 昇腾 MindIE 本地推理服务（OpenAI 兼容，默认端口以实际部署为准）
HW_MINDIE_BASE_URL = "http://localhost:1025/v1"


def _env(name: str, default: str = "") -> str:
    """读取环境变量，去除首尾空白，空值返回 default"""
    val = os.getenv(name)
    if val is None:
        return default
    val = val.strip()
    return val if val else default


@dataclass
class Config:
    """系统配置"""

    # ============ Provider 开关 ============
    # deepseek / huawei_maas / huawei_mindie / openai
    llm_provider: str = field(default_factory=lambda: _env("LLM_PROVIDER", "deepseek"))
    # auto / neo4j / huawei_ges / sample
    graph_provider: str = field(default_factory=lambda: _env("GRAPH_PROVIDER", "auto"))
    # auto / llm / huawei_nlp / keyword
    nlp_provider: str = field(default_factory=lambda: _env("NLP_PROVIDER", "auto"))
    # auto / bm25 / none（全文检索 Provider）
    retrieval_provider: str = field(default_factory=lambda: _env("RETRIEVAL_PROVIDER", "auto"))

    # ============ LLM 配置（生效值，由 provider 归一化） ============
    llm_api_key: str = field(default_factory=lambda: _env("LLM_API_KEY", ""))
    llm_base_url: str = field(default_factory=lambda: _env("LLM_BASE_URL", "https://api.deepseek.com/v1"))
    llm_model: str = field(default_factory=lambda: _env("LLM_MODEL", "deepseek-chat"))
    llm_temperature: float = field(default_factory=lambda: float(_env("LLM_TEMPERATURE", "0.1")))
    llm_max_tokens: int = field(default_factory=lambda: int(_env("LLM_MAX_TOKENS", "2048")))
    llm_timeout: int = field(default_factory=lambda: int(_env("LLM_TIMEOUT", "60")))

    # ============ 华为云通用（IAM） ============
    # MaaS 仅支持“西南-贵阳一”，region id = cn-southwest-2
    hw_region: str = field(default_factory=lambda: _env("HUAWEI_REGION", "cn-southwest-2"))
    hw_project_id: str = field(default_factory=lambda: _env("HUAWEI_PROJECT_ID", ""))
    hw_ak: str = field(default_factory=lambda: _env("HUAWEI_ACCESS_KEY", ""))
    hw_sk: str = field(default_factory=lambda: _env("HUAWEI_SECRET_KEY", ""))

    # ============ 华为云 MaaS（盘古，OpenAI 兼容，Bearer 鉴权） ============
    hw_maas_base_url: str = field(default_factory=lambda: _env("HUAWEI_MAAS_BASE_URL", HW_MAAS_BASE_URL))
    hw_maas_api_key: str = field(default_factory=lambda: _env("HUAWEI_MAAS_API_KEY", ""))
    # 模型名以 MaaS 控制台“API 调用”显示为准（盘古/DeepSeek/千问/GLM 等）
    hw_maas_model: str = field(default_factory=lambda: _env("HUAWEI_MAAS_MODEL", "openPangu"))

    # ============ 昇腾 MindIE（本地，OpenAI 兼容） ============
    hw_mindie_base_url: str = field(default_factory=lambda: _env("HUAWEI_MINDIE_BASE_URL", HW_MINDIE_BASE_URL))
    hw_mindie_model: str = field(default_factory=lambda: _env("HUAWEI_MINDIE_MODEL", "openPangu"))

    # ============ 华为云 GES（图引擎） ============
    # 图 ID / 图名称（GES 控制台创建图后获取）
    hw_ges_graph_id: str = field(default_factory=lambda: _env("HUAWEI_GES_GRAPH_ID", ""))
    # 留空则使用 hw_project_id
    hw_ges_project_id: str = field(default_factory=lambda: _env("HUAWEI_GES_PROJECT_ID", ""))
    # 图访问地址（SERVER_URL，公网/内网访问地址），从 GES 控制台图详情获取
    hw_ges_endpoint: str = field(default_factory=lambda: _env("HUAWEI_GES_ENDPOINT", ""))

    # ============ 华为云 NLP（自然语言处理基础） ============
    # NLP 基础服务常用“北京四”，可与 MaaS 区域不同
    hw_nlp_endpoint: str = field(default_factory=lambda: _env("HUAWEI_NLP_ENDPOINT", "nlp-ext.cn-north-4.myhuaweicloud.com"))
    # 留空则使用 hw_project_id（注意：跨区域 project_id 不同）
    hw_nlp_project_id: str = field(default_factory=lambda: _env("HUAWEI_NLP_PROJECT_ID", ""))

    # ============ Neo4j 配置 ============
    neo4j_uri: str = field(default_factory=lambda: _env("NEO4J_URI", "bolt://localhost:7687"))
    neo4j_user: str = field(default_factory=lambda: _env("NEO4J_USER", "neo4j"))
    neo4j_password: str = field(default_factory=lambda: _env("NEO4J_PASSWORD", ""))
    neo4j_database: str = field(default_factory=lambda: _env("NEO4J_DATABASE", "neo4j"))

    # ============ 向量配置 ============
    embedding_model: str = field(default_factory=lambda: _env("EMBEDDING_MODEL", "BAAI/bge-small-zh-v1.5"))
    vector_dimension: int = field(default_factory=lambda: int(_env("VECTOR_DIMENSION", "512")))
    vector_index_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "vector_indices"))

    # ============ 缓存配置 ============
    cache_ttl: int = field(default_factory=lambda: int(_env("CACHE_TTL", "86400")))
    cache_max_entries: int = field(default_factory=lambda: int(_env("CACHE_MAX_ENTRIES", "500")))

    # ============ 系统配置 ============
    max_history_rounds: int = field(default_factory=lambda: int(_env("MAX_HISTORY_ROUNDS", "5")))
    knowledge_version: str = field(default_factory=lambda: _env("KNOWLEDGE_VERSION", "v1.0"))

    # ============ 数据路径 ============
    data_dir: str = field(default_factory=lambda: str(PROJECT_ROOT / "data"))
    sample_data_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "data" / "sample"))
    # 扩充版知识图谱（4 Word 构建）
    knowledge_data_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "data" / "knowledge"))
    # RAG 语料库
    corpus_data_path: str = field(default_factory=lambda: str(PROJECT_ROOT / "data" / "corpus"))
    # 全文检索返回条数
    retrieval_top_k: int = field(default_factory=lambda: int(_env("RETRIEVAL_TOP_K", "6")))

    def __post_init__(self):
        """根据 LLM Provider 把生效值归一化到 llm_* 字段"""
        provider = (self.llm_provider or "deepseek").lower().strip()

        if provider == "huawei_maas":
            # 华为云 MaaS（盘古），OpenAI 兼容
            self.llm_base_url = self.hw_maas_base_url
            self.llm_model = self.hw_maas_model
            if self.hw_maas_api_key:
                self.llm_api_key = self.hw_maas_api_key

        elif provider == "huawei_mindie":
            # 昇腾 MindIE 本地服务，OpenAI 兼容，通常不校验 key
            self.llm_base_url = self.hw_mindie_base_url
            self.llm_model = self.hw_mindie_model
            if not self.llm_api_key:
                self.llm_api_key = "mindie_local"

        # deepseek / openai：沿用 LLM_* 显式配置，不做覆盖

    def llm_enabled(self) -> bool:
        """是否启用了真实 LLM（非占位、非空）"""
        key = (self.llm_api_key or "").strip()
        return bool(key) and key not in ("your_api_key_here", "your_password_here")

    def validate(self) -> bool:
        """验证必要配置（仅打印告警，不阻断，系统可降级运行）"""
        warnings = []
        if not self.llm_enabled():
            warnings.append(f"LLM 未配置（provider={self.llm_provider}），将使用知识库直答降级模式")
        if self.graph_provider == "neo4j" and not self.neo4j_password:
            warnings.append("NEO4J_PASSWORD 未配置，将使用示例数据")
        if self.graph_provider == "huawei_ges" and not (self.hw_ak and self.hw_ges_graph_id):
            warnings.append("华为云 GES 凭证/图 ID 未配置，将使用示例数据")
        if self.nlp_provider == "huawei_nlp" and not (self.hw_ak and self.hw_sk):
            warnings.append("华为云 NLP 凭证未配置，将使用关键词/LLM 降级")

        if warnings:
            print("[Config] 配置提示:")
            for w in warnings:
                print(f"  - {w}")
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
