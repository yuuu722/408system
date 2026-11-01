-- Rebuild filtered snapshots without deleting raw files or provenance records.
UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE NOT deleted;
