BEGIN;
-- Only untouched, system-provided examples are removed. Administrator edits and uploads survive.
CREATE TEMP TABLE retired_examples ON COMMIT DROP AS
SELECT a.id,a.document_id FROM learning_assets a JOIN knowledge_bases kb ON kb.user_id=a.user_id AND kb.is_system
WHERE a.seed_key IS NOT NULL AND a.revision=1;
DELETE FROM documents d USING retired_examples a
WHERE d.id=a.document_id AND d.filename=a.id::text || '-r1.md'
AND NOT EXISTS(SELECT 1 FROM document_revisions r WHERE r.document_id=d.id GROUP BY r.document_id HAVING count(*)>1);
DELETE FROM learning_assets a USING retired_examples e WHERE a.id=e.id;
DELETE FROM knowledge_entities e WHERE NOT e.pinned
AND EXISTS(SELECT 1 FROM knowledge_bases kb WHERE kb.id=e.knowledge_base_id AND kb.is_system)
AND NOT EXISTS(SELECT 1 FROM knowledge_entity_evidence s WHERE s.entity_id=e.id);
UPDATE knowledge_bases SET description='管理员上传和维护的 408 学习资料' WHERE is_system;
COMMIT;
