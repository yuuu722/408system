BEGIN;

ALTER TABLE exam408_entity_mappings
  ADD COLUMN evidence_chunk_id uuid REFERENCES document_chunks(id) ON DELETE SET NULL;

CREATE INDEX exam408_entity_mappings_evidence_idx
  ON exam408_entity_mappings(evidence_chunk_id)
  WHERE evidence_chunk_id IS NOT NULL;

COMMIT;
