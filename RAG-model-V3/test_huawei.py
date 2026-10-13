"""华为云适配层验证脚本（无 Key 环境）"""
import asyncio
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from config import Config
from providers.graph.factory import create_graph_store
from providers.nlp.factory import create_nlp_provider


def test_imports():
    print("== 1. 模块导入 ==")
    import providers.graph.neo4j_store
    import providers.graph.ges_store
    import providers.graph.sample_store
    import providers.nlp.keyword_nlp
    import providers.nlp.llm_nlp
    import providers.nlp.huawei_nlp
    import gateway
    import searcher
    print("  所有模块导入成功")


def test_graph_factory():
    print("== 2. 图存储工厂 ==")
    # sample
    cfg = Config()
    cfg.graph_provider = "sample"
    s = create_graph_store(cfg)
    print(f"  sample -> {s.name}, available={s.available}")
    assert s.name == "sample"

    # ges 无 key（应降级 sample）
    cfg.graph_provider = "huawei_ges"
    s = create_graph_store(cfg)
    print(f"  huawei_ges(无key) -> {s.name}")

    # auto 无 neo4j（应降级 sample）
    cfg.graph_provider = "auto"
    s = create_graph_store(cfg)
    print(f"  auto(无neo4j) -> {s.name}")


def test_nlp_factory():
    print("== 3. NLP 工厂 ==")
    cfg = Config()
    cfg.nlp_provider = "keyword"
    n = create_nlp_provider(cfg)
    print(f"  keyword -> {n.name}")

    cfg.nlp_provider = "huawei_nlp"
    n = create_nlp_provider(cfg)
    print(f"  huawei_nlp(无key) -> {n.name}")

    cfg.nlp_provider = "auto"
    n = create_nlp_provider(cfg)
    print(f"  auto(无llm) -> {n.name}")


def test_gateway():
    print("== 4. 网关分析 ==")
    from gateway import SemanticGateway
    cfg = Config()
    gw = SemanticGateway(cfg)

    async def run():
        # 问候
        r = await gw.analyze("你好", [])
        print(f"  '你好' -> intent={r.intent}")
        assert r.intent == "greeting"

        # 专业问题
        r = await gw.analyze("什么是虚拟内存？它的作用是什么", [])
        print(f"  '虚拟内存' -> intent={r.intent}, subject={r.subject}, "
              f"entities={[e.name for e in r.entities]}")
        assert r.subject == "os"

        r = await gw.analyze("栈和队列的区别", [])
        print(f"  '栈和队列' -> subject={r.subject}, entities={[e.name for e in r.entities]}")

    asyncio.run(run())


def test_searcher():
    print("== 5. 检索器（sample 降级） ==")
    from searcher import KnowledgeSearcher
    cfg = Config()
    cfg.graph_provider = "sample"
    searcher = KnowledgeSearcher(cfg)

    async def run():
        from gateway import Entity
        entities = [Entity(name="栈", type="concept"), Entity(name="队列", type="concept")]
        r = await searcher.search("栈和队列", entities, "comparison")
        print(f"  节点数={len(r.graph_nodes)}, 边数={len(r.graph_edges)}, 证据数={len(r.evidence)}")
        assert len(r.graph_nodes) > 0

    asyncio.run(run())


def test_llm_provider_normalize():
    print("== 6. LLM Provider 归一化 ==")
    cfg = Config()
    cfg.llm_provider = "huawei_maas"
    cfg.__post_init__()
    print(f"  huawei_maas -> base_url={cfg.llm_base_url}, model={cfg.llm_model}")
    assert "modelarts-maas" in cfg.llm_base_url

    cfg2 = Config()
    cfg2.llm_provider = "huawei_mindie"
    cfg2.__post_init__()
    print(f"  huawei_mindie -> base_url={cfg2.llm_base_url}, key={cfg2.llm_api_key}")
    assert "1025" in cfg2.llm_base_url


if __name__ == "__main__":
    test_imports()
    test_graph_factory()
    test_nlp_factory()
    test_gateway()
    test_searcher()
    test_llm_provider_normalize()
    print("\n所有验证通过")
