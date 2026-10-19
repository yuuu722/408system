BEGIN;

ALTER TABLE knowledge_bases
  ADD COLUMN is_public boolean NOT NULL DEFAULT false;

CREATE INDEX knowledge_bases_public_idx
  ON knowledge_bases(is_public, updated_at DESC)
  WHERE is_public = true;

COMMIT;
