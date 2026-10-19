BEGIN;
CREATE TABLE document_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 fingerprint text NOT NULL, file_hash text NOT NULL, parse_key text NOT NULL, chunk_key text NOT NULL, embedding_space text NOT NULL,
 parsed_done boolean NOT NULL DEFAULT false, chunked boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'staging' CHECK(state IN ('staging','complete')),
 total_pages integer NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(document_id,fingerprint)
);
ALTER TABLE documents ADD COLUMN active_revision_id uuid REFERENCES document_revisions(id) ON DELETE SET NULL;
ALTER TABLE document_jobs DROP CONSTRAINT document_jobs_state_check;
ALTER TABLE document_jobs ADD CONSTRAINT document_jobs_state_check CHECK(state IN ('queued','running','completed','failed'));
ALTER TABLE document_jobs ADD COLUMN execution bigint NOT NULL DEFAULT 0;
ALTER TABLE document_jobs ADD COLUMN lease_until timestamptz;
CREATE TABLE revision_pages (
 revision_id uuid NOT NULL REFERENCES document_revisions(id) ON DELETE CASCADE,
 ordinal integer NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(revision_id,ordinal)
);
CREATE TABLE revision_chunks (
 revision_id uuid NOT NULL REFERENCES document_revisions(id) ON DELETE CASCADE,
 ordinal integer NOT NULL, content text NOT NULL, metadata jsonb NOT NULL, embedding vector,
 PRIMARY KEY(revision_id,ordinal)
);
-- Preserve the old active generation. New versions are staged independently.
INSERT INTO document_revisions(document_id,fingerprint,file_hash,parse_key,chunk_key,embedding_space,parsed_done,chunked,state)
SELECT id,'legacy','legacy','legacy','legacy','legacy',true,true,'complete' FROM documents WHERE status='ready';
UPDATE documents d SET active_revision_id=r.id FROM document_revisions r WHERE r.document_id=d.id AND r.fingerprint='legacy';
INSERT INTO revision_pages(revision_id,ordinal,payload)
SELECT d.active_revision_id,(p.ordinality-1)::int,p.value FROM documents d JOIN document_parses s ON s.document_id=d.id,
 LATERAL jsonb_array_elements(s.payload->'Pages') WITH ORDINALITY p WHERE d.active_revision_id IS NOT NULL;
INSERT INTO revision_chunks(revision_id,ordinal,content,metadata,embedding)
SELECT d.active_revision_id,c.chunk_index,c.content,c.metadata,c.embedding FROM document_chunks c JOIN documents d ON d.id=c.document_id WHERE d.active_revision_id IS NOT NULL;
UPDATE document_revisions r SET total_pages=(SELECT count(*) FROM revision_pages p WHERE p.revision_id=r.id);
CREATE INDEX document_revisions_reuse ON document_revisions(document_id,parse_key,chunk_key);
COMMIT;
