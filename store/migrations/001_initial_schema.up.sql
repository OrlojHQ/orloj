CREATE TABLE IF NOT EXISTS resources (
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(kind, name)
);

CREATE INDEX IF NOT EXISTS idx_resources_kind_updated_at ON resources(kind, updated_at DESC);

CREATE TABLE IF NOT EXISTS task_logs (
    id BIGSERIAL PRIMARY KEY,
    task_name TEXT NOT NULL,
    entry TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_logs_task_created_at ON task_logs(task_name, created_at ASC);

CREATE TABLE IF NOT EXISTS webhook_dedupe (
    endpoint_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    task_name TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(endpoint_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_webhook_dedupe_expires_at ON webhook_dedupe(expires_at ASC);
