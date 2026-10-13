"""
知识图谱构建器
解析「课本知识点精选」4 个 Word，自动生成四科知识图谱数据。

输出：data/knowledge/knowledge_points.json（与原 sample 结构兼容，节点大幅扩充）
节点含：subject / chapter / section / importance(必考/轮考/冷门) / definition
关系：显式「vs 对比」+ 同节「相关」星型连接（保证图谱连通）
"""
import re
import json
from pathlib import Path
from docx import Document

# ---------- 路径 ----------
ROOT = Path(__file__).resolve().parent.parent
SRC_DIR = Path(r"E:\组会考研408资料\课本知识点精选")
OUT_DIR = ROOT / "data" / "knowledge"
OUT_DIR.mkdir(parents=True, exist_ok=True)

SUBJECT_FILES = {
    "os": "操作系统课本知识点.docx",
    "ds": "数据结构课本知识点精选.docx",
    "co": "计算机组成原理课本知识点.docx",
    "cn": "计算机网络课本知识点.docx",
}
SUBJECT_NAMES = {"ds": "数据结构", "co": "计算机组成原理", "os": "操作系统", "cn": "计算机网络"}

# ---------- 正则 ----------
CHAPTER_RE = re.compile(r"^第\s*([0-9]+)\s*章\s*(.*)$")
SECTION_RE = re.compile(r"^(\d+)\.(\d+)[\s.]+(.*)$")

IMPORTANCE_RE = re.compile(r"[（(]\s*(必考|轮考|冷门)\s*[）)]")
NOTE_RE = re.compile(r"[（(]\s*注[：:][^）)]*[）)]")
# 章节强调标注（删除，非内容）
MARK_RE = re.compile(r"[（(]\s*[⭐⭐⭐]?\s*(核心概念|重点章节|超级重点|大题核心区|核心中的核心|重中之重|核心|重点章节|⭐ 重点章节)?\s*[）)]")
STAR_RE = re.compile(r"⭐+")

# 跳过行（图例 / 战略总结，非知识点）
SKIP_RE = re.compile(
    r"(^[（(](必考|轮考|冷门)[）)][：:])"
    r"|(复习战略)|(^💡)|(^📚)|(^计算题[“”\"']?三|^计算题.{0,4}金刚)"
    r"|(^对策[：:])|(^概念辨析)|(^大题[“”\"']?两|^大题 预测|^大题预测)"
    r"|(^协议分析)|(^复习战略总结)|(综合应用题通常)"
)

# 复习总结区域标题（进入后，后续条目归到「复习总结」专章，不污染正式章节）
SUMMARY_TITLE_RE = re.compile(
    r"^(计算题|对策|协议分析|概念辨析|大题预测|复习战略|大题|💡|📚|综合应用题通常|问题可能涉及)"
)


def strip_annotations(text: str) -> str:
    """删除考点/注释/强调标注，保留内容性括号"""
    text = NOTE_RE.sub("", text)
    text = MARK_RE.sub("", text)
    text = STAR_RE.sub("", text)
    # 考点标注单独处理（importance 已先提取）
    text = IMPORTANCE_RE.sub("", text)
    text = re.sub(r"\*+", "", text)  # markdown 加粗残留
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def clean_term(text: str) -> str:
    """清理对比实体名：去英文/拼音括号、空格、markdown 残留"""
    text = re.sub(r"[（(][^）)]*[）)]", "", text)
    text = re.sub(r"\*+", "", text)
    text = re.sub(r"\s+", "", text)
    return text.strip(" /、，,")


def split_name_def(text: str):
    """切分名称与说明（第一个冒号）"""
    m = re.match(r"^([^：:]{1,60})[：:](.*)$", text)
    if m:
        return m.group(1).strip(), m.group(2).strip()
    return text.strip(), ""


def split_vs_terms(name: str) -> list:
    """拆分 vs 对比实体，每个实体只保留冒号前的名称部分"""
    terms = []
    for p in re.split(r"\s*(?:vs|VS|ｖｓ)\s*", name):
        # 实体名不含冒号后说明
        p = p.split("：")[0].split(":")[0]
        t = clean_term(p)
        if t:
            terms.append(t)
    return terms


def parse_docx(subject: str, fname: str, nodes: dict, name_index: dict, kp_counter: list):
    """解析单个 docx，返回该科 (节点数, 关系列表)"""
    f = SRC_DIR / fname
    if not f.exists():
        print(f"缺失: {fname}")
        return

    doc = Document(str(f))
    cur_chapter_num = 0
    cur_chapter_name = ""
    cur_section = ""
    cur_section_first = None  # 本节第一个知识点 id（星型连接）
    section_anchor = {}
    in_summary = False  # 是否处于「复习总结」区域

    def get_kp_id():
        kp_counter[0] += 1
        return f"{subject}_{cur_chapter_num:02d}_{kp_counter[0]:03d}"

    def make_node(name, definition, importance):
        cname = clean_term(name)
        if not cname or len(cname) < 1:
            return None
        # 同科同名去重
        key = (subject, cname)
        if key in name_index:
            return name_index[key]
        kid = get_kp_id()
        if in_summary:
            chapter = "复习总结"
        else:
            chapter = f"第{cur_chapter_num}章 {cur_chapter_name}".strip()
        node = {
            "id": kid,
            "name": cname,
            "subject": subject,
            "chapter": chapter,
            "section": cur_section,
            "importance": importance or "轮考",
            "definition": definition,
            "aliases": [],
            "relations": [],
        }
        nodes[kid] = node
        name_index[key] = kid
        return kid

    for para in doc.paragraphs:
        raw = para.text.strip()
        if not raw:
            continue

        # 章标题
        mc = CHAPTER_RE.match(raw)
        if mc:
            cur_chapter_num = int(mc.group(1))
            cur_chapter_name = mc.group(2).strip()
            cur_section = ""
            cur_section_first = None
            in_summary = False
            continue

        # 节标题
        ms = SECTION_RE.match(raw)
        if ms:
            cn, sn, sname = int(ms.group(1)), int(ms.group(2)), ms.group(3).strip()
            if cn != cur_chapter_num:
                cur_chapter_num = cn
                cur_chapter_name = cur_chapter_name or ""
            cur_section = f"{cn}.{sn} {sname}".strip()
            cur_section_first = None
            in_summary = False
            # 清理节名里的标注
            cur_section = strip_annotations(cur_section)
            continue

        # 复习总结区域标题（进入总结模式，优先于 SKIP_RE）
        if SUMMARY_TITLE_RE.match(raw):
            in_summary = True
            cur_section = re.split(r"[：:]", raw)[0]
            cur_section_first = None
            continue

        # 跳过图例/总结
        if SKIP_RE.search(raw):
            continue

        # 提取重要性（可能多个，取最后）
        imps = IMPORTANCE_RE.findall(raw)
        importance = imps[-1] if imps else ""
        body = strip_annotations(raw)
        if len(body) < 2:
            continue

        name, definition = split_name_def(body)

        # vs 对比：拆成两个节点
        created = []
        if re.search(r"\bvs\b|VS|ｖｓ", name):
            terms = split_vs_terms(name)
            if len(terms) >= 2:
                for t in terms:
                    kid = make_node(t, definition or body, importance)
                    if kid:
                        created.append(kid)
                # 对比关系
                for i in range(len(created)):
                    for j in range(i + 1, len(created)):
                        nodes[created[i]]["relations"].append(
                            {"target": created[j], "type": "对比"}
                        )
                        nodes[created[j]]["relations"].append(
                            {"target": created[i], "type": "对比"}
                        )
        else:
            kid = make_node(name, definition or (body if not definition else ""), importance)
            if kid:
                created.append(kid)

        # 同节星型连接
        if created:
            if cur_section_first is None:
                cur_section_first = created[0]
            else:
                anchor = cur_section_first
                for kid in created:
                    if kid != anchor:
                        rels = nodes[kid]["relations"]
                        if not any(r["target"] == anchor for r in rels):
                            rels.append({"target": anchor, "type": "相关"})


def main():
    nodes = {}
    name_index = {}
    kp_counter = [0]

    for subject, fname in SUBJECT_FILES.items():
        before = len(nodes)
        parse_docx(subject, fname, nodes, name_index, kp_counter)
        print(f"{SUBJECT_NAMES[subject]}: {len(nodes) - before} 个知识点")

    # 组装输出
    out = {
        "subjects": SUBJECT_NAMES,
        "knowledge_points": list(nodes.values()),
    }
    out_path = OUT_DIR / "knowledge_points.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=2)

    # 统计
    n_edges = sum(len(n["relations"]) for n in nodes.values())
    imp_count = {}
    for n in nodes.values():
        imp_count[n["importance"]] = imp_count.get(n["importance"], 0) + 1
    print(f"\n总节点: {len(nodes)}, 总关系: {n_edges}")
    print(f"重要性分布: {imp_count}")
    print(f"输出: {out_path}")


if __name__ == "__main__":
    main()
