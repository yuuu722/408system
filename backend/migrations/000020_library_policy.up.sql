ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_resource_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_resource_type_check CHECK(resource_type IN ('textbook','notes','past_exam','exercise','mock_exam','solution','courseware','outline','other'));
ALTER TABLE users ADD COLUMN personal_kb_limit integer NOT NULL DEFAULT 5 CHECK(personal_kb_limit BETWEEN 0 AND 100);
