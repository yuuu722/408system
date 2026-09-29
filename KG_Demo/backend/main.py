"""
408考研知识图谱问答系统 - 后端服务
FastAPI + SSE 流式问答接口
"""
import sys
import os
import json
import asyncio
from pathlib import Path
from typing import Optional

# ---------- 项目根目录 ----------
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
sys.path.insert(0, str(PROJECT_ROOT / "RAG-model-V3"))
sys.path.insert(0, str(PROJECT_ROOT))

# ---------- Windows 编码修复 ----------
if sys.platform == 'win32':
    if hasattr(sys.stdout, 'reconfigure'):
        sys.stdout.reconfigure(encoding='utf-8', errors='replace')
    if hasattr(sys.stderr, 'reconfigure'):
        sys.stderr.reconfigure(encoding='utf-8', errors='replace')

from fastapi import FastAPI, Request, UploadFile, File, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse, FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

# ---------- 导入 RAG 模块 ----------
from config import get_config
from gateway import SemanticGateway
from searcher import KnowledgeSearcher
from generator import ResponseGenerator
from conversation import ConversationManager
from cache import ResponseCache

# ---------- FastAPI App ----------
app = FastAPI(title="CS408 Knowledge Graph QA System", version="1.0.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=False,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ---------- 全局组件 ----------
_config = None
_gateway: Optional[SemanticGateway] = None
_searcher: Optional[KnowledgeSearcher] = None
_generator: Optional[ResponseGenerator] = None
_conversation: Optional[ConversationManager] = None
_cache: Optional[ResponseCache] = None


def init_components():
    """初始化所有组件（懒加载）"""
    global _config, _gateway, _searcher, _generator, _conversation, _cache
    if _gateway is not None:
        return

    _config = get_config()
    _gateway = SemanticGateway(_config)
    _searcher = KnowledgeSearcher(_config)
    _generator = ResponseGenerator(_config)
    _conversation = ConversationManager(max_history=5)
    _cache = ResponseCache(ttl=86400)
    print("[System] 所有组件初始化完成")


# ---------- 请求模型 ----------
class ChatMessage(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    query: str = Field(max_length=2000, description="用户问题")
    history: list[ChatMessage] = Field(default_factory=list, max_length=20)
    session_id: Optional[str] = Field(default=None, max_length=128)


# ---------- SSE 辅助 ----------
def sse_event(data: dict) -> str:
    """生成 SSE 事件字符串"""
    return f"data: {json.dumps(data, ensure_ascii=False)}\n\n"


# ---------- API 路由 ----------
@app.get("/api/health")
async def health_check():
    """健康检查接口"""
    return {
        "status": "ok",
        "service": "cs408-qa-system",
        "version": "1.0.0",
        "components": {
            "gateway": _gateway is not None,
            "searcher": _searcher is not None,
            "generator": _generator is not None,
        }
    }


@app.post("/api/chat")
async def chat(request: ChatRequest):
    """流式问答接口（SSE）"""
    init_components()

    async def event_stream():
        try:
            # 1. 更新对话历史
            history = _conversation.update_history(
                request.session_id or "default",
                request.query,
                [{"role": m.role, "content": m.content} for m in request.history]
            )

            # 2. 检查缓存
            cache_key = _cache.make_key(request.query, history)
            cached = _cache.get(cache_key)
            if cached:
                yield sse_event({"type": "cached", "data": cached})
                yield sse_event({"type": "done"})
                return

            # 3. 语义网关：意图识别 + 实体提取
            yield sse_event({"type": "status", "data": "正在理解问题..."})
            intent_result = await _gateway.analyze(request.query, history)

            # 4. 知识检索
            yield sse_event({"type": "status", "data": "正在检索知识点..."})
            search_result = await _searcher.search(
                query=request.query,
                entities=intent_result.entities,
                intent=intent_result.intent
            )

            # 5. 流式生成回答
            yield sse_event({"type": "status", "data": "正在生成回答..."})
            evidence_data = [
                {
                    "id": ev.id,
                    "title": ev.title,
                    "content": ev.content,
                    "source": ev.source,
                    "subject": ev.subject,
                    "relevance": ev.relevance
                }
                for ev in search_result.evidence
            ]
            yield sse_event({"type": "evidence", "data": evidence_data})

            full_answer = ""
            async for chunk in _generator.generate(
                query=request.query,
                intent=intent_result,
                search_result=search_result,
                history=history
            ):
                full_answer += chunk
                yield sse_event({"type": "token", "data": chunk})

            # 6. 缓存结果
            _cache.set(cache_key, {
                "answer": full_answer,
                "evidence": evidence_data
            })

            yield sse_event({"type": "done", "data": {"answer": full_answer}})

        except Exception as e:
            yield sse_event({"type": "error", "data": str(e)})

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
        }
    )


@app.get("/api/knowledge-graph")
async def get_knowledge_graph(subject: Optional[str] = None):
    """获取知识图谱数据"""
    init_components()
    graph_data = await _searcher.get_graph_data(subject=subject)
    return graph_data


@app.get("/api/subjects")
async def get_subjects():
    """获取所有课程科目"""
    return {
        "subjects": [
            {"id": "ds", "name": "数据结构", "icon": "tree"},
            {"id": "co", "name": "计算机组成原理", "icon": "cpu"},
            {"id": "os", "name": "操作系统", "icon": "settings"},
            {"id": "cn", "name": "计算机网络", "icon": "network"},
        ]
    }


# ---------- 知识库文件上传 ----------

UPLOAD_DIR = PROJECT_ROOT / "data" / "documents"
UPLOAD_DIR.mkdir(parents=True, exist_ok=True)

ALLOWED_EXTENSIONS = {".pdf", ".txt", ".md", ".docx"}


@app.post("/api/knowledge/upload")
async def upload_knowledge_file(file: UploadFile = File(...)):
    """上传408知识库资料"""

    if not file.filename:
        raise HTTPException(status_code=400, detail="文件名不能为空")

    suffix = Path(file.filename).suffix.lower()

    if suffix not in ALLOWED_EXTENSIONS:
        raise HTTPException(
            status_code=400,
            detail="暂只支持 PDF、TXT、Markdown、DOCX 文件"
        )

    save_path = UPLOAD_DIR / file.filename

    content = await file.read()

    with open(save_path, "wb") as f:
        f.write(content)

    return {
        "success": True,
        "filename": file.filename,
        "size": len(content),
        "path": str(save_path.relative_to(PROJECT_ROOT))
    }


@app.get("/api/knowledge/files")
async def get_knowledge_files():
    """获取已经上传的知识库文件"""

    files = []

    if UPLOAD_DIR.exists():
        for file_path in UPLOAD_DIR.iterdir():
            if file_path.is_file():
                files.append({
                    "filename": file_path.name,
                    "size": file_path.stat().st_size,
                    "suffix": file_path.suffix.lower()
                })

    return {
        "success": True,
        "files": files
    }

# ---------- 前端静态文件托管（生产模式，无需 Node.js） ----------
FRONTEND_DIST = PROJECT_ROOT / "KG_Demo" / "frontend" / "dist"

if FRONTEND_DIST.exists():
    # 托管 js/css/图片等静态资源
    app.mount(
        "/assets",
        StaticFiles(directory=str(FRONTEND_DIST / "assets")),
        name="assets"
    )

    @app.get("/")
    async def index():
        """返回前端首页"""
        return FileResponse(str(FRONTEND_DIST / "index.html"))


# ---------- 启动入口 ----------
if __name__ == "__main__":
    import uvicorn
    init_components()
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=8000,
        reload=True,
        log_level="info"
    )
