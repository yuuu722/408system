BEGIN;
CREATE TABLE daily_plans(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 plan_date date NOT NULL,budget_minutes integer NOT NULL CHECK(budget_minutes BETWEEN 10 AND 600),
 strategy text NOT NULL DEFAULT '408-rules-v1',created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(user_id,plan_date)
);
ALTER TABLE study_tasks ADD COLUMN plan_id uuid REFERENCES daily_plans(id) ON DELETE CASCADE;
ALTER TABLE study_tasks ADD COLUMN estimated_minutes integer NOT NULL DEFAULT 0;
ALTER TABLE study_tasks ADD COLUMN duration_source text NOT NULL DEFAULT 'unknown';
ALTER TABLE study_tasks ADD COLUMN priority real NOT NULL DEFAULT 0;
CREATE TABLE assessment_result_changes(
 sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,question_id text NOT NULL,
 occurred_at timestamptz NOT NULL,confirmed_at timestamptz NOT NULL DEFAULT now(),snapshot jsonb NOT NULL
);
CREATE INDEX result_changes_attempt ON assessment_result_changes(assessment_id,question_id,sequence DESC);
CREATE TABLE question_generations(
 id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,request_hash text NOT NULL,
 state text NOT NULL DEFAULT 'running' CHECK(state IN('running','complete','failed')),assets jsonb NOT NULL DEFAULT '[]',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX question_generations_budget ON question_generations(user_id,created_at);
CREATE UNIQUE INDEX learning_assets_generated_hash ON learning_assets(user_id,(payload->>'ContentHash')) WHERE payload->>'QuestionOrigin'='ai_generated' AND COALESCE(payload->>'ContentHash','')<>'';
COMMIT;
