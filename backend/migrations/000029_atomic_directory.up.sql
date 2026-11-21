BEGIN;
-- Keep existing point IDs and their learning history. Add independent child concepts.
UPDATE exam408_knowledge_points SET name='算法复杂度' WHERE code='DS.BASIC' AND name IS DISTINCT FROM '算法复杂度';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.BASIC.TIME','时间复杂度',1),
  ('DS.BASIC.SPACE','空间复杂度',2),
  ('DS.BASIC.ADT','抽象数据类型',3)
) AS v(code,name,position) WHERE p.code='DS.BASIC' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='线性表存储结构' WHERE code='DS.SEQ' AND name IS DISTINCT FROM '线性表存储结构';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.SEQ.ARRAY','顺序表',1),
  ('DS.SEQ.SINGLE','单链表',2),
  ('DS.SEQ.DOUBLE','双链表',3),
  ('DS.SEQ.CIRCULAR','循环链表',4)
) AS v(code,name,position) WHERE p.code='DS.SEQ' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='受限线性结构' WHERE code='DS.STACK' AND name IS DISTINCT FROM '受限线性结构';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.STACK.STACK','栈',1),
  ('DS.STACK.QUEUE','队列',2),
  ('DS.STACK.CIRCULAR','循环队列',3),
  ('DS.STACK.DEQUE','双端队列',4),
  ('DS.STACK.ARRAY','数组',5),
  ('DS.STACK.SPARSE','稀疏矩阵',6)
) AS v(code,name,position) WHERE p.code='DS.STACK' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='串' WHERE code='DS.STRING' AND name IS DISTINCT FROM '串';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.STRING.MATCH','模式匹配',1),
  ('DS.STRING.KMP','KMP 算法',2)
) AS v(code,name,position) WHERE p.code='DS.STRING' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='树' WHERE code='DS.TREE' AND name IS DISTINCT FROM '树';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.TREE.BT','二叉树',1),
  ('DS.TREE.FOREST','森林',2)
) AS v(code,name,position) WHERE p.code='DS.TREE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='二叉树遍历' WHERE code='DS.BINARY' AND name IS DISTINCT FROM '二叉树遍历';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.BINARY.PRE','先序遍历',1),
  ('DS.BINARY.IN','中序遍历',2),
  ('DS.BINARY.POST','后序遍历',3),
  ('DS.BINARY.LEVEL','层序遍历',4),
  ('DS.BINARY.THREAD','线索二叉树',5)
) AS v(code,name,position) WHERE p.code='DS.BINARY' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='树的应用' WHERE code='DS.TREEAPP' AND name IS DISTINCT FROM '树的应用';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.TREEAPP.CS','孩子兄弟表示法',1),
  ('DS.TREEAPP.HUFFMAN','哈夫曼树',2),
  ('DS.TREEAPP.HUFFMAN_CODE','哈夫曼编码',3),
  ('DS.TREEAPP.UNION','并查集',4)
) AS v(code,name,position) WHERE p.code='DS.TREEAPP' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='查找算法' WHERE code='DS.SEARCH' AND name IS DISTINCT FROM '查找算法';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.SEARCH.LINEAR','顺序查找',1),
  ('DS.SEARCH.BINARY','折半查找',2),
  ('DS.SEARCH.BLOCK','分块查找',3),
  ('DS.SEARCH.BST','二叉排序树',4),
  ('DS.SEARCH.AVL','平衡二叉树',5),
  ('DS.SEARCH.RB','红黑树',6),
  ('DS.SEARCH.BTREE','B 树',7),
  ('DS.SEARCH.BPLUS','B+ 树',8)
) AS v(code,name,position) WHERE p.code='DS.SEARCH' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='散列表' WHERE code='DS.HASH' AND name IS DISTINCT FROM '散列表';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.HASH.FUNC','散列函数',1),
  ('DS.HASH.COLLISION','散列冲突',2),
  ('DS.HASH.OPEN','开放定址法',3),
  ('DS.HASH.CHAIN','链地址法',4)
) AS v(code,name,position) WHERE p.code='DS.HASH' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='图的存储结构' WHERE code='DS.GRAPHSTORE' AND name IS DISTINCT FROM '图的存储结构';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.GRAPHSTORE.MATRIX','邻接矩阵',1),
  ('DS.GRAPHSTORE.LIST','邻接表',2),
  ('DS.GRAPHSTORE.DFS','深度优先遍历',3),
  ('DS.GRAPHSTORE.BFS','广度优先遍历',4)
) AS v(code,name,position) WHERE p.code='DS.GRAPHSTORE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='图算法' WHERE code='DS.GRAPHAPP' AND name IS DISTINCT FROM '图算法';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.GRAPHAPP.MST','最小生成树',1),
  ('DS.GRAPHAPP.PRIM','Prim 算法',2),
  ('DS.GRAPHAPP.KRUSKAL','Kruskal 算法',3),
  ('DS.GRAPHAPP.SHORTEST','最短路径',4),
  ('DS.GRAPHAPP.DIJKSTRA','Dijkstra 算法',5),
  ('DS.GRAPHAPP.FLOYD','Floyd 算法',6),
  ('DS.GRAPHAPP.TOPO','拓扑排序',7),
  ('DS.GRAPHAPP.CRITICAL','关键路径',8)
) AS v(code,name,position) WHERE p.code='DS.GRAPHAPP' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='排序算法' WHERE code='DS.SORT' AND name IS DISTINCT FROM '排序算法';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('DS.SORT.INSERT','直接插入排序',1),
  ('DS.SORT.SHELL','希尔排序',2),
  ('DS.SORT.BUBBLE','冒泡排序',3),
  ('DS.SORT.QUICK','快速排序',4),
  ('DS.SORT.SELECT','简单选择排序',5),
  ('DS.SORT.HEAP','堆排序',6),
  ('DS.SORT.MERGE','归并排序',7),
  ('DS.SORT.RADIX','基数排序',8),
  ('DS.SORT.EXTERNAL','外部排序',9)
) AS v(code,name,position) WHERE p.code='DS.SORT' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='计算机系统' WHERE code='CO.SYSTEM' AND name IS DISTINCT FROM '计算机系统';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.SYSTEM.CPI','每条指令平均时钟周期数',1),
  ('CO.SYSTEM.PERF','计算机性能指标',2)
) AS v(code,name,position) WHERE p.code='CO.SYSTEM' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='数据表示' WHERE code='CO.NUMBER' AND name IS DISTINCT FROM '数据表示';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.NUMBER.BASE','进制转换',1),
  ('CO.NUMBER.CODES','定点数编码',2),
  ('CO.NUMBER.TWOS','补码',3),
  ('CO.NUMBER.FIXED','定点运算',4),
  ('CO.NUMBER.OVERFLOW','溢出判断',5)
) AS v(code,name,position) WHERE p.code='CO.NUMBER' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='浮点数' WHERE code='CO.FLOAT' AND name IS DISTINCT FROM '浮点数';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.FLOAT.IEEE','IEEE 754',1),
  ('CO.FLOAT.OP','浮点运算',2),
  ('CO.FLOAT.ALU','算术逻辑单元',3)
) AS v(code,name,position) WHERE p.code='CO.FLOAT' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='高速缓存' WHERE code='CO.CACHE' AND name IS DISTINCT FROM '高速缓存';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.CACHE.MAPPING','Cache 映射',1),
  ('CO.CACHE.REPLACE','Cache 替换策略',2),
  ('CO.CACHE.WRITE','Cache 写策略',3),
  ('CO.CACHE.HIT','Cache 命中率',4)
) AS v(code,name,position) WHERE p.code='CO.CACHE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='主存储器' WHERE code='CO.RAM' AND name IS DISTINCT FROM '主存储器';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.RAM.SRAM','静态随机存储器',1),
  ('CO.RAM.DRAM','动态随机存储器',2),
  ('CO.RAM.ROM','只读存储器',3),
  ('CO.RAM.INTERLEAVE','多体交叉存储',4),
  ('CO.RAM.VM','虚拟存储器',5),
  ('CO.RAM.TLB','快表',6)
) AS v(code,name,position) WHERE p.code='CO.RAM' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='指令系统' WHERE code='CO.ISA' AND name IS DISTINCT FROM '指令系统';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.ISA.FORMAT','指令格式',1),
  ('CO.ISA.ADDR','寻址方式',2),
  ('CO.ISA.RISC','精简指令集',3),
  ('CO.ISA.CISC','复杂指令集',4)
) AS v(code,name,position) WHERE p.code='CO.ISA' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='数据通路' WHERE code='CO.DATAPATH' AND name IS DISTINCT FROM '数据通路';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.DATAPATH.CONTROL','控制器',1),
  ('CO.DATAPATH.HARD','硬布线控制',2),
  ('CO.DATAPATH.MICRO','微程序控制',3)
) AS v(code,name,position) WHERE p.code='CO.DATAPATH' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='指令流水线' WHERE code='CO.PIPE' AND name IS DISTINCT FROM '指令流水线';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.PIPE.HAZARD','流水线冒险',1),
  ('CO.PIPE.SPEED','流水线加速比',2),
  ('CO.PIPE.PAR','指令级并行',3)
) AS v(code,name,position) WHERE p.code='CO.PIPE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='总线' WHERE code='CO.BUS' AND name IS DISTINCT FROM '总线';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.BUS.ARB','总线仲裁',1),
  ('CO.BUS.CLK','总线时序',2),
  ('CO.BUS.BW','总线带宽',3)
) AS v(code,name,position) WHERE p.code='CO.BUS' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='输入输出系统' WHERE code='CO.IO' AND name IS DISTINCT FROM '输入输出系统';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CO.IO.PROG','程序查询方式',1),
  ('CO.IO.IRQ','程序中断方式',2),
  ('CO.IO.DMA','直接存储器访问',3)
) AS v(code,name,position) WHERE p.code='CO.IO' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='操作系统' WHERE code='OS.INTRO' AND name IS DISTINCT FROM '操作系统';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.INTRO.MODE','处理器运行模式',1),
  ('OS.INTRO.CALL','系统调用',2),
  ('OS.INTRO.INT','中断',3)
) AS v(code,name,position) WHERE p.code='OS.INTRO' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='进程管理' WHERE code='OS.PROCESS' AND name IS DISTINCT FROM '进程管理';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.PROCESS.PROC','进程',1),
  ('OS.PROCESS.THREAD','线程',2),
  ('OS.PROCESS.PCB','进程控制块',3),
  ('OS.PROCESS.STATE','进程状态',4),
  ('OS.PROCESS.SWITCH','上下文切换',5)
) AS v(code,name,position) WHERE p.code='OS.PROCESS' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='线程管理' WHERE code='OS.THREAD' AND name IS DISTINCT FROM '线程管理';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.THREAD.USER','用户级线程',1),
  ('OS.THREAD.KERNEL','内核级线程',2)
) AS v(code,name,position) WHERE p.code='OS.THREAD' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='处理机调度' WHERE code='OS.SCHED' AND name IS DISTINCT FROM '处理机调度';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.SCHED.FCFS','先来先服务调度',1),
  ('OS.SCHED.SJF','短作业优先调度',2),
  ('OS.SCHED.RR','时间片轮转调度',3),
  ('OS.SCHED.PRIORITY','优先级调度',4),
  ('OS.SCHED.MLFQ','多级反馈队列',5)
) AS v(code,name,position) WHERE p.code='OS.SCHED' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='进程协调' WHERE code='OS.SYNC' AND name IS DISTINCT FROM '进程协调';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.SYNC.SYNC','进程同步',1),
  ('OS.SYNC.MUTEX','进程互斥',2),
  ('OS.SYNC.CS','临界区',3),
  ('OS.SYNC.SEM','信号量',4),
  ('OS.SYNC.MON','管程',5),
  ('OS.SYNC.BUFFER','生产者消费者问题',6)
) AS v(code,name,position) WHERE p.code='OS.SYNC' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='死锁' WHERE code='OS.DEADLOCK' AND name IS DISTINCT FROM '死锁';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.DEADLOCK.COND','死锁必要条件',1),
  ('OS.DEADLOCK.PREVENT','死锁预防',2),
  ('OS.DEADLOCK.AVOID','死锁避免',3),
  ('OS.DEADLOCK.BANK','银行家算法',4),
  ('OS.DEADLOCK.DETECT','死锁检测',5)
) AS v(code,name,position) WHERE p.code='OS.DEADLOCK' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='存储分配' WHERE code='OS.PAGING' AND name IS DISTINCT FROM '存储分配';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.PAGING.PAGE','分页',1),
  ('OS.PAGING.SEG','分段',2),
  ('OS.PAGING.MIX','段页式管理',3),
  ('OS.PAGING.TABLE','页表',4),
  ('OS.PAGING.ADDR','地址变换',5)
) AS v(code,name,position) WHERE p.code='OS.PAGING' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='虚拟内存' WHERE code='OS.VIRTUAL' AND name IS DISTINCT FROM '虚拟内存';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.VIRTUAL.DEMAND','请求分页',1),
  ('OS.VIRTUAL.FAULT','缺页中断',2),
  ('OS.VIRTUAL.REPLACE','页面置换',3),
  ('OS.VIRTUAL.FIFO','先进先出页面置换',4),
  ('OS.VIRTUAL.LRU','最近最久未使用页面置换',5),
  ('OS.VIRTUAL.CLOCK','时钟置换',6),
  ('OS.VIRTUAL.THRASH','抖动',7),
  ('OS.VIRTUAL.WORK','工作集',8)
) AS v(code,name,position) WHERE p.code='OS.VIRTUAL' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='文件系统' WHERE code='OS.FILES' AND name IS DISTINCT FROM '文件系统';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.FILES.DIR','文件目录',1),
  ('OS.FILES.ALLOC','文件分配',2),
  ('OS.FILES.FREE','空闲空间管理',3),
  ('OS.FILES.INODE','索引结点',4)
) AS v(code,name,position) WHERE p.code='OS.FILES' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='设备管理' WHERE code='OS.IO' AND name IS DISTINCT FROM '设备管理';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('OS.IO.SPOOL','假脱机技术',1),
  ('OS.IO.DISK','磁盘调度',2),
  ('OS.IO.SSD','固态硬盘',3),
  ('OS.IO.BUFFER','缓冲区',4)
) AS v(code,name,position) WHERE p.code='OS.IO' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='计算机网络' WHERE code='CN.OVERVIEW' AND name IS DISTINCT FROM '计算机网络';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.OVERVIEW.MODEL','分层参考模型',1),
  ('CN.OVERVIEW.DELAY','网络时延',2),
  ('CN.OVERVIEW.RATE','吞吐量',3)
) AS v(code,name,position) WHERE p.code='CN.OVERVIEW' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='物理层' WHERE code='CN.PHYSICAL' AND name IS DISTINCT FROM '物理层';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.PHYSICAL.CODE','信道编码',1),
  ('CN.PHYSICAL.MOD','调制',2),
  ('CN.PHYSICAL.MULTI','信道复用',3),
  ('CN.PHYSICAL.MEDIA','传输介质',4)
) AS v(code,name,position) WHERE p.code='CN.PHYSICAL' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='数据链路层' WHERE code='CN.LINK' AND name IS DISTINCT FROM '数据链路层';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.LINK.FRAME','成帧',1),
  ('CN.LINK.ERR','差错检测',2),
  ('CN.LINK.CRC','循环冗余校验',3),
  ('CN.LINK.ARQ','自动重传请求',4),
  ('CN.LINK.PPP','PPP',5)
) AS v(code,name,position) WHERE p.code='CN.LINK' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='介质访问控制' WHERE code='CN.MAC' AND name IS DISTINCT FROM '介质访问控制';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.MAC.ETH','以太网',1),
  ('CN.MAC.CD','CSMA/CD',2),
  ('CN.MAC.CA','CSMA/CA',3),
  ('CN.MAC.VLAN','虚拟局域网',4)
) AS v(code,name,position) WHERE p.code='CN.MAC' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='IP 协议' WHERE code='CN.IP' AND name IS DISTINCT FROM 'IP 协议';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.IP.V4','IPv4',1),
  ('CN.IP.V6','IPv6',2),
  ('CN.IP.SUB','子网划分',3),
  ('CN.IP.CIDR','无类别域间路由',4),
  ('CN.IP.NAT','网络地址转换',5),
  ('CN.IP.FRAG','IP 分片',6)
) AS v(code,name,position) WHERE p.code='CN.IP' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='路由选择' WHERE code='CN.ROUTE' AND name IS DISTINCT FROM '路由选择';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.ROUTE.RIP','RIP',1),
  ('CN.ROUTE.OSPF','OSPF',2),
  ('CN.ROUTE.BGP','BGP',3),
  ('CN.ROUTE.ARP','ARP',4),
  ('CN.ROUTE.ICMP','ICMP',5)
) AS v(code,name,position) WHERE p.code='CN.ROUTE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='UDP' WHERE code='CN.UDP' AND name IS DISTINCT FROM 'UDP';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.UDP.PORT','传输层端口',1),
  ('CN.UDP.CHECK','UDP 校验和',2)
) AS v(code,name,position) WHERE p.code='CN.UDP' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='TCP' WHERE code='CN.TCP' AND name IS DISTINCT FROM 'TCP';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.TCP.RELIABLE','TCP 可靠传输',1),
  ('CN.TCP.FLOW','TCP 流量控制',2),
  ('CN.TCP.CONGEST','TCP 拥塞控制',3),
  ('CN.TCP.CONNECT','TCP 连接管理',4)
) AS v(code,name,position) WHERE p.code='CN.TCP' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='TCP 可靠传输' WHERE code='CN.TCP.RELIABLE' AND name IS DISTINCT FROM 'TCP 可靠传输';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.TCP.RELIABLE.SEQ','TCP 序号',1),
  ('CN.TCP.RELIABLE.ACK','TCP 累计确认',2),
  ('CN.TCP.RELIABLE.CHECK','TCP 校验和',3),
  ('CN.TCP.RELIABLE.RETX','TCP 重传',4),
  ('CN.TCP.RELIABLE.ORDER','TCP 按序交付',5)
) AS v(code,name,position) WHERE p.code='CN.TCP.RELIABLE' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='TCP 重传' WHERE code='CN.TCP.RELIABLE.RETX' AND name IS DISTINCT FROM 'TCP 重传';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.TCP.RELIABLE.RETX.TIMEOUT','TCP 超时重传',1),
  ('CN.TCP.RELIABLE.RETX.FAST','TCP 快速重传',2)
) AS v(code,name,position) WHERE p.code='CN.TCP.RELIABLE.RETX' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='TCP 拥塞控制' WHERE code='CN.TCP.CONGEST' AND name IS DISTINCT FROM 'TCP 拥塞控制';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.TCP.CONGEST.SLOW','TCP 慢开始',1),
  ('CN.TCP.CONGEST.AVOID','TCP 拥塞避免',2),
  ('CN.TCP.CONGEST.RECOVER','TCP 快速恢复',3)
) AS v(code,name,position) WHERE p.code='CN.TCP.CONGEST' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='TCP 连接管理' WHERE code='CN.TCP.CONNECT' AND name IS DISTINCT FROM 'TCP 连接管理';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.TCP.CONNECT.HANDSHAKE','TCP 三次握手',1),
  ('CN.TCP.CONNECT.CLOSE','TCP 连接释放',2)
) AS v(code,name,position) WHERE p.code='CN.TCP.CONNECT' ON CONFLICT(code) DO NOTHING;

UPDATE exam408_knowledge_points SET name='应用层协议' WHERE code='CN.APP' AND name IS DISTINCT FROM '应用层协议';
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,keywords,level,sort_order)
SELECT p.subject_id,p.id,v.code,v.name,ARRAY[v.name],p.level+1,v.position
FROM exam408_knowledge_points p CROSS JOIN (VALUES
  ('CN.APP.DNS','DNS',1),
  ('CN.APP.HTTP','HTTP',2),
  ('CN.APP.SMTP','SMTP',3),
  ('CN.APP.POP','POP3',4),
  ('CN.APP.FTP','FTP',5),
  ('CN.APP.DHCP','DHCP',6)
) AS v(code,name,position) WHERE p.code='CN.APP' ON CONFLICT(code) DO NOTHING;

UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
COMMIT;
