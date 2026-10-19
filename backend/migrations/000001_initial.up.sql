BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), username varchar(64) NOT NULL UNIQUE,
  email varchar(255) NOT NULL UNIQUE, password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_bases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name varchar(120) NOT NULL, description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, name)
);
CREATE TABLE documents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, filename text NOT NULL,
  original_filename text NOT NULL, mime_type varchar(120) NOT NULL, size bigint NOT NULL CHECK(size >= 0),
  storage_path text NOT NULL, resource_type varchar(32) NOT NULL DEFAULT 'other'
    CHECK(resource_type IN ('textbook','notes','past_exam','solution','courseware','other')),
  status varchar(24) NOT NULL DEFAULT 'uploaded' CHECK(status IN ('uploaded','processing','ready','failed')),
  error_message text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documents_owner_kb_idx ON documents(user_id, knowledge_base_id, created_at DESC);
CREATE TABLE document_chunks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, content text NOT NULL,
  chunk_index integer NOT NULL, source_location text NOT NULL DEFAULT '', metadata jsonb NOT NULL DEFAULT '{}',
  embedding vector(1536), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(document_id, chunk_index)
);
CREATE INDEX document_chunks_owner_kb_idx ON document_chunks(user_id, knowledge_base_id);
CREATE INDEX document_chunks_embedding_idx ON document_chunks USING hnsw (embedding vector_cosine_ops);

CREATE TABLE exam408_subjects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code varchar(16) NOT NULL UNIQUE, name varchar(64) NOT NULL,
  sort_order integer NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE exam408_knowledge_points (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), subject_id uuid NOT NULL REFERENCES exam408_subjects(id) ON DELETE CASCADE,
  parent_id uuid REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE, code varchar(64) NOT NULL UNIQUE,
  name varchar(120) NOT NULL, description text NOT NULL DEFAULT '', keywords text[] NOT NULL DEFAULT '{}',
  level integer NOT NULL DEFAULT 1, sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE exam408_knowledge_point_relations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), from_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  to_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  relation_type varchar(24) NOT NULL CHECK(relation_type IN ('prerequisite','related')),
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(from_point_id,to_point_id,relation_type)
);
CREATE TABLE exam408_resource_mappings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  document_id uuid REFERENCES documents(id) ON DELETE CASCADE, chunk_id uuid REFERENCES document_chunks(id) ON DELETE CASCADE,
  confidence real NOT NULL DEFAULT 1 CHECK(confidence BETWEEN 0 AND 1), source varchar(24) NOT NULL DEFAULT 'manual',
  created_at timestamptz NOT NULL DEFAULT now(), CHECK(document_id IS NOT NULL OR chunk_id IS NOT NULL)
);
CREATE INDEX resource_mappings_owner_point_idx ON exam408_resource_mappings(user_id, knowledge_point_id);
CREATE UNIQUE INDEX resource_mappings_document_unique ON exam408_resource_mappings(user_id,knowledge_point_id,document_id) WHERE document_id IS NOT NULL;
CREATE UNIQUE INDEX resource_mappings_chunk_unique ON exam408_resource_mappings(user_id,knowledge_point_id,chunk_id) WHERE chunk_id IS NOT NULL;
CREATE TABLE knowledge_entities (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  name varchar(200) NOT NULL, entity_type varchar(64) NOT NULL DEFAULT 'concept', description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(knowledge_base_id, name, entity_type)
);
CREATE TABLE knowledge_relations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  from_entity_id uuid NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
  to_entity_id uuid NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
  relation_type varchar(64) NOT NULL, evidence_chunk_id uuid REFERENCES document_chunks(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE exam408_entity_mappings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  entity_id uuid NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
  confidence real NOT NULL DEFAULT 0.5, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, knowledge_point_id, entity_id)
);
CREATE TABLE conversations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  title varchar(200) NOT NULL DEFAULT '新对话', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, role varchar(16) NOT NULL CHECK(role IN ('user','assistant')),
  content text NOT NULL, citations jsonb NOT NULL DEFAULT '[]', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE exam408_learning_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  subject_id uuid REFERENCES exam408_subjects(id) ON DELETE SET NULL,
  knowledge_point_id uuid REFERENCES exam408_knowledge_points(id) ON DELETE SET NULL,
  resource_id uuid, event_type varchar(40) NOT NULL CHECK(event_type IN
    ('resource_view','resource_complete','rag_question','knowledge_point_view','quiz_correct','quiz_wrong','recommendation_view','recommendation_click')),
  metadata jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX learning_events_owner_time_idx ON exam408_learning_events(user_id, created_at DESC);
CREATE TABLE exam408_user_mastery (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  mastery_score real NOT NULL DEFAULT 0 CHECK(mastery_score BETWEEN 0 AND 1),
  weakness_score real NOT NULL DEFAULT 0 CHECK(weakness_score BETWEEN 0 AND 1),
  confidence real NOT NULL DEFAULT 0 CHECK(confidence BETWEEN 0 AND 1),
  interaction_count integer NOT NULL DEFAULT 0, last_interaction_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id, knowledge_point_id)
);
CREATE TABLE exam408_recommendations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  knowledge_point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
  score real NOT NULL, reason text NOT NULL, recommended_action varchar(64) NOT NULL,
  status varchar(16) NOT NULL DEFAULT 'active', generated_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days'
);
CREATE INDEX recommendations_owner_score_idx ON exam408_recommendations(user_id,status,score DESC);

INSERT INTO exam408_subjects(code,name,sort_order) VALUES
('DS','数据结构',1),('CO','计算机组成原理',2),('OS','操作系统',3),('CN','计算机网络',4);
INSERT INTO exam408_knowledge_points(subject_id,code,name,description,keywords,level,sort_order)
SELECT id,'DS.LINEAR','线性表','线性表、栈、队列及其应用',ARRAY['线性表','栈','队列'],1,10 FROM exam408_subjects WHERE code='DS'
UNION ALL SELECT id,'DS.TREE','树与二叉树','树、二叉树、遍历与应用',ARRAY['树','二叉树','遍历'],1,20 FROM exam408_subjects WHERE code='DS'
UNION ALL SELECT id,'DS.GRAPH','图','图的存储、遍历与最短路径',ARRAY['图','最短路径','拓扑排序'],1,30 FROM exam408_subjects WHERE code='DS'
UNION ALL SELECT id,'CO.CPU','中央处理器','指令系统、数据通路与控制器',ARRAY['CPU','指令','数据通路'],1,10 FROM exam408_subjects WHERE code='CO'
UNION ALL SELECT id,'CO.MEMORY','存储系统','层次化存储、Cache 和主存',ARRAY['Cache','主存','存储'],1,20 FROM exam408_subjects WHERE code='CO'
UNION ALL SELECT id,'OS.PROCESS','进程与线程','进程、线程、调度、同步与死锁',ARRAY['进程','线程','调度','死锁'],1,10 FROM exam408_subjects WHERE code='OS'
UNION ALL SELECT id,'OS.MEMORY','内存管理','分页、分段与虚拟内存',ARRAY['内存管理','虚拟内存','分页','页面置换'],1,20 FROM exam408_subjects WHERE code='OS'
UNION ALL SELECT id,'OS.FILE','文件管理','文件系统和磁盘管理',ARRAY['文件系统','磁盘'],1,30 FROM exam408_subjects WHERE code='OS'
UNION ALL SELECT id,'CN.LAYER','网络体系结构','OSI 与 TCP/IP 分层体系',ARRAY['OSI','TCP/IP','网络体系'],1,10 FROM exam408_subjects WHERE code='CN'
UNION ALL SELECT id,'CN.TRANSPORT','传输层','TCP、UDP、可靠传输与拥塞控制',ARRAY['TCP','UDP','拥塞控制'],1,20 FROM exam408_subjects WHERE code='CN'
UNION ALL SELECT id,'CN.NETWORK','网络层','IP、路由与网络互联',ARRAY['IP','路由','子网'],1,30 FROM exam408_subjects WHERE code='CN';
INSERT INTO exam408_knowledge_point_relations(from_point_id,to_point_id,relation_type)
SELECT a.id,b.id,'prerequisite' FROM exam408_knowledge_points a, exam408_knowledge_points b
WHERE (a.code,b.code) IN (('CO.MEMORY','OS.MEMORY'),('CN.LAYER','CN.NETWORK'),('CN.NETWORK','CN.TRANSPORT'));
COMMIT;
