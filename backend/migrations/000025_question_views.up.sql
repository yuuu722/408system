BEGIN;
CREATE TABLE document_question_views (
 document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
 revision_id text NOT NULL, base_fingerprint text NOT NULL, source_hash text NOT NULL,
 parser_version text NOT NULL, payload jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE question_family_aliases (
 legacy_family_id text PRIMARY KEY, canonical_family_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
COMMIT;
