"""
响应生成模块
基于检索结果流式生成回答
"""
import json
from openai import AsyncOpenAI

from config import Config
from gateway import IntentResult
from searcher import SearchResult


# 回答生成 Prompt
GENERATE_PROMPT = """你是一个专业的408考研辅导老师，精通数据结构、计算机组成原理、操作系统、计算机网络四门课程。

## 回答要求
1. 语言专业但通俗易懂，适合考研学生理解
2. 结构清晰，重点突出，必要时分点说明
3. 结合考研考点，指出易混淆点和常考题型
4. 引用检索到的知识点时，标注来源（如 [E1]、[E2]）
5. 字数控制在 300-800 字，根据问题复杂度调整

## 用户问题
{query}

## 问题分析
- 意图：{intent}
- 科目：{subject}
- 涉及知识点：{entities}

## 检索到的知识点资料
{evidence}

## 对话历史
{history}

## 回答风格
- 先直接回答问题核心
- 再展开详细解释
- 最后给出考研备考建议或易错提示
- 不要编造检索资料中没有的内容
"""


class ResponseGenerator:
    """回答生成器"""

    def __init__(self, config: Config):
        self.config = config
        self.client = None
        if config.llm_api_key and config.llm_api_key != "your_api_key_here":
            self.client = AsyncOpenAI(
                api_key=config.llm_api_key,
                base_url=config.llm_base_url,
                timeout=config.llm_timeout
            )

    async def generate(
        self,
        query: str,
        intent: IntentResult,
        search_result: SearchResult,
        history: list[dict]
    ):
        """流式生成回答"""

        # 没有配置 LLM，使用知识库直接回答（降级模式）
        if self.client is None:
            async for chunk in self._simple_generate(query, intent, search_result):
                yield chunk
            return

        # 构建证据文本
        evidence_text = self._build_evidence_text(search_result)

        # 构建实体列表
        entities_text = ", ".join([e.name for e in intent.entities]) or "无明确实体"

        # 构建 Prompt
        prompt = GENERATE_PROMPT.format(
            query=query,
            intent=intent.intent,
            subject=intent.subject or "跨科目",
            entities=entities_text,
            evidence=evidence_text,
            history=json.dumps(history[-3:], ensure_ascii=False) if history else "无"
        )

        # 流式调用 LLM
        try:
            stream = await self.client.chat.completions.create(
                model=self.config.llm_model,
                messages=[
                    {"role": "system", "content": "你是一个专业的408考研辅导老师，回答准确、清晰、有针对性。"},
                    {"role": "user", "content": prompt}
                ],
                temperature=self.config.llm_temperature,
                max_tokens=self.config.llm_max_tokens,
                stream=True
            )

            async for chunk in stream:
                if chunk.choices and chunk.choices[0].delta.content:
                    yield chunk.choices[0].delta.content

        except Exception as e:
            print(f"[Generator] 生成失败: {e}")
            yield f"抱歉，生成回答时出现错误：{str(e)}"

    def _build_evidence_text(self, search_result: SearchResult) -> str:
        """构建证据文本"""
        if not search_result.evidence:
            return "未检索到相关知识点资料，将基于通识回答。"

        lines = []
        for i, ev in enumerate(search_result.evidence, 1):
            lines.append(f"[E{i}] {ev.title}（来源：{ev.source}）")
            lines.append(f"    {ev.content}")
            lines.append("")

        return "\n".join(lines)

    async def _simple_generate(self, query: str, intent: IntentResult, search_result: SearchResult):
        """无 LLM 时的简单回答（降级模式，基于知识库直接回答）"""
        import asyncio

        if intent.intent == "greeting":
            text = "你好！我是408考研智能辅导助手，可以帮你解答数据结构、计算机组成原理、操作系统、计算机网络的相关问题。请问有什么可以帮你的？"
        elif not search_result.evidence:
            text = f"抱歉，我在知识库中没有找到与「{query}」直接相关的知识点。\n\n目前知识库还在建设中，你可以尝试：\n1. 换个关键词提问\n2. 配置 LLM API Key 获得更智能的回答\n3. 查看知识图谱了解已有的知识点"
        else:
            # 基于检索到的知识点组织回答
            lines = []
            lines.append(f"关于「{query}」，我在知识库中找到以下内容：\n")

            for i, ev in enumerate(search_result.evidence, 1):
                lines.append(f"**{ev.title}** [E{i}]")
                lines.append(f"{ev.content}")
                lines.append(f"📖 来源：{ev.source}\n")

            if intent.intent == "comparison" and len(search_result.evidence) >= 2:
                lines.append("💡 **对比分析**：以上知识点各有特点，建议结合具体场景理解它们的区别和联系。")

            lines.append("\n---")
            lines.append("⚠️ 当前为知识库直答模式，配置 LLM API Key 后可获得更详细、更有针对性的解答。")

            text = "\n".join(lines)

        # 模拟流式输出
        for char in text:
            yield char
            await asyncio.sleep(0.01)
