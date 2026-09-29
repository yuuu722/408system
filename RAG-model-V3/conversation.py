"""
对话管理模块
管理多轮对话历史
"""
from collections import defaultdict, deque
from typing import Optional


class ConversationManager:
    """对话历史管理器"""

    def __init__(self, max_history: int = 5):
        self.max_history = max_history
        self.sessions: dict[str, deque] = defaultdict(lambda: deque(maxlen=max_history * 2))

    def update_history(
        self,
        session_id: str,
        user_query: str,
        existing_history: list[dict]
    ) -> list[dict]:
        """更新对话历史"""
        session = self.sessions[session_id]

        # 清空并重新填充
        session.clear()
        for msg in existing_history[-self.max_history * 2:]:
            session.append({
                "role": msg.get("role", "user"),
                "content": msg.get("content", "")
            })

        # 添加当前用户问题
        session.append({"role": "user", "content": user_query})

        return list(session)

    def add_assistant_reply(self, session_id: str, reply: str):
        """添加助手回复"""
        session = self.sessions[session_id]
        session.append({"role": "assistant", "content": reply})

    def get_history(self, session_id: str) -> list[dict]:
        """获取对话历史"""
        return list(self.sessions.get(session_id, []))

    def clear_history(self, session_id: str):
        """清空对话历史"""
        if session_id in self.sessions:
            del self.sessions[session_id]
