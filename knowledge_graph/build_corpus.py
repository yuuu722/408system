"""
RAG 语料提取器
从文字版 PDF（原课本 + 王道综合题/真题分类做题本）提取文本并智能分块。

输出：data/corpus/chunks.json
每块含：subject / source / type / page / chapter / text
"""
import re
import json
import pathlib
import fitz

# ---------- 路径 ----------
ROOT = pathlib.Path(__file__).resolve().parent.parent
BASE = pathlib.Path(r"E:\组会考研408资料")
OUT_DIR = ROOT / "data" / "corpus"
OUT_DIR.mkdir(parents=True, exist_ok=True)

# ---------- 语料源 ----------
SOURCES = [
    # 原课本（文字版）
    ("ds", r"原课本\数据结构\07.《数据结构》（C语言版） (严蔚敏 吴伟民) .pdf", "textbook", "严蔚敏数据结构"),
    ("co", r"原课本\计算机组成原理\01.计算机组成原理第3版 (唐朔飞) .pdf", "textbook", "唐朔飞计算机组成原理"),
    # 真题分类（四科，按知识点组织，针对性强）
    ("ds", r"王道版综合题真题分类（知识针对性强）\单本\26王道版数据结构综合题真题分类做题本.pdf", "zhenti", "数据结构真题分类"),
    ("co", r"王道版综合题真题分类（知识针对性强）\单本\26王道版计组综合题真题分类做题本.pdf", "zhenti", "计组真题分类"),
    ("os", r"王道版综合题真题分类（知识针对性强）\单本\26王道版操作系统综合题真题分类做题本.pdf", "zhenti", "操作系统真题分类"),
    ("cn", r"王道版综合题真题分类（知识针对性强）\单本\26王道版计网综合题真题分类做题本.pdf", "zhenti", "计网真题分类"),
    # 应用题做题本（四科，覆盖面广）
    ("ds", r"王道综合题做题本（知识覆盖面广）\数据结构综合题做题本\【A4版】2026WD数据结构应用题做题本.pdf", "yingyong", "数据结构应用题"),
    ("co", r"王道综合题做题本（知识覆盖面广）\计算机组成原理综合题做题本\【分享版】2026WD计算机组成原理应用题做题本.pdf", "yingyong", "计组应用题"),
    ("os", r"王道综合题做题本（知识覆盖面广）\操作系统综合题做题本\【分享版】2026WD操作系统应用题做题本.pdf", "yingyong", "操作系统应用题"),
    ("cn", r"王道综合题做题本（知识覆盖面广）\计算机网络综合题做题本\【分享版】2026计算机网络应用题做题本.pdf", "yingyong", "计网应用题"),
]

# ---------- 分块参数 ----------
TARGET_CHARS = 450       # 目标块长
MAX_CHARS = 650          # 最大块长
MIN_CHARS = 40           # 过短块忽略

# 做题本切分点：章节号 / 题号 / 真题标记
SPLIT_MARK_RE = re.compile(
    r"(\d+\.\d+\.\d+[^\n]{0,30})"
    r"|(\(\d{1,2}\)\s)"
    r"|(【\d{4}\s*统考真题】)"
    r"|(应用题[（(]原书P\d+[）)])"
)

# 噪声行：目录填充 / 水印 / 版权信息
NOISE_LINE_RE = re.compile(
    r"(公众号)|(做题本集结地)|(第\s*\d+\s*页[，,]\s*共)|(盗版|侵权|举报)"
    r"|(印刷者)|(装订者)|(经销\s*[:：])|(投稿与读者)|(质量反馈)"
    r"|(版权所有)|(定\s*价)|(产品编号)|(印\s*次)|(印\s*数)|(开\s*本)"
    r"|(印\s*张)|(附光盘)|(邮\s*编)|(邮\s*购)|(by\s*本本)"
    r"|(WD.{0,10}做题本)|(王道版.{0,10}真题分类)|(26\s*王道版)"
)


def is_noise(line: str) -> bool:
    """判断是否为目录/水印/版权噪声行"""
    # 目录填充：大量私有区字符（\uf001 等）
    if len(re.findall(r"[\ue000-\uf8ff]", line)) >= 5:
        return True
    if NOISE_LINE_RE.search(line):
        return True
    # 目录条目：章节号开头，中间大量分隔符，结尾页码
    if re.match(r"^\d+\.\d+\.\d+", line) and re.search(r"[.·\s_]{5,}\d+\s*$", line):
        return True
    # 连续点/下划线填充
    if re.search(r"\.{4,}|_{4,}|·{4,}", line):
        return True
    return False


def clean_line(line: str) -> str:
    """清理私有区图标字符（正文行保留时）"""
    return re.sub(r"[\ue000-\uf8ff]", "", line).strip()


def extract_pages(f: pathlib.Path):
    """提取每页文本，返回 [(page_num, text)]"""
    doc = fitz.open(str(f))
    pages = []
    for i, page in enumerate(doc):
        text = page.get_text().strip()
        text = re.sub(r"[ \t]+", " ", text)
        text = re.sub(r"\n{2,}", "\n", text)
        if text:
            pages.append((i + 1, text))
    doc.close()
    return pages


def chunk_textbook(pages, source_name):
    """课本按段落聚合 + 固定长度切分"""
    chunks = []
    buf = ""
    buf_page = pages[0][0] if pages else 0

    def flush():
        nonlocal buf
        if len(buf.strip()) >= MIN_CHARS:
            chunks.append({"page": buf_page, "text": buf.strip()})
        buf = ""

    for page, text in pages:
        # 跳过封面/目录噪声（前几页）
        paras = text.split("\n")
        for para in paras:
            para = clean_line(para)
            if not para or is_noise(para):
                continue
            if not buf:
                buf_page = page
            if len(buf) + len(para) > TARGET_CHARS and buf:
                flush()
                buf_page = page
            buf += para + "\n"
            if len(buf) >= MAX_CHARS:
                flush()
    flush()
    return chunks


def chunk_workbook(pages, source_name):
    """做题本按题号/真题边界切分"""
    full = "\n".join(t for _, t in pages)
    page_map = []  # 不可靠，页码用起始页
    # 记录起始页
    start_page = pages[0][0] if pages else 0

    # 在切分点处断开
    # 先按行处理，识别标记行
    units = []
    cur = ""
    cur_page = start_page

    for page, text in pages:
        lines = text.split("\n")
        for line in lines:
            line = clean_line(line)
            if not line or is_noise(line):
                continue
            is_mark = bool(
                re.match(r"^\d+\.\d+\.\d+", line)
                or re.match(r"^\(\d{1,2}\)", line)
                or re.match(r"^【\d{4}", line)
                or "应用题(原书" in line or "应用题（原书" in line
            )
            if is_mark:
                if len(cur.strip()) >= MIN_CHARS:
                    units.append((cur_page, cur.strip()))
                cur = line + "\n"
                cur_page = page
            else:
                cur += line + "\n"
                if len(cur) >= MAX_CHARS:
                    units.append((cur_page, cur.strip()))
                    cur = ""
    if len(cur.strip()) >= MIN_CHARS:
        units.append((cur_page, cur.strip()))

    return [{"page": p, "text": t} for p, t in units]


def main():
    all_chunks = []
    counter = 0
    stats = {}

    for subject, rel, ctype, sname in SOURCES:
        f = BASE / rel
        if not f.exists():
            print(f"缺失: {rel}")
            continue
        pages = extract_pages(f)
        if ctype == "textbook":
            raw = chunk_textbook(pages, sname)
        else:
            raw = chunk_workbook(pages, sname)

        n = 0
        for item in raw:
            counter += 1
            cid = f"{subject}_{counter:05d}"
            all_chunks.append({
                "id": cid,
                "subject": subject,
                "source": sname,
                "type": ctype,
                "page": item["page"],
                "chapter": "",
                "text": item["text"],
            })
            n += 1
        stats[sname] = n
        print(f"{sname}: {n} 块")

    out = {"chunks": all_chunks}
    out_path = OUT_DIR / "chunks.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False)

    total_chars = sum(len(c["text"]) for c in all_chunks)
    sub_stats = {}
    for c in all_chunks:
        sub_stats[c["subject"]] = sub_stats.get(c["subject"], 0) + 1
    print(f"\n总块数: {len(all_chunks)}, 总字数: {total_chars}")
    print(f"各科块数: {sub_stats}")
    print(f"输出: {out_path}")


if __name__ == "__main__":
    main()
