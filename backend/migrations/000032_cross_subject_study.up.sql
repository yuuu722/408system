BEGIN;
-- Curated conceptual links. They do not serve as textbook claim evidence.
INSERT INTO exam408_knowledge_point_relations(from_point_id,to_point_id,relation_type)
SELECT a.id,b.id,'related'
FROM (VALUES
 ('DS.GRAPHAPP.DIJKSTRA','CN.ROUTE.OSPF'),
 ('DS.GRAPHAPP.DIJKSTRA','CN.ROUTE.LS'),
 ('CO.CPU.INTERRUPT','OS.INTRO.INT'),
 ('CO.CPU.EXCEPTION','OS.VIRTUAL.FAULT'),
 ('CO.RAM.TLB','OS.PAGING.ADDR'),
 ('CO.RAM.VM','OS.VIRTUAL'),
 ('CO.IO.DMA','OS.IO.DRIVER'),
 ('DS.STACK.QUEUE','OS.SCHED.RR'),
 ('CO.CACHE.REPLACE','OS.VIRTUAL.REPLACE')
) AS links(left_code,right_code)
JOIN exam408_knowledge_points a ON a.code=links.left_code
JOIN exam408_knowledge_points b ON b.code=links.right_code
WHERE a.subject_id<>b.subject_id
ON CONFLICT DO NOTHING;

INSERT INTO learning_assets(user_id,kind,point_id,title,payload,status,is_public,seed_key)
SELECT kb.user_id,v.kind,p.id,v.title,
       v.payload::jsonb || '{"Source":"408 考纲范围 · 项目原创学习资源","AnswerOrigin":"source","QuestionOrigin":"editorial","Difficulty":1}'::jsonb,
       'ready',true,v.seed
FROM (VALUES
 ('material','408-review-2026:process-note','进程 · 定义与易错点','{"Stem":"**进程**是程序的一次执行活动。程序描述要执行的操作，进程还包含运行状态和所持资源。操作系统通过进程控制块记录进程状态及调度所需的信息。","Explanation":"**例子**：同一个编辑器程序打开两个独立进程，可以使用同一个可执行文件，但两个进程的执行位置和私有数据可以不同。**易错点**：等待 I/O 的进程处于阻塞状态；事件完成后先进入就绪状态，由调度决定何时运行。"}'),
 ('question','408-practice-2026:process-concept','进程例题 · 程序与进程','{"Stem":"同一个可执行程序可以启动为两个不同进程。哪个描述正确？","Options":["两者必定共享同一个进程控制块","两者可以有不同的执行状态","两者的私有变量只能使用同一份数据","只允许其中一个进入就绪队列"],"Answer":1,"Explanation":"同一个程序可以对应多次独立执行。进程控制块和执行上下文随进程建立，因此两次执行的状态可以不同。"}'),
 ('question','408-practice-2026:process-block','进程练习 · 主动等待','{"Stem":"某进程正在运行，发出同步磁盘读取请求并等待结果。假设该请求不能立即完成，进程转入什么状态？","Options":["运行","就绪","阻塞","终止"],"Answer":2,"Explanation":"进程在等待 I/O 事件完成，暂时没有继续运行的条件，因此转入阻塞状态。就绪进程只等待获得处理机。"}'),
 ('question','408-practice-2026:process-preemption','进程复测 · 时间片耗尽','{"Stem":"采用时间片轮转调度时，一个正在执行的进程时间片用完，且它仍可继续执行。此时通常进入什么状态？","Options":["阻塞","就绪","终止","创建"],"Answer":1,"Explanation":"该进程没有等待其他事件，只是让出处理机，因此返回就绪队列等待下次调度。"}')
) AS v(kind,seed,title,payload)
JOIN exam408_knowledge_points p ON p.code='OS.PROCESS.PROC'
CROSS JOIN (SELECT user_id FROM knowledge_bases WHERE is_system LIMIT 1) kb
ON CONFLICT(seed_key) DO NOTHING;
COMMIT;
