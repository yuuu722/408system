"""
LLM 语义理解
调用大模型（DeepSeek / 华为云 MaaS 盘古 / 昇腾 MindIE）完成意图识别与实体抽取。
对 JSON 模式做容错：不支持 response_format 时回退普通调用并从文本中提取 JSON。
"""
import json
import re

from .base import NLPProvider


INTENT_PROMPT = """你是一个408考研问答系统的语义理解模块。请分析用户的问题，输出JSON格式的意图识别结果。

## 任务
1. 识别问题类型（intent）：
   - greeting: 问候、打招呼
   - definition: 概念定义、名词解释
   - principle: 原理、机制解释
   - comparison: 对比、比较
   - calculation: 计算、分析过程
   - relationship: 关系、关联
   - recommendation: 学习建议、备考指导

2. 提取涉及的知识点实体（entities）：
   - name: 知识点名称
   - type: 类型（concept概念 / algorithm算法 / structure结构 / protocol协议 / mechanism机制）
   - aliases: 别名列表

3. 判断所属科目（subject）：
   - ds: 数据结构
   - co: 计算机组成原理
   - os: 操作系统
   - cn: 计算机网络
   - null: 跨科目或无法判断

## 输出格式
严格输出以下JSON格式，不要有其他内容：
{
  "intent": "意图类型",
  "subject": "科目代码或null",
  "entities": [
    {"name": "知识点名", "type": "类型", "aliases": ["别名1"]}
  ],
  "sub_intent": "更具体的子意图",
  "confidence": 0.0到1.0的浮点数
}

## 用户问题
{query}

## 对话历史
{history}
"""


class LLMNLP(NLPProvider):
    """LLM 语义理解"""

    name = "llm"

    def __init__(self, config):
        super().__init__(config)
        self.client = None
        if config.llm_enabled():
            try:
                from openai import AsyncOpenAI
                self.client = AsyncOpenAI(
                    api_key=config.llm_api_key,
                    base_url=config.llm_base_url,
                    timeout=config.llm_timeout
                )
                self.available = True
            except Exception as e:
                print(f"[NLP] LLM 客户端初始化失败: {e}")

    async def analyze(self, query: str, history: list) -> dict:
        if self.client is None:
            return self.default_result(query)

        prompt = INTENT_PROMPT.format(
            query=query,
            history=json.dumps(history[-3:], ensure_ascii=False) if history else "无"
        )
        messages = [
            {"role": "system", "content": "你是一个专业的408考研知识图谱问答系统语义分析模块。"},
            {"role": "user", "content": prompt}
        ]

        try:
            # 优先尝试 JSON 模式（部分兼容服务可能不支持）
            try:
                response = await self.client.chat.completions.create(
                    model=self.config.llm_model, messages=messages,
                    temperature=0.1, max_tokens=500,
                    response_format={"type": "json_object"}
                )
            except Exception:
                response = await self.client.chat.completions.create(
                    model=self.config.llm_model, messages=messages,
                    temperature=0.1, max_tokens=500
                )

            content = response.choices[0].message.content or ""
            result = self._loads(content)
            return self._normalize(result, query)

        except Exception as e:
            print(f"[NLP] LLM 意图识别失败: {e}")
            return self.default_result(query)

    @staticmethod
    def _loads(content: str) -> dict:
        """解析 JSON，失败时从文本中提取首个 JSON 对象"""
        try:
            return json.loads(content)
        except Exception:
            match = re.search(r"\{.*\}", content, re.DOTALL)
            if match:
                try:
                    return json.loads(match.group(0))
                except Exception:
                    pass
        return {}

    @staticmethod
    def _normalize(result: dict, query: str) -> dict:
        if not result:
            return NLPProvider.default_result(query)
        entities = [
            {
                "name": e.get("name", ""),
                "type": e.get("type", "concept"),
                "aliases": e.get("aliases", [])
            }
            for e in result.get("entities", [])
        ]
        return {
            "intent": result.get("intent", "definition"),
            "subject": result.get("subject"),
            "entities": entities or [{"name": query, "type": "concept", "aliases": []}],
            "sub_intent": result.get("sub_intent", ""),
            "confidence": float(result.get("confidence", 0.5) or 0.5),
        }
