CREATE TABLE IF NOT EXISTS mcp_servers (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT 'Pending',
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mcp_servers_namespace ON mcp_servers(namespace);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_status_phase ON mcp_servers(status_phase);
