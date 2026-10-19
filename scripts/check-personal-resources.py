"""Opt-in live acceptance: creates a disposable account and synthetic files only.
Uses the configured text model once. Leaves account ID in .cache for explicit cleanup.
Run with the document_parser virtual environment after starting the application.
"""
import json
import time
import uuid
from pathlib import Path
import requests
from PIL import Image, ImageDraw, ImageFont

root = Path(__file__).resolve().parents[1]
cache = root / ".cache"
cache.mkdir(exist_ok=True)
api = "http://127.0.0.1:8080/api/v1"
session = requests.Session()


def call(method, path, **kwargs):
    response = session.request(method, api + path, timeout=110, **kwargs)
    response.raise_for_status()
    return response.json()


username = "personal_test_" + uuid.uuid4().hex[:8]
password = "Synthetic-408-Only!"
auth = call("POST", "/auth/register", json={"Username": username, "Email": username + "@example.invalid", "Password": password})
session.headers["Authorization"] = "Bearer " + auth["token"]
state = {"username": username, "password": password, "user_id": auth["user"]["id"], "token": auth["token"]}
(cache / "personal-test-state.json").write_text(json.dumps(state), encoding="utf-8")
kb = call("POST", "/knowledge-bases", json={"Name": "合成工程验收", "Description": "非真实教材"})
state["kb_id"] = kb["ID"]

image = Image.new("RGB", (1200, 800), "white")
draw = ImageDraw.Draw(image)
font = ImageFont.truetype("C:/Windows/Fonts/simsun.ttc", 36)
for y, text in enumerate(["合成扫描试题（非教材）", "1. 下列关于内存管理的描述哪一个正确？", "A. 虚拟地址需要转换", "B. 虚拟地址不需要转换", "答案：A", "解析：这是工程验收样本。"]):
    draw.text((60, 50 + y * 85), text, font=font, fill="black")
scan = cache / "synthetic-scan.pdf"
image.save(scan, "PDF", resolution=144)
with scan.open("rb") as file:
    doc = call("POST", f"/knowledge-bases/{kb['ID']}/documents", files={"file": ("synthetic-scan.pdf", file, "application/pdf")}, data={"resource_type": "notes"})
state["document_id"] = doc["ID"]
(cache / "personal-test-state.json").write_text(json.dumps(state), encoding="utf-8")
for _ in range(90):
    docs = call("GET", f"/knowledge-bases/{kb['ID']}/documents")
    current = next(d for d in docs if d["ID"] == doc["ID"])
    if current["Status"] in ("ready", "failed"):
        break
    time.sleep(5)
assert current["Status"] == "ready", current.get("ErrorMessage")
parsed = call("GET", f"/documents/{doc['ID']}/structure")
assert "内存" in parsed["Pages"][0]["Text"], parsed
assert not parsed["Pages"][0]["Warnings"], parsed["Pages"][0]["Warnings"]
assets = call("GET", "/exam408/assets")
assert any(a["Payload"]["DocumentID"] == doc["ID"] and a["Status"] == "ready" for a in assets), assets
points = call("GET", "/exam408/knowledge-points")
point = next(p for p in points if p["Name"] == "内存管理")
unknown = call("POST", "/exam408/assets", json={"Kind": "question", "PointID": point["ID"], "Title": "无来源答案合成题", "Payload": {"Stem": "虚拟内存地址是否通常需要转换为物理地址？", "Options": ["需要", "不需要"], "Answer": -1, "Difficulty": 1, "Source": "工程合成测试"}})
exam = call("POST", "/exam408/assessments", json={"Kind": "practice", "AssetID": unknown["ID"]})
result = call("POST", f"/exam408/assessments/{exam['ID']}/submit", json={"Answers": {unknown["ID"]: 0}})
assert result["Counted"] == 0
prefix = f"/exam408/assessments/{exam['ID']}/questions/{unknown['ID']}"
result = call("POST", prefix + "/infer", json={})
assert result["Counted"] == 0 and result["Items"][0]["AnswerOrigin"] == "ai"
for decision in ("correct", "wrong", "uncertain"):
    result = call("POST", prefix + "/confirm", json={"Decision": decision})
    assert result["Counted"] == (0 if decision == "uncertain" else 1)
    radar = call("GET", "/exam408/radar")
    assert sum(s["Tested"] for s in radar) == result["Counted"]
state["assessment_id"] = exam["ID"]
(cache / "personal-test-state.json").write_text(json.dumps(state), encoding="utf-8")
report = {"scan_ocr": "passed", "automatic_question": "passed", "real_text_model_inference": "passed", "confirmation_withdrawal": "passed", "user_id": state["user_id"], "username": username}
(cache / "personal-acceptance.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
print(json.dumps(report, ensure_ascii=False), flush=True)
