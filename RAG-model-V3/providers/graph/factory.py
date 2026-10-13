"""
图存储工厂
根据 config.graph_provider 创建对应的图存储，任何外部服务不可用时自动降级到示例数据。
"""
from .base import GraphStore
from .sample_store import SampleGraphStore


def create_graph_store(config) -> GraphStore:
    """创建图存储实例"""
    provider = (config.graph_provider or "auto").lower().strip()

    if provider == "sample":
        return SampleGraphStore(config)

    if provider == "neo4j":
        from .neo4j_store import Neo4jGraphStore
        store = Neo4jGraphStore(config)
        if store.available:
            return store
        print("[GraphStore] 降级到示例数据")
        return SampleGraphStore(config)

    if provider == "huawei_ges":
        from .ges_store import GESGraphStore
        store = GESGraphStore(config)
        if store.available:
            return store
        print("[GraphStore] 降级到示例数据")
        return SampleGraphStore(config)

    # auto：优先本地 Neo4j，失败用示例数据
    from .neo4j_store import Neo4jGraphStore
    store = Neo4jGraphStore(config)
    if store.available:
        return store
    return SampleGraphStore(config)
