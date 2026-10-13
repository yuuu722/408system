"""
文档分块器
采用「标题/章节优先 + 长度约束 + 重叠」策略，将文档切分为适合检索的 Chunk。
"""
import re
from dataclasses import dataclass, field

from .loader import Document


@dataclass
class Chunk:
    """文本块"""
    id: str
    text: str
    metadata: dict = field(default_factory=dict)


# 章节标题模式
HEADING_PATTERNS = [
    re.compile(r"^第[一二三四五六七八九十百\d]+[章篇讲]\s*"),
    re.compile(r"^第[一二三四五六七八九十百\d]+节\s*"),
    re.compile(r"^\d+\.\d+(\.\d+)?\s+\S"),
    re.compile(r"^#{1,4}\s+"),
    re.compile(r"^[（(]?[一二三四五六七八九十\d]+[)）]?\s*[\u4e00-\u9fa5]{2,}$"),
]


def is_heading(line: str) -> bool:
    line = line.strip()
    if not line or len(line) > 40:
        return False
    return any(p.match(line) for p in HEADING_PATTERNS)


def split_by_heading(text: str) -> list[tuple[str, str]]:
    """按标题切分，返回 (标题, 内容) 列表"""
    sections = []
    current_heading = ""
    buffer = []
    for line in text.splitlines():
        if is_heading(line):
            if buffer:
                sections.append((current_heading, "\n".join(buffer)))
            current_heading = line.strip()
            buffer = []
        else:
            buffer.append(line)
    if buffer:
        sections.append((current_heading, "\n".join(buffer)))
    return sections


def split_long_text(text: str, max_len: int) -> list[str]:
    """将长文本按段落/句子进一步切到不超过 max_len"""
    if len(text) <= max_len:
        return [text]

    pieces = []
    # 先按段落
    for para in re.split(r"\n\s*\n", text):
        if len(para) <= max_len:
            pieces.append(para)
        else:
            # 按句子切
            sentences = re.split(r"(?<=[。！？；\n])", para)
            buf = ""
            for s in sentences:
                if len(buf) + len(s) <= max_len:
                    buf += s
                else:
                    if buf:
                        pieces.append(buf)
                    # 单句超长则硬切
                    while len(s) > max_len:
                        pieces.append(s[:max_len])
                        s = s[max_len:]
                    buf = s
            if buf:
                pieces.append(buf)
    return pieces


def chunk_documents(documents: list[Document], max_len: int = 500,
                    overlap: int = 50) -> list[Chunk]:
    """将文档分块"""
    chunks = []
    counter = 0

    for doc in documents:
        base_meta = dict(doc.metadata)
        sections = split_by_heading(doc.text)
        if not sections:
            sections = [("", doc.text)]

        for heading, content in sections:
            content = content.strip()
            if not content:
                continue
            chapter = heading or base_meta.get("chapter", "")
            pieces = split_long_text(content, max_len)

            for i, piece in enumerate(pieces):
                piece = piece.strip()
                if not piece:
                    continue
                # 重叠：除第一块外，拼上一块末尾
                if overlap and i > 0:
                    prev = pieces[i - 1]
                    tail = prev[-overlap:]
                    piece = tail + piece

                meta = dict(base_meta)
                meta["chapter"] = chapter
                meta["heading"] = heading
                chunk_id = f"chunk_{counter:06d}"
                chunks.append(Chunk(id=chunk_id, text=piece, metadata=meta))
                counter += 1

    return chunks
