-- Durable audit trail written by the built-in Postgres audit sink.
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    component TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    outcome TEXT NOT NULL,
    namespace TEXT NOT NULL DEFAULT '',
    resource_kind TEXT NOT NULL DEFAULT '',
    resource_name TEXT NOT NULL DEFAULT '',
    principal TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at
    ON audit_events(occurred_at);
