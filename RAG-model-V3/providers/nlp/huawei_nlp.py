"""
华为云 NLP 语义理解
调用华为云「自然语言处理基础」中文分词与词性标注 API（AK/SK 签名），
再结合内置 408 专业词表完成意图判断与知识点实体抽取。

依赖（可选）：
    pip install huaweicloudsdkcore requests

分词 API：
    POST https://{endpoint}/v1/{project_id}/nlp-fundamental/segment
    body: {"text": "...", "pos_switch": 1, "lang": "zh"}
    返回: {"words": [{"content": "词", "pos": "词性"}]}

说明：华为云通用 NER 主要识别人名/地名/机构等通用实体，对 408 专业术语，
这里用其分词/词性结果 + 内置 408 专业词表共同抽取，保证专业知识点不丢失。
"""
import json

from .base import NLPProvider
from .keyword_nlp import SUBJECT_KEYWORDS, INTENT_KEYWORDS


class HuaweiNLP(NLPProvider):
    """华为云 NLP 语义理解"""

    name = "huawei_nlp"

    def __init__(self, config):
        super().__init__(config)
        self.project_id = config.hw_nlp_project_id or config.hw_project_id
        self.endpoint = config.hw_nlp_endpoint.replace("https://", "").replace("http://", "").strip("/")
        self._signer = None
        self._connect()

    def _connect(self):
        missing = []
        if not self.config.hw_ak:
            missing.append("HUAWEI_ACCESS_KEY")
        if not self.config.hw_sk:
            missing.append("HUAWEI_SECRET_KEY")
        if not self.project_id:
            missing.append("HUAWEI_PROJECT_ID")
        if not self.endpoint:
            missing.append("HUAWEI_NLP_ENDPOINT")
        if missing:
            print(f"[NLP] 华为云 NLP 不可用，缺少: {', '.join(missing)}")
            return
        try:
            from huaweicloudsdkcore.signer.signer import Signer
            from huaweicloudsdkcore.auth.credentials import BasicCredentials
            creds = BasicCredentials(self.config.hw_ak, self.config.hw_sk, self.project_id)
            self._signer = Signer(creds)
            self.available = True
            print("[NLP] 华为云 NLP 客户端就绪")
        except Exception as e:
            print(f"[NLP] 华为云签名组件初始化失败（请 pip install huaweicloudsdkcore）: {e}")

    def _segment(self, text: str) -> list:
        """调用华为云分词，返回 [{"content","pos"}]"""
        import requests

        url = f"https://{self.endpoint}/v1/{self.project_id}/nlp-fundamental/segment"
        body = json.dumps({"text": text, "pos_switch": 1, "lang": "zh"},
                          ensure_ascii=False).encode("utf-8")
        req = requests.Request(
            "POST", url, data=body,
            headers={"Content-Type": "application/json", "Host": self.endpoint.split(":")[0]}
        )
        req = self._signer.sign(req)
        with requests.Session() as s:
            resp = s.send(req, timeout=30)

        if resp.status_code != 200:
            print(f"[NLP] 华为云分词 HTTP {resp.status_code}: {resp.text[:400]}")
            return []
        return resp.json().get("words", []) or []

    async def analyze(self, query: str, history: list) -> dict:
        if not self.available:
            return self.default_result(query)

        # 1. 真实调用华为云分词
        words = self._segment(query)
        if words:
            print("[NLP] 华为云分词: " + " / ".join(w.get("content", "") for w in words[:12]))

        # 2. 科目判断（专业词表）
        subject = None
        for sub, keywords in SUBJECT_KEYWORDS.items():
            if any(kw in query for kw in keywords):
                subject = sub
                break

        # 3. 意图判断（关键词）
        intent = "definition"
        for itype, keywords in INTENT_KEYWORDS.items():
            if any(kw in query for kw in keywords):
                intent = itype
                break

        # 4. 实体抽取：专业词表命中 + 华为云分词中的名词性词
        entities = []
        seen = set()
        for keywords in SUBJECT_KEYWORDS.values():
            for kw in keywords:
                if kw in query and kw not in seen:
                    seen.add(kw)
                    entities.append({"name": kw, "type": "concept", "aliases": []})
        for w in words:
            term = w.get("content", "")
            pos = w.get("pos", "")
            if len(term) >= 2 and pos.startswith("n") and term not in seen:
                seen.add(term)
                entities.append({"name": term, "type": "concept", "aliases": []})
        if not entities:
            entities = [{"name": query, "type": "concept", "aliases": []}]

        return {
            "intent": intent,
            "subject": subject,
            "entities": entities,
            "sub_intent": "华为云NLP分词+专业词表",
            "confidence": 0.8 if words else 0.5,
        }
