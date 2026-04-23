CREATE TABLE IF NOT EXISTS sealed_secrets (
    name TEXT PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'default',
    status_phase TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sealed_secrets_namespace ON sealed_secrets(namespace);

CREATE TABLE IF NOT EXISTS sealing_keys (
    key_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    public_key_pem TEXT NOT NULL,
    private_key_ciphertext TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sealing_keys_single_active
    ON sealing_keys(status)
    WHERE status = 'active';
