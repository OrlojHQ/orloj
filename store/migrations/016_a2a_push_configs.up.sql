CREATE TABLE IF NOT EXISTS a2a_push_configs (
    task_id TEXT NOT NULL,
    config_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (task_id, config_id)
);

CREATE INDEX IF NOT EXISTS idx_a2a_push_configs_task_id
    ON a2a_push_configs(task_id);
