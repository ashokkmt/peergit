ALTER TABLE campus_email_deliveries
    ADD COLUMN provider text CHECK (provider IN ('smtp', 'resend')),
    ADD COLUMN provider_message_id text,
    ADD COLUMN delivery_status text CHECK (delivery_status IN ('accepted', 'delivered', 'bounced', 'failed', 'suppressed')),
    ADD COLUMN status_at timestamptz,
    ADD COLUMN last_error_code text;

CREATE UNIQUE INDEX campus_email_provider_message_uidx
    ON campus_email_deliveries(provider, provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE TABLE email_webhook_receipts (
    event_id text PRIMARY KEY,
    provider_message_id text NOT NULL,
    event_type text NOT NULL,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    CHECK (length(event_id) BETWEEN 1 AND 200)
);
CREATE INDEX email_webhook_pending_idx ON email_webhook_receipts(received_at, event_id)
    WHERE processed_at IS NULL;
CREATE INDEX email_webhook_message_idx ON email_webhook_receipts(provider_message_id);

CREATE TABLE email_suppressions (
    recipient_key bytea PRIMARY KEY CHECK (octet_length(recipient_key) = 32),
    reason text NOT NULL CHECK (reason IN ('hard_bounce', 'complaint', 'provider_suppressed')),
    recorded_at timestamptz NOT NULL DEFAULT now()
);
