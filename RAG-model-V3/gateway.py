"""
语义网关模块
负责意图识别和实体提取
"""
import json
from dataclasses import dataclass, field
from typing import Optional
from openai import AsyncOpenAI

from config import Config


@dataclass
class Entity:
    """提取的实体"""
    name: str
    type: str  # concept / chapter / subject / algorithm / structure
    aliases: list[str] = field(default_factory=list)


@dataclass
class IntentResult:
    """意图识别结果"""
    intent: str  # definition / comparison / calculation / principle / relationship / greeting
    entities: list[Entity] = field(default_factory=list)
    subject: Optional[str] = None  # ds / co / os / cn
    sub_intent: str = ""
    confidence: float = 0.0


# 意图识别 Prompt
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
{{
  "intent": "意图类型",
  "subject": "科目代码或null",
  "entities": [
    {{"name": "知识点名", "type": "类型", "aliases": ["别名1"]}}
  ],
  "sub_intent": "更具体的子意图",
  "confidence": 0.0到1.0的浮点数
}}

## 用户问题
{query}

## 对话历史
{history}
"""


class SemanticGateway:
    """语义网关：意图识别 + 实体提取"""

    def __init__(self, config: Config):
        self.config = config
        self.client = None
        if config.llm_api_key and config.llm_api_key != "your_api_key_here":
            self.client = AsyncOpenAI(
                api_key=config.llm_api_key,
                base_url=config.llm_base_url,
                timeout=config.llm_timeout
            )

    async def analyze(self, query: str, history: list[dict]) -> IntentResult:
        """分析用户问题"""
        # 简单问候直接返回
        if self._is_greeting(query):
            return IntentResult(
                intent="greeting",
                entities=[],
                sub_intent="用户问候"
            )

        # 没有配置 LLM，使用关键词匹配降级
        if self.client is None:
            return self._simple_analyze(query)

        # 调用 LLM 做意图识别
        prompt = INTENT_PROMPT.format(
            query=query,
            history=json.dumps(history[-3:], ensure_ascii=False) if history else "无"
        )

        try:
            response = await self.client.chat.completions.create(
                model=self.config.llm_model,
                messages=[
                    {"role": "system", "content": "你是一个专业的408考研知识图谱问答系统语义分析模块。"},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.1,
                max_tokens=500,
                response_format={"type": "json_object"}
            )

            result = json.loads(response.choices[0].message.content)

            entities = [
                Entity(
                    name=e.get("name", ""),
                    type=e.get("type", "concept"),
                    aliases=e.get("aliases", [])
                )
                for e in result.get("entities", [])
            ]

            return IntentResult(
                intent=result.get("intent", "definition"),
                entities=entities,
                subject=result.get("subject"),
                sub_intent=result.get("sub_intent", ""),
                confidence=float(result.get("confidence", 0.5))
            )

        except Exception as e:
            print(f"[Gateway] 意图识别失败: {e}")
            # 降级：返回默认意图
            return IntentResult(
                intent="definition",
                entities=[Entity(name=query, type="concept")],
                sub_intent="默认处理",
                confidence=0.3
            )

    def _is_greeting(self, query: str) -> bool:
        """判断是否为问候"""
        greetings = ["你好", "您好", "hi", "hello", "在吗", "早上好", "下午好", "晚上好"]
        query_lower = query.lower().strip()
        for g in greetings:
            if g in query_lower and len(query) < 20:
                return True
        return False

    def _simple_analyze(self, query: str) -> IntentResult:
        """无 LLM 时的简单关键词分析（降级方案）"""
        # 科目关键词
        subject_keywords = {
            "ds": ["数组", "链表", "栈", "队列", "树", "二叉树", "图", "排序", "查找", "哈希", "堆", "BST", "AVL", "红黑树"],
            "co": ["CPU", "指令", "寄存器", "存储器", "缓存", "总线", "IO", "中断", "流水线", "运算器", "控制器"],
            "os": ["进程", "线程", "调度", "内存", "分页", "分段", "虚拟内存", "文件", "死锁", "同步", "互斥", "信号量"],
            "cn": ["TCP", "UDP", "IP", "HTTP", "网络", "路由", "交换机", "数据链路", "传输层", "应用层", "拥塞控制", "三次握手"]
        }

        # 意图关键词
        intent_keywords = {
            "comparison": ["区别", "对比", "比较", "不同", "差异", "哪个好"],
            "principle": ["原理", "为什么", "怎么工作", "机制", "过程"],
            "calculation": ["计算", "算", "求", "时间复杂度", "空间复杂度"],
            "relationship": ["关系", "关联", "联系", "依赖"],
            "recommendation": ["建议", "怎么学", "备考", "复习", "重点"],
        }

        # 判断科目
        subject = None
        for sub, keywords in subject_keywords.items():
            for kw in keywords:
                if kw in query:
                    subject = sub
                    break
            if subject:
                break

        # 判断意图
        intent = "definition"
        for itype, keywords in intent_keywords.items():
            for kw in keywords:
                if kw in query:
                    intent = itype
                    break
            if intent != "definition":
                break

        # 提取实体（简单提取关键词）
        entities = []
        all_keywords = []
        for keywords in subject_keywords.values():
            all_keywords.extend(keywords)
        for kw in all_keywords:
            if kw in query:
                entities.append(Entity(name=kw, type="concept"))

        if not entities:
            entities = [Entity(name=query, type="concept")]

        return IntentResult(
            intent=intent,
            entities=entities,
            subject=subject,
            sub_intent="关键词匹配",
            confidence=0.5
        )
