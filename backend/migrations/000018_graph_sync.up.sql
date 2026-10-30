CREATE TABLE IF NOT EXISTS graph_sync_state (
 knowledge_base_id uuid PRIMARY KEY,
 revision bigint NOT NULL DEFAULT 1,
 synced_revision bigint NOT NULL DEFAULT 0,
 last_tx bigint NOT NULL DEFAULT 0,
 deleted boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO graph_sync_state(knowledge_base_id) SELECT id FROM knowledge_bases ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION mark_408_graph_dirty() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE kid uuid;
BEGIN
 IF TG_TABLE_NAME='knowledge_bases' THEN
  IF TG_OP='DELETE' THEN kid=OLD.id; ELSE kid=NEW.id; END IF;
 ELSIF TG_TABLE_NAME IN ('knowledge_entity_evidence','exam408_entity_mappings') THEN
  IF TG_OP='DELETE' THEN SELECT knowledge_base_id INTO kid FROM knowledge_entities WHERE id=OLD.entity_id; ELSE SELECT knowledge_base_id INTO kid FROM knowledge_entities WHERE id=NEW.entity_id; END IF;
 ELSE
  IF TG_OP='DELETE' THEN kid=OLD.knowledge_base_id; ELSE kid=NEW.knowledge_base_id; END IF;
 END IF;
 IF kid IS NULL THEN RETURN NULL; END IF;
 INSERT INTO graph_sync_state(knowledge_base_id,last_tx,deleted) VALUES(kid,txid_current(),TG_TABLE_NAME='knowledge_bases' AND TG_OP='DELETE')
 ON CONFLICT(knowledge_base_id) DO UPDATE SET revision=graph_sync_state.revision+1,last_tx=txid_current(),deleted=EXCLUDED.deleted,updated_at=now()
 WHERE graph_sync_state.last_tx<>txid_current() OR EXCLUDED.deleted;
 RETURN NULL;
END $$;
CREATE TRIGGER graph_dirty_entities AFTER INSERT OR UPDATE OR DELETE ON knowledge_entities FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();
CREATE TRIGGER graph_dirty_edges AFTER INSERT OR UPDATE OR DELETE ON knowledge_relations FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();
CREATE TRIGGER graph_dirty_chunks AFTER INSERT OR UPDATE OR DELETE ON document_chunks FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();
CREATE TRIGGER graph_dirty_bases AFTER INSERT OR DELETE ON knowledge_bases FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();

CREATE TRIGGER graph_dirty_evidence AFTER INSERT OR DELETE ON knowledge_entity_evidence FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();
CREATE TRIGGER graph_dirty_point_mapping AFTER INSERT OR UPDATE OR DELETE ON exam408_entity_mappings FOR EACH ROW EXECUTE FUNCTION mark_408_graph_dirty();
