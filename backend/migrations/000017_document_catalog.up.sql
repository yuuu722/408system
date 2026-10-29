CREATE TABLE IF NOT EXISTS document_catalog (
 document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
 resource_type text NOT NULL DEFAULT 'other',
 subject_code text NOT NULL DEFAULT '',
 purpose text NOT NULL DEFAULT 'knowledge' CHECK(purpose IN ('knowledge','questions','both')),
 review_state text NOT NULL DEFAULT 'pending' CHECK(review_state IN ('pending','confirmed')),
 reason text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Previously extracted public items have not been manually checked. Preserve
-- content and assessment snapshots, but keep these candidates out of new tests.
UPDATE learning_assets SET status='unavailable',
 payload=jsonb_set(payload,'{ReviewStatus}','"pending"'::jsonb),revision=revision+1
 WHERE is_public AND COALESCE(payload->>'ExtractionKey','')<>''
 AND COALESCE(payload->>'ReviewStatus','')='';
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_resource_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_resource_type_check
 CHECK(resource_type IN ('textbook','notes','past_exam','solution','courseware','outline','other'));
