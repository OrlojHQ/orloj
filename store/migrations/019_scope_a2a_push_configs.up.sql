CREATE TABLE a2a_push_configs_scoped (
    task_name TEXT NOT NULL REFERENCES tasks(name) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    config_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (task_name, config_id)
);

WITH matched AS (
    SELECT
        pc.task_id,
        pc.config_id,
        pc.payload,
        pc.updated_at,
        t.name AS task_name,
        COUNT(*) OVER (PARTITION BY pc.task_id, pc.config_id) AS match_count
    FROM a2a_push_configs pc
    JOIN tasks t
      ON t.payload #>> '{metadata,labels,orloj.dev/a2a-task-id}' = pc.task_id
)
INSERT INTO a2a_push_configs_scoped(task_name, task_id, config_id, payload, updated_at)
SELECT task_name, task_id, config_id, payload, updated_at
FROM matched
WHERE match_count = 1;

DROP TABLE a2a_push_configs;
ALTER TABLE a2a_push_configs_scoped RENAME TO a2a_push_configs;

CREATE INDEX idx_a2a_push_configs_task_id
    ON a2a_push_configs(task_id);
