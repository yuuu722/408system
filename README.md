# 408考研知识图谱智能问答系统

面向计算机考研408学生的智能问答系统，基于知识图谱 + RAG（检索增强生成）技术。

## 项目简介

本系统仿照中草药知识图谱问答系统架构，针对408考研四门课程（数据结构、计算机组成原理、操作系统、计算机网络）构建智能问答平台。

### 核心功能
- **智能问答**：基于 RAG 技术，准确解答考研知识点问题
- **知识图谱**：展示知识点之间的关联关系，可视化学习路径
- **流式输出**：回答逐字输出，提升用户体验
- **依据展示**：每条回答标注知识点来源，可展开查看
- **连续对话**：支持上下文追问，保留多轮对话历史

### 技术栈
| 层级 | 技术 |
|------|------|
| 前端 | Vue 3 + Vite + ECharts |
| 后端 | FastAPI + Python |
| 知识图谱 | Neo4j |
| 向量检索 | FAISS / 语义检索 |
| LLM | OpenAI 兼容接口（DeepSeek等） |

## 项目结构

```
cs408-qa-system/
├── KG_Demo/                 # 演示应用
│   ├── backend/            # FastAPI 后端
│   │   └── main.py        # API 入口
│   └── frontend/           # Vue 前端
│       ├── src/
│       │   ├── App.vue     # 主界面
│       │   ├── main.js     # 入口
│       │   └── style.css   # 样式
│       ├── index.html
│       └── package.json
├── RAG-model-V3/           # RAG 问答流水线
│   ├── config.py          # 配置管理
│   ├── gateway.py         # 语义网关（意图识别）
│   ├── searcher.py        # 知识检索
│   ├── generator.py       # 回答生成
│   ├── conversation.py    # 对话管理
│   └── cache.py           # 缓存
├── knowledge_graph/        # 知识图谱
│   └── init_graph.py      # 图谱初始化脚本
├── data/
│   └── sample/            # 示例数据
│       └── knowledge_points.json
├── docs/                   # 文档
├── .env.example           # 环境变量模板
├── requirements.txt       # Python 依赖
└── README.md              # 项目说明
```

## 快速开始

### 环境要求
- Python 3.10+
- Node.js 16+
- Neo4j 5.x（可选，不连接时使用示例数据）

### 1. 后端启动

```bash
# 进入项目目录
cd cs408-qa-system

# 创建虚拟环境（推荐）
python -m venv .venv

# 激活虚拟环境
# Windows:
.venv\Scripts\activate
# Linux/Mac:
source .venv/bin/activate

# 安装依赖
pip install -r requirements.txt

# 配置环境变量
copy .env.example .env
# 编辑 .env 填入 LLM API Key 和 Neo4j 配置

# 启动后端
python KG_Demo/backend/main.py
```

后端运行在 http://localhost:8000

### 2. 前端启动

```bash
# 新开终端，进入前端目录
cd KG_Demo/frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev
```

前端运行在 http://localhost:3000

### 3. 知识图谱初始化（可选）

如果需要使用 Neo4j 知识图谱：

```bash
# 确保 Neo4j 已启动
# 配置好 .env 中的 NEO4J_* 参数

# 初始化图谱并导入示例数据
python knowledge_graph/init_graph.py
```

## 配置说明

编辑 `.env` 文件：

```ini
# LLM 配置（必须）
LLM_API_KEY=sk-xxx
LLM_BASE_URL=https://api.deepseek.com/v1
LLM_MODEL=deepseek-chat

# Neo4j 配置（可选）
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASSWORD=your_password
```

支持任何 OpenAI 兼容接口：
- DeepSeek
- 通义千问
- 智谱AI
- OpenAI
- 本地 Ollama 等

## 知识库扩展

### 添加新知识点

编辑 `data/sample/knowledge_points.json`，按格式添加：

```json
{
  "id": "ds_bubble_sort",
  "name": "冒泡排序",
  "subject": "ds",
  "chapter": "第八章 排序",
  "definition": "冒泡排序是一种简单的排序算法，重复地走访过要排序的数列，一次比较两个元素...",
  "properties": "时间复杂度O(n²)，稳定排序，原地排序",
  "relations": [
    {"target": "ds_quick_sort", "type": "对比"}
  ]
}
```

### 批量导入教材内容

后续可扩展：
1. 从 PDF 教材提取知识点
2. 自动构建实体关系
3. 批量导入 Neo4j

## 大创项目扩展方向

### 可扩展功能
1. **智能刷题**：根据知识点自动生成练习题
2. **学习路径推荐**：基于知识图谱推荐学习顺序
3. **错题本**：记录易错知识点，针对性练习
4. **学习进度可视化**：展示知识点掌握程度
5. **多人协作**：支持学生和老师多角色

### 创新点
- 知识图谱 + RAG 结合，回答有据可依
- 考研考点导向，针对性强
- 可视化知识关联，辅助理解
- 流式输出 + 证据展示，提升学习效果

## 开发计划

- [x] 项目框架搭建
- [x] RAG 问答流水线
- [x] 前端聊天界面
- [x] 知识图谱可视化
- [ ] 向量检索模块
- [ ] 批量知识库导入
- [ ] 智能刷题功能
- [ ] 学习进度跟踪

## 许可证

MIT License
