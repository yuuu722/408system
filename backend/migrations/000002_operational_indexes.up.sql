CREATE INDEX IF NOT EXISTS documents_owner_kb_status_idx
  ON documents(user_id, knowledge_base_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS conversations_owner_updated_idx
  ON conversations(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS messages_conversation_created_idx
  ON messages(conversation_id, created_at);
