CREATE TABLE IF NOT EXISTS session_checkpoints (
    session_name TEXT NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    checkpoint_id TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    task_name TEXT NOT NULL DEFAULT '',
    agent_name TEXT NOT NULL DEFAULT '',
    agent_index INTEGER NOT NULL DEFAULT 0,
    message_id TEXT NOT NULL DEFAULT '',
    event_seq BIGINT NOT NULL,
    safe_point TEXT NOT NULL,
    state_version INTEGER NOT NULL,
    state_hash TEXT NOT NULL,
    payload JSONB NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(session_name, checkpoint_id)
);

CREATE INDEX IF NOT EXISTS idx_session_checkpoints_latest
    ON session_checkpoints(session_name, event_seq DESC, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_session_checkpoints_turn
    ON session_checkpoints(session_name, turn_id, agent_name, agent_index, message_id, event_seq DESC);

CREATE INDEX IF NOT EXISTS idx_session_checkpoints_expiry
    ON session_checkpoints(expires_at)
    WHERE expires_at IS NOT NULL;
