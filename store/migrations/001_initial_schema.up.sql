-- Agents
CREATE TABLE IF NOT EXISTS agents (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agents_namespace ON agents(namespace);

-- Agent Systems
CREATE TABLE IF NOT EXISTS agent_systems (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_systems_namespace ON agent_systems(namespace);

-- Model Endpoints
CREATE TABLE IF NOT EXISTS model_endpoints (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    provider TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_model_endpoints_namespace ON model_endpoints(namespace);

-- Tools
CREATE TABLE IF NOT EXISTS tools (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    risk_level TEXT NOT NULL DEFAULT 'low',
    isolation_mode TEXT NOT NULL DEFAULT 'none',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tools_namespace ON tools(namespace);

-- Secrets
CREATE TABLE IF NOT EXISTS secrets (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_secrets_namespace ON secrets(namespace);

-- Memories
CREATE TABLE IF NOT EXISTS memories (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_memories_namespace ON memories(namespace);

-- Agent Policies
CREATE TABLE IF NOT EXISTS agent_policies (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    apply_mode TEXT NOT NULL DEFAULT 'scoped',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_policies_namespace ON agent_policies(namespace);

-- Agent Roles
CREATE TABLE IF NOT EXISTS agent_roles (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_roles_namespace ON agent_roles(namespace);

-- Tool Permissions
CREATE TABLE IF NOT EXISTS tool_permissions (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    tool_ref TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_permissions_namespace ON tool_permissions(namespace);

-- Tasks (hottest table -- most indexes)
CREATE TABLE IF NOT EXISTS tasks (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    system_ref TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT 'run',
    status_phase TEXT NOT NULL DEFAULT '',
    assigned_worker TEXT NOT NULL DEFAULT '',
    claimed_by TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    priority TEXT NOT NULL DEFAULT 'normal',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tasks_namespace_phase ON tasks(namespace, status_phase);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_worker ON tasks(assigned_worker, status_phase);
CREATE INDEX IF NOT EXISTS idx_tasks_system_ref ON tasks(system_ref);
CREATE INDEX IF NOT EXISTS idx_tasks_claimable ON tasks(status_phase, lease_until, next_attempt_at) WHERE status_phase IN ('', 'pending', 'running');

-- Task Schedules
CREATE TABLE IF NOT EXISTS task_schedules (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    task_ref TEXT NOT NULL DEFAULT '',
    schedule TEXT NOT NULL DEFAULT '',
    suspend BOOLEAN NOT NULL DEFAULT false,
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_task_schedules_namespace ON task_schedules(namespace);

-- Task Webhooks
CREATE TABLE IF NOT EXISTS task_webhooks (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    task_ref TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_task_webhooks_namespace ON task_webhooks(namespace);

-- Workers
CREATE TABLE IF NOT EXISTS workers (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    region TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT '',
    current_tasks INT NOT NULL DEFAULT 0,
    max_concurrent_tasks INT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_workers_namespace ON workers(namespace);
CREATE INDEX IF NOT EXISTS idx_workers_phase ON workers(status_phase);

-- Task Logs (unchanged from original schema)
CREATE TABLE IF NOT EXISTS task_logs (
    id BIGSERIAL PRIMARY KEY,
    task_name TEXT NOT NULL,
    entry TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_task_logs_task_created_at ON task_logs(task_name, created_at ASC);

-- Webhook Deduplication (unchanged from original schema)
CREATE TABLE IF NOT EXISTS webhook_dedupe (
    endpoint_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    task_name TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(endpoint_id, event_id)
);
CREATE INDEX IF NOT EXISTS idx_webhook_dedupe_expires_at ON webhook_dedupe(expires_at ASC);
