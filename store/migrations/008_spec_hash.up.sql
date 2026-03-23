-- 008: Add spec_hash column for lightweight upsert metadata checks.
-- Instead of deserializing the full JSONB payload to compare specs,
-- the upsert path can compare a SHA-256 hash of the spec blob.

ALTER TABLE agents           ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_systems    ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE model_endpoints  ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE tools            ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets          ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE memories         ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_policies   ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_roles      ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_permissions ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_approvals   ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks            ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE task_schedules   ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE task_webhooks    ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE workers          ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE mcp_servers      ADD COLUMN IF NOT EXISTS spec_hash TEXT NOT NULL DEFAULT '';
