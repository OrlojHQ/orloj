-- 006: Expression index for JSONB region filter on the task claim hot path,
-- plus advisory-lock helper for safe concurrent migrations.

-- The worker claim query filters by payload->'spec'->'requirements'->>'region'
-- without an index. This partial expression index covers claimable tasks so the
-- planner can skip a full JSONB parse per candidate row.
CREATE INDEX IF NOT EXISTS idx_tasks_region_requirement
    ON tasks ((LOWER(payload->'spec'->'requirements'->>'region')))
    WHERE status_phase IN ('', 'pending', 'running');
