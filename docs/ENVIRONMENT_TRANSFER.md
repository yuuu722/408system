# 两台电脑之间复制完整开发环境（不重新解析）

这与GitHub同步源码不同：私下交付运行镜像、四个数据卷、源码历史和`.env`，接收方恢复到独立的新项目。恢复后是快照副本，后续数据不会自动同步。源/目标均需Linux容器模式，Docker CPU架构相同。不要把本备份包放在公开GitHub、项目目录或公共网盘。

## 发送方（现有Windows电脑）

前提：Docker Desktop正常运行，所有改动已提交（脚本拒绝有未提交跟踪文件的工作区），不要同时下载或删除Ollama模型，也不要另开终端启动或修改Compose项目。需要留出足够空间容纳运行镜像、四份压缩数据及可选模型。镜像取自容器实际运行的不可变ID，不重新构建；源码取当前Git提交，运行镜像若是旧版本也不会被脚本擅自更新。

在项目终端运行：

```powershell
py -3 scripts/transfer-environment.py export --output "D:\408-transfer" --include-ollama
```

输出目录必须尚不存在，且在项目和模型目录外。若不需要复制模型，可省略`--include-ollama`；对方需自行准备相同模型。模型目录使用当前`OLLAMA_MODELS`，否则为用户目录下`.ollama/models`。脚本不复制Windows Ollama程序、驱动或其他宿主机Python服务。

脚本先导出镜像/源码/模型，然后停止本项目的六个服务，压缩PostgreSQL、Redis、Neo4j和上传文件的实际卷。数据库若被强制终止（退出码137），拒绝备份。无论压缩成功与否，尝试启动原本运行的服务；若终端被强制关闭/断电，需要自行执行`docker compose start`。只有生成`manifest.json`且输出“完成”才是可交付的备份。

导出过程不运行数据库迁移、文档重处理、删除文件或删除数据卷。不使用PowerShell重定向二进制数据库输出。备份期间不要手动启动服务或直接改卷。原有排队任务可能在服务恢复启动后继续。

把整个`D:\408-transfer`文件夹通过移动硬盘或私密渠道交给同学，不能漏文件。它包含全部已有账号、个人资料及API/JWT/Neo4j密钥，不是脱敏演示包。接收方会获得原系统权限，需由你信任的人保管。

## 接收方

先安装/启动Docker Desktop（Linux容器）、Git、Python 3.8以上。使用本地问答时安装并启动Ollama，保持模型名称和运行版本尽量一致。此工具不安装驱动、Ollama软件或远程解析/嵌入服务，外部API也仍需网络可达。

假设备份包在`D:\408-transfer`，目标项目使用一个不存在的新目录：

```powershell
py -3 "D:\408-transfer\transfer-environment.py" restore --bundle "D:\408-transfer" --project-directory "D:\408-clone" --project-name exam408-copy-local
```

工具验证文件SHA-256及Docker架构，拒绝覆盖已有项目目录/数据卷，导入镜像，以原镜像恢复四个新卷；从离线Git bundle获得源码，并将origin设回项目GitHub。恢复的源码检出为快照提交。若同学已有不同的Ollama模型文件，脚本停止而不是覆盖；核对后可使用`--skip-ollama`保留本机模型。

恢复后在VS Code打开`D:\408-clone`，浏览器访问`http://localhost:5173`，使用备份时已有账号登录。检查`docker compose ps -a`、资料数量、题库、图谱、原文文件和问答。端口5173/8080/5432/6379需未被其他本机项目占用；不要为腾端口删除其他项目的数据卷。

工具生成本机专用`compose.transfer.json`，通过`.env`设置Compose项目名/文件，指向新数据卷及导入镜像；该覆盖文件已被Git忽略。以后仍可在该目录运行正常Compose命令。不要复制原项目的卷前缀到同学电脑，也不要使用`docker compose down -v`。

开发前创建自己的分支：

```powershell
cd "D:\408-clone"
git switch -c feature/my-work
code .
```

两个人可通过GitHub合并代码，但新上传资料/重新解析结果不会跟着Git合并。如果以后要再次获得你的新数据，需要重新导出/恢复到新的目录和新项目名，或改为共享开发服务器。复制环境不保证不同硬件/模型运行版本的生成文本逐字相同。

## 验证边界

脚本具备校验值、目标防覆盖、同架构限制、数据卷只读导出和恢复后的私有项目隔离。自动测试覆盖路径/清单拒绝、只读快照、坏备份不写入及真实Docker四卷往返；Windows Docker Desktop还需在实际设备上验收。文件校验用于发现损坏，不是签名验证，请只导入你信任的人提供的私人备份。

验证补记：18项Python脚本测试通过；隔离Docker实测完整导出/恢复后，PostgreSQL测试行、Neo4j测试节点、Redis键值、上传文件均读回，且源服务恢复启动、目标项目/数据卷独立。测试使用合成数据；未在用户Windows机器执行实际备份。
