-- ContextAdapter: sanitizes task input before agents run (references a Tool CRD).
CREATE TABLE IF NOT EXISTS context_adapters (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_context_adapters_namespace ON context_adapters(namespace);
