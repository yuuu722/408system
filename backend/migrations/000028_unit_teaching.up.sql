BEGIN;
CREATE TABLE learning_unit_lessons (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 resource_key text NOT NULL,
 payload jsonb NOT NULL,
 source_fingerprint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,point_id,resource_key)
);
CREATE TABLE learning_example_attempts (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 resource_key text NOT NULL,
 assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
 feedback jsonb,
 PRIMARY KEY(user_id,point_id,resource_key,assessment_id)
);
CREATE INDEX learning_example_attempt_owner ON learning_example_attempts(user_id,assessment_id);
COMMIT;
