-- Each capacity reservation has an owner and expires if its worker disappears.
CREATE TABLE worker_slot_leases (
    worker_name TEXT NOT NULL,
    token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (worker_name, token)
);
CREATE INDEX worker_slot_leases_expiry_idx ON worker_slot_leases (worker_name, expires_at);
