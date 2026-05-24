-- 014: scoped API tokens for inbound A2A invocation.

ALTER TABLE auth_api_tokens
    ADD COLUMN IF NOT EXISTS a2a_agent_systems TEXT NOT NULL DEFAULT '[]';

ALTER TABLE auth_api_tokens
    DROP CONSTRAINT IF EXISTS auth_api_tokens_role;

ALTER TABLE auth_api_tokens
    ADD CONSTRAINT auth_api_tokens_role
    CHECK (role IN ('admin', 'writer', 'reader', 'controller', 'a2a'));
