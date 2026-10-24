#!/usr/bin/env python3
"""Local, resumable 408 folder import. Never sends files to GitHub or a model."""
import argparse
import csv
import getpass
import hashlib
import http.client
import json
import mimetypes
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

TYPES = {"textbook", "past_exam", "solution", "courseware", "notes", "other"}
SUPPORTED = {".pdf", ".docx", ".txt", ".md", ".markdown"}
RULES = [
    ("solution", r"解析|详解|答案|解答|solution|answer"),
    ("past_exam", r"真题|历年|统考|past.?exam"),
    ("textbook", r"教材|教科书|课本|textbook|computer textbook"),
    ("courseware", r"课件|讲义|幻灯|courseware|slides"),
    ("notes", r"笔记|总结|复习|提纲|notes"),
]


def classify(path, root):
    # Specific file labels override directory labels; ambiguous files remain "other".
    for label in [path.stem] + list(reversed(path.relative_to(root).parts[:-1])):
        for kind, pattern in RULES:
            if re.search(pattern, label, re.I):
                return kind
    return "other"


def inventory(root):
    if not root.is_dir():
        raise ValueError("资料目录不存在：" + str(root))
    files = []
    for p in sorted(root.rglob("*")):
        if p.is_symlink():
            raise ValueError("请先移除资料目录中的符号链接：" + str(p))
        if p.is_file() and not p.name.startswith("~$"):
            # Reading and hashing every file happens before any remote mutation.
            h = hashlib.sha256()
            with p.open("rb") as stream:
                for block in iter(lambda: stream.read(1024 * 1024), b""):
                    h.update(block)
            files.append({"Path": str(p), "RelativePath": str(p.relative_to(root)),
                          "ResourceType": classify(p, root), "Bytes": p.stat().st_size,
                          "SHA256": h.hexdigest(), "Supported": p.suffix.lower() in SUPPORTED})
    if not files:
        raise ValueError("资料目录为空，未清理任何公共资料")
    return files


def save_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_suffix(".tmp")
    temp.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
    temp.replace(path)


class API:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.token = ""

    def request(self, method, route, data=None):
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        body = None if data is None else json.dumps(data).encode("utf-8")
        req = urllib.request.Request(self.base + route, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=90) as response:
                raw = response.read()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as e:
            detail = e.read(4096).decode("utf-8", errors="replace")
            raise RuntimeError(f"{method} {route}: HTTP {e.code}: {detail}") from None

    def upload(self, kb, item):
        p = Path(item["Path"])
        boundary = "exam408" + uuid.uuid4().hex
        filename = urllib.parse.quote(p.name, safe="")
        head = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"resource_type\"\r\n\r\n"
                f"{item['ResourceType']}\r\n--{boundary}\r\n"
                f"Content-Disposition: form-data; name=\"file\"; filename*=UTF-8''{filename}\r\n"
                f"Content-Type: {mimetypes.guess_type(p.name)[0] or 'application/octet-stream'}\r\n\r\n").encode()
        tail = f"\r\n--{boundary}--\r\n".encode()
        target = urllib.parse.urlsplit(self.base + f"/knowledge-bases/{kb}/documents")
        connection = (http.client.HTTPSConnection if target.scheme == "https" else http.client.HTTPConnection)(
            target.hostname, target.port, timeout=1800)
        try:
            connection.putrequest("POST", target.path)
            connection.putheader("Authorization", "Bearer " + self.token)
            connection.putheader("Content-Type", "multipart/form-data; boundary=" + boundary)
            connection.putheader("Content-Length", str(len(head) + item["Bytes"] + len(tail)))
            connection.endheaders()
            connection.send(head)
            with p.open("rb") as stream:
                for block in iter(lambda: stream.read(65536), b""):
                    connection.send(block)
            connection.send(tail)
            response = connection.getresponse()
            raw = response.read()
            if response.status != 202:
                raise RuntimeError(f"上传 {p.name}: HTTP {response.status}: {raw[:4096].decode('utf-8', errors='replace')}")
            return json.loads(raw)
        finally:
            connection.close()


def run(args):
    root = Path(args.source).resolve()
    work = Path(args.report_dir).resolve()
    # Reports must not become additional inputs on the next run.
    if work == root or root in work.parents:
        raise ValueError("导入记录目录不能放在资料目录内")
    items = inventory(root)
    work.mkdir(parents=True, exist_ok=True)
    plan_path = work / "classification.csv"
    if args.plan:
        overrides = {row["RelativePath"]: row["ResourceType"] for row in csv.DictReader(
            Path(args.plan).open(encoding="utf-8-sig", newline=""))}
        if set(overrides) != {item["RelativePath"] for item in items}:
            raise ValueError("分类清单与当前文件不一致，请重新生成清单")
        for item in items:
            item["ResourceType"] = overrides[item["RelativePath"]]
    for item in items:
        if item["ResourceType"] not in TYPES:
            raise ValueError("无效分类：" + item["RelativePath"])
    if not args.plan or Path(args.plan).resolve() != plan_path:
        with plan_path.open("w", encoding="utf-8-sig", newline="") as out:
            writer = csv.DictWriter(out, fieldnames=list(items[0]))
            writer.writeheader()
            writer.writerows(items)
    print(f"共 {len(items)} 个文件；分类清单：{plan_path}")
    for kind in sorted(TYPES):
        print(f"  {kind}: {sum(i['ResourceType'] == kind for i in items)}")
    unsupported = [i for i in items if not i["Supported"] or i["Bytes"] == 0]
    if unsupported:
        print("以下文件格式暂不支持或为空；请解压压缩包、转换文件或移出资料目录后重试：")
        for item in unsupported:
            print("  " + item["RelativePath"])
        if args.apply:
            raise ValueError("未导入、未清理旧资料：存在不支持的文件")
    if not args.apply:
        print("仅生成清单，未登录、未上传、未删除。请核对 ResourceType；可以编辑 CSV 后用 --plan 指定。")
        return
    if not args.replace:
        raise ValueError("整库替换需同时指定 --apply --replace")
    target = urllib.parse.urlsplit(args.api)
    if target.scheme not in {"http", "https"} or target.hostname not in {"localhost", "127.0.0.1", "::1"}:
        raise ValueError("本机导入仅允许连接 localhost/127.0.0.1，避免将管理员凭据发往其他地址")
    api = API(args.api)
    login = input("管理员邮箱或用户名：").strip()
    password = getpass.getpass("管理员密码（不显示）：")
    result = api.request("POST", "/auth/login", {"Login": login, "Password": password})
    del password
    api.token = result["token"]
    if result["user"]["role"] != "admin":
        raise ValueError("必须使用管理员账号，未更改任何资料")
    bases = api.request("GET", "/knowledge-bases") or []
    chosen = [b for b in bases if b["IsPublic"] and
              (b["ID"] == args.kb_id if args.kb_id else b["Name"] == "408 系统公共知识库")]
    if len(chosen) != 1:
        raise ValueError("无法唯一确定 408 系统公共知识库，请通过 --kb-id 指定公共库 ID")
    kb = chosen[0]["ID"]
    cap = api.request("GET", "/system/capabilities")
    oversize = [i for i in items if i["Bytes"] > cap["max_upload_bytes"]]
    if oversize:
        raise ValueError("以下文件超过服务端上传限制，未上传或删除：" + ", ".join(i["RelativePath"] for i in oversize))
    journal = work / "import-state.json"
    fingerprint = [{k: i[k] for k in ("RelativePath", "SHA256", "ResourceType")} for i in items]
    if journal.exists():
        state = json.loads(journal.read_text(encoding="utf-8"))
        if (state["Source"], state["API"], state["KB"], state["Files"]) != (str(root), api.base, kb, fingerprint):
            raise ValueError("导入记录不匹配。请使用新的 --report-dir，避免混淆不同批次")
        if state.get("Complete"):
            print("该批次已完成，无需重复导入")
            return
    else:
        old_docs = api.request("GET", f"/knowledge-bases/{kb}/documents") or []
        old_ids = {d["ID"] for d in old_docs}
        assets = api.request("GET", "/exam408/assets") or []
        state = {"Source": str(root), "API": api.base, "KB": kb, "Files": fingerprint,
                 "OldDocuments": old_docs, "OldAssets": [a for a in assets if a["IsPublic"] and
                 (not a.get("Payload", {}).get("DocumentID") or a["Payload"]["DocumentID"] in old_ids)], "Uploaded": {}, "Deleted": [], "Complete": False}
        save_json(journal, state)
    print(f"目标：{chosen[0]['Name']}；旧文件 {len(state['OldDocuments'])} 份。新文件全部就绪后才删除旧资源。")
    for index, item in enumerate(items, 1):
        key = item["RelativePath"]
        if key not in state["Uploaded"]:
            print(f"上传 {index}/{len(items)}：{key} ({item['ResourceType']})", flush=True)
            document = api.upload(kb, item)
            state["Uploaded"][key] = document["ID"]
            save_json(journal, state)
    end = time.monotonic() + args.wait_hours * 3600
    while True:
        documents = {d["ID"]: d for d in api.request("GET", f"/knowledge-bases/{kb}/documents") or []}
        new = [documents.get(did, {"Status": "missing", "OriginalFilename": path}) for path, did in state["Uploaded"].items()]
        save_json(work / "processing-results.json", new)
        failed = [d for d in new if d["Status"] in {"failed", "missing"}]
        if failed:
            for d in failed:
                print(d["OriginalFilename"], d.get("ErrorMessage", "文件已被删除"))
            raise RuntimeError("新资料有处理失败，旧资源全部保留。修复并在网页重试失败任务，再运行同一命令继续")
        ready = sum(d["Status"] == "ready" for d in new)
        print(f"处理就绪 {ready}/{len(new)}", flush=True)
        if ready == len(new):
            break
        if time.monotonic() >= end:
            raise RuntimeError("等待超时，旧资源保留。稍后运行同一命令继续")
        time.sleep(5)
    # The deletion set is frozen before uploading; never includes the new batch.
    for collection, route in [(state["OldAssets"], "/exam408/assets/"), (state["OldDocuments"], "/documents/")]:
        existing = {a["ID"] for a in (api.request("GET", "/exam408/assets") if route.startswith("/exam") else
                    api.request("GET", f"/knowledge-bases/{kb}/documents")) or []}
        for item in collection:
            key = route + item["ID"]
            if key in state["Deleted"]:
                continue
            if item["ID"] in existing:
                api.request("DELETE", key)
            state["Deleted"].append(key)
            save_json(journal, state)
    state["Complete"] = True
    save_json(journal, state)
    print("完成：新资料已就绪，旧资料及旧公共练习已清理。四科目录、账号和个人资料不变。")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True)
    parser.add_argument("--api", default="http://127.0.0.1:8080/api/v1")
    parser.add_argument("--report-dir", default=".cache/public-import")
    parser.add_argument("--kb-id")
    parser.add_argument("--plan", help="审核后的 classification.csv")
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--replace", action="store_true")
    parser.add_argument("--wait-hours", type=float, default=24)
    args = parser.parse_args()
    try:
        run(args)
    except (OSError, ValueError, RuntimeError, KeyError) as e:
        print("操作未完成：" + str(e), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
