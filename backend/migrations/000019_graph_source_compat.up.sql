-- Replay active snapshots once with the legacy direct evidence FK fallback.
-- No source documents, vectors, graph provenance or learning history are deleted.
UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE deleted=false;
