# -*- coding: utf-8 -*-
import urllib.request
import json
import re

BASE = "http://localhost:8000"

# 1. 前端首页
print("=== 1. 前端首页 http://localhost:8000/ ===")
try:
    with urllib.request.urlopen(BASE + "/", timeout=10) as resp:
        html = resp.read().decode("utf-8")
        print("状态码:", resp.status)
        print("页面大小:", len(html), "字符")
        print("包含408标题:", "408" in html)
        print("引用JS资源:", "assets/index" in html)
except Exception as e:
    print("失败:", e)

# 2. 静态资源
print("\n=== 2. 静态资源 ===")
try:
    with urllib.request.urlopen(BASE + "/", timeout=10) as resp:
        html = resp.read().decode("utf-8")
    js_match = re.search(r"/assets/(index-[^\"]+\.js)", html)
    css_match = re.search(r"/assets/(index-[^\"]+\.css)", html)
    if js_match:
        with urllib.request.urlopen(BASE + "/assets/" + js_match.group(1), timeout=10) as r:
            print("JS资源状态:", r.status, "大小:", len(r.read()), "字节")
    if css_match:
        with urllib.request.urlopen(BASE + "/assets/" + css_match.group(1), timeout=10) as r:
            print("CSS资源状态:", r.status, "大小:", len(r.read()), "字节")
except Exception as e:
    print("失败:", e)

# 3. API
print("\n=== 3. 后端API ===")
try:
    with urllib.request.urlopen(BASE + "/api/health", timeout=10) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print("健康检查:", data["status"])
except Exception as e:
    print("失败:", e)

# 4. 知识图谱
print("\n=== 4. 知识图谱API ===")
try:
    with urllib.request.urlopen(BASE + "/api/knowledge-graph", timeout=10) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print("节点数:", len(data["nodes"]), "边数:", len(data["edges"]))
except Exception as e:
    print("失败:", e)

print("\n=== 测试完成 ===")
