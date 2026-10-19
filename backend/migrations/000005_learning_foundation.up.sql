BEGIN;
CREATE TABLE document_jobs (
 document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
 attempts integer NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','completed','failed')),
 available_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO document_jobs(document_id) SELECT id FROM documents WHERE status IN ('uploaded','processing');
ALTER TABLE messages ADD COLUMN sequence bigint GENERATED ALWAYS AS IDENTITY;
CREATE INDEX messages_conversation_sequence ON messages(conversation_id,sequence);
CREATE TABLE learning_profiles (
 user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 payload jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE learning_assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('question','video','material')),
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id),
 title text NOT NULL, payload jsonb NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','published')),
 revision integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX learning_assets_owner ON learning_assets(user_id,status,kind);
CREATE TABLE assessments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('diagnostic','practice','retest')),
 questions jsonb NOT NULL, result jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), submitted_at timestamptz
);
CREATE INDEX assessments_owner ON assessments(user_id,created_at DESC);
CREATE TABLE study_tasks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 asset_id uuid NOT NULL REFERENCES learning_assets(id) ON DELETE CASCADE,
 reason text NOT NULL, completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX study_tasks_active ON study_tasks(user_id,asset_id) WHERE completed_at IS NULL;
COMMIT;
