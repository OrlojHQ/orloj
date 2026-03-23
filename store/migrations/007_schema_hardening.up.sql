-- 007: Schema hardening -- CHECK constraints, name length limits, default
-- normalization, and created_at columns for auditability.

-- ---------------------------------------------------------------------------
-- Name length limit (253 matches Kubernetes naming convention)
-- ---------------------------------------------------------------------------

ALTER TABLE agents           ADD CONSTRAINT chk_agents_name_len           CHECK (length(name) <= 253);
ALTER TABLE agent_systems    ADD CONSTRAINT chk_agent_systems_name_len    CHECK (length(name) <= 253);
ALTER TABLE model_endpoints  ADD CONSTRAINT chk_model_endpoints_name_len  CHECK (length(name) <= 253);
ALTER TABLE tools            ADD CONSTRAINT chk_tools_name_len            CHECK (length(name) <= 253);
ALTER TABLE secrets          ADD CONSTRAINT chk_secrets_name_len          CHECK (length(name) <= 253);
ALTER TABLE memories         ADD CONSTRAINT chk_memories_name_len         CHECK (length(name) <= 253);
ALTER TABLE agent_policies   ADD CONSTRAINT chk_agent_policies_name_len   CHECK (length(name) <= 253);
ALTER TABLE agent_roles      ADD CONSTRAINT chk_agent_roles_name_len      CHECK (length(name) <= 253);
ALTER TABLE tool_permissions ADD CONSTRAINT chk_tool_permissions_name_len CHECK (length(name) <= 253);
ALTER TABLE tool_approvals   ADD CONSTRAINT chk_tool_approvals_name_len   CHECK (length(name) <= 253);
ALTER TABLE tasks            ADD CONSTRAINT chk_tasks_name_len            CHECK (length(name) <= 253);
ALTER TABLE task_schedules   ADD CONSTRAINT chk_task_schedules_name_len   CHECK (length(name) <= 253);
ALTER TABLE task_webhooks    ADD CONSTRAINT chk_task_webhooks_name_len    CHECK (length(name) <= 253);
ALTER TABLE workers          ADD CONSTRAINT chk_workers_name_len          CHECK (length(name) <= 253);
ALTER TABLE mcp_servers      ADD CONSTRAINT chk_mcp_servers_name_len      CHECK (length(name) <= 253);

-- ---------------------------------------------------------------------------
-- Normalize mcp_servers default from 'Pending' to '' (matches all other tables;
-- the Go upsert layer normalizes to lowercase on every write).
-- ---------------------------------------------------------------------------

ALTER TABLE mcp_servers ALTER COLUMN status_phase SET DEFAULT '';

-- ---------------------------------------------------------------------------
-- Add created_at to resource tables that lack it. Existing rows are backfilled
-- from updated_at as a reasonable approximation.
-- ---------------------------------------------------------------------------

ALTER TABLE agents           ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE agent_systems    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE model_endpoints  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE tools            ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE secrets          ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE memories         ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE agent_policies   ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE agent_roles      ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE tool_permissions ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE tool_approvals   ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE tasks            ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE task_schedules   ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE task_webhooks    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE workers          ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;

UPDATE agents           SET created_at = updated_at WHERE created_at IS NULL;
UPDATE agent_systems    SET created_at = updated_at WHERE created_at IS NULL;
UPDATE model_endpoints  SET created_at = updated_at WHERE created_at IS NULL;
UPDATE tools            SET created_at = updated_at WHERE created_at IS NULL;
UPDATE secrets          SET created_at = updated_at WHERE created_at IS NULL;
UPDATE memories         SET created_at = updated_at WHERE created_at IS NULL;
UPDATE agent_policies   SET created_at = updated_at WHERE created_at IS NULL;
UPDATE agent_roles      SET created_at = updated_at WHERE created_at IS NULL;
UPDATE tool_permissions SET created_at = updated_at WHERE created_at IS NULL;
UPDATE tool_approvals   SET created_at = updated_at WHERE created_at IS NULL;
UPDATE tasks            SET created_at = updated_at WHERE created_at IS NULL;
UPDATE task_schedules   SET created_at = updated_at WHERE created_at IS NULL;
UPDATE task_webhooks    SET created_at = updated_at WHERE created_at IS NULL;
UPDATE workers          SET created_at = updated_at WHERE created_at IS NULL;

ALTER TABLE agents           ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE agent_systems    ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE model_endpoints  ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE tools            ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE secrets          ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE memories         ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE agent_policies   ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE agent_roles      ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE tool_permissions ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE tool_approvals   ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE tasks            ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE task_schedules   ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE task_webhooks    ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE workers          ALTER COLUMN created_at SET NOT NULL, ALTER COLUMN created_at SET DEFAULT NOW();
