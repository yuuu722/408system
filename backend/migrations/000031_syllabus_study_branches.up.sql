BEGIN;
INSERT INTO exam408_knowledge_points(subject_id,parent_id,code,name,description,keywords,level,sort_order,syllabus_priority)
SELECT p.subject_id,p.id,v.code,v.name,format('2026 年 408 大纲附件第 %s 页；目录覆盖项。',v.page),v.terms,p.level+1,v.page,3
FROM (VALUES
  ('CO.RAM','CO.RAM.CLASS','存储器分类',4,ARRAY['存储器的分类']),
  ('OS.FILES','OS.FILES.PROTECTION','文件保护',7,ARRAY['文件的保护']),
  ('OS.FILES','OS.FILES.LOGICAL','文件逻辑结构',7,ARRAY['文件的逻辑结构']),
  ('OS.FILES','OS.FILES.PHYSICAL','文件物理结构',7,ARRAY['文件的物理结构']),
  ('CN.PHYSICAL','CN.PHYSICAL.REPEATER','中继器',8,ARRAY[]::text[]),
  ('CN.PHYSICAL','CN.PHYSICAL.HUB','集线器',8,ARRAY['Hub']),
  ('CN.LINK','CN.LINK.FEC','纠错编码',9,ARRAY['前向纠错']),
  ('CN.MAC','CN.MAC.TOKEN','令牌传递',9,ARRAY['令牌传递协议']),
  ('CN.ROUTE','CN.ROUTE.STATIC','静态路由',9,ARRAY[]::text[]),
  ('CN.ROUTE','CN.ROUTE.DYNAMIC','动态路由',9,ARRAY[]::text[]),
  ('CN.ROUTE','CN.ROUTE.AS','自治系统',10,ARRAY[]::text[]),
  ('CN.APP.DNS','CN.APP.DNS.RESOLUTION','域名解析过程',11,ARRAY['DNS 解析过程','DNS解析过程'])
) AS v(parent,code,name,page,terms)
JOIN exam408_knowledge_points p ON p.code=v.parent
ON CONFLICT(code) DO NOTHING;

-- Existing original questions explicitly ask how the resolver performs a lookup.
UPDATE learning_assets a SET point_id=p.id
FROM exam408_knowledge_points p
WHERE p.code='CN.APP.DNS.RESOLUTION'
  AND a.seed_key IN ('408-diagnostic-2026:24','408-practice-2026:24','408-review-2026:24')
  AND a.point_id=(SELECT id FROM exam408_knowledge_points WHERE code='CN.APP.DNS');
UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
COMMIT;
