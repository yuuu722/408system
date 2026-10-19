BEGIN;
ALTER TABLE users ADD COLUMN disabled boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge_entities ADD COLUMN pinned boolean NOT NULL DEFAULT false;
UPDATE knowledge_entities e SET pinned=true FROM knowledge_bases kb WHERE kb.id=e.knowledge_base_id AND kb.is_system AND e.source='canonical';
CREATE TABLE knowledge_entity_evidence (
 entity_id uuid NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
 chunk_id uuid NOT NULL REFERENCES document_chunks(id) ON DELETE CASCADE,
 PRIMARY KEY(entity_id,chunk_id)
);
INSERT INTO knowledge_entity_evidence SELECT id,evidence_chunk_id FROM knowledge_entities WHERE evidence_chunk_id IS NOT NULL ON CONFLICT DO NOTHING;
ALTER TABLE knowledge_relations DROP CONSTRAINT knowledge_relations_evidence_chunk_id_fkey;
ALTER TABLE knowledge_relations ADD CONSTRAINT knowledge_relations_evidence_chunk_id_fkey FOREIGN KEY(evidence_chunk_id) REFERENCES document_chunks(id) ON DELETE CASCADE;
ALTER TABLE knowledge_entities DROP CONSTRAINT knowledge_entities_source_check;
ALTER TABLE knowledge_entities ADD CONSTRAINT knowledge_entities_source_check CHECK(source IN ('canonical','llm_candidate','rule_candidate'));
ALTER TABLE knowledge_relations DROP CONSTRAINT knowledge_relations_source_check;
ALTER TABLE knowledge_relations ADD CONSTRAINT knowledge_relations_source_check CHECK(source IN ('canonical','llm_candidate','rule_candidate'));
-- Previously deleted files left null-evidence candidate edges behind.
DELETE FROM knowledge_relations WHERE source<>'canonical' AND evidence_chunk_id IS NULL;
CREATE TABLE document_revision_files (
 revision_id uuid PRIMARY KEY REFERENCES document_revisions(id) ON DELETE CASCADE,
 document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 storage_path text NOT NULL, original_filename text NOT NULL, mime_type text NOT NULL
);
INSERT INTO document_revision_files SELECT active_revision_id,id,storage_path,original_filename,mime_type FROM documents WHERE active_revision_id IS NOT NULL ON CONFLICT DO NOTHING;
COMMIT;
