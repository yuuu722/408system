UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE deleted=false;
