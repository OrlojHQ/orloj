CREATE TABLE IF NOT EXISTS tool_approvals (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    task_ref TEXT NOT NULL DEFAULT '',
    tool TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT 'pending',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_namespace ON tool_approvals(namespace);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_task_ref ON tool_approvals(task_ref);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_status_phase ON tool_approvals(status_phase);
