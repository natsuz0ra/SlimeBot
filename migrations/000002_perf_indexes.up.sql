CREATE INDEX IF NOT EXISTS idx_messages_session_created_seq
    ON messages(session_id, created_at, seq);

CREATE INDEX IF NOT EXISTS idx_messages_session_seq
    ON messages(session_id, seq);

CREATE INDEX IF NOT EXISTS idx_session_memories_session_active_updated
    ON session_memories(session_id, is_active, updated_at);

CREATE INDEX IF NOT EXISTS idx_tool_call_records_session_assistant_started_created
    ON tool_call_records(session_id, assistant_message_id, started_at, created_at);
