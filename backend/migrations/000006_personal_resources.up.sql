BEGIN;
ALTER TABLE learning_assets DROP CONSTRAINT learning_assets_status_check;
UPDATE learning_assets SET status='unavailable';
ALTER TABLE learning_assets ADD CONSTRAINT learning_assets_status_check CHECK(status IN ('ready','unavailable'));
ALTER TABLE learning_assets ALTER COLUMN status SET DEFAULT 'unavailable';
ALTER TABLE learning_assets ALTER COLUMN point_id DROP NOT NULL;
ALTER TABLE learning_assets ADD COLUMN document_id uuid REFERENCES documents(id) ON DELETE CASCADE;
UPDATE learning_assets a SET document_id=d.id FROM documents d WHERE a.payload->>'DocumentID'=d.id::text AND a.user_id=d.user_id;
UPDATE learning_assets SET payload=payload || jsonb_build_object('AnswerOrigin',CASE WHEN COALESCE((payload->>'Answer')::int,-1)>=0 THEN 'source' ELSE 'ai_pending' END);
CREATE TABLE document_parses (
 document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
 payload jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX learning_assets_extracted ON learning_assets(user_id,(payload->>'DocumentID'),(payload->>'ExtractionKey')) WHERE COALESCE(payload->>'ExtractionKey','')<>'';
COMMIT;
