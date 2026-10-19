-- Preserve historical vectors during provider changes. Small local deployments use
-- exact search; a provider-specific partial HNSW index can be added after profiling.
DROP INDEX IF EXISTS document_chunks_embedding_idx;
ALTER TABLE document_chunks ALTER COLUMN embedding TYPE vector;
