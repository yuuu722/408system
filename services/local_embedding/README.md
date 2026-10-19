# 本地嵌入服务（手动运行）

> 2026-10-15 交接校准：下文 Conda/模型路径为旧机器路径，新电脑不需要整个中草药项目。复制通用模型目录并重建 Python 环境即可；已查到的关键依赖版本、新路径启动示例和费用说明见 [使用与代码指南](../../docs/USER_AND_CODE_GUIDE.md)。全量依赖锁定及干净机器复现尚未完成，不能把直接复制旧虚拟环境视作已验收部署。

此服务是临时本地开发方案，不预设未来服务器模型。使用已有通用开源模型权重，不依赖中草药业务代码或数据库，不包含自动启动脚本。

- 模型来源：`shibing624/text2vec-base-chinese`（保留原模型目录的 README、许可证信息）。
- 本次环境：`D:\Software\Anaconda\envs\tcm_rag_demo\python.exe`，CPU 推理，4 个计算线程。
- 本次权重路径：`D:\school\论文项目\GraphRAG-KG-TCM\TCM_knowledge_graph-main\models\text2vec-base-chinese`。路径由命令行指定，不写死在服务内，不自动下载模型。
- API 模型标识：`text2vec-base-chinese-windowed-v1`，768 维。按 tokenizer 窗口编码后平均并归一化，以避免默认 128 token 截断；该策略不是训练所得模型，也不是已经完成质量评测的长文本方法。
- HTTP：`GET /healthz`；`POST /v1/embeddings`，Bearer 认证。监听 8090，以供 Docker 访问宿主机。仅用于可信本地开发网络，不直接暴露公网；单进程串行处理，未做并发和大规模吞吐验收。

在项目根目录的 PowerShell 手动运行（前台运行，Ctrl+C 停止）：

```powershell
$line = Get-Content .env | Where-Object { $_ -match '^EMBEDDING_API_KEY=' }
$env:LOCAL_EMBEDDING_API_KEY = $line.Substring($line.IndexOf('=') + 1).Trim()
& 'D:\Software\Anaconda\envs\tcm_rag_demo\python.exe' services/local_embedding/server.py --model-path 'D:\school\论文项目\GraphRAG-KG-TCM\TCM_knowledge_graph-main\models\text2vec-base-chinese'
```

不要在已有服务占用 8090 时重复启动。服务依赖 torch、sentence-transformers、numpy；本次复用已有环境，未来独立服务器部署前需固定经过验证的依赖版本。

后端 `.env`：`EMBEDDING_BASE_URL=http://host.docker.internal:8090/v1`、`EMBEDDING_MODEL=text2vec-base-chinese-windowed-v1`、`EMBEDDING_DIMENSION=768`，Key 与服务保持一致。后端在宿主机直接运行时地址需使用 `127.0.0.1`；更换端点会改变向量空间标识，需重新处理资料。

迁移 000003 保留旧向量，将列改为可变维度；检索按端点、模型名称、维度及演示模式生成的空间标识过滤，再计算距离。旧资料即使显示 ready，也需手动重新处理才能参与当前检索。当前为精确检索，暂移除旧 HNSW 索引；大规模服务器部署时按模型空间建立索引。相同名称下替换权重或算法时必须更换模型版本标识。

手动验收：`python scripts/check-local-embedding.py`。会创建独立测试账号与资料，并实际调用已配置的 LLM（产生服务商费用）；不重处理用户资料。测试包括长输入、PDF 入库、中文资料问答、认证及维度错误。此脚本不启动任何服务。
