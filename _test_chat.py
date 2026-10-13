# -*- coding: utf-8 -*-
import urllib.request
import json

# 健康检查
with urllib.request.urlopen("http://localhost:8000/api/health", timeout=10) as r:
    print("后端健康:", json.loads(r.read())["status"])

# 问答测试
data = json.dumps({"query": "栈和队列的区别", "history": [], "session_id": "t"}).encode()
req = urllib.request.Request("http://localhost:8000/api/chat", data=data,
                             headers={"Content-Type": "application/json"})
answer = ""
ev = 0
with urllib.request.urlopen(req, timeout=60) as r:
    for chunk in r.read().decode().split("\n\n"):
        if chunk.startswith("data: "):
            d = json.loads(chunk[6:])
            if d["type"] == "token":
                answer += d["data"]
            elif d["type"] == "evidence":
                ev = len(d["data"])
            elif d["type"] == "error":
                print("错误:", d["data"])
print("证据条数:", ev)
print("回答字数:", len(answer))
print("回答预览:", answer[:120])
