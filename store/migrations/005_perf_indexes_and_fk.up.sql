-- 005: Performance indexes and task_logs foreign key
--
-- idx_tasks_claimable_updated: the claimable partial index is used for the
-- worker claim query which ORDER BY updated_at ASC. Including updated_at lets
-- Postgres return pre-sorted rows without a sort step.
CREATE INDEX IF NOT EXISTS idx_tasks_claimable_updated
    ON tasks(updated_at ASC)
    WHERE status_phase IN ('', 'pending', 'running');

-- Lookup webhooks by task_ref (used by webhook controller and API).
CREATE INDEX IF NOT EXISTS idx_task_webhooks_task_ref
    ON task_webhooks(task_ref);

-- Schedule controller filters active (non-suspended) schedules and
-- looks up schedules by task_ref.
CREATE INDEX IF NOT EXISTS idx_task_schedules_task_ref
    ON task_schedules(task_ref);
CREATE INDEX IF NOT EXISTS idx_task_schedules_active
    ON task_schedules(suspend, status_phase);

-- Tool permission controller looks up permissions by tool_ref.
CREATE INDEX IF NOT EXISTS idx_tool_permissions_tool_ref
    ON tool_permissions(tool_ref);

-- Add foreign key on task_logs so orphaned rows are cleaned up automatically
-- when a task is deleted, and so we can drop the SELECT existence check.
ALTER TABLE task_logs
    ADD CONSTRAINT fk_task_logs_task_name
    FOREIGN KEY (task_name) REFERENCES tasks(name) ON DELETE CASCADE;
