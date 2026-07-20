CREATE TABLE IF NOT EXISTS sessions (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    system_ref TEXT NOT NULL,
    status_phase TEXT NOT NULL DEFAULT 'waitinginput',
    claimed_by TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    spec_hash TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_namespace_phase
    ON sessions(namespace, status_phase);
CREATE INDEX IF NOT EXISTS idx_sessions_system_ref
    ON sessions(system_ref);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry
    ON sessions(expires_at)
    WHERE status_phase NOT IN ('failed', 'cancelled', 'completed', 'expired');

CREATE TABLE IF NOT EXISTS session_turns (
    session_name TEXT NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    turn_id TEXT NOT NULL,
    queue_sequence BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL,
    claimed_by TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(session_name, turn_id)
);

CREATE INDEX IF NOT EXISTS idx_session_turns_claimable
    ON session_turns(status_phase, queue_sequence, lease_until)
    WHERE status_phase IN ('queued', 'running');
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_turns_idempotency
    ON session_turns(session_name, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS session_events (
    session_name TEXT NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(session_name, seq),
    UNIQUE(event_id)
);

CREATE INDEX IF NOT EXISTS idx_session_events_turn
    ON session_events(session_name, turn_id, seq);
