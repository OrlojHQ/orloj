CREATE TABLE IF NOT EXISTS task_approvals (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    task_ref TEXT NOT NULL DEFAULT '',
    checkpoint_id TEXT NOT NULL DEFAULT '',
    checkpoint_type TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT 'pending',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_task_approvals_namespace ON task_approvals(namespace);
CREATE INDEX IF NOT EXISTS idx_task_approvals_task_ref ON task_approvals(task_ref);
CREATE INDEX IF NOT EXISTS idx_task_approvals_checkpoint_id ON task_approvals(checkpoint_id);
CREATE INDEX IF NOT EXISTS idx_task_approvals_status_phase ON task_approvals(status_phase);
