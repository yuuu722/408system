# 本地结构化解析服务

> 2026-10-15 校准：正常 PostgreSQL worker 的单任务总时限现为 30 分钟，分页 HTTP 请求时限为 9 分钟；以下格式限制中的旧“10 分钟”已更新。安装、三终端启动、备份恢复和逐文件索引见 [使用与代码指南](../../docs/USER_AND_CODE_GUIDE.md)。OCR 在 CPU 上运行，不消耗云端大模型额度；视觉语义理解是另配的 API。

2026-10-15 协议升级：`POST /parse` 新增 multipart `start_page`、`page_limit`，返回 `TotalPages`。PDF 为物理页；DOCX 为每 16 个正文节点一段，段内图片识别可复用已提交检查点。新版 worker 需要同时启动本目录新版服务。物理页面保存尺寸、旋转、裁切及坐标空间；裁切页目前不提供矩形定位。详见 docs/TASKS_3_5_IMPLEMENTATION.md。

独立的 408 HTTP 协议，复用 PaddleOCR PP-StructureV3、pdfplumber、PDFium、Pillow 等通用开源组件；不是自研 OCR 模型。参考官方接口：[PP-StructureV3](https://github.com/PaddlePaddle/PaddleOCR/blob/main/docs/version3.x/pipeline_usage/PP-StructureV3.en.md)。依赖及模型保留各自许可证。当前在 Windows CPU 上验证，不依赖参考项目。

## 安装与启动

在项目根目录运行，安装到独立环境：

```powershell
uv venv .cache/parser-venv --python 3.11
uv pip install --python .cache/parser-venv/Scripts/python.exe -r services/document_parser/requirements-windows.lock.txt
```

`.env` 设置 `DOCUMENT_PARSER_URL=http://host.docker.internal:8091`，并为 `DOCUMENT_PARSER_TOKEN` 填写随机令牌。Docker 中 API/worker 共用该设置；后端直接在宿主机运行时 URL 改为 `http://127.0.0.1:8091`。

```powershell
$line = Get-Content .env | Where-Object { $_ -match '^DOCUMENT_PARSER_TOKEN=' }
$env:PARSER_TOKEN = $line.Substring($line.IndexOf('=') + 1).Trim()
$env:PADDLE_PDX_CACHE_HOME = Join-Path (Get-Location) '.cache/paddle'
$env:PADDLE_PDX_MODEL_SOURCE = 'BOS'
$env:DISABLE_MODEL_SOURCE_CHECK = 'True'
& .cache/parser-venv/Scripts/python.exe services/document_parser/server.py
```

首次图像处理下载并加载模型；`GET /healthz` 的 `layout_loaded` 区分 HTTP 启动与模型已加载。当前使用 CPU，复杂公式明显慢于普通文字。监听 8091，`POST /parse` 需 Bearer 认证。一次只处理一份文件，忙碌返回 503 供现有 worker 重试；不面向公网部署。

## 输入输出与边界

- PDF：普通文本直接提取；扫描页、含图/线条/数学字体等复杂页交给版面、OCR、表格和公式模型。保存页码、内容块、归一化坐标、结构化文本和图表公式 PNG 裁图。
- DOCX：提取段落、表格、嵌入图片并对图片调用识别。无可靠物理页码，页码为 0。原生 OMML 公式尚不转换，明确提示导出 PDF；不能把此情况称为完整 DOCX 公式支持。
- 单页失败保留原图和警告，不伪造识别内容。文件上限 20 MiB、2000 页；并不保证 2000 页可在现有 worker 的 30 分钟任务限制内完成，分页 HTTP 请求另有 9 分钟时限。长教材仍应拆章。
- 原图中的流程、树结构等不等于 OCR 文字。后端的图像题辅助判分及带图 RAG 使用独立 `VISION_BASE_URL / VISION_API_KEY / VISION_MODEL`；未配置时给出明确提示，不把文本模型当视觉模型。
- 图表裁图的语义解释由视觉模型提供，需标记为推断。当前没有真实视觉供应商配置，协议测试不等于真实图像理解验收。
- 数学公式和表格识别不保证无误；真实教材、多栏试卷、跨页题、图文对应关系仍需样本评测。

解析服务是宿主机独立进程，Compose 不会自动启动/停止它。`.cache` 中环境、权重和运行日志不提交；可按上述步骤恢复。`requirements-windows.lock.txt` 是本次 Windows 环境快照，跨平台需重新验证。

## 离线回归

```powershell
& .cache/parser-venv/Scripts/python.exe -m unittest discover -s services/document_parser -p test_server.py
```

仅生成合成 DOCX/图片，替换识别函数，不下载权重或调用模型接口。覆盖图像失败保留原图、图文顺序、唯一块 ID、未引用媒体排除、表格正文、缺失图片与原生公式警告。实际 OCR/公式模型效果需单独验收；离线通过不能作为效果证明。

2026-10-15：DOCX 按正文图片关系读取，不遍历所有媒体；表格内图片按表格整体关联，尚未恢复精确单元格图文版面。可解码图片识别失败仍保存原图；无法解码或图片文件缺失时给出警告，原 DOCX 仍可访问。
