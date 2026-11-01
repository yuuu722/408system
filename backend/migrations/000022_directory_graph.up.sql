UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
CREATE OR REPLACE FUNCTION mark_408_directory_dirty() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
 RETURN NULL;
END $$;
CREATE TRIGGER graph_directory_points AFTER INSERT OR UPDATE OR DELETE ON exam408_knowledge_points FOR EACH STATEMENT EXECUTE FUNCTION mark_408_directory_dirty();
CREATE TRIGGER graph_directory_relations AFTER INSERT OR UPDATE OR DELETE ON exam408_knowledge_point_relations FOR EACH STATEMENT EXECUTE FUNCTION mark_408_directory_dirty();
