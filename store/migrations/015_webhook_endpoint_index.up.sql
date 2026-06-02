-- Indexed lookup for inbound webhook delivery by endpoint ID.
CREATE INDEX IF NOT EXISTS idx_task_webhooks_endpoint_id
    ON task_webhooks ((payload->'status'->>'endpointID'));
