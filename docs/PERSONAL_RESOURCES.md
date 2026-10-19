# 个人资料自动处理与辅助判分（更新 2026-10-15）

2026-10-15 校准：本文详细说明任务 1、2 的资料和判分规则。任务 3–5 已实现首版，见 [任务 3–5 实施](TASKS_3_5_IMPLEMENTATION.md)；当前部署与使用以 [使用指南](USER_AND_CODE_GUIDE.md) 为准。下方按日期保留的停机/未实施记录只代表历史时点。

## 当前流程

学生负责上传资料内容正确性；系统自动处理并检查结构完整性，没有教师角色、人工校对、审核发布门槛。

1. 上传 TXT/PDF/DOCX，worker 解析。PDF/DOCX 通过独立解析服务得到文字、图片、公式、表格和位置。
2. 保存解析结果，可在“学习资料 → 查看解析结果”查看原图、文字和异常原因。保留原文访问权限。
3. 自动提取具有数字题号与 A–F 选项标记的单选题；匹配当前知识点关键词。题目进入“题目与资源”。
4. 完整且已匹配知识点的题目自动可用；缺图、选项缺失/重复、来源答案越界、页面异常、未匹配知识点等显示“暂不可用”和原因。非单选、复杂题号或无法识别的版式暂不抽取，原文仍可阅读。
5. 资料答案优先，支持题后答案以及单独“参考答案/答案与解析/答案”段的题号匹配。题号重复或对应不明确时，不强行匹配答案。
6. 无来源答案的完整题目可作答。交卷后点击“获取 AI 参考答案与解析”，模型可以返回答案或无法确定；明确标记可能有误。图像题需要视觉模型配置。

自动抽题目前为独立规则解析，不是任意试卷的通用结构化理解。跨题图片匹配为页面级关联，可能包含同页其他题的图，尚未完成图题精确对应评测。扫描件正文模型可能误识别数字/选项，完整性检查不能证明内容正确。

2026-10-15 修复：跨页题的无文字图片续页也会关联；无正文失败续页的警告会阻止该题进入测评。异常答案页不用于资料答案判分，完整题可保留为无资料答案题；不会自动生成标准答案。DOCX 沿正文引用顺序识别图片并保留可解码原图，不引用的媒体不加入正文；图像损坏则保留原文件访问及警告。DOCX 表格内图片暂按所属表格整体关联，不代表精确单元格图文排版恢复。

## 学情口径

| 依据/操作 | 记录 | 是否计入学情 |
|---|---|---|
| 资料或学生提供的答案 | source | 是 |
| AI 未返回答案 | ai_pending | 否 |
| AI 推断答案 | ai | 默认否 |
| 用户确认自己作答正确 | confirmation=correct | 是，正确 |
| 用户确认自己作答错误 | confirmation=wrong | 是，错误 |
| 不确定或撤回确认 | confirmation=uncertain | 否 |

AI 推断、用户确认分别保存。模型结果后到不会覆盖用户确认；重复推断读取已保存参考答案。用户可修改确认结果，替换该测评本次贡献并重算掌握度；不会追加重复分数。确认历史报告保留原交卷时间，避免旧作答因晚确认挤掉较新的有效作答。雷达取同一道题最近有效结果；撤回后如存在更早有效记录，则恢复该记录。

问答、阅读与任务完成不直接计分。确认结果来源是用户自评，不宣称具有标准化考试测量效度。

## API 与代码入口

- 删除旧 `POST /exam408/assets/{id}/publish`；资源新增/修改由后端计算 `ready/unavailable`，不信任客户端状态。
- `GET /documents/{id}/structure`：本人解析结果。
- `POST /exam408/assessments/{id}/questions/{qid}/infer`：交卷后获取 AI 参考结果。
- `POST /exam408/assessments/{id}/questions/{qid}/confirm`：`Decision` 为 correct/wrong/uncertain。
- 主要实现：`service/question_extract.go`、`service/question_ai.go`、`repository/personal.go`、`repository/document_structure.go`、`httpapi/personal.go`。
- 结构与前端：`domain/structure.go`、`services/document_parser`、`AssetsView.vue`、`AssessmentView.vue`、`ResourcesView.vue`。

迁移 `000006` 保留旧资源和测评快照，资源改用自动可用性；首次启动按现有内容重新检查，不将不完整旧草稿直接启用。来源文档删除会连带删除关联资源，已保存测评快照保留。重新处理文档按稳定提取键更新资源，不重复插入；抽取顺序发生变化时仍可能改变题目身份，尚无跨版本语义去重。

## 历史验收与运维记录（不代表当前在线状态）

2026-10-15 接续状态：用户明确视觉模型配置留空，真实图意理解验收暂缓。本轮仅修复上述解析缺陷；Go 全套非数据库测试及 vet、Python 6 项离线回归、语法和独立性检查通过；不代表实际 OCR 模型新验收或真实教材效果验证。前后端、解析与嵌入服务本轮未启动。没有调用收费模型、改动用户真实材料或配置密钥。

离线 DOCX 回归（合成 XML/图片，识别函数使用测试替身，不下载模型）：

```powershell
& .cache/parser-venv/Scripts/python.exe -m unittest discover -s services/document_parser -p test_server.py
```

下次体验需要先分别按辅助服务 README 启动解析与嵌入服务，再在根目录执行 `docker compose up -d --build backend worker frontend` 以包含本轮 Go 改动；依赖数据库和 Redis 随 Compose 启动。不要只重启旧容器并认为已应用新代码。无需新数据库迁移。

2026-10-14 暂停点：用户因额度不足要求停止；实现已部署，服务保持运行。实际合成中文扫描、表格、积分公式与真实文本模型辅助判分链路通过；实际视觉模型尚未配置和验收。最终补充修复：配置远程解析服务后，Markdown/TXT 仍走本地解析；历史测评恢复已选答案。

验收证据：`.cache/personal-acceptance.json`、`.cache/formula-table-acceptance.json`（含结构与图像数据）、`output/playwright/personal-ai-confirmation.png`。一次性测试账号及文档已删除；`.cache/personal-test-state.json` 仅为失效历史记录。复跑脚本会新建账号并调用模型，不要为确认文档而无意重跑。

接续步骤：先读 WORK_PROGRESS.md 顶部暂停记录并检查工作区；检查前端 5173、后端 8080、嵌入 8090、解析 8091 的实际运行状态。宿主机解析与嵌入服务分别按各自 README 启动，Compose 不会自动启动它们。需要继续图片理解时再配置 `VISION_*` 并进行真实模型验证；真实教材抽样和任务 3–5 留待后续明确范围。

先运行 `docs/ENGINEERING_FOUNDATION.md` 中 Go 隔离数据库测试、前端构建与独立性检查。新增测试覆盖无答案不计分、确认/改判/撤回、幂等、跨用户隔离、自动抽题重跑去重、原文删除关联清理、AI JSON 与视觉 SSE 协议。

`scripts/check-personal-resources.py` 是显式运行的真实链路验收，会创建一次性账号及合成扫描 PDF，并调用当前文本模型一次；不读取真实教材。使用解析服务虚拟环境运行。验收状态保存于 `.cache/personal-acceptance.json`；临时账号标识在 `.cache/personal-test-state.json`，验收后按精确 ID 清理，勿误删真实账号。

本次迁移前备份：`.cache/before-personal-resources.dump`。不要使用 `docker compose down -v` 作为升级步骤。API 与 worker 使用相同模型、解析令牌和上传卷。启动解析服务后，再 `docker compose up -d backend worker frontend`；嵌入服务仍按其 README 单独启动。

本次没有导入用户教材数据集，没有实现任务 3–5，也没有接入 AgentArts。OCR/公式/表格属于通用开源引擎能力，个人判分与学情规则、数据契约和页面流程是本项目实现。
