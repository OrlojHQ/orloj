-- 013: Agent Evaluation Framework -- EvalDataset and EvalRun resources.

CREATE TABLE IF NOT EXISTS eval_datasets (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_eval_datasets_name_len CHECK (length(name) <= 253)
);
CREATE INDEX IF NOT EXISTS idx_eval_datasets_namespace ON eval_datasets(namespace);

CREATE TABLE IF NOT EXISTS eval_runs (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    dataset_ref TEXT NOT NULL DEFAULT '',
    system_ref TEXT NOT NULL DEFAULT '',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_eval_runs_name_len CHECK (length(name) <= 253)
);
CREATE INDEX IF NOT EXISTS idx_eval_runs_namespace ON eval_runs(namespace);
CREATE INDEX IF NOT EXISTS idx_eval_runs_dataset_ref ON eval_runs(dataset_ref);
CREATE INDEX IF NOT EXISTS idx_eval_runs_status_phase ON eval_runs(status_phase);
