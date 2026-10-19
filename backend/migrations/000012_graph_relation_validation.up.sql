BEGIN;

ALTER TABLE knowledge_relations
  ADD COLUMN confidence real NOT NULL DEFAULT 1.0 CHECK(confidence BETWEEN 0 AND 1),
  ADD COLUMN source varchar(24) NOT NULL DEFAULT 'canonical'
    CHECK(source IN ('canonical','llm_candidate'));

COMMIT;
