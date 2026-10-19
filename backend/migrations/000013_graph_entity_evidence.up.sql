BEGIN;

ALTER TABLE knowledge_entities
  ADD COLUMN confidence real NOT NULL DEFAULT 1.0 CHECK(confidence BETWEEN 0 AND 1),
  ADD COLUMN source varchar(24) NOT NULL DEFAULT 'canonical'
    CHECK(source IN ('canonical','llm_candidate')),
  ADD COLUMN evidence_chunk_id uuid REFERENCES document_chunks(id) ON DELETE SET NULL;

CREATE INDEX knowledge_entities_evidence_idx
  ON knowledge_entities(evidence_chunk_id)
  WHERE evidence_chunk_id IS NOT NULL;

COMMIT;
