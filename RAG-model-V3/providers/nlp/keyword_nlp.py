"""
关键词规则语义理解（降级方案）
无需任何外部服务，基于内置 408 专业词表做科目/意图判断与实体抽取。
"""
from .base import NLPProvider


# 科目关键词
SUBJECT_KEYWORDS = {
    "ds": ["数组", "链表", "栈", "队列", "树", "二叉树", "图", "排序", "查找", "哈希", "堆", "BST", "AVL", "红黑树", "KMP", "B树"],
    "co": ["CPU", "指令", "寄存器", "存储器", "缓存", "总线", "IO", "中断", "流水线", "运算器", "控制器", "浮点数", "寻址", "校验"],
    "os": ["进程", "线程", "调度", "内存", "分页", "分段", "虚拟内存", "文件", "死锁", "同步", "互斥", "信号量", "页面", "磁盘"],
    "cn": ["TCP", "UDP", "IP", "HTTP", "网络", "路由", "交换机", "数据链路", "传输层", "应用层", "拥塞控制", "三次握手", "DNS", "以太网"]
}

# 意图关键词
INTENT_KEYWORDS = {
    "comparison": ["区别", "对比", "比较", "不同", "差异", "哪个好"],
    "principle": ["原理", "为什么", "怎么工作", "机制", "过程"],
    "calculation": ["计算", "算", "求", "时间复杂度", "空间复杂度"],
    "relationship": ["关系", "关联", "联系", "依赖"],
    "recommendation": ["建议", "怎么学", "备考", "复习", "重点"],
}


class KeywordNLP(NLPProvider):
    """关键词规则语义理解"""

    name = "keyword"

    def __init__(self, config):
        super().__init__(config)
        self.available = True

    async def analyze(self, query: str, history: list) -> dict:
        # 判断科目
        subject = None
        for sub, keywords in SUBJECT_KEYWORDS.items():
            if any(kw in query for kw in keywords):
                subject = sub
                break

        # 判断意图
        intent = "definition"
        for itype, keywords in INTENT_KEYWORDS.items():
            if any(kw in query for kw in keywords):
                intent = itype
                break

        # 抽取实体
        entities = []
        for keywords in SUBJECT_KEYWORDS.values():
            for kw in keywords:
                if kw in query:
                    entities.append({"name": kw, "type": "concept", "aliases": []})
        if not entities:
            entities = [{"name": query, "type": "concept", "aliases": []}]

        return {
            "intent": intent,
            "subject": subject,
            "entities": entities,
            "sub_intent": "关键词匹配",
            "confidence": 0.5,
        }
