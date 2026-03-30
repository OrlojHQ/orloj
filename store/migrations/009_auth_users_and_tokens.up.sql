-- 009: multi-user native auth and store-managed API tokens.

CREATE TABLE IF NOT EXISTS auth_local_users (
    username TEXT PRIMARY KEY,
    role TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_local_users_role CHECK (role IN ('admin', 'writer', 'reader', 'controller'))
);

CREATE INDEX IF NOT EXISTS idx_auth_local_users_role ON auth_local_users(role);

INSERT INTO auth_local_users(username, role, password_hash, created_at, updated_at)
SELECT
    username,
    'admin'::TEXT,
    password_hash,
    updated_at,
    updated_at
FROM auth_local_admin
ON CONFLICT (username) DO UPDATE SET
    role = EXCLUDED.role,
    password_hash = EXCLUDED.password_hash,
    updated_at = EXCLUDED.updated_at;

CREATE TABLE IF NOT EXISTS auth_api_tokens (
    name TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_api_tokens_role CHECK (role IN ('admin', 'writer', 'reader', 'controller'))
);

CREATE INDEX IF NOT EXISTS idx_auth_api_tokens_role ON auth_api_tokens(role);
