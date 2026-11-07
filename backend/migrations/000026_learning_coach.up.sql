BEGIN;
ALTER TABLE study_tasks ADD COLUMN superseded boolean NOT NULL DEFAULT false;
CREATE TABLE learner_memories (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 memory_key text NOT NULL, point_id uuid REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('preference','goal','doubt')), content text NOT NULL,
 resolved boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,memory_key)
);
CREATE TABLE point_frequency_sources (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 organization text NOT NULL, title text NOT NULL, url text NOT NULL, excerpt text NOT NULL,
 start_year int NOT NULL, end_year int NOT NULL, exams int NOT NULL, appearances int NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(point_id,organization,start_year,end_year)
);
CREATE TABLE coaching_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
 knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 teacher text NOT NULL, lesson jsonb NOT NULL, feedback jsonb, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX coaching_sessions_owner ON coaching_sessions(user_id,created_at DESC);
-- Preserve point IDs, assets and scores while fixing two inherited directory
-- groupings. These are curricular containment edges, not inferred causality.
INSERT INTO exam408_knowledge_points(subject_id,code,name,description,level,sort_order)
SELECT id,'DS.SORTING','排序','内部排序与外部排序的课程目录',1,40 FROM exam408_subjects WHERE code='DS' ON CONFLICT(code) DO NOTHING;
UPDATE exam408_knowledge_points child SET parent_id=root.id FROM exam408_knowledge_points root,exam408_knowledge_points old_parent
WHERE child.code='DS.SORT' AND root.code='DS.SORTING' AND old_parent.id=child.parent_id AND old_parent.code='DS.GRAPH';
INSERT INTO exam408_knowledge_points(subject_id,code,name,description,level,sort_order)
SELECT id,'CN.APPLICATION','应用层','DNS、HTTP、邮件与文件传输的课程目录',1,40 FROM exam408_subjects WHERE code='CN' ON CONFLICT(code) DO NOTHING;
UPDATE exam408_knowledge_points child SET parent_id=root.id FROM exam408_knowledge_points root,exam408_knowledge_points old_parent
WHERE child.code='CN.APP' AND root.code='CN.APPLICATION' AND old_parent.id=child.parent_id AND old_parent.code='CN.TRANSPORT';
UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
COMMIT;
