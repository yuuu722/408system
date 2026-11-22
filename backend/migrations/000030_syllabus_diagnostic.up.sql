BEGIN;
ALTER TABLE exam408_knowledge_points ADD COLUMN syllabus_priority smallint NOT NULL DEFAULT 2 CHECK (syllabus_priority BETWEEN 1 AND 5);
ALTER TABLE daily_plans ADD COLUMN baseline_assessment_id uuid REFERENCES assessments(id) ON DELETE SET NULL;

-- Add only topics visible in the supplied outline pages. Do not embed the PDF.
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,description,keywords,level,sort_order,syllabus_priority)
SELECT p.subject_id,p.id,v.code,v.name,format('2026 年 408 大纲附件第 %s 页；目录覆盖项。',v.page),ARRAY[v.name],p.level+1,v.page,3
FROM (VALUES
  ('DS.TREE','DS.TREE.REP','树的存储结构',2),
  ('DS.TREE.FOREST','DS.TREE.FOREST.CONVERT','森林与二叉树的转换',2),
  ('DS.TREEAPP','DS.TREEAPP.HEAP','堆',2),
  ('DS.GRAPHSTORE','DS.GRAPHSTORE.MULTILIST','邻接多重表',2),
  ('DS.GRAPHSTORE','DS.GRAPHSTORE.ORTHOGONAL','十字链表',2),
  ('DS.SORT','DS.SORT.BINARY_INSERT','折半插入排序',2),
  ('DS.SORT.EXTERNAL','DS.SORT.EXTERNAL.RUNS','初始归并段',3),
  ('CO.SYSTEM','CO.SYSTEM.STORED_PROGRAM','存储程序',3),
  ('CO.NUMBER.FIXED','CO.NUMBER.FIXED.FLAGS','运算标志位',3),
  ('CO.RAM','CO.RAM.FLASH','Flash 存储器',4),
  ('CO.RAM','CO.RAM.CONNECT','主存与 CPU 的连接',4),
  ('CO.RAM.VM','CO.RAM.VM.SEG','段式虚拟存储器',4),
  ('CO.RAM.VM','CO.RAM.VM.SEG_PAGE','段页式虚拟存储器',4),
  ('CO.ISA','CO.ISA.ALIGN','数据对齐',4),
  ('CO.ISA','CO.ISA.ENDIAN','字节序',4),
  ('CO.ISA','CO.ISA.CALL','函数调用的机器级表示',4),
  ('CO.CPU','CO.CPU.CYCLE','指令执行过程',4),
  ('CO.CPU','CO.CPU.EXCEPTION','处理器异常',4),
  ('CO.CPU','CO.CPU.INTERRUPT','处理器中断',4),
  ('CO.PIPE','CO.PIPE.SUPERSCALAR','超标量处理器',5),
  ('CO.PIPE','CO.PIPE.DYNAMIC','动态流水线',5),
  ('CO.CPU','CO.CPU.SIMD','SIMD',5),
  ('CO.CPU','CO.CPU.MIMD','MIMD',5),
  ('CO.CPU','CO.CPU.VECTOR','向量处理器',5),
  ('CO.CPU','CO.CPU.SMP','共享内存多处理器',5),
  ('CO.CPU','CO.CPU.SMT','硬件多线程',5),
  ('CO.IO','CO.IO.PORT','I/O 端口编址',5),
  ('OS.INTRO','OS.INTRO.KERNEL','内核结构',6),
  ('OS.INTRO','OS.INTRO.BOOT','操作系统引导',6),
  ('OS.INTRO','OS.INTRO.VM','虚拟机',6),
  ('OS.PROCESS','OS.PROCESS.IPC','进程间通信',6),
  ('OS.PROCESS','OS.PROCESS.SHARED','共享内存通信',6),
  ('OS.PROCESS','OS.PROCESS.MESSAGE','消息传递',6),
  ('OS.PROCESS','OS.PROCESS.PIPE','管道',6),
  ('OS.SYNC','OS.SYNC.CONDVAR','条件变量',6),
  ('OS.SYNC','OS.SYNC.RW','读者写者问题',6),
  ('OS.SYNC','OS.SYNC.DINING','哲学家进餐问题',6),
  ('OS.MEMORY','OS.MEMORY.CONTIGUOUS','连续内存分配',7),
  ('OS.MEMORY','OS.MEMORY.PROTECT','内存保护',7),
  ('OS.VIRTUAL','OS.VIRTUAL.FRAME','页框分配',7),
  ('OS.VIRTUAL','OS.VIRTUAL.MMAP','内存映射文件',7),
  ('OS.FILES','OS.FILES.HARDLINK','硬链接',7),
  ('OS.FILES','OS.FILES.SYMLINK','软链接',7),
  ('OS.FILES','OS.FILES.VFS','虚拟文件系统',7),
  ('OS.FILES','OS.FILES.MOUNT','文件系统挂载',7),
  ('OS.IO','OS.IO.DRIVER','设备驱动程序',8),
  ('OS.IO','OS.IO.NONBLOCK','非阻塞 I/O',8),
  ('CN.PHYSICAL','CN.PHYSICAL.NYQUIST','奈奎斯特定理',8),
  ('CN.PHYSICAL','CN.PHYSICAL.SHANNON','香农定理',8),
  ('CN.PHYSICAL','CN.PHYSICAL.DATAGRAM','数据报交换',8),
  ('CN.PHYSICAL','CN.PHYSICAL.VIRTUAL_CIRCUIT','虚电路',8),
  ('CN.LINK.ARQ','CN.LINK.ARQ.STOP_WAIT','停止等待协议',9),
  ('CN.LINK.ARQ','CN.LINK.ARQ.GBN','后退 N 帧协议',9),
  ('CN.LINK.ARQ','CN.LINK.ARQ.SR','选择重传协议',9),
  ('CN.MAC','CN.MAC.SWITCH','以太网交换机',9),
  ('CN.MAC','CN.MAC.WIFI','IEEE 802.11',9),
  ('CN.NETWORK','CN.NETWORK.SDN','软件定义网络',9),
  ('CN.ROUTE','CN.ROUTE.DV','距离向量路由算法',9),
  ('CN.ROUTE','CN.ROUTE.LS','链路状态路由算法',9),
  ('CN.NETWORK','CN.NETWORK.MULTICAST','IP 组播',10),
  ('CN.NETWORK','CN.NETWORK.MOBILE','移动 IP',10),
  ('CN.NETWORK','CN.NETWORK.FORWARD','路由器转发',10),
  ('CN.TCP','CN.TCP.SEGMENT','TCP 报文段',10),
  ('CN.APP','CN.APP.P2P','对等网络模型',11),
  ('CN.APP','CN.APP.MIME','MIME',11),
  ('CN.APP.FTP','CN.APP.FTP.CONTROL','FTP 控制连接',11),
  ('CN.APP.FTP','CN.APP.FTP.DATA','FTP 数据连接',11)
) AS v(parent,code,name,page)
JOIN exam408_knowledge_points p ON p.code=v.parent
WHERE NOT EXISTS (SELECT 1 FROM exam408_knowledge_points x WHERE x.subject_id=p.subject_id AND x.name=v.name)
ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points p SET syllabus_priority=v.priority
FROM (VALUES
  ('DS.BASIC.TIME',5),
  ('DS.SEQ.SINGLE',4),
  ('DS.STACK.STACK',4),
  ('DS.BINARY.IN',5),
  ('DS.GRAPHSTORE.BFS',5),
  ('DS.SEARCH.BINARY',4),
  ('DS.SORT.MERGE',5),
  ('CO.SYSTEM.CPI',5),
  ('CO.NUMBER.TWOS',5),
  ('CO.FLOAT.IEEE',4),
  ('CO.CACHE.MAPPING',5),
  ('CO.ISA.ADDR',4),
  ('CO.PIPE.HAZARD',5),
  ('CO.IO.DMA',4),
  ('OS.PROCESS.STATE',4),
  ('OS.SCHED.RR',4),
  ('OS.SYNC.SEM',5),
  ('OS.DEADLOCK.COND',5),
  ('OS.PAGING.ADDR',5),
  ('OS.VIRTUAL.DEMAND',5),
  ('CN.LINK.CRC',4),
  ('CN.IP.SUB',5),
  ('CN.TCP.RELIABLE.ACK',5),
  ('CN.APP.DNS',4)
) AS v(code,priority) WHERE p.code=v.code;

-- Correct teaching links while retaining parent hierarchies and original IDs.
INSERT INTO exam408_knowledge_point_relations(from_point_id,to_point_id,relation_type)
SELECT a.id,b.id,'related'
FROM (VALUES ('DS.TREEAPP.HEAP','DS.SORT.HEAP'), ('CN.LINK.ARQ.GBN','CN.LINK.ARQ.SR'),
             ('OS.VIRTUAL.MMAP','OS.FILES.VFS'), ('OS.PAGING.TABLE','CO.RAM.TLB'),
             ('CN.IP.SUB','CN.NETWORK.FORWARD')
) AS v(a,b)
JOIN exam408_knowledge_points a ON a.code=v.a
JOIN exam408_knowledge_points b ON b.code=v.b
ON CONFLICT DO NOTHING;

-- Origin is project-authored. The outline supplies scope, not the question or answer.
INSERT INTO learning_assets(user_id,kind,point_id,title,payload,status,is_public,seed_key)
SELECT kb.user_id,'question',p.id,v.title,v.payload::jsonb,'ready',true,v.seed
FROM (VALUES
  ('DS.BASIC.TIME','408-diagnostic-2026:01','408 前测 01','{"Stem":"外层循环执行 n 次，内层每次把 j 从 1 开始翻倍，直到 j 大于 n。总时间复杂度是什么？","Options":["O(n)","O(log n)","O(n²)","O(n log n)"],"Answer":3,"Explanation":"内层经历 1、2、4 等值，每轮外循环执行约 log₂ n 次，故总量是 n 与对数项的乘积。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.BASIC.TIME','408-practice-2026:01','408 巩固 01','{"Stem":"若把内层改成从 1 逐次加 1 直到 n，总时间复杂度是什么？","Options":["O(n²)","O(n log n)","O(log n)","O(n)"],"Answer":0,"Explanation":"每一次外层循环都执行 n 次内层循环，因此执行次数是 n²。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.SEQ.SINGLE','408-diagnostic-2026:02','408 前测 02','{"Stem":"已知单链表结点 p，要把新结点 s 插在 p 后面，哪组操作正确？","Options":["直接把 s.next 设为空","把头指针改为 s","先令 s.next 指向 p.next，再令 p.next 指向 s","先令 p.next 指向 s，再令 s.next 指向 p.next"],"Answer":2,"Explanation":"先保存原后继，再接入 s。若反过来读取 p.next，会得到 s 自身，无法连接旧后继。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.SEQ.SINGLE','408-practice-2026:02','408 巩固 02','{"Stem":"单链表已知待删除结点的前驱 p，且 p.next 存在。如何从链上断开该结点？","Options":["将头指针设为空","令 p.next 指回 p","仅改待删除结点的数据域","将 p.next 改为原后继的 next"],"Answer":3,"Explanation":"修改前驱的指针，让它跳过待删除结点，链表剩余结点仍相连。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.STACK.STACK','408-diagnostic-2026:03','408 前测 03','{"Stem":"按 A、B、C 的顺序依次入栈，中途允许出栈。下列哪个出栈顺序不可能？","Options":["A、C、B","C、A、B","C、B、A","B、A、C"],"Answer":1,"Explanation":"先弹出 C 意味着 A 和 B 已在栈中，此时 B 位于 A 上方，所以 A 不能先于 B 弹出。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.STACK.STACK','408-practice-2026:03','408 巩固 03','{"Stem":"对栈依次执行入栈 4、入栈 7、出栈、入栈 2，再出栈，此次弹出的值是什么？","Options":["7","无法确定","2","4"],"Answer":2,"Explanation":"第一次出栈弹出 7；入栈 2 后，2 位于栈顶，下一次就弹出 2。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.BINARY.IN','408-diagnostic-2026:04','408 前测 04','{"Stem":"一棵二叉树以 A 为根，左孩子为 B，右孩子为 C，B 的左孩子为 D。它的中序遍历序列是什么？","Options":["D B A C","D B C A","B D C A","C D B A"],"Answer":0,"Explanation":"中序遍历按左、根、右执行。A 的左子树先访问 D 后访问 B，接着访问 A 与右孩子 C。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.BINARY.IN','408-practice-2026:04','408 巩固 04','{"Stem":"二叉搜索树关键字互异，哪种遍历会从小到大依次访问关键字？","Options":["层序遍历","中序遍历","先序遍历","后序遍历"],"Answer":1,"Explanation":"二叉搜索树左子树键小于根、右子树键大于根，中序按左、根、右访问，因此递增。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.GRAPHSTORE.BFS','408-diagnostic-2026:05','408 前测 05','{"Stem":"在无权图中从顶点 s 开始广度优先搜索，第一次发现顶点 v 时可确定什么？","Options":["从 s 到 v 的最长简单路径长度","v 的所有前驱","s 所在连通分量一定无环","从 s 到 v 的最少边数"],"Answer":3,"Explanation":"队列使搜索逐层扩张；第一次发现 v 的层数等于从 s 到 v 所需的最少边数。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.GRAPHSTORE.BFS','408-practice-2026:05','408 巩固 05','{"Stem":"使用队列做广度优先搜索时，通常何时把一个新顶点标记为已发现？","Options":["首次入队时","首次出队之后才标记","所有相邻顶点访问完以后","搜索全部完成以后"],"Answer":0,"Explanation":"入队即标记可防止不同前驱在该顶点尚未出队时重复将它加入队列。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.SEARCH.BINARY','408-diagnostic-2026:06','408 前测 06','{"Stem":"对含 15 个互异元素的升序数组做折半查找，查找成功最多比较几次关键字？","Options":["5","8","4","3"],"Answer":2,"Explanation":"最坏的候选区间大小依次为 15、7、3、1，最后一次比较确认结果，共四次。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.SEARCH.BINARY','408-practice-2026:06','408 巩固 06','{"Stem":"使用折半查找时，目标关键字小于当前区间的中间元素，下一步应保留哪个区间？","Options":["右边的子区间","整个区间","只保留中间元素","中间元素左边的子区间"],"Answer":3,"Explanation":"升序条件保证中间元素及其右侧都不小于它，目标只能在左侧，故更新右边界。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('DS.SORT.MERGE','408-diagnostic-2026:07','408 前测 07','{"Stem":"两路归并合并两个有序段时，如何保持排序稳定？","Options":["交替取相同的关键字","关键字相等时先取左段元素","相等时总是先取右段元素","相等时随机取一个"],"Answer":1,"Explanation":"原序列中左段先于右段。相等时先取左段，可保留相同关键字的原有相对顺序。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('DS.SORT.MERGE','408-practice-2026:07','408 巩固 07','{"Stem":"合并长度分别为 3 和 4 的两个非空有序段，最坏需要比较几次关键字？","Options":["12","4","6","7"],"Answer":2,"Explanation":"最后只剩一个元素时无需再比较；两段在剩余一个元素前均未提前耗尽时，需要 3+4−1 次。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.SYSTEM.CPI','408-diagnostic-2026:08','408 前测 08','{"Stem":"程序执行 600 万条指令，平均 CPI 为 2，处理器主频为 3 GHz。其 CPU 执行时间是多少？","Options":["4 ms","2 ms","6 ms","12 ms"],"Answer":0,"Explanation":"总时钟周期数为 1200 万，除以每秒 30 亿周期得到 0.004 秒。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.SYSTEM.CPI','408-practice-2026:08','408 巩固 08','{"Stem":"主频给定、指令条数相同，平均 CPI 由 2 降到 1.5，则 CPU 时间变为原来的多少？","Options":["150%","75%","50%","125%"],"Answer":1,"Explanation":"CPU 时间等于指令条数乘平均 CPI 再除主频，其他因素不变时，时间比例就是 1.5/2。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.NUMBER.TWOS','408-diagnostic-2026:09','408 前测 09','{"Stem":"8 位机器中，二进制补码 11110110 表示哪个十进制数？","Options":["-9","118","246","-10"],"Answer":3,"Explanation":"该位串按无符号解释是 246，最高位为 1，用 246 减去 256 得到补码真值。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.NUMBER.TWOS','408-practice-2026:09','408 巩固 09','{"Stem":"8 位补码能表示的最小整数是多少？","Options":["-128","-127","-255","0"],"Answer":0,"Explanation":"最高位权为负的 128，其他各位为零时取得最小值，位形是 10000000。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.FLOAT.IEEE','408-diagnostic-2026:10','408 前测 10','{"Stem":"IEEE 754 单精度规格化有限数中，尾数字段 23 位对应多少位有效二进制精度？","Options":["8 位","32 位","24 位","23 位"],"Answer":2,"Explanation":"规格化数的隐含首位为 1，23 位小数字段加上这一个隐含位，合计 24 位。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.FLOAT.IEEE','408-practice-2026:10','408 巩固 10','{"Stem":"IEEE 754 单精度中阶码全 1、尾数字段全 0 表示什么？","Options":["非数值","零","最小规格化数","无穷大，符号位决定正负"],"Answer":3,"Explanation":"全 1 阶码配全零尾数编码无穷，正负由符号位区分；非零尾数才编码非数值。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.CACHE.MAPPING','408-diagnostic-2026:11','408 前测 11','{"Stem":"按字节编址，Cache 块为 32 字节且有 64 行，采用直接映射。块内偏移和行号分别占几位？","Options":["6 位和 6 位","5 位和 6 位","6 位和 5 位","5 位和 5 位"],"Answer":1,"Explanation":"块内 32 个字节需要 5 位，选择 64 行需要 6 位，其余高位用于标记。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.CACHE.MAPPING','408-practice-2026:11','408 巩固 11','{"Stem":"一个直接映射 Cache 有 8 行。主存块号 3 与 19 分别映射到哪一行？","Options":["都映射到第 0 行","分别映射到第 1 和第 2 行","都映射到第 3 行（从 0 编号）","分别映射到第 3 和第 7 行"],"Answer":2,"Explanation":"行号是主存块号对 8 取模，3 与 19 除以 8 的余数均为 3。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.ISA.ADDR','408-diagnostic-2026:12','408 前测 12','{"Stem":"某指令使用寄存器间接寻址，寄存器 R 中保存 1200。操作数从哪里取得？","Options":["内存地址 1200 指向的位置","寄存器 R 中的常数 1200","本条指令的地址","下一条指令的地址"],"Answer":0,"Explanation":"寄存器间接寻址把寄存器内容视为有效地址，再从该地址读取操作数。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.ISA.ADDR','408-practice-2026:12','408 巩固 12','{"Stem":"地址字段为常数 8，基址寄存器内容为 1000，采用基址加位移寻址。有效地址是什么？","Options":["992","1008","1000","8"],"Answer":1,"Explanation":"有效地址由基址寄存器内容与给定位移相加得到。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.PIPE.HAZARD','408-diagnostic-2026:13','408 前测 13','{"Stem":"相邻两条指令中，后一条需要读取前一条尚未写回的结果。这是哪类流水线冒险？","Options":["控制冒险","结构冒险","没有冒险","数据冒险"],"Answer":3,"Explanation":"这是写后读依赖。结果未及时可用时需要转发、暂停等机制；部件冲突和分支改变分别属于其他冒险。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.PIPE.HAZARD','408-practice-2026:13','408 巩固 13','{"Stem":"同一时钟周期，取指和访存都要访问一个单端口存储器。这是哪类冒险？","Options":["结构冒险","数据冒险","控制冒险","异常中断"],"Answer":0,"Explanation":"两条流水线操作争用不能同时使用的硬件端口，因此是硬件资源冲突。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CO.IO.DMA','408-diagnostic-2026:14','408 前测 14','{"Stem":"DMA 控制器传输一批数据期间，CPU 通常主要负责什么？","Options":["把数据逐字复制到 Cache","一直轮询每次握手","初始化传输并在结束后处理通知","每传一个字都执行一次读写指令"],"Answer":2,"Explanation":"CPU 设置地址、方向和计数等参数；DMA 控制器组织数据传输，完成时通常通知 CPU。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CO.IO.DMA','408-practice-2026:14','408 巩固 14','{"Stem":"DMA 采用周期挪用方式占用总线时，CPU 可能受到什么影响？","Options":["所有寄存器内容被清空","主频永久降低","必须复制每个数据块","需要访问同一总线时短暂等待"],"Answer":3,"Explanation":"DMA 临时取得总线使用权，CPU 同时需要访问总线时须等待，内部独立操作可继续。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.PROCESS.STATE','408-diagnostic-2026:15','408 前测 15','{"Stem":"阻塞进程等待的 I/O 完成后，在尚未获得 CPU 前转入哪种状态？","Options":["终止","就绪","运行","新建"],"Answer":1,"Explanation":"等待条件消失只代表具备运行资格，仍需调度分配处理机才进入运行状态。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.PROCESS.STATE','408-practice-2026:15','408 巩固 15','{"Stem":"单处理机上，运行进程因时间片用尽被抢占且未等待 I/O，接下来处于什么状态？","Options":["终止","新建","就绪","阻塞"],"Answer":2,"Explanation":"时间片用尽使进程失去处理机，但它仍满足运行条件，因此重新进入就绪队列。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.SCHED.RR','408-diagnostic-2026:16','408 前测 16','{"Stem":"时间片轮转使用固定时间片。将时间片设得极大时，行为接近哪种调度？","Options":["先来先服务","短作业优先","最短剩余时间优先","最高响应比优先"],"Answer":0,"Explanation":"时间片足够大时，通常每个进程能在首次获得 CPU 后运行至完成或阻塞，效果接近先来先服务。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.SCHED.RR','408-practice-2026:16','408 巩固 16','{"Stem":"时间片轮转中，进程运行了一个时间片且尚未结束，通常被放到哪里？","Options":["终止队列","就绪队列尾部","阻塞队列尾部","就绪队列头部"],"Answer":1,"Explanation":"轮转把没完成且没有等待事件的运行进程放回就绪队列尾部，让等待者获得 CPU。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.SYNC.SEM','408-diagnostic-2026:17','408 前测 17','{"Stem":"用一个信号量记录容量为 N 的缓冲区中空槽的数量。初始缓冲区为空时，该信号量应设为多少？","Options":["0","1","−N","N"],"Answer":3,"Explanation":"每个空槽代表一个可用资源，初始有 N 个可用空槽，生产者存入前需先申请一个。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.SYNC.SEM','408-practice-2026:17','408 巩固 17','{"Stem":"只用一个信号量约束同一时刻至多一个进程进入临界区，该信号量初值应是多少？","Options":["1","0","2","进程个数"],"Answer":0,"Explanation":"初始只有一个进入许可；进入前申请，离开后释放，即可实现互斥。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.DEADLOCK.COND','408-diagnostic-2026:18','408 前测 18','{"Stem":"为消除循环等待，可规定进程采用哪种资源申请规则？","Options":["优先申请编号最低的任意资源且允许降序追加","只要求每种资源互斥使用","始终按统一资源编号的递增顺序申请","遇到阻塞就继续持有所有资源并随机申请"],"Answer":2,"Explanation":"若按严格递增的全局顺序获取资源，等待链的编号不可能闭合成环，因此破坏循环等待条件。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.DEADLOCK.COND','408-practice-2026:18','408 巩固 18','{"Stem":"四个死锁必要条件中，允许系统强制收回进程已占有的资源，直接破坏了哪项？","Options":["互斥条件","循环等待条件","请求并保持条件","不剥夺条件"],"Answer":3,"Explanation":"不剥夺指资源只能由占用者主动释放。允许系统强制收回，直接破坏该条件。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.PAGING.ADDR','408-diagnostic-2026:19','408 前测 19','{"Stem":"页大小为 4 KiB，十进制逻辑地址为 8200。页号和页内偏移分别是多少？","Options":["3 和 8","2 和 8","1 和 4104","2 和 16"],"Answer":1,"Explanation":"一页有 4096 字节，8200 除以 4096 的商为 2、余数为 8，页号从 0 开始。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.PAGING.ADDR','408-practice-2026:19','408 巩固 19','{"Stem":"页大小 1024 字节，页表将逻辑页 2 映射到物理块 5。逻辑地址 2051 对应的物理地址是多少？","Options":["2051","6147","5123","5120"],"Answer":2,"Explanation":"2051 属于逻辑页 2、页内偏移为 3；换成物理块 5 后地址为 5×1024+3。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('OS.VIRTUAL.DEMAND','408-diagnostic-2026:20','408 前测 20','{"Stem":"请求分页系统中，访问一个合法但当前不在内存的页面时，首先触发什么？","Options":["缺页异常","死锁检测","时钟中断","非法地址异常"],"Answer":0,"Explanation":"地址合法而页表显示页面不在内存，硬件触发缺页异常，操作系统随后负责调页。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('OS.VIRTUAL.DEMAND','408-practice-2026:20','408 巩固 20','{"Stem":"采用请求分页时，为什么程序通常可以在只有部分页面驻留时运行？","Options":["不需要维护虚拟地址到物理地址的映射","程序在一段时间内往往集中访问一小部分页面","处理器能直接执行磁盘中的指令","所有页面在一次访存时一起调入"],"Answer":1,"Explanation":"时间或空间局部性使当前活跃访问集中于少量页面，未驻留页需要时才调入。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CN.LINK.CRC','408-diagnostic-2026:21','408 前测 21','{"Stem":"接收方按双方约定的生成多项式进行 CRC 校验，若得到非零余数，应得出什么结论？","Options":["已经确定每一个错误比特的位置","一定可以自动恢复原始帧","传输没有发生错误","检测到了传输差错"],"Answer":3,"Explanation":"正确编码的帧应能被生成多项式整除；非零余数可检出差错，但常规 CRC 校验不能直接纠错。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CN.LINK.CRC','408-practice-2026:21','408 巩固 21','{"Stem":"接收方对收到的帧做 CRC 运算，余数为零意味着什么？","Options":["本次校验未检出差错","一定没有任何差错","已经修复全部差错","该帧一定是重复帧"],"Answer":0,"Explanation":"某些错误模式仍可能产生零余数，零余数只能说明未检出错误。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CN.IP.SUB','408-diagnostic-2026:22','408 前测 22','{"Stem":"传统 IPv4 子网 192.168.1.64/26 中，可分配给普通主机的地址个数是多少？","Options":["63","30","62","64"],"Answer":2,"Explanation":"主机位有 6 位，共 64 个地址，扣除全零网络地址与全一广播地址后为 62 个。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CN.IP.SUB','408-practice-2026:22','408 巩固 22','{"Stem":"IPv4 地址 192.168.1.130/26 所在子网的网络地址是什么？","Options":["192.168.1.64","192.168.1.130","192.168.1.192","192.168.1.128"],"Answer":3,"Explanation":"前缀长度为 26 时最后一字节按 64 一段分块，130 落在 128 到 191 这段，网络地址的主机位清零。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CN.TCP.RELIABLE.ACK','408-diagnostic-2026:23','408 前测 23','{"Stem":"接收方期待字节 1000，随后按序收到从字节 1000 开始的 200 个字节。它的累计确认号是多少？","Options":["200","1200","1000","1199"],"Answer":1,"Explanation":"已连续接收 1000 至 1199，累计确认号指出下一个期望字节的序号，所以为 1200。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CN.TCP.RELIABLE.ACK','408-practice-2026:23','408 巩固 23','{"Stem":"TCP 发送方收到累计确认号 7000。在该连接的序号空间内，该确认号直接表示什么？","Options":["本次收到的数据有 7000 字节","接收窗口大小是 7000 字节","接收方下一步期待序号 7000 的字节","序号 7000 的字节也一定已收到"],"Answer":2,"Explanation":"累计确认号是下一个期望的序号，确认范围到该序号之前，窗口大小在另一个字段。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}'),
  ('CN.APP.DNS','408-diagnostic-2026:24','408 前测 24','{"Stem":"无缓存时，主机向配置的本地 DNS 解析器发送递归查询。本地解析器通常负责什么？","Options":["代表主机继续查询并返回最终应答","改写网页内容后返回","为每个域名分配一个 TCP 连接","把获取 IP 地址的工作交给浏览器页面"],"Answer":0,"Explanation":"递归解析器负责获取最终应答，可以向其他名称服务器迭代查询，并按规则缓存结果。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创诊断题"}'),
  ('CN.APP.DNS','408-practice-2026:24','408 巩固 24','{"Stem":"DNS 资源记录的 TTL 到期后，缓存解析器继续解析该名称时应如何处理？","Options":["自动把域名改成 IP","重新查询或使用仍有效的其他来源","永久沿用过期记录","提高手动配置的网络前缀长度"],"Answer":1,"Explanation":"TTL 限制缓存记录可直接使用的时长。过期记录不能作为普通有效缓存，通常需要重新解析。","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":2,"DifficultyOrigin":"editorial","EstimatedMinutes":3,"Source":"2026 408 考纲范围 · 项目原创巩固题"}')
) AS v(code,seed,title,payload)
JOIN exam408_knowledge_points p ON p.code=v.code
CROSS JOIN (SELECT user_id FROM knowledge_bases WHERE is_system ORDER BY id LIMIT 1) kb
ON CONFLICT(seed_key) DO NOTHING;

INSERT INTO learning_assets(user_id,kind,point_id,title,payload,status,is_public,seed_key)
SELECT a.user_id,'material',a.point_id,p.name || ' · 复习要点',
       jsonb_build_object('Stem',a.payload->>'Explanation','Source','2026 408 考纲范围 · 项目原创复习资料','Answer',-1,'EstimatedMinutes',8),
       'ready',true,replace(a.seed_key,'408-practice-2026:','408-review-2026:')
FROM learning_assets a JOIN exam408_knowledge_points p ON p.id=a.point_id
WHERE a.seed_key LIKE '408-practice-2026:%'
ON CONFLICT(seed_key) DO NOTHING;

UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
COMMIT;
