"""408 owned document contract; PaddleOCR/PDF libraries are third-party engines."""
import base64
import io
import logging
import os
import posixpath
import re
import tempfile
import threading
import zipfile
from pathlib import Path
from xml.etree import ElementTree as ET

import pdfplumber
import pypdfium2 as pdfium
from PIL import Image
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile

app = FastAPI()
lock = threading.Lock()
pipeline = None
MAX_BYTES = int(os.environ.get("MAX_UPLOAD_MB", "200")) * 1024 * 1024


def png(image):
    image = image.copy().convert("RGB")
    image.thumbnail((1400, 1400))
    out = io.BytesIO()
    image.save(out, format="PNG", optimize=True)
    return "data:image/png;base64," + base64.b64encode(out.getvalue()).decode()


def box(coords, width, height):
    x0, y0, x1, y1 = map(float, coords)
    return [max(0, min(1, x0 / width)), max(0, min(1, y0 / height)),
            max(0, min(1, x1 / width)), max(0, min(1, y1 / height))]


def structure(image):
    global pipeline
    if pipeline is None:
        from paddleocr import PPStructureV3
        pipeline = PPStructureV3(
            device=os.getenv("PARSER_DEVICE", "cpu"),
            use_doc_orientation_classify=False, use_doc_unwarping=False,
            use_textline_orientation=False, use_table_recognition=True,
            use_formula_recognition=True,
        )
    import numpy as np
    result = next(iter(pipeline.predict(np.array(image.convert("RGB"))[:, :, ::-1].copy())))
    data = result.json
    if isinstance(data, str):
        import json
        data = json.loads(data)
    data = data.get("res", data)
    ocr = data.get("overall_ocr_res", {})
    ocr_texts = ocr.get("rec_texts", [])
    ocr_boxes = ocr.get("rec_boxes", [])
    blocks = []
    for i, item in enumerate(data.get("parsing_res_list", [])):
        kind = item.get("block_label", "text")
        coords = item.get("block_bbox", [0, 0, image.width, image.height])
        text = item.get("block_content", "")
        # The layout engine can concatenate lines. Preserve OCR line boundaries for numbered questions.
        if kind in ("text", "paragraph_title", "doc_title") and len(ocr_boxes) == len(ocr_texts):
            lines = [(float(b[1]), float(b[0]), t) for b, t in zip(ocr_boxes, ocr_texts)
                     if coords[0] <= (float(b[0]) + float(b[2])) / 2 <= coords[2]
                     and coords[1] <= (float(b[1]) + float(b[3])) / 2 <= coords[3]]
            if lines:
                text = "\n".join(t for _, _, t in sorted(lines))
        block = {"ID": str(i), "Kind": kind, "Text": text,
                 "Box": box(coords, image.width, image.height)}
        if kind in ("image", "figure", "chart", "table", "formula", "display_formula", "inline_formula"):
            block["Image"] = png(image.crop(tuple(coords)))
        blocks.append(block)
    if not blocks:
        raise ValueError("OCR engine produced no layout blocks")
    return blocks


def parse_pdf(path, start=1, limit=0):
    pages = []
    with pdfplumber.open(path) as document, pdfium.PdfDocument(str(path)) as renderer:
        if len(document.pages) > 2000:
            raise ValueError("PDF exceeds 2000 pages")
        for number, page in enumerate(document.pages, 1):
            if number < start or (limit and number >= start + limit):
                continue
            text = page.extract_text(layout=False) or ""
            blocks, warnings = [], []
            # Formula/complex graphical regions and scans need the layout engine.
            scanned = len(re.sub(r"\s", "", text)) < 20
            complex_page = bool(page.images or page.curves or page.lines or
                                re.search(r"[∑∫√≤≥∈∞]|\^|_{", text) or
                                any(re.search(r"math|symbol|cmsy|cmmi", c.get("fontname", ""), re.I) for c in page.chars))
            if scanned or complex_page:
                bitmap = renderer[number - 1].render(scale=1.8)
                image = bitmap.to_pil().copy()
                bitmap.close()
                try:
                    blocks = structure(image)
                    text = "\n".join(b["Text"] for b in blocks if b["Text"])
                except Exception:
                    logging.exception("page %s layout parsing failed", number)
                    warnings.append("本页OCR或图表公式识别失败，请查看原文；相关题目不进入测评。")
                    blocks = [{"ID": "original", "Kind": "image", "Text": "", "Box": [0, 0, 1, 1], "Image": png(image)}]
                image.close()
            else:
                lines = []
                for word in page.extract_words():
                    if not lines or abs(lines[-1][0]["top"] - word["top"]) > 3:
                        lines.append([])
                    lines[-1].append(word)
                for i, words in enumerate(lines):
                    blocks.append({"ID": str(i), "Kind": "text", "Text": " ".join(w["text"] for w in words),
                                   "Box": box([min(w["x0"] for w in words), min(w["top"] for w in words), max(w["x1"] for w in words), max(w["bottom"] for w in words)], page.width, page.height)})
                for table in page.find_tables():
                    rows = table.extract()
                    table_text = "\n".join(" | ".join(cell or "" for cell in row) for row in rows)
                    bitmap = renderer[number - 1].render(scale=1.5)
                    image = bitmap.to_pil().copy(); bitmap.close()
                    crop = image.crop(tuple(v * 1.5 for v in table.bbox))
                    blocks.append({"ID": "table-" + str(len(blocks)), "Kind": "table", "Text": table_text,
                                   "Box": box(table.bbox, page.width, page.height), "Image": png(crop)})
                    crop.close(); image.close()
            if not text.strip():
                warnings.append("未提取到文字（No extracted text）")
            pages.append({"Number": number, "Text": text, "Blocks": blocks, "Warnings": warnings,
                          "Width": page.width, "Height": page.height, "Rotation": page.rotation,
                          "CropBox": list(page.cropbox or page.mediabox), "MediaBox": list(page.mediabox),
                          "CoordinateSpace": "normalized-top-left-displayed-page" if not page.cropbox or list(page.cropbox) == list(page.mediabox) else "unsupported-crop"})
    return pages


def parse_docx(path, start=1, limit=0):
    blocks, warnings, texts = [], [], []
    with zipfile.ZipFile(path) as archive:
        if sum(f.file_size for f in archive.infolist()) > 100 * 1024 * 1024:
            raise ValueError("expanded DOCX too large")
        root = ET.fromstring(archive.read("word/document.xml"))
        ns = {"w": "http://schemas.openxmlformats.org/wordprocessingml/2006/main",
              "a": "http://schemas.openxmlformats.org/drawingml/2006/main",
              "r": "http://schemas.openxmlformats.org/officeDocument/2006/relationships"}
        relationships = {}
        if "word/_rels/document.xml.rels" in archive.namelist():
            for rel in ET.fromstring(archive.read("word/_rels/document.xml.rels")):
                if rel.get("Type", "").endswith("/image"):
                    relationships[rel.get("Id")] = rel

        def append_text(text, kind="text"):
            if text.strip():
                texts.append(text)
                blocks.append({"ID": str(len(blocks)), "Kind": kind, "Text": text})

        def append_image(element):
            rid = element.get("{" + ns["r"] + "}embed") or element.get("{" + ns["r"] + "}id")
            rel = relationships.get(rid)
            if rel is None or rel.get("TargetMode") == "External":
                warnings.append("DOCX图片引用缺失或为外部链接，请查看原文。")
                return
            name = posixpath.normpath(posixpath.join("word", rel.get("Target", "")))
            if not name.startswith("word/media/") or name not in archive.namelist():
                warnings.append("DOCX图片文件缺失，请查看原文。")
                return
            try:
                with Image.open(io.BytesIO(archive.read(name))) as source:
                    image = source.convert("RGB")
                try:
                    # Keep a viewable original even when recognition fails or yields text only.
                    blocks.append({"ID": str(len(blocks)), "Kind": "image", "Text": "", "Image": png(image)})
                    try:
                        image_blocks = structure(image)
                    except Exception:
                        logging.exception("DOCX image recognition failed")
                        warnings.append("DOCX图片识别失败，已保留原图；相关题目暂不进入测评。")
                        return
                    for item in image_blocks:
                        block = dict(item)
                        block["ID"] = str(len(blocks))
                        blocks.append(block)
                        if block.get("Text"):
                            texts.append(block["Text"])
                finally:
                    image.close()
            except Exception:
                logging.exception("DOCX image decoding failed")
                warnings.append("DOCX图片无法解码，请查看原文件；相关题目暂不进入测评。")

        nodes = root.findall(".//w:body/*", ns)
        selected = nodes[start-1:start-1+limit] if limit else nodes
        for node in selected:
            kind = "table" if node.tag.endswith("}tbl") else "text"
            if kind == "table":
                text = "\n".join(" | ".join(
                    " ".join("".join(t.text or "" for t in p.findall(".//w:t", ns))
                             for p in c.findall(".//w:p", ns))
                    for c in row.findall("w:tc", ns)) for row in node.findall("w:tr", ns))
                append_text(text, "table")
                for element in node.iter():
                    if element.tag in ("{" + ns["a"] + "}blip", "{urn:schemas-microsoft-com:vml}imagedata"):
                        append_image(element)
            else:
                pending = []
                for element in node.iter():
                    if element.tag == "{" + ns["w"] + "}t":
                        pending.append(element.text or "")
                    elif element.tag == "{" + ns["w"] + "}br":
                        pending.append("\n")
                    elif element.tag == "{" + ns["w"] + "}tab":
                        pending.append("\t")
                    elif element.tag in ("{" + ns["a"] + "}blip", "{urn:schemas-microsoft-com:vml}imagedata"):
                        append_text("".join(pending))
                        pending.clear()
                        append_image(element)
                append_text("".join(pending))
        if any("oMath" in n.tag for node in selected for n in node.iter()):
            warnings.append("DOCX原生公式暂不转换，请导出PDF以保留公式版面。")
    return [{"Number": 0, "Text": "\n".join(texts), "Blocks": blocks, "Warnings": warnings}]


@app.get("/healthz")
def health():
    return {"status": "ok", "layout_loaded": pipeline is not None, "engine": "PP-StructureV3"}


@app.post("/parse")
def parse(file: UploadFile = File(...), authorization: str = Header(default=""),
          start_page: int = Form(default=1), page_limit: int = Form(default=0)):
    token = os.getenv("PARSER_TOKEN", "")
    if not token or authorization != "Bearer " + token:
        raise HTTPException(401, "Unauthorized")
    extension = Path(file.filename or "").suffix.lower()
    if extension not in (".pdf", ".docx"):
        raise HTTPException(400, "Unsupported document")
    if start_page < 1 or page_limit < 0 or page_limit > 16:
        raise HTTPException(400, "Invalid page range")
    data = file.file.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise HTTPException(413, "Document too large")
    if not lock.acquire(blocking=False):
        raise HTTPException(503, "Parser busy; retry later")
    try:
        with tempfile.TemporaryDirectory(prefix="408-parser-") as directory:
            path = Path(directory) / ("input" + extension)
            path.write_bytes(data)
            if extension == ".pdf":
                with pdfium.PdfDocument(str(path)) as document:
                    total = len(document)
                pages = parse_pdf(path, start_page, page_limit)
            else:
                with zipfile.ZipFile(path) as archive:
                    if sum(f.file_size for f in archive.infolist()) > 100 * 1024 * 1024:
                        raise ValueError("expanded DOCX too large")
                    root = ET.fromstring(archive.read("word/document.xml"))
                    total = len(root.findall(".//{http://schemas.openxmlformats.org/wordprocessingml/2006/main}body/*"))
                if page_limit:
                    # Rescan the ZIP, but recognize only this bounded logical segment.
                    total = (total + 15) // 16
                    pages = parse_docx(path, (start_page - 1) * 16 + 1, 16)
                else:
                    pages = parse_docx(path)
            if start_page > total:
                raise ValueError("page range exceeds document")
            return {"Engine": "408-structure-v2/paddleocr-3.3.2", "Pages": pages,
                    "TotalPages": total if page_limit else len(pages)}
    except Exception:
        logging.exception("document parse failed")
        raise HTTPException(422, "Cannot parse document")
    finally:
        lock.release()


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8091)
