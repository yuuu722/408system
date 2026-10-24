# 本机文件夹批量替换公共资料

适用于 Windows Python 3.9+。仅连接本机已启动的 408 系统 API，文件直接从本机传给本机后端，不会上传到 GitHub 或回答模型。管理员密码以隐藏输入读取，不写入记录。

## 操作

在包含 compose.yaml 的项目根目录运行，服务需保持启动：

```powershell
py -3 scripts/import-public-folder.py --source "D:\Edge Download\qq\组会考研408资料"
```

先只扫描。核对 `.cache/public-import/classification.csv` 的 `ResourceType`，必要时用 Excel/文本编辑器修正（保持 RelativePath 不变）。可用值：textbook 教材、past_exam 真题、solution 解析、courseware 课件、notes 笔记、other 其他。分类仅根据文件名和目录名，不宣称已识别内容；无法确定的为 other。归类核对后：

```powershell
py -3 scripts/import-public-folder.py --source "D:\Edge Download\qq\组会考研408资料" --plan ".cache/public-import/classification.csv" --apply --replace
```

输入管理员账号与密码（不要发给他人）。脚本默认选取名称为 `408 系统公共知识库` 的公共库；如已重命名，使用 `--kb-id ID`。服务地址默认为 http://127.0.0.1:8080/api/v1，可用 --api 指定其他本机端口。

## 范围与恢复

支持递归扫描 PDF、DOCX、TXT、MD。压缩包需先解压，DOC/PPT/图片等需转换为当前解析支持的格式；存在不支持、空白或超限文件时中止，不静默跳过，不清理旧资源。原始文件不会从资料文件夹删除。上传限制以服务端能力接口为准。

新文件全部上传并处理 ready 后，才删除该公共库导入前的旧文件、由其关联的公共学习资源及未绑定文档的公共资源；其他公共库的文件及绑定资源、个人资料、四科目录和账号保持不动。通过正常删除接口同步移除索引与图谱证据。原有历史对话中的旧文件链接可能失效。操作记录只保存元数据，不是旧原文件或数据库的完整备份。批次进行时不要同时编辑或上传该库的资料。

`.cache/public-import/import-state.json` 保存批次状态，`processing-results.json` 保存就绪/失败信息。处理失败时旧资料保留，先在网页修复并重试失败任务，再运行相同命令继续，已记录的文件不会重复上传。网络在服务接收成功但客户端未记账时中断，重试可能产生单个重复文件，应检查记录与网页。处理就绪不保证 OCR 或图谱语义质量，请抽查解析结果和引用原文。默认最多等待 24 小时，可通过 --wait-hours 修改。

完成后同一批次重复运行不会重复导入。导入不同文件集时使用不同 --report-dir。若提示找不到 py，需要先安装 Python 3 并启用启动器，或使用已安装的 python 替换 py -3。


大于 200 MiB 的教材：可将本机 `.env` 的 `MAX_UPLOAD_MB` 改为 `2048`（最大 2 GiB），执行 `docker compose up -d --build --force-recreate backend worker frontend`。API 能力接口会公布生效限制，无需修改分类清单；保持原批次命令即可继续。默认值仍为 200 MiB，独立 Python 解析服务若启用也须同步其环境变量并重启。当前内置 PDF 页数上限仍为 2000 页，大文件仍应抽查解析结果。
