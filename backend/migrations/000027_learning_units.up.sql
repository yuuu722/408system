BEGIN;
CREATE TABLE learning_unit_steps (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 step text NOT NULL CHECK(step IN ('knowledge','example')),
 resource_key text NOT NULL,
 completed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,point_id,step,resource_key)
);
CREATE TABLE learning_unit_checks (
 plan_id uuid NOT NULL REFERENCES daily_plans(id) ON DELETE CASCADE,
 point_id uuid NOT NULL REFERENCES exam408_knowledge_points(id) ON DELETE CASCADE,
 assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
 PRIMARY KEY(plan_id,point_id,assessment_id)
);
CREATE TABLE learning_unit_examples (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 family_id text NOT NULL,
 first_seen_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,family_id)
);
COMMIT;
